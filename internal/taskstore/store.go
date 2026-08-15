package taskstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

const (
	descriptorName      = "descriptor.json"
	controlLockName     = "control.lock"
	workerHeartbeatName = "worker.heartbeat"
	tasksDirectoryName  = "tasks"
	requestName         = "request.json"
	statesDirectoryName = "states"
	cancelName          = "cancel.json"
	startedName         = "started.json"
	heartbeatName       = "executor.heartbeat"
)

var (
	ErrDisabled            = operation.New(operation.KindConflict, "persistent task store is not configured")
	ErrNotFound            = operation.New(operation.KindNotFound, "task not found")
	ErrQueueFull           = operation.New(operation.KindLimit, "task queue is full")
	ErrIdempotencyConflict = operation.New(operation.KindConflict, "idempotency key was already used with a different request")
)

type persistedRequest struct {
	TaskID      string    `json:"taskId"`
	CreatedAt   time.Time `json:"createdAt"`
	PayloadHash string    `json:"payloadHash"`
	Request     Request   `json:"request"`
}

type Store struct {
	root       string
	tasksRoot  string
	descriptor descriptor
	limits     Limits
}

func Initialize(root string, publicAllowedDirectories []string, limits Limits) (*Store, error) {
	if len(publicAllowedDirectories) == 0 {
		return nil, errors.New("task store initialization requires at least one allowed directory")
	}
	store, err := openStore(root, publicAllowedDirectories, limits, true)
	if err != nil {
		return nil, err
	}
	return store, nil
}

func Open(root string, publicAllowedDirectories []string, limits Limits) (*Store, error) {
	return openStore(root, publicAllowedDirectories, limits, false)
}

// OpenExecutor opens an existing store for the per-task helper, which receives
// no public filesystem authority and can execute only a token-bound launch
// record prepared by the worker.
func OpenExecutor(root string, limits Limits) (*Store, error) {
	return openStore(root, nil, limits, false)
}

func openStore(root string, publicAllowedDirectories []string, limits Limits, create bool) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, ErrDisabled
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve task store directory: %w", err)
	}
	root = filepath.Clean(abs)
	rootSet, err := security.NormalizeAllowedDirectorySet([]string{root})
	if err != nil || len(rootSet.Requested) != 1 || len(rootSet.Resolved) != 1 || !security.PathsEqual(rootSet.Requested[0], rootSet.Resolved[0]) {
		return nil, errors.New("task store path must not traverse a link or reparse point")
	}
	for _, publicRoot := range publicAllowedDirectories {
		if security.PathsOverlap(rootSet.Requested[0], publicRoot) || security.PathsOverlap(rootSet.Resolved[0], publicRoot) {
			return nil, errors.New("task store must not overlap an allowed public directory")
		}
	}
	if err := validateLimits(limits); err != nil {
		return nil, err
	}
	if create {
		_, statErr := os.Lstat(root)
		if errors.Is(statErr, os.ErrNotExist) {
			parent := filepath.Dir(root)
			if parent == root {
				return nil, errors.New("task store must not be a filesystem root")
			}
			if err := os.MkdirAll(parent, 0o700); err != nil {
				return nil, fmt.Errorf("create task store: %w", err)
			}
			mkdirErr := os.Mkdir(root, 0o700)
			if mkdirErr == nil {
				if err := securePath(root, true); err != nil {
					return nil, fmt.Errorf("secure task store: %w", err)
				}
			} else if !errors.Is(mkdirErr, os.ErrExist) {
				return nil, fmt.Errorf("create task store: %w", mkdirErr)
			} else if err := waitForSecurePath(root, true, 5*time.Second); err != nil {
				return nil, fmt.Errorf("secure task store: %w", err)
			}
		} else if statErr != nil {
			return nil, fmt.Errorf("inspect task store: %w", statErr)
		}
	}
	if create {
		if err := waitForSecurePath(root, true, 5*time.Second); err != nil {
			return nil, fmt.Errorf("validate task store: %w", err)
		}
	} else if err := validateSecurePath(root, true); err != nil {
		return nil, fmt.Errorf("validate task store: %w", err)
	}
	var initializationLock *controlLock
	if create {
		initializationLock, err = acquireControlLock(filepath.Join(root, "initialize.lock"))
		if err != nil {
			return nil, fmt.Errorf("lock task store initialization: %w", err)
		}
		defer initializationLock.close()
	}

	tasksRoot := filepath.Join(root, tasksDirectoryName)
	if create {
		_, statErr := os.Lstat(tasksRoot)
		if errors.Is(statErr, os.ErrNotExist) {
			if err := os.Mkdir(tasksRoot, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return nil, fmt.Errorf("create task namespace: %w", err)
			}
			if err := securePath(tasksRoot, true); err != nil {
				return nil, fmt.Errorf("secure task namespace: %w", err)
			}
		} else if statErr != nil {
			return nil, fmt.Errorf("inspect task namespace: %w", statErr)
		}
	}
	if err := validateSecurePath(tasksRoot, true); err != nil {
		return nil, fmt.Errorf("validate task namespace: %w", err)
	}

	descriptorPath := filepath.Join(root, descriptorName)
	if create {
		if _, statErr := os.Lstat(descriptorPath); errors.Is(statErr, os.ErrNotExist) {
			salt, err := randomHex(32)
			if err != nil {
				return nil, fmt.Errorf("create task descriptor entropy: %w", err)
			}
			desc := descriptor{Format: FormatVersion, Salt: salt, CreatedAt: time.Now().UTC(), Limits: limits}
			if err := writeJSONExclusive(descriptorPath, desc); err != nil && !errors.Is(err, os.ErrExist) {
				return nil, fmt.Errorf("create task descriptor: %w", err)
			}
		}
	}
	if err := validateSecurePath(descriptorPath, false); err != nil {
		return nil, fmt.Errorf("validate task descriptor: %w", err)
	}
	var desc descriptor
	if err := readJSON(descriptorPath, &desc, 16*1024); err != nil {
		return nil, fmt.Errorf("read task descriptor: %w", err)
	}
	_, saltErr := hex.DecodeString(desc.Salt)
	if desc.Format != FormatVersion || len(desc.Salt) != 64 || saltErr != nil || desc.Limits != limits {
		return nil, errors.New("unsupported or invalid task store descriptor")
	}
	return &Store{root: root, tasksRoot: tasksRoot, descriptor: desc, limits: limits}, nil
}

