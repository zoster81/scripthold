package taskstore

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zoster81/scripthold/internal/execution"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/security"
)

const (
	workerPollInterval        = 250 * time.Millisecond
	workerHeartbeatInterval   = time.Second
	executorHeartbeatInterval = time.Second
	executorStaleAfter        = 15 * time.Second
	recoveryGrace             = 20 * time.Second
)

var (
	errTaskStateChanged   = errors.New("task state changed before dispatch")
	errTaskStartFinalized = errors.New("task start failure was finalized")
)

type Worker struct {
	store            *Store
	executable       string
	allowedRequested []string
	allowedResolved  []string
	policy           WorkerPolicy
	logger           *slog.Logger
	startedAt        time.Time
	suspectSince     map[string]time.Time
	reconcileCycle   func(context.Context) error
}

func NewWorker(store *Store, executable string, allowedDirectories []string, policy WorkerPolicy, logger *slog.Logger) (*Worker, error) {
	if store == nil {
		return nil, ErrDisabled
	}
	if executable == "" {
		return nil, errors.New("worker executable is required")
	}
	set, err := security.NormalizeAllowedDirectorySet(allowedDirectories)
	if err != nil {
		return nil, err
	}
	if len(set.Resolved) == 0 {
		return nil, errors.New("task worker requires at least one allowed directory")
	}
	if logger == nil {
		logger = slog.Default()
	}
	worker := &Worker{store: store, executable: executable, allowedRequested: set.Requested, allowedResolved: set.Resolved, policy: policy, logger: logger, startedAt: time.Now(), suspectSince: make(map[string]time.Time)}
	worker.reconcileCycle = worker.reconcile
	return worker, nil
}

func (worker *Worker) Run(ctx context.Context) error {
	lock, err := tryAcquireWorkerLock(filepath.Join(worker.store.root, "worker.lock"))
	if err != nil {
		return err
	}
	defer lock.close()
	heartbeatPath := filepath.Join(worker.store.root, workerHeartbeatName)
	if err := touch(heartbeatPath); err != nil {
		return fmt.Errorf("update worker heartbeat: %w", err)
	}
	heartbeatContext, stopHeartbeat := context.WithCancel(ctx)
	heartbeatErrors := make(chan error, 1)
	defer stopHeartbeat()
	go func() {
		ticker := time.NewTicker(workerHeartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatContext.Done():
				return
			case <-ticker.C:
				if err := touch(heartbeatPath); err != nil {
					heartbeatErrors <- err
					return
				}
			}
		}
	}()
	worker.logger.Info("task worker started", "maxConcurrency", worker.store.limits.MaxConcurrency, "maxQueued", worker.store.limits.MaxQueued)
	ticker := time.NewTicker(workerPollInterval)
	defer ticker.Stop()
	retentionTicker := time.NewTicker(time.Minute)
	defer retentionTicker.Stop()
	for {
		if err := worker.reconcileCycle(ctx); err != nil {
			worker.logger.Error("task worker reconciliation failed", "error", err)
		}
		select {
		case <-ctx.Done():
			worker.logger.Info("task worker stopped")
			return nil
		case err := <-heartbeatErrors:
			return fmt.Errorf("update worker heartbeat: %w", err)
		case <-retentionTicker.C:
			if err := worker.store.Purge(); err != nil {
				worker.logger.Error("task retention failed", "error", err)
			}
		case <-ticker.C:
		}
	}
}

type queuedCandidate struct {
	request persistedRequest
	state   stateRecord
}

