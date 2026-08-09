package taskstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// RunExecutor runs one already-authorized launch record. It is intentionally
// process-independent from the task worker and MCP frontends.
func RunExecutor(ctx context.Context, store *Store, taskID, token string) error {
	if store == nil || !validTaskID(taskID) || len(token) != 64 {
		return errors.New("invalid task executor invocation")
	}
	taskDir := store.taskDir(taskID)
	var launch launchRecord
	if err := readJSON(filepath.Join(taskDir, "launch.json"), &launch, 2*1024*1024); err != nil {
		return err
	}
	if launch.ExecutorToken != token {
		return errors.New("task executor token mismatch")
	}
	current, err := store.latestState(taskID)
	if err != nil {
		return err
	}
	if current.Status != StatusStarting || current.ExecutorToken != token {
		return errors.New("task is not in the expected starting state")
	}

	stdout, err := newBoundedLogWriter(taskDir, "stdout", store.limits.MaxLogBytesPerStream)
	if err != nil {
		return err
	}
	stderr, err := newBoundedLogWriter(taskDir, "stderr", store.limits.MaxLogBytesPerStream)
	if err != nil {
		_ = stdout.Close()
		return err
	}
	defer stdout.Close()
	defer stderr.Close()

	now := time.Now().UTC()
	startedMarker := struct {
		ExecutorToken string    `json:"executorToken"`
		ExecutorPID   int       `json:"executorPid"`
		MarkedAt      time.Time `json:"markedAt"`
	}{token, os.Getpid(), now}
	if err := writeJSONExclusive(filepath.Join(taskDir, startedName), startedMarker); err != nil {
		return fmt.Errorf("claim task execution: %w", err)
	}
	if err := touch(filepath.Join(taskDir, heartbeatName)); err != nil {
		return err
	}
	running := stateRecord{Status: StatusRunning, Revision: current.Revision + 1, UpdatedAt: now, StartedAt: &now, ExecutorPID: os.Getpid(), ExecutorToken: token}
	if err := store.appendState(taskID, running); err != nil {
		return err
	}

	if fileExists(filepath.Join(taskDir, cancelName)) {
		return finishExecutor(store, taskID, running, StatusCancelled, -1, "TASK_CANCELLED", "cancelled before process start", now)
	}

	command := exec.Command(launch.Program, launch.Args...)
	command.Dir = launch.WorkingDirectory
	command.Stdin = nil
	command.Stdout = stdout
	command.Stderr = stderr
	command.WaitDelay = 2 * time.Second
	configureProcessGroup(command)
	if err := command.Start(); err != nil {
		return finishExecutor(store, taskID, running, StatusFailed, -1, "PROCESS_START_FAILED", "process could not be started", now)
	}

	wait := make(chan error, 1)
	go func() { wait <- command.Wait() }()
	ticker := time.NewTicker(executorHeartbeatInterval)
	defer ticker.Stop()
	var deadline <-chan time.Time
	var timer *time.Timer
	if launch.MaxRuntimeSeconds > 0 {
		timer = time.NewTimer(time.Duration(launch.MaxRuntimeSeconds) * time.Second)
		deadline = timer.C
		defer timer.Stop()
	}
	startedAt := time.Now()
	status := StatusSucceeded
	code := ""
	message := ""
	exitCode := 0
	var waitErr error
	completed := false
	for !completed {
		select {
		case waitErr = <-wait:
			completed = true
		case <-ticker.C:
			_ = touch(filepath.Join(taskDir, heartbeatName))
			_ = stdout.Snapshot()
			_ = stderr.Snapshot()
			if fileExists(filepath.Join(taskDir, cancelName)) {
				status, code, message = StatusCancelled, "TASK_CANCELLED", "task cancellation was requested"
				terminateProcessTree(command)
				waitErr = <-wait
				completed = true
			}
		case <-deadline:
			status, code, message = StatusTimedOut, "TASK_TIMED_OUT", "configured maximum runtime was reached"
			terminateProcessTree(command)
			waitErr = <-wait
			completed = true
		case <-ctx.Done():
			status, code, message = StatusInterrupted, "EXECUTOR_STOPPED", "task executor was stopped"
			terminateProcessTree(command)
			waitErr = <-wait
			completed = true
		}
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if status == StatusSucceeded {
			status, code, message, exitCode = StatusFailed, "PROCESS_WAIT_FAILED", "process wait failed", -1
		}
		if status == StatusSucceeded {
			status, code, message = StatusFailed, "PROCESS_EXIT_NONZERO", "process exited with a non-zero status"
		}
	}
	finished := time.Now().UTC()
	result := &Result{ExitCode: exitCode, DurationMillis: time.Since(startedAt).Milliseconds(), ErrorCode: code, Message: message}
	terminal := stateRecord{Status: status, Revision: running.Revision + 1, UpdatedAt: finished, StartedAt: running.StartedAt, FinishedAt: &finished, ExecutorPID: os.Getpid(), ExecutorToken: token, Result: result}
	_ = stdout.Snapshot()
	_ = stderr.Snapshot()
	if err := store.appendState(taskID, terminal); err != nil {
		return err
	}
	return nil
}

func finishExecutor(store *Store, taskID string, current stateRecord, status Status, exitCode int, code, message string, started time.Time) error {
	finished := time.Now().UTC()
	result := &Result{ExitCode: exitCode, DurationMillis: time.Since(started).Milliseconds(), ErrorCode: code, Message: message}
	return store.appendState(taskID, stateRecord{Status: status, Revision: current.Revision + 1, UpdatedAt: finished, StartedAt: current.StartedAt, FinishedAt: &finished, ExecutorPID: os.Getpid(), ExecutorToken: current.ExecutorToken, Result: result})
}
