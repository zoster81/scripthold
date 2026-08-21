package deferredoperation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"
)

const (
	executionPollInterval      = 100 * time.Millisecond
	executionHeartbeatInterval = time.Second
)

// Execute owns one admitted operation independently from the MCP frontend. It
// claims one global execution slot, writes the durable started marker before
// invoking user-visible work, and never replays an already-started operation.
func (store *Store) Execute(ctx context.Context, operationID string, execute func(context.Context, Request) ([]byte, ResultMetadata, error)) error {
	if store == nil || !ValidOperationID(operationID) || execute == nil {
		return ErrInvalidInput
	}
	ctx = nonNilContext(ctx)
	slot, err := store.acquireExecutionSlot(ctx, operationID)
	if err != nil {
		return err
	}
	defer slot.close()

	claim, err := store.Claim(ctx, operationID)
	if err != nil {
		return err
	}

	runtimeSeconds := claim.Request.MaxRuntimeSeconds
	if runtimeSeconds <= 0 || runtimeSeconds > store.limits.MaxRuntimeSeconds {
		runtimeSeconds = store.limits.MaxRuntimeSeconds
	}
	var (
		executionCtx context.Context
		cancel       context.CancelFunc
	)
	if runtimeSeconds > 0 {
		executionCtx, cancel = context.WithTimeout(ctx, time.Duration(runtimeSeconds)*time.Second)
	} else {
		executionCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	monitorCtx, stopMonitor := context.WithCancel(context.Background())
	monitorErrors := make(chan error, 1)
	go store.monitorExecution(monitorCtx, operationID, cancel, monitorErrors)

	payload, metadata, executeErr := execute(executionCtx, claim.Request)
	stopMonitor()
	select {
	case monitorErr := <-monitorErrors:
		if monitorErr != nil {
			_, _ = store.Fail(operationID, StatusFailed, "EXECUTOR_HEARTBEAT_FAILED", "deferred executor heartbeat failed")
			return monitorErr
		}
	default:
	}

	if fileExists(filepath.Join(store.operationDir(operationID), cancelName)) {
		_, err := store.Fail(operationID, StatusCancelled, "CANCELLED", "deferred operation was cancelled")
		return err
	}
	if errors.Is(executionCtx.Err(), context.DeadlineExceeded) {
		_, err := store.Fail(operationID, StatusTimedOut, "TIMEOUT", "deferred operation exceeded its runtime limit")
		return err
	}
	if errors.Is(executionCtx.Err(), context.Canceled) {
		_, err := store.Fail(operationID, StatusCancelled, "CANCELLED", "deferred operation was cancelled")
		return err
	}
	if executeErr != nil {
		_, err := store.Fail(operationID, StatusFailed, "EXECUTION_FAILED", "deferred operation execution failed")
		return errors.Join(executeErr, err)
	}
	if len(payload) == 0 {
		_, err := store.Fail(operationID, StatusFailed, "RESULT_INVALID", "deferred operation produced no result")
		return err
	}
	if _, err := store.Complete(operationID, payload, metadata); err != nil {
		if errors.Is(err, ErrCapacity) {
			_, failErr := store.Fail(operationID, StatusFailed, "RESULT_LIMIT", "deferred operation result exceeded its retention limit")
			return errors.Join(err, failErr)
		}
		return err
	}
	return nil
}

func (store *Store) acquireExecutionSlot(ctx context.Context, operationID string) (*storeLock, error) {
	heartbeatPath := filepath.Join(store.operationDir(operationID), heartbeatName)
	if err := touch(heartbeatPath); err != nil {
		return nil, err
	}
	lastHeartbeat := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		state, err := store.latestState(operationID)
		if err != nil {
			return nil, err
		}
		if state.Status.Terminal() {
			return nil, ErrTerminal
		}
		if fileExists(filepath.Join(store.operationDir(operationID), cancelName)) && !fileExists(filepath.Join(store.operationDir(operationID), startedName)) {
			_, _ = store.Cancel(context.Background(), operationID, store.requestAllowedDirectories(operationID))
			return nil, ErrTerminal
		}
		for slot := 0; slot < store.limits.MaxConcurrency; slot++ {
			path := filepath.Join(store.root, fmt.Sprintf("execution-%03d.lock", slot))
			lock, lockErr := acquireStoreLock(ctx, path, false)
			if lockErr == nil {
				return lock, nil
			}
		}
		if time.Since(lastHeartbeat) >= executionHeartbeatInterval {
			if err := touch(heartbeatPath); err != nil {
				return nil, err
			}
			lastHeartbeat = time.Now()
		}
		timer := time.NewTimer(executionPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (store *Store) monitorExecution(ctx context.Context, operationID string, cancel context.CancelFunc, errorsOut chan<- error) {
	ticker := time.NewTicker(executionPollInterval)
	defer ticker.Stop()
	lastHeartbeat := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if fileExists(filepath.Join(store.operationDir(operationID), cancelName)) {
				cancel()
			}
			if lastHeartbeat.IsZero() || now.Sub(lastHeartbeat) >= executionHeartbeatInterval {
				if err := touch(filepath.Join(store.operationDir(operationID), heartbeatName)); err != nil {
					select {
					case errorsOut <- err:
					default:
					}
					cancel()
					return
				}
				lastHeartbeat = now
			}
		}
	}
}

func (store *Store) requestAllowedDirectories(operationID string) []string {
	request, err := store.readRequest(operationID)
	if err != nil {
		return nil
	}
	return append([]string(nil), request.Request.AllowedDirectories...)
}
