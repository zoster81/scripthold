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
	if ctx == nil {
		ctx = context.Background()
	}
	currentAllowed, err := security.NormalizeAllowedDirs(currentAllowedDirectories)
	if err != nil || len(currentAllowed) == 0 {
		return nil, ErrAccessDenied
	}
	lock, err := acquireStoreLock(ctx, filepath.Join(store.root, controlName), true)
	if err != nil {
		return nil, err
	}
	defer lock.close()

	entries, err := os.ReadDir(store.operationsRoot)
	if err != nil {
		return nil, err
	}
	maximum := store.limits.MaxConcurrency + store.limits.MaxQueued + store.limits.MaxTerminal + 1024
	if len(entries) > maximum {
		return nil, ErrCapacity
	}

	now := store.now().UTC()
	candidates := make([]recoveryCandidate, 0, min(len(entries), store.limits.MaxConcurrency+store.limits.MaxQueued))
	for _, entry := range entries {
		if !entry.IsDir() || !ValidOperationID(entry.Name()) {
			continue
		}
		operationID := entry.Name()
		request, requestErr := store.readRequest(operationID)
		if requestErr != nil {
			return nil, requestErr
		}
		state, stateErr := store.latestState(operationID)
		if stateErr != nil {
			return nil, stateErr
		}
		if state.Status.Terminal() {
			continue
		}

		if !recoveryRequestAuthorized(request.Request, currentAllowed) {
			if err := store.writeRecoveryTerminal(operationID, state, StatusFailed, "ACCESS_DENIED", "deferred operation is no longer authorized by current roots", now); err != nil {
				return nil, err
			}
			continue
		}

		started := fileExists(filepath.Join(store.operationDir(operationID), startedName))
		if started {
			if store.executorHeartbeatFresh(operationID, state, now) {
				continue
			}
			if err := store.writeRecoveryTerminal(operationID, state, StatusInterrupted, "EXECUTOR_LOST", "deferred executor heartbeat was lost; operation was not rerun", now); err != nil {
				return nil, err
			}
			continue
		}

		if fileExists(filepath.Join(store.operationDir(operationID), cancelName)) {
			if err := store.writeRecoveryTerminal(operationID, state, StatusCancelled, "CANCELLED", "deferred operation was cancelled before execution", now); err != nil {
				return nil, err
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
				return nil, err
			}
			continue
		}

		if state.Revision >= maxStateRecords-1 {
			if err := store.writeRecoveryTerminal(operationID, state, StatusFailed, "DISPATCH_RETRIES_EXHAUSTED", "deferred operation exhausted bounded pre-start recovery attempts", now); err != nil {
				return nil, err
			}
			continue
		}
		lease := stateRecord{Status: StatusStarting, Revision: state.Revision + 1, UpdatedAt: now}
		if err := store.writeStateExclusive(operationID, lease); err != nil {
			return nil, err
		}
		if err := touch(filepath.Join(store.operationDir(operationID), heartbeatName)); err != nil {
			return nil, err
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

func (store *Store) executorHeartbeatFresh(operationID string, state stateRecord, now time.Time) bool {
	info, err := os.Stat(filepath.Join(store.operationDir(operationID), heartbeatName))
	if err == nil {
		age := now.Sub(info.ModTime())
		return age >= 0 && age <= executorStaleAfter
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false
	}
	// Claim writes the running state immediately before the first heartbeat. A
	// short grace avoids declaring a live just-started executor lost in that gap.
	age := now.Sub(state.UpdatedAt)
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
