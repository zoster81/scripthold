package deferredoperation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/zoster81/scripthold/internal/security"
)

type recoveryCandidate struct {
	operationID string
	createdAt   time.Time
}

// prepareRecovery converts safely recoverable queued/pre-start operations into
// fresh starting leases before returning them. The lease prevents concurrent
// frontends from repeatedly relaunching the same pre-start operation.
func (store *Store) prepareRecovery(ctx context.Context, currentAllowedDirectories []string) ([]string, error) {
	if store == nil {
		return nil, ErrDisabled
	}
	ctx = nonNilContext(ctx)
	currentAllowed, err := security.NormalizeAllowedDirs(currentAllowedDirectories)
	if err != nil || len(currentAllowed) == 0 {
		return nil, wrapRecoveryFailure(recoveryFailureRootPolicy, ErrAccessDenied)
	}
	lock, err := store.acquireControlLock(ctx)
	if err != nil {
		return nil, wrapRecoveryFailure(recoveryFailureControlLock, err)
	}
	defer lock.close()

	entries, err := os.ReadDir(store.operationsRoot)
	if err != nil {
		return nil, wrapRecoveryFailure(recoveryFailureStoreScan, err)
	}
	maximum := store.limits.MaxConcurrency + store.limits.MaxQueued + store.limits.MaxTerminal + 1024
	if len(entries) > maximum {
		return nil, wrapRecoveryFailure(recoveryFailureStoreScan, ErrCapacity)
	}

	now := store.now().UTC()
	candidates := make([]recoveryCandidate, 0, min(len(entries), store.limits.MaxConcurrency+store.limits.MaxQueued))
	for _, entry := range entries {
		if !entry.IsDir() || !ValidOperationID(entry.Name()) {
			continue
		}
		operationID := entry.Name()
		request, state, readErr := store.readOperationLocked(operationID)
		if readErr != nil {
			return nil, wrapRecoveryFailure(recoveryFailureRecordRead, readErr)
		}
		if state.Status.Terminal() {
			continue
		}

		if !recoveryRequestAuthorized(request.Request, currentAllowed) {
			if err := store.writeRecoveryTerminal(operationID, state, StatusFailed, "ACCESS_DENIED", "deferred operation is no longer authorized by current roots", now); err != nil {
				return nil, wrapRecoveryFailure(recoveryFailureStateWrite, err)
			}
			continue
		}

		started := fileExists(filepath.Join(store.operationDir(operationID), startedName))
		if started {
			if store.executorHeartbeatFresh(operationID, state, now) {
				continue
			}
			if err := store.writeRecoveryTerminal(operationID, state, StatusInterrupted, "EXECUTOR_LOST", "deferred executor heartbeat was lost; operation was not rerun", now); err != nil {
				return nil, wrapRecoveryFailure(recoveryFailureStateWrite, err)
			}
			continue
		}

		if fileExists(filepath.Join(store.operationDir(operationID), cancelName)) {
			if err := store.writeRecoveryTerminal(operationID, state, StatusCancelled, "CANCELLED", "deferred operation was cancelled before execution", now); err != nil {
				return nil, wrapRecoveryFailure(recoveryFailureStateWrite, err)
			}
			continue
		}

		switch state.Status {
		case StatusQueued:
			// A queued operation has never been dispatched and is immediately safe
			// to lease to the recovering frontend.
		case StatusStarting:
			if store.executorHeartbeatFresh(operationID, state, now) || !stateStaleAt(state, now) {
				continue
			}
		default:
			if err := store.writeRecoveryTerminal(operationID, state, StatusFailed, "STATE_INVALID", "deferred operation had an invalid pre-start state", now); err != nil {
				return nil, wrapRecoveryFailure(recoveryFailureStateWrite, err)
			}
			continue
		}

		if state.Revision >= maxStateRecords-1 {
			if err := store.writeRecoveryTerminal(operationID, state, StatusFailed, "DISPATCH_RETRIES_EXHAUSTED", "deferred operation exhausted bounded pre-start recovery attempts", now); err != nil {
				return nil, wrapRecoveryFailure(recoveryFailureStateWrite, err)
			}
			continue
		}
		lease := stateRecord{Status: StatusStarting, Revision: state.Revision + 1, UpdatedAt: now}
		if err := store.writeStateExclusive(operationID, lease); err != nil {
			return nil, wrapRecoveryFailure(recoveryFailureStateWrite, err)
		}
		if err := touch(filepath.Join(store.operationDir(operationID), heartbeatName)); err != nil {
			return nil, wrapRecoveryFailure(recoveryFailureHeartbeat, err)
		}
		candidates = append(candidates, recoveryCandidate{operationID: operationID, createdAt: request.CreatedAt})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].createdAt.Equal(candidates[j].createdAt) {
			return candidates[i].operationID < candidates[j].operationID
		}
		return candidates[i].createdAt.Before(candidates[j].createdAt)
	})
	result := make([]string, len(candidates))
	for index, candidate := range candidates {
		result[index] = candidate.operationID
	}
	return result, nil
}

func recoveryRequestAuthorized(request Request, currentAllowed []string) bool {
	for _, admittedRoot := range request.AllowedDirectories {
		if _, err := security.ValidatePathWithAllowedDirectories(admittedRoot, currentAllowed, currentAllowed); err != nil {
			return false
		}
	}
	for _, origin := range request.OriginPaths {
		if _, err := security.ValidatePathWithAllowedDirectories(origin, currentAllowed, currentAllowed); err != nil {
			return false
		}
	}
	return true
}

func stateStaleAt(state stateRecord, now time.Time) bool {
	age := now.Sub(state.UpdatedAt)
	return age > executorStaleAfter
}

func (store *Store) executorHeartbeatFresh(operationID string, state stateRecord, observation time.Time) bool {
	info, err := os.Stat(filepath.Join(store.operationDir(operationID), heartbeatName))
	if err == nil {
		modifiedAt := info.ModTime()
		if modifiedAt.After(observation) {
			// The executor may refresh its heartbeat after the caller captured its
			// observation time. Refresh the clock once instead of classifying a
			// live executor as lost or accepting an arbitrarily future timestamp.
			observation = store.now().UTC()
		}
		return timestampFreshAt(modifiedAt, observation)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	// Claim writes the running state immediately before the first heartbeat. A
	// short grace avoids declaring a live just-started executor lost in that gap.
	return timestampFreshAt(state.UpdatedAt, observation)
}

func timestampFreshAt(timestamp, observation time.Time) bool {
	age := observation.Sub(timestamp)
	return age >= 0 && age <= executorStaleAfter
}

func (store *Store) writeRecoveryTerminal(operationID string, current stateRecord, status Status, code, message string, now time.Time) error {
	if current.Revision >= maxStateRecords {
		return errors.New("deferred operation state history exhausted")
	}
	metadata := ResultMetadata{ErrorCode: code}
	finished := now
	state := stateRecord{
		Status:     status,
		Revision:   current.Revision + 1,
		UpdatedAt:  now,
		StartedAt:  current.StartedAt,
		FinishedAt: &finished,
		Result:     &metadata,
		Message:    boundedMessage(message),
	}
	return store.writeStateExclusive(operationID, state)
}