func waitForSecurePath(path string, directory bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		if err := validateSecurePath(path, directory); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func validateLimits(limits Limits) error {
	if limits.MaxConcurrency < 1 || limits.MaxQueued < 1 || limits.MaxLogBytesPerStream < 4096 ||
		limits.MaxRuntimeSeconds < 0 || limits.RetentionDays < 1 || limits.MaxTerminal < 1 || limits.MaxTotalBytes < 1024*1024 {
		return errors.New("task store limits are invalid")
	}
	return nil
}

func (store *Store) Root() string {
	if store == nil {
		return ""
	}
	return store.root
}
func (store *Store) Limits() Limits {
	if store == nil {
		return Limits{}
	}
	return store.limits
}

func (store *Store) Submit(ctx context.Context, request Request) (SubmitResult, error) {
	if store == nil {
		return SubmitResult{}, ErrDisabled
	}
	if err := validateRequest(request, store.limits); err != nil {
		return SubmitResult{}, operation.Wrap(operation.KindInvalidInput, "submit task", "", err)
	}
	if err := ctx.Err(); err != nil {
		return SubmitResult{}, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return SubmitResult{}, err
	}
	payloadDigest := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadDigest[:])
	idDigest := sha256.Sum256([]byte(store.descriptor.Salt + "\x00" + request.IdempotencyKey))
	taskID := "tsk_" + hex.EncodeToString(idDigest[:16])

	lock, err := acquireControlLockContext(ctx, filepath.Join(store.root, controlLockName))
	if err != nil {
		return SubmitResult{}, fmt.Errorf("lock task store: %w", err)
	}
	defer lock.close()
	if err := ctx.Err(); err != nil {
		return SubmitResult{}, err
	}

	taskDir := filepath.Join(store.tasksRoot, taskID)
	if _, err := os.Lstat(taskDir); err == nil {
		existing, readErr := store.readPersistedRequest(taskID)
		if readErr != nil {
			if repairErr := store.removeIncompleteAdmissionUnlocked(taskID); repairErr != nil {
				return SubmitResult{}, fmt.Errorf("existing task admission is incomplete and cannot be repaired: %w", errors.Join(readErr, repairErr))
			}
		} else {
			if existing.PayloadHash != payloadHash {
				return SubmitResult{}, ErrIdempotencyConflict
			}
			task, getErr := store.getTaskUnlocked(taskID)
			if getErr == nil {
				return SubmitResult{Task: task, Duplicated: true}, nil
			}
			if repairErr := store.removeIncompleteAdmissionUnlocked(taskID); repairErr != nil {
				return SubmitResult{}, fmt.Errorf("existing task admission is incomplete and cannot be repaired: %w", errors.Join(getErr, repairErr))
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return SubmitResult{}, err
	}

	queued, err := store.countQueuedUnlocked()
	if err != nil {
		return SubmitResult{}, err
	}
	if queued >= store.limits.MaxQueued {
		return SubmitResult{}, ErrQueueFull
	}

	if err := os.Mkdir(taskDir, 0o700); err != nil {
		return SubmitResult{}, fmt.Errorf("create task: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(taskDir)
		}
	}()
	if err := securePath(taskDir, true); err != nil {
		return SubmitResult{}, err
	}
	statesDir := filepath.Join(taskDir, statesDirectoryName)
	if err := os.Mkdir(statesDir, 0o700); err != nil {
		return SubmitResult{}, err
	}
	if err := securePath(statesDir, true); err != nil {
		return SubmitResult{}, err
	}
	now := time.Now().UTC()
	persisted := persistedRequest{TaskID: taskID, CreatedAt: now, PayloadHash: payloadHash, Request: request}
	if err := writeJSONExclusive(filepath.Join(taskDir, requestName), persisted); err != nil {
		return SubmitResult{}, err
	}
	if err := store.appendStateUnlocked(taskID, stateRecord{Status: StatusQueued, Revision: 1, UpdatedAt: now}); err != nil {
		return SubmitResult{}, err
	}
	cleanup = false
	task, err := store.getTaskUnlocked(taskID)
	return SubmitResult{Task: task}, err
}

// removeIncompleteAdmissionUnlocked repairs only admissions that provably have
// not begun execution. A started marker or a non-queued valid state makes the
// directory untouchable, preserving the at-most-once execution guarantee.
func (store *Store) removeIncompleteAdmissionUnlocked(taskID string) error {
	taskDir := store.taskDir(taskID)
	if fileExists(filepath.Join(taskDir, startedName)) {
		return errors.New("task has an execution start marker")
	}
	if state, err := store.latestState(taskID); err == nil && state.Status != StatusQueued {
		return fmt.Errorf("task reached non-repairable state %s", state.Status)
	}
	if err := validateSecurePath(taskDir, true); err != nil {
		return err
	}
	if !security.IsPathWithinAllowedDirectories(taskDir, []string{store.tasksRoot}) {
		return errors.New("incomplete task escaped the task namespace")
	}
	return os.RemoveAll(taskDir)
}

func validateRequest(request Request, limits Limits) error {
	if request.Kind != KindShell && request.Kind != KindScript {
		return errors.New("kind must be shell or script")
	}
	if !validBoundedText(request.Name, maxNameBytes, true) {
		return fmt.Errorf("name must be valid UTF-8 and at most %d bytes", maxNameBytes)
	}
	if !validBoundedText(request.Description, maxDescriptionBytes, true) {
		return fmt.Errorf("description must be valid UTF-8 and at most %d bytes", maxDescriptionBytes)
	}
	if !validBoundedText(request.IdempotencyKey, maxIdempotencyKeyBytes, false) {
		return fmt.Errorf("idempotencyKey is required and must be at most %d bytes", maxIdempotencyKeyBytes)
	}
	if len(request.Tags) > maxTags || !validUniqueStrings(request.Tags, maxTagBytes) {
		return errors.New("tags are invalid, duplicated, or exceed limits")
	}
	if len(request.LockKeys) > maxLockKeys || !validUniqueStrings(request.LockKeys, maxLockKeyBytes) {
		return errors.New("lockKeys are invalid, duplicated, or exceed limits")
	}
	if request.WorkingDirectory == "" || !filepath.IsAbs(request.WorkingDirectory) {
		return errors.New("workingDirectory must be an absolute path")
	}
	if len(request.WorkingDirectory) > maxPathBytes || strings.ContainsRune(request.WorkingDirectory, 0) {
		return errors.New("workingDirectory exceeds limits")
	}
	if request.MaxRuntimeSeconds < 0 || (limits.MaxRuntimeSeconds > 0 && request.MaxRuntimeSeconds > limits.MaxRuntimeSeconds) {
		return errors.New("maxRuntimeSeconds exceeds the configured task limit")
	}
	if len(request.Args) > maxArgs {
		return fmt.Errorf("args exceeds %d entries", maxArgs)
	}
	for _, arg := range request.Args {
		if len(arg) > maxArgumentBytes || !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
			return errors.New("an argument is invalid or exceeds limits")
		}
	}
	if request.Kind == KindShell {
		if strings.TrimSpace(request.Command) == "" {
			return errors.New("command is required for shell tasks")
		}
		if len(request.Command) > maxCommandBytes || !utf8.ValidString(request.Command) || strings.ContainsRune(request.Command, 0) {
			return errors.New("command is invalid or exceeds limits")
		}
		if len(request.Shell) > 64 || !utf8.ValidString(request.Shell) || strings.ContainsRune(request.Shell, 0) {
			return errors.New("shell is invalid or exceeds limits")
		}
		if request.ScriptPath != "" || request.ScriptDigest != "" || request.ScriptSize != 0 || len(request.Args) != 0 {
			return errors.New("scriptPath and args are not valid for shell tasks")
		}
	} else {
		if request.ScriptPath == "" || !filepath.IsAbs(request.ScriptPath) {
			return errors.New("scriptPath must be absolute for script tasks")
		}
		if len(request.ScriptPath) > maxPathBytes || len(request.ScriptDigest) != 64 || request.ScriptSize < 0 || request.ScriptSize > 1024*1024*1024 {
			return errors.New("scriptPath or scriptDigest is invalid")
		}
		if _, err := hex.DecodeString(request.ScriptDigest); err != nil {
			return errors.New("scriptDigest must be SHA-256 hexadecimal")
		}
		if request.Command != "" || request.Shell != "" {
			return errors.New("command and shell are not valid for script tasks")
		}
	}
	return nil
}

func validBoundedText(value string, maximum int, emptyAllowed bool) bool {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) || len(value) > maximum {
		return false
	}
	return emptyAllowed || strings.TrimSpace(value) != ""
}