func (worker *Worker) reconcile(ctx context.Context) error {
	entries, err := readDirectoryBounded(worker.store.tasksRoot, worker.store.limits.MaxTerminal+worker.store.limits.MaxQueued+worker.store.limits.MaxConcurrency+1024)
	if err != nil {
		return err
	}
	active := 0
	heldLocks := make(map[string]struct{})
	queued := make([]queuedCandidate, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !validTaskID(entry.Name()) {
			continue
		}
		request, requestErr := worker.store.readPersistedRequest(entry.Name())
		state, stateErr := worker.store.latestState(entry.Name())
		if requestErr != nil || stateErr != nil {
			continue
		}
		cancelled := fileExists(filepath.Join(worker.store.taskDir(entry.Name()), cancelName))
		switch state.Status {
		case StatusQueued:
			if cancelled {
				_ = worker.finishWithoutExecution(request, state, StatusCancelled, "TASK_CANCELLED", "cancelled before execution")
				continue
			}
			queued = append(queued, queuedCandidate{request: request, state: state})
		case StatusStarting, StatusRunning:
			fresh := worker.executorFresh(entry.Name())
			started := fileExists(filepath.Join(worker.store.taskDir(entry.Name()), startedName))
			if fresh {
				delete(worker.suspectSince, entry.Name())
				active++
				for _, key := range request.Request.LockKeys {
					heldLocks[key] = struct{}{}
				}
				continue
			}
			suspectAt, suspected := worker.suspectSince[entry.Name()]
			if !suspected {
				suspectAt = time.Now()
				worker.suspectSince[entry.Name()] = suspectAt
			}
			withinGrace := time.Since(worker.startedAt) < recoveryGrace || time.Since(state.UpdatedAt) < recoveryGrace || time.Since(suspectAt) < recoveryGrace
			if withinGrace {
				active++
				for _, key := range request.Request.LockKeys {
					heldLocks[key] = struct{}{}
				}
				continue
			}
			if !started && state.Status == StatusStarting {
				now := time.Now().UTC()
				_ = worker.store.appendStateUnlocked(entry.Name(), stateRecord{Status: StatusQueued, Revision: state.Revision + 1, UpdatedAt: now})
				delete(worker.suspectSince, entry.Name())
				continue
			}
			_ = worker.finishWithoutExecution(request, state, StatusInterrupted, "EXECUTOR_LOST", "executor heartbeat was lost; task was not rerun")
			delete(worker.suspectSince, entry.Name())
		}
	}
	sort.Slice(queued, func(i, j int) bool { return queued[i].request.CreatedAt.Before(queued[j].request.CreatedAt) })
	for _, candidate := range queued {
		if ctx.Err() != nil || active >= worker.store.limits.MaxConcurrency {
			break
		}
		if locksConflict(candidate.request.Request.LockKeys, heldLocks) {
			continue
		}
		attempts, err := worker.dispatchAttempts(candidate.request.TaskID)
		if err != nil {
			return err
		}
		if attempts >= maxDispatchAttempts {
			_ = worker.finishWithoutExecution(candidate.request, candidate.state, StatusFailed, "TASK_DISPATCH_RETRIES_EXHAUSTED", "task exceeded the bounded pre-start recovery attempts")
			continue
		}
		if err := worker.start(candidate.request, candidate.state); err != nil {
			worker.logger.Error("task start failed", "taskId", candidate.request.TaskID, "kind", candidate.request.Request.Kind, "error", err)
			if errors.Is(err, errTaskStateChanged) || errors.Is(err, errTaskStartFinalized) {
				continue
			}
			_ = worker.finishWithoutExecution(candidate.request, candidate.state, StatusFailed, "TASK_START_FAILED", "task could not be prepared or started")
			continue
		}
		active++
		for _, key := range candidate.request.Request.LockKeys {
			heldLocks[key] = struct{}{}
		}
	}
	return nil
}

func (worker *Worker) dispatchAttempts(taskID string) (int, error) {
	states, err := worker.store.stateHistory(taskID)
	if err != nil {
		return 0, err
	}
	attempts := 0
	for _, state := range states {
		if state.Status == StatusStarting {
			attempts++
		}
	}
	return attempts, nil
}