func validUniqueStrings(values []string, maximum int) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validBoundedText(value, maximum, false) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func (store *Store) Get(ctx context.Context, taskID string) (Task, error) {
	if store == nil {
		return Task{}, ErrDisabled
	}
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	if !validTaskID(taskID) {
		return Task{}, ErrNotFound
	}
	return store.getTaskUnlocked(taskID)
}

func (store *Store) getTaskUnlocked(taskID string) (Task, error) {
	persisted, err := store.readPersistedRequest(taskID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Task{}, ErrNotFound
		}
		return Task{}, err
	}
	states, err := store.stateHistory(taskID)
	if err != nil {
		return Task{}, err
	}
	state := states[len(states)-1]
	_, cancelErr := os.Lstat(filepath.Join(store.taskDir(taskID), cancelName))
	task := Task{
		ID: taskID, Kind: persisted.Request.Kind, Name: persisted.Request.Name, Description: persisted.Request.Description,
		Tags: append([]string(nil), persisted.Request.Tags...), LockKeys: append([]string(nil), persisted.Request.LockKeys...),
		Status: state.Status, CreatedAt: persisted.CreatedAt, UpdatedAt: state.UpdatedAt, StartedAt: state.StartedAt,
		FinishedAt: state.FinishedAt, MaxRuntimeSeconds: persisted.Request.MaxRuntimeSeconds,
		CancelRequested: cancelErr == nil, WorkerOnline: store.workerOnline(), Result: state.Result, Revision: state.Revision,
	}
	task.History = make([]TaskEvent, 0, len(states))
	for _, event := range states {
		item := TaskEvent{Status: event.Status, Revision: event.Revision, Timestamp: event.UpdatedAt}
		if event.Result != nil {
			item.ErrorCode = event.Result.ErrorCode
		}
		task.History = append(task.History, item)
	}
	return task, nil
}

func (store *Store) List(ctx context.Context, options ListOptions) (ListResult, error) {
	if store == nil {
		return ListResult{}, ErrDisabled
	}
	if err := ctx.Err(); err != nil {
		return ListResult{}, err
	}
	for _, status := range options.Statuses {
		if !validStatus(status) {
			return ListResult{}, operation.Wrap(operation.KindInvalidInput, "list tasks", "", fmt.Errorf("invalid task status filter %q", status))
		}
	}
	for _, kind := range options.Kinds {
		if kind != KindShell && kind != KindScript {
			return ListResult{}, operation.Wrap(operation.KindInvalidInput, "list tasks", "", fmt.Errorf("invalid task kind filter %q", kind))
		}
	}
	if len(options.Tags) > maxTags || !validUniqueStrings(options.Tags, maxTagBytes) {
		return ListResult{}, operation.Wrap(operation.KindInvalidInput, "list tasks", "", errors.New("tag filters are invalid or exceed limits"))
	}
	limit := options.Limit
	if limit == 0 {
		limit = defaultPageSize
	}
	if limit < 1 || limit > maximumPageSize {
		return ListResult{}, operation.Wrap(operation.KindInvalidInput, "list tasks", "", fmt.Errorf("limit must be between 1 and %d", maximumPageSize))
	}
	maximumEntries := store.limits.MaxTerminal + store.limits.MaxQueued + store.limits.MaxConcurrency + 1024
	entries, err := readDirectoryBounded(store.tasksRoot, maximumEntries)
	if err != nil {
		return ListResult{}, err
	}
	tasks := make([]Task, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return ListResult{}, err
		}
		if !entry.IsDir() || !validTaskID(entry.Name()) {
			continue
		}
		task, taskErr := store.getTaskUnlocked(entry.Name())
		if taskErr != nil {
			continue
		}
		if matchesTask(task, options) {
			task.History = nil
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].CreatedAt.Equal(tasks[j].CreatedAt) {
			return tasks[i].ID > tasks[j].ID
		}
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})
	start := 0
	if options.Cursor != "" {
		found := false
		for index := range tasks {
			if tasks[index].ID == options.Cursor {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			return ListResult{}, operation.Wrap(operation.KindInvalidInput, "list tasks", "", errors.New("cursor is invalid or no longer retained"))
		}
	}
	end := start + limit
	if end > len(tasks) {
		end = len(tasks)
	}
	result := ListResult{Tasks: append([]Task(nil), tasks[start:end]...), Truncated: end < len(tasks)}
	if result.Truncated && end > start {
		result.NextCursor = tasks[end-1].ID
	}
	return result, nil
}