func (worker *Worker) start(request persistedRequest, current stateRecord) error {
	launch, err := worker.prepareLaunch(request.TaskID, request.Request)
	if err != nil {
		return err
	}
	lock, err := acquireControlLock(filepath.Join(worker.store.root, controlLockName))
	if err != nil {
		return fmt.Errorf("lock task dispatch: %w", err)
	}
	defer lock.close()
	latest, err := worker.store.latestState(request.TaskID)
	if err != nil || latest.Status != StatusQueued || latest.Revision != current.Revision || fileExists(filepath.Join(worker.store.taskDir(request.TaskID), cancelName)) {
		return errTaskStateChanged
	}
	launch.ExecutorToken, err = randomHex(32)
	if err != nil {
		return fmt.Errorf("create executor token: %w", err)
	}
	taskDir := worker.store.taskDir(request.TaskID)
	launchPath := filepath.Join(taskDir, "launch.json")
	if !fileExists(filepath.Join(taskDir, startedName)) {
		if err := os.Remove(launchPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := writeJSONExclusive(launchPath, launch); err != nil {
		return err
	}
	now := time.Now().UTC()
	state := stateRecord{Status: StatusStarting, Revision: latest.Revision + 1, UpdatedAt: now, ExecutorToken: launch.ExecutorToken}
	if err := worker.store.appendStateUnlocked(request.TaskID, state); err != nil {
		return err
	}
	command := exec.Command(worker.executable, "_task-exec", worker.store.root, request.TaskID)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	command.Env = replaceEnvironmentValue(os.Environ(), "MCP_TASK_EXECUTOR_TOKEN", launch.ExecutorToken)
	configureDetachedHelper(command)
	if err := command.Start(); err != nil {
		finishErr := worker.finishWithoutExecution(request, state, StatusFailed, "TASK_START_FAILED", "task executor process could not be started")
		return fmt.Errorf("%w: %v", errTaskStartFinalized, errors.Join(err, finishErr))
	}
	if command.Process != nil {
		_ = command.Process.Release()
	}
	worker.logger.Info("task dispatched", "taskId", request.TaskID, "kind", request.Request.Kind)
	return nil
}

func replaceEnvironmentValue(environment []string, name, value string) []string {
	prefix := strings.ToUpper(name) + "="
	filtered := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.HasPrefix(strings.ToUpper(entry), prefix) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, name+"="+value)
}

func (worker *Worker) prepareLaunch(taskID string, request Request) (launchRecord, error) {
	if request.Kind == KindShell && !worker.policy.AllowShell {
		return launchRecord{}, errors.New("shell task execution is disabled in the worker")
	}
	if request.Kind == KindScript && !worker.policy.AllowRunScript {
		return launchRecord{}, errors.New("script task execution is disabled in the worker")
	}
	cwd, err := security.ValidatePathWithAllowedDirectories(request.WorkingDirectory, worker.allowedRequested, worker.allowedResolved)
	if err != nil {
		return launchRecord{}, err
	}
	if _, err := execution.ValidateWorkingDirectory(cwd); err != nil {
		return launchRecord{}, err
	}
	maximum := request.MaxRuntimeSeconds
	if worker.store.limits.MaxRuntimeSeconds > 0 && (maximum == 0 || maximum > worker.store.limits.MaxRuntimeSeconds) {
		maximum = worker.store.limits.MaxRuntimeSeconds
	}
	var program string
	var args []string
	if request.Kind == KindShell {
		program, args, err = execution.BuildShellCommand(request.Shell, request.Command)
	} else {
		path, pathErr := security.ValidatePathWithAllowedDirectories(request.ScriptPath, worker.allowedRequested, worker.allowedResolved)
		if pathErr != nil {
			return launchRecord{}, pathErr
		}
		snapshotPath, snapshotErr := worker.snapshotScript(taskID, path, request.ScriptSize, request.ScriptDigest)
		if snapshotErr != nil {
			return launchRecord{}, snapshotErr
		}
		program, args, err = execution.BuildScriptCommand(snapshotPath, request.Args)
	}
	if err != nil {
		return launchRecord{}, err
	}
	return launchRecord{Program: program, Args: args, WorkingDirectory: cwd, MaxRuntimeSeconds: maximum}, nil
}

func (worker *Worker) snapshotScript(taskID, sourcePath string, expectedSize int64, expectedDigest string) (destination string, err error) {
	extension := strings.ToLower(filepath.Ext(sourcePath))
	destination = filepath.Join(worker.store.taskDir(taskID), "script.snapshot"+extension)
	if fileExists(filepath.Join(worker.store.taskDir(taskID), startedName)) {
		return "", errors.New("task already has an execution start marker")
	}
	if removeErr := os.Remove(destination); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
		return "", removeErr
	}
	session, err := filesystem.OpenReadSession(sourcePath)
	if err != nil {
		return "", err
	}
	defer func() { err = errors.Join(err, session.Close()) }()
	if session.Size() != expectedSize {
		return "", errors.New("script changed size after task admission")
	}
	if err := session.Start(0); err != nil {
		return "", err
	}
	mode := os.FileMode(0o600)
	if extension == ".exe" || extension == ".com" {
		mode = 0o700
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return "", err
	}
	cleanup := true
	closed := false
	defer func() {
		var closeErr error
		if !closed {
			closeErr = file.Close()
		}
		if cleanup {
			_ = os.Remove(destination)
		}
		err = errors.Join(err, closeErr)
	}()
	if err := securePath(destination, false); err != nil {
		return "", err
	}
	if mode == 0o700 {
		if err := os.Chmod(destination, mode); err != nil {
			return "", err
		}
	}
	if _, err := io.CopyN(file, session, expectedSize); err != nil {
		return "", err
	}
	snapshot, err := session.Finish()
	if err != nil {
		return "", err
	}
	digest, ok := snapshot.ContentDigest()
	if !ok || !strings.EqualFold(hex.EncodeToString(digest[:]), expectedDigest) {
		return "", errors.New("script changed after task admission")
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	closed = true
	cleanup = false
	return destination, nil
}

func (worker *Worker) finishWithoutExecution(request persistedRequest, current stateRecord, status Status, code, message string) error {
	now := time.Now().UTC()
	started := current.StartedAt
	return worker.store.appendStateUnlocked(request.TaskID, stateRecord{Status: status, Revision: current.Revision + 1, UpdatedAt: now, StartedAt: started, FinishedAt: &now, Result: &Result{ExitCode: -1, ErrorCode: code, Message: message}})
}

func (worker *Worker) executorFresh(taskID string) bool {
	info, err := os.Stat(filepath.Join(worker.store.taskDir(taskID), heartbeatName))
	if err != nil {
		return false
	}
	age := time.Since(info.ModTime())
	return age >= 0 && age <= executorStaleAfter
}

func locksConflict(keys []string, held map[string]struct{}) bool {
	for _, key := range keys {
		if _, ok := held[key]; ok {
			return true
		}
	}
	return false
}
func fileExists(path string) bool { _, err := os.Lstat(path); return err == nil }

func (store *Store) Purge() error {
	if store == nil {
		return ErrDisabled
	}
	lock, err := acquireControlLock(filepath.Join(store.root, controlLockName))
	if err != nil {
		return err
	}
	defer lock.close()
	entries, err := readDirectoryBounded(store.tasksRoot, store.limits.MaxTerminal+store.limits.MaxQueued+store.limits.MaxConcurrency+1024)
	if err != nil {
		return err
	}
	type terminal struct {
		id       string
		finished time.Time
		bytes    int64
	}
	terminalTasks := make([]terminal, 0)
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() || !validTaskID(entry.Name()) {
			continue
		}
		size, sizeErr := directorySizeBounded(store.taskDir(entry.Name()), store.limits.MaxTotalBytes+1)
		if sizeErr != nil {
			continue
		}
		total += size
		state, stateErr := store.latestState(entry.Name())
		if stateErr == nil && state.Status.Terminal() && state.FinishedAt != nil {
			terminalTasks = append(terminalTasks, terminal{entry.Name(), *state.FinishedAt, size})
		}
	}
	sort.Slice(terminalTasks, func(i, j int) bool { return terminalTasks[i].finished.Before(terminalTasks[j].finished) })
	cutoff := time.Now().Add(-time.Duration(store.limits.RetentionDays) * 24 * time.Hour)
	remaining := len(terminalTasks)
	for _, task := range terminalTasks {
		if remaining <= store.limits.MaxTerminal && total <= store.limits.MaxTotalBytes && !task.finished.Before(cutoff) {
			continue
		}
		path := store.taskDir(task.id)
		if !security.IsPathWithinAllowedDirectories(path, []string{store.tasksRoot}) {
			return errors.New("retention target escaped the task namespace")
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		remaining--
		total -= task.bytes
	}
	return nil
}

func directorySizeBounded(root string, maximum int64) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("link found in task store")
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if total > maximum {
			return errors.New("task registry size scan exceeded its bound")
		}
		return nil
	})
	return total, err
}