func validStatus(status Status) bool {
	switch status {
	case StatusQueued, StatusStarting, StatusRunning, StatusSucceeded, StatusFailed, StatusTimedOut, StatusCancelled, StatusInterrupted:
		return true
	default:
		return false
	}
}

func matchesTask(task Task, options ListOptions) bool {
	if len(options.Statuses) > 0 {
		found := false
		for _, value := range options.Statuses {
			if task.Status == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(options.Kinds) > 0 {
		found := false
		for _, value := range options.Kinds {
			if task.Kind == value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	for _, required := range options.Tags {
		found := false
		for _, value := range task.Tags {
			if value == required {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (store *Store) Cancel(ctx context.Context, taskID, reason string) (Task, error) {
	if store == nil {
		return Task{}, ErrDisabled
	}
	if err := ctx.Err(); err != nil {
		return Task{}, err
	}
	if !validTaskID(taskID) {
		return Task{}, ErrNotFound
	}
	if !validBoundedText(reason, 500, true) {
		return Task{}, operation.Wrap(operation.KindInvalidInput, "cancel task", "", errors.New("reason is invalid or exceeds 500 bytes"))
	}
	lock, err := acquireControlLock(filepath.Join(store.root, controlLockName))
	if err != nil {
		return Task{}, fmt.Errorf("lock task cancellation: %w", err)
	}
	defer lock.close()
	task, err := store.getTaskUnlocked(taskID)
	if err != nil {
		return Task{}, err
	}
	if task.Status.Terminal() {
		return task, nil
	}
	path := filepath.Join(store.taskDir(taskID), cancelName)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		if err := writeJSONExclusive(path, struct {
			RequestedAt time.Time `json:"requestedAt"`
			Reason      string    `json:"reason,omitempty"`
		}{time.Now().UTC(), reason}); err != nil && !errors.Is(err, os.ErrExist) {
			return Task{}, err
		}
	}
	if task.Status == StatusQueued {
		state, err := store.latestState(taskID)
		if err != nil {
			return Task{}, err
		}
		now := time.Now().UTC()
		if err := store.appendStateUnlocked(taskID, stateRecord{Status: StatusCancelled, Revision: state.Revision + 1, UpdatedAt: now, FinishedAt: &now, Result: &Result{ExitCode: -1, ErrorCode: "TASK_CANCELLED", Message: "cancelled before execution"}}); err != nil {
			return Task{}, err
		}
	}
	return store.getTaskUnlocked(taskID)
}

func (store *Store) countQueuedUnlocked() (int, error) {
	entries, err := readDirectoryBounded(store.tasksRoot, store.limits.MaxTerminal+store.limits.MaxQueued+store.limits.MaxConcurrency+1024)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() || !validTaskID(entry.Name()) {
			continue
		}
		state, stateErr := store.latestState(entry.Name())
		if stateErr == nil && state.Status == StatusQueued {
			count++
		}
	}
	return count, nil
}

func (store *Store) readPersistedRequest(taskID string) (persistedRequest, error) {
	var request persistedRequest
	if err := validateSecurePath(store.taskDir(taskID), true); err != nil {
		return request, err
	}
	err := readJSON(filepath.Join(store.taskDir(taskID), requestName), &request, 2*1024*1024)
	if err != nil {
		return request, err
	}
	if request.TaskID != taskID || request.CreatedAt.IsZero() {
		return request, errors.New("task request identity or creation time is invalid")
	}
	if err := validateRequest(request.Request, store.limits); err != nil {
		return request, fmt.Errorf("task request is invalid: %w", err)
	}
	payload, err := json.Marshal(request.Request)
	if err != nil {
		return request, err
	}
	payloadDigest := sha256.Sum256(payload)
	if request.PayloadHash != hex.EncodeToString(payloadDigest[:]) {
		return request, errors.New("task request payload hash mismatch")
	}
	idDigest := sha256.Sum256([]byte(store.descriptor.Salt + "\x00" + request.Request.IdempotencyKey))
	if taskID != "tsk_"+hex.EncodeToString(idDigest[:16]) {
		return request, errors.New("task request identifier derivation mismatch")
	}
	return request, nil
}

func (store *Store) latestState(taskID string) (stateRecord, error) {
	states, err := store.stateHistory(taskID)
	if err != nil {
		return stateRecord{}, err
	}
	return states[len(states)-1], nil
}

func (store *Store) stateHistory(taskID string) ([]stateRecord, error) {
	statesPath := filepath.Join(store.taskDir(taskID), statesDirectoryName)
	if err := validateSecurePath(statesPath, true); err != nil {
		return nil, err
	}
	entries, err := readDirectoryBounded(statesPath, maxStateRecords+64)
	if err != nil {
		return nil, err
	}
	states := make([]stateRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var state stateRecord
		if err := readJSON(filepath.Join(store.taskDir(taskID), statesDirectoryName, entry.Name()), &state, 64*1024); err != nil {
			continue
		}
		if state.Revision < 1 || state.Revision > maxStateRecords || !validStatus(state.Status) || state.UpdatedAt.IsZero() {
			continue
		}
		states = append(states, state)
	}
	if len(states) == 0 {
		return nil, errors.New("task has no valid state")
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Revision < states[j].Revision })
	if states[0].Revision != 1 || states[0].Status != StatusQueued {
		return nil, errors.New("task state history has an invalid initial state")
	}
	for index := 1; index < len(states); index++ {
		if states[index].Revision != states[index-1].Revision+1 || !validStateTransition(states[index-1].Status, states[index].Status) {
			return nil, errors.New("task state history is non-contiguous or contains an invalid transition")
		}
	}
	return states, nil
}

func (store *Store) appendStateUnlocked(taskID string, state stateRecord) error {
	if state.Revision < 1 || state.Revision > maxStateRecords || !validStatus(state.Status) || state.UpdatedAt.IsZero() {
		return errors.New("task state record is invalid or exceeds its immutable history bound")
	}
	if state.Revision == 1 {
		if state.Status != StatusQueued {
			return errors.New("first task state must be queued")
		}
	} else {
		current, err := store.latestState(taskID)
		if err != nil {
			return err
		}
		if state.Revision != current.Revision+1 || !validStateTransition(current.Status, state.Status) {
			return errors.New("task state transition is stale or invalid")
		}
	}
	name := fmt.Sprintf("%020d-%s.json", state.Revision, state.Status)
	return writeJSONExclusive(filepath.Join(store.taskDir(taskID), statesDirectoryName, name), state)
}

// appendState serializes state transitions across frontends, the worker, and
// detached executors. Callers that already hold control.lock must use
// appendStateUnlocked to avoid self-deadlock.
func (store *Store) appendState(taskID string, state stateRecord) (err error) {
	lock, err := acquireControlLock(filepath.Join(store.root, controlLockName))
	if err != nil {
		return fmt.Errorf("lock task state transition: %w", err)
	}
	defer func() { err = errors.Join(err, lock.close()) }()
	return store.appendStateUnlocked(taskID, state)
}

func validStateTransition(from, to Status) bool {
	switch from {
	case StatusQueued:
		return to == StatusStarting || to == StatusCancelled || to == StatusFailed
	case StatusStarting:
		return to == StatusRunning || to == StatusQueued || to == StatusFailed || to == StatusInterrupted
	case StatusRunning:
		return to == StatusSucceeded || to == StatusFailed || to == StatusTimedOut || to == StatusCancelled || to == StatusInterrupted
	default:
		return false
	}
}

func (store *Store) taskDir(taskID string) string { return filepath.Join(store.tasksRoot, taskID) }

func validTaskID(id string) bool {
	if len(id) != 36 || !strings.HasPrefix(id, "tsk_") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, "tsk_"))
	return err == nil
}

func (store *Store) workerOnline() bool {
	info, err := os.Stat(filepath.Join(store.root, workerHeartbeatName))
	if err != nil {
		return false
	}
	age := time.Since(info.ModTime())
	return age >= 0 && age < 5*time.Second
}

func writeJSONExclusive(path string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	directory := filepath.Dir(path)
	suffix, err := randomHex(8)
	if err != nil {
		return err
	}
	temporary := filepath.Join(directory, "."+filepath.Base(path)+"."+suffix+".tmp")
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporary)
		}
	}()
	if err := securePath(temporary, false); err != nil {
		_ = file.Close()
		return err
	}
	_, writeErr := file.Write(payload)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := filesystem.MoveNoReplace(temporary, path); err != nil {
		if errors.Is(err, filesystem.ErrDestinationExists) {
			return os.ErrExist
		}
		if secureFileMatches(path, payload) {
			committed = true
			return nil
		}
		return err
	}
	committed = true
	return nil
}

func secureFileMatches(path string, expected []byte) bool {
	if err := validateSecurePath(path, false); err != nil {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || info.Size() != int64(len(expected)) {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	actual, err := io.ReadAll(io.LimitReader(file, int64(len(expected))+1))
	return err == nil && bytes.Equal(actual, expected)
}

func readJSON(path string, target any, maximum int64) error {
	if err := validateSecurePath(path, false); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maximum+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return errors.New("JSON document contains trailing data")
	}
	if info, err := file.Stat(); err != nil || info.Size() > maximum {
		return errors.New("JSON document exceeds its size limit")
	}
	return nil
}

func randomHex(bytesCount int) (string, error) {
	if bytesCount < 1 {
		return "", errors.New("random byte count must be positive")
	}
	payload := make([]byte, bytesCount)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return hex.EncodeToString(payload), nil
}

func touch(path string) error {
	now := time.Now()
	if err := os.Chtimes(path, now, now); err == nil {
		return nil
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	if err := securePath(path, false); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readDirectoryBounded(path string, limit int) ([]os.DirEntry, error) {
	if limit < 0 {
		return nil, errors.New("directory limit must not be negative")
	}
	directory, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	entries, err := directory.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > limit {
		return nil, errors.New("task registry exceeds its bounded namespace")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}
