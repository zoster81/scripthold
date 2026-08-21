package deferredoperation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zoster81/scripthold/internal/security"
)

const (
	requestName     = "request.json"
	resultName      = "result.json"
	startedName     = "started"
	exposedName     = "exposed"
	cancelName      = "cancel"
	heartbeatName   = "heartbeat"
	controlName     = "control.lock"
	operationsDir   = "operations"
	maxRequestBytes = 2 * 1024 * 1024
)

type Store struct {
	root           string
	operationsRoot string
	limits         Limits
	now            func() time.Time
}

func Initialize(root string, publicAllowedDirectories, otherPrivateRoots []string, limits Limits) (*Store, error) {
	limits, err := normalizeLimits(limits)
	if err != nil {
		return nil, err
	}
	clean, err := validateStoreRootBeforeCreate(root, publicAllowedDirectories, otherPrivateRoots)
	if err != nil {
		return nil, err
	}
	if err := createSecureDirectory(clean); err != nil {
		return nil, err
	}
	if err := validateSecurePath(clean, true); err != nil {
		return nil, err
	}
	store := &Store{root: clean, operationsRoot: filepath.Join(clean, operationsDir), limits: limits, now: time.Now}
	if err := createSecureDirectory(store.operationsRoot); err != nil {
		return nil, err
	}
	lock, err := store.acquireControlLock(context.Background())
	if err != nil {
		return nil, err
	}
	defer lock.close()
	descriptorPath := filepath.Join(clean, "store.json")
	if _, statErr := os.Lstat(descriptorPath); errors.Is(statErr, os.ErrNotExist) {
		if err := writeJSONExclusive(descriptorPath, descriptor{Format: FormatVersion, CreatedAt: time.Now().UTC(), Limits: limits}); err != nil {
			return nil, err
		}
	} else if statErr != nil {
		return nil, statErr
	} else {
		var existing descriptor
		if err := readJSONBounded(descriptorPath, 64*1024, &existing); err != nil {
			return nil, err
		}
		if existing.Format != FormatVersion {
			return nil, errors.New("deferred operation store format is unsupported")
		}
		if !reflect.DeepEqual(existing.Limits, limits) {
			return nil, errors.New("deferred operation store limits do not match its immutable descriptor")
		}
	}
	return store, nil
}

func OpenExecutor(root string) (*Store, error) {
	clean, err := filepath.Abs(root)
	if err != nil || strings.TrimSpace(root) == "" {
		return nil, ErrInvalidInput
	}
	clean = filepath.Clean(clean)
	resolved, err := filepath.EvalSymlinks(clean)
	if err != nil || !security.PathsEqual(clean, resolved) {
		return nil, errors.New("deferred operation store path must not traverse a link or reparse point")
	}
	if err := validateSecurePath(clean, true); err != nil {
		return nil, err
	}
	var existing descriptor
	if err := readJSONBounded(filepath.Join(clean, "store.json"), 64*1024, &existing); err != nil {
		return nil, err
	}
	if existing.Format != FormatVersion {
		return nil, errors.New("deferred operation store format is unsupported")
	}
	limits, err := normalizeLimits(existing.Limits)
	if err != nil {
		return nil, err
	}
	operations := filepath.Join(clean, operationsDir)
	if err := validateSecurePath(operations, true); err != nil {
		return nil, err
	}
	return &Store{root: clean, operationsRoot: operations, limits: limits, now: time.Now}, nil
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

// acquireControlLock serializes durable state/marker publication and coherent observation.
func (store *Store) acquireControlLock(ctx context.Context) (*storeLock, error) {
	return acquireStoreLock(nonNilContext(ctx), filepath.Join(store.root, controlName), true)
}

// readOperationLocked reads one immutable request and its latest state while the caller owns the control lock.
func (store *Store) readOperationLocked(operationID string) (persistedRequest, stateRecord, error) {
	request, err := store.readRequest(operationID)
	if err != nil {
		return persistedRequest{}, stateRecord{}, err
	}
	state, err := store.latestState(operationID)
	if err != nil {
		return persistedRequest{}, stateRecord{}, err
	}
	return request, state, nil
}

func normalizeLimits(limits Limits) (Limits, error) {
	if limits.MaxConcurrency <= 0 {
		limits.MaxConcurrency = 4
	}
	if limits.MaxQueued <= 0 {
		limits.MaxQueued = 64
	}
	if limits.MaxRuntimeSeconds <= 0 {
		limits.MaxRuntimeSeconds = 300
	}
	if limits.RetentionSeconds <= 0 {
		limits.RetentionSeconds = 3600
	}
	if limits.MaxTerminal <= 0 {
		limits.MaxTerminal = 64
	}
	if limits.MaxTotalBytes <= 0 {
		limits.MaxTotalBytes = 256 * 1024 * 1024
	}
	if limits.MaxResultBytes <= 0 {
		limits.MaxResultBytes = 64 * 1024 * 1024
	}
	if limits.MaxChunkBytes <= 0 {
		limits.MaxChunkBytes = 1024 * 1024
	}
	if limits.MaxConcurrency > 64 || limits.MaxQueued > 1024 || limits.MaxRuntimeSeconds > 24*60*60 || limits.RetentionSeconds > 7*24*60*60 || limits.MaxTerminal > 4096 || limits.MaxTotalBytes > 1<<40 || limits.MaxResultBytes > limits.MaxTotalBytes || limits.MaxChunkBytes > 8*1024*1024 {
		return Limits{}, ErrInvalidInput
	}
	return limits, nil
}

func validateStoreRootBeforeCreate(root string, publicAllowedDirectories, otherPrivateRoots []string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", ErrDisabled
	}
	clean, err := filepath.Abs(root)
	if err != nil {
		return "", ErrInvalidInput
	}
	clean = filepath.Clean(clean)
	ancestor := clean
	for {
		if _, statErr := os.Lstat(ancestor); statErr == nil {
			break
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", statErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", errors.New("deferred operation store has no existing ancestor")
		}
		ancestor = parent
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(ancestor, clean)
	if err != nil {
		return "", err
	}
	resolvedCandidate := filepath.Clean(filepath.Join(resolvedAncestor, relative))
	if !security.PathsEqual(clean, resolvedCandidate) {
		return "", errors.New("deferred operation store path must not traverse a link or reparse point")
	}
	for _, candidate := range append(append([]string(nil), publicAllowedDirectories...), otherPrivateRoots...) {
		if strings.TrimSpace(candidate) == "" {
			continue
		}
		absolute, absErr := filepath.Abs(candidate)
		if absErr != nil {
			return "", ErrInvalidInput
		}
		if security.PathsOverlap(clean, filepath.Clean(absolute)) {
			return "", errors.New("deferred operation store must not overlap public or private authority roots")
		}
	}
	return clean, nil
}

func createSecureDirectory(path string) error {
	if info, err := os.Lstat(path); err == nil {
		if !info.IsDir() {
			return errors.New("deferred operation store path is not a directory")
		}
		return securePath(path, true)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent != path {
		if _, err := os.Lstat(parent); errors.Is(err, os.ErrNotExist) {
			if err := createSecureDirectory(parent); err != nil {
				return err
			}
		}
	}
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return securePath(path, true)
}

func (store *Store) Admit(ctx context.Context, request Request) (Operation, error) {
	if store == nil {
		return Operation{}, ErrDisabled
	}
	ctx = nonNilContext(ctx)
	if err := validateRequest(request, store.limits); err != nil {
		return Operation{}, err
	}
	resolvedAllowed, err := security.NormalizeAllowedDirs(request.AllowedDirectories)
	if err != nil || len(resolvedAllowed) == 0 {
		return Operation{}, ErrAccessDenied
	}
	request.AllowedDirectories = resolvedAllowed
	for index, origin := range request.OriginPaths {
		validated, validateErr := security.ValidatePathWithAllowedDirectories(origin, resolvedAllowed, resolvedAllowed)
		if validateErr != nil {
			return Operation{}, ErrAccessDenied
		}
		request.OriginPaths[index] = validated
	}
	lock, err := store.acquireControlLock(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer lock.close()
	if err := store.purgeLocked(); err != nil {
		return Operation{}, err
	}
	active, err := store.activeCountLocked()
	if err != nil {
		return Operation{}, err
	}
	if active >= store.limits.MaxConcurrency+store.limits.MaxQueued {
		return Operation{}, ErrCapacity
	}
	id, err := randomOperationID()
	if err != nil {
		return Operation{}, err
	}
	now := store.now().UTC()
	directory := store.operationDir(id)
	if err := os.Mkdir(directory, 0o700); err != nil {
		return Operation{}, err
	}
	if err := securePath(directory, true); err != nil {
		_ = os.RemoveAll(directory)
		return Operation{}, err
	}
	persisted := persistedRequest{OperationID: id, CreatedAt: now, Request: request}
	if err := writeJSONExclusive(filepath.Join(directory, requestName), persisted); err != nil {
		_ = os.RemoveAll(directory)
		return Operation{}, err
	}
	state := stateRecord{Status: StatusQueued, Revision: 1, UpdatedAt: now}
	if err := store.writeStateExclusive(id, state); err != nil {
		_ = os.RemoveAll(directory)
		return Operation{}, err
	}
	return store.operationFrom(persisted, state), nil
}

func validateRequest(request Request, limits Limits) error {
	if strings.TrimSpace(request.Tool) == "" || len(request.Tool) > 128 || len(request.Arguments) == 0 || len(request.Arguments) > maxRequestBytes || !json.Valid(request.Arguments) || len(request.AllowedDirectories) == 0 || len(request.AllowedDirectories) > 64 || len(request.OriginPaths) > 256 {
		return ErrInvalidInput
	}
	for _, value := range append(append([]string(nil), request.AllowedDirectories...), request.OriginPaths...) {
		if strings.TrimSpace(value) == "" || len(value) > 32*1024 {
			return ErrInvalidInput
		}
	}
	if request.MaxRuntimeSeconds < 0 || request.MaxRuntimeSeconds > limits.MaxRuntimeSeconds {
		return ErrInvalidInput
	}
	return nil
}

func (store *Store) MarkStarting(ctx context.Context, operationID string) (Operation, error) {
	return store.transition(ctx, operationID, func(request persistedRequest, current stateRecord) (stateRecord, error) {
		if current.Status.Terminal() {
			return stateRecord{}, ErrTerminal
		}
		if current.Status != StatusQueued {
			return stateRecord{}, ErrAlreadyStarted
		}
		now := store.now().UTC()
		return stateRecord{Status: StatusStarting, Revision: current.Revision + 1, UpdatedAt: now}, nil
	})
}

func (store *Store) Claim(ctx context.Context, operationID string) (Claim, error) {
	if store == nil || !ValidOperationID(operationID) {
		return Claim{}, ErrInvalidInput
	}
	ctx = nonNilContext(ctx)
	lock, err := store.acquireControlLock(ctx)
	if err != nil {
		return Claim{}, err
	}
	defer lock.close()
	request, current, err := store.readOperationLocked(operationID)
	if err != nil {
		return Claim{}, err
	}
	if current.Status.Terminal() {
		return Claim{}, ErrTerminal
	}
	if fileExists(filepath.Join(store.operationDir(operationID), startedName)) {
		return Claim{}, ErrAlreadyStarted
	}
	if fileExists(filepath.Join(store.operationDir(operationID), cancelName)) {
		now := store.now().UTC()
		terminal := stateRecord{Status: StatusCancelled, Revision: current.Revision + 1, UpdatedAt: now, FinishedAt: &now, Message: "operation cancelled before execution"}
		if err := store.writeStateExclusive(operationID, terminal); err != nil {
			return Claim{}, err
		}
		return Claim{}, ErrTerminal
	}
	if err := writeMarkerExclusive(filepath.Join(store.operationDir(operationID), startedName)); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Claim{}, ErrAlreadyStarted
		}
		return Claim{}, err
	}
	now := store.now().UTC()
	started := now
	state := stateRecord{Status: StatusRunning, Revision: current.Revision + 1, UpdatedAt: now, StartedAt: &started}
	if err := store.writeStateExclusive(operationID, state); err != nil {
		return Claim{}, err
	}
	if err := touch(filepath.Join(store.operationDir(operationID), heartbeatName)); err != nil {
		return Claim{}, err
	}
	return Claim{Request: request.Request, Operation: store.operationFrom(request, state)}, nil
}

func (store *Store) Complete(operationID string, payload []byte, metadata ResultMetadata) (Operation, error) {
	if store == nil || !ValidOperationID(operationID) || len(payload) == 0 || !utf8.Valid(payload) || int64(len(payload)) > store.limits.MaxResultBytes {
		return Operation{}, ErrCapacity
	}
	lock, err := store.acquireControlLock(context.Background())
	if err != nil {
		return Operation{}, err
	}
	defer lock.close()
	request, current, err := store.readOperationLocked(operationID)
	if err != nil {
		return Operation{}, err
	}
	if current.Status.Terminal() {
		return store.operationFrom(request, current), ErrTerminal
	}
	if current.Status != StatusRunning || !fileExists(filepath.Join(store.operationDir(operationID), startedName)) {
		return Operation{}, ErrAlreadyStarted
	}
	if err := writeBytesExclusive(filepath.Join(store.operationDir(operationID), resultName), payload); err != nil {
		return Operation{}, err
	}
	now := store.now().UTC()
	state := stateRecord{Status: StatusCompleted, Revision: current.Revision + 1, UpdatedAt: now, StartedAt: current.StartedAt, FinishedAt: &now, Result: &metadata, TotalBytes: int64(len(payload))}
	if err := store.writeStateExclusive(operationID, state); err != nil {
		return Operation{}, err
	}
	return store.operationFrom(request, state), nil
}

func (store *Store) Fail(operationID string, status Status, code, message string) (Operation, error) {
	if store == nil || !ValidOperationID(operationID) || (status != StatusFailed && status != StatusTimedOut && status != StatusCancelled && status != StatusInterrupted) {
		return Operation{}, ErrInvalidInput
	}
	lock, err := store.acquireControlLock(context.Background())
	if err != nil {
		return Operation{}, err
	}
	defer lock.close()
	request, current, err := store.readOperationLocked(operationID)
	if err != nil {
		return Operation{}, err
	}
	if current.Status.Terminal() {
		return store.operationFrom(request, current), nil
	}
	now := store.now().UTC()
	metadata := ResultMetadata{ErrorCode: code}
	state := stateRecord{Status: status, Revision: current.Revision + 1, UpdatedAt: now, StartedAt: current.StartedAt, FinishedAt: &now, Result: &metadata, Message: boundedMessage(message)}
	if err := store.writeStateExclusive(operationID, state); err != nil {
		return Operation{}, err
	}
	return store.operationFrom(request, state), nil
}

func (store *Store) MarkExposed(ctx context.Context, operationID string, currentAllowedDirectories []string) (Operation, error) {
	if _, err := store.GetContext(ctx, operationID, currentAllowedDirectories); err != nil {
		return Operation{}, err
	}
	path := filepath.Join(store.operationDir(operationID), exposedName)
	if err := writeMarkerExclusive(path); err != nil && !errors.Is(err, os.ErrExist) {
		return Operation{}, err
	}
	return store.GetContext(ctx, operationID, currentAllowedDirectories)
}

func (store *Store) Cancel(ctx context.Context, operationID string, currentAllowedDirectories []string) (Operation, error) {
	if store == nil || !ValidOperationID(operationID) {
		return Operation{}, ErrInvalidInput
	}
	ctx = nonNilContext(ctx)
	if err := store.validateVisibility(operationID, currentAllowedDirectories); err != nil {
		return Operation{}, err
	}
	lock, err := store.acquireControlLock(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer lock.close()
	request, current, err := store.readOperationLocked(operationID)
	if err != nil {
		return Operation{}, err
	}
	if current.Status.Terminal() {
		return store.operationFrom(request, current), nil
	}
	if err := writeMarkerExclusive(filepath.Join(store.operationDir(operationID), cancelName)); err != nil && !errors.Is(err, os.ErrExist) {
		return Operation{}, err
	}
	if !fileExists(filepath.Join(store.operationDir(operationID), startedName)) {
		now := store.now().UTC()
		state := stateRecord{Status: StatusCancelled, Revision: current.Revision + 1, UpdatedAt: now, FinishedAt: &now, Message: "operation cancelled before execution"}
		if err := store.writeStateExclusive(operationID, state); err != nil {
			return Operation{}, err
		}
		return store.operationFrom(request, state), nil
	}
	operation := store.operationFrom(request, current)
	operation.CancelRequested = true
	return operation, nil
}

func (store *Store) Get(operationID string, currentAllowedDirectories []string) (Operation, error) {
	return store.GetContext(context.Background(), operationID, currentAllowedDirectories)
}

func (store *Store) GetContext(ctx context.Context, operationID string, currentAllowedDirectories []string) (Operation, error) {
	if store == nil || !ValidOperationID(operationID) {
		return Operation{}, ErrInvalidInput
	}
	ctx = nonNilContext(ctx)
	if err := ctx.Err(); err != nil {
		return Operation{}, err
	}
	if err := store.validateVisibility(operationID, currentAllowedDirectories); err != nil {
		return Operation{}, err
	}
	lock, err := store.acquireControlLock(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer lock.close()
	if err := store.reconcileLostExecutorLocked(operationID); err != nil {
		return Operation{}, err
	}
	request, state, err := store.readOperationLocked(operationID)
	if err != nil {
		return Operation{}, err
	}
	return store.operationFrom(request, state), nil
}

// ReadResult returns the complete retained result only when it fits the store's
// configured per-result bound. It is used by a frontend that observed a fast
// completion during the synchronous grace window.
func (store *Store) ReadResult(operationID string, currentAllowedDirectories []string) ([]byte, Operation, error) {
	return store.ReadResultContext(context.Background(), operationID, currentAllowedDirectories)
}

func (store *Store) ReadResultContext(ctx context.Context, operationID string, currentAllowedDirectories []string) ([]byte, Operation, error) {
	ctx = nonNilContext(ctx)
	operation, err := store.GetContext(ctx, operationID, currentAllowedDirectories)
	if err != nil {
		return nil, Operation{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, operation, err
	}
	if operation.Status != StatusCompleted || !operation.ResultAvailable || operation.TotalBytes <= 0 || operation.TotalBytes > store.limits.MaxResultBytes {
		return nil, operation, ErrResultMissing
	}
	path := filepath.Join(store.operationDir(operationID), resultName)
	if err := validateSecurePath(path, false); err != nil {
		return nil, operation, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, operation, err
	}
	if !info.Mode().IsRegular() || info.Size() != operation.TotalBytes || info.Size() > store.limits.MaxResultBytes {
		return nil, operation, ErrResultMissing
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, operation, err
	}
	if int64(len(payload)) != operation.TotalBytes || !utf8.Valid(payload) {
		return nil, operation, ErrResultMissing
	}
	return payload, operation, nil
}

func (store *Store) ResultChunk(operationID string, currentAllowedDirectories []string, offset int64, limitBytes int) (Chunk, error) {
	return store.ResultChunkContext(context.Background(), operationID, currentAllowedDirectories, offset, limitBytes)
}

func (store *Store) ResultChunkContext(ctx context.Context, operationID string, currentAllowedDirectories []string, offset int64, limitBytes int) (Chunk, error) {
	ctx = nonNilContext(ctx)
	operation, err := store.GetContext(ctx, operationID, currentAllowedDirectories)
	if err != nil {
		return Chunk{}, err
	}
	if err := ctx.Err(); err != nil {
		return Chunk{}, err
	}
	if operation.Status != StatusCompleted || !operation.ResultAvailable {
		return Chunk{}, ErrResultMissing
	}
	if offset < 0 || offset > operation.TotalBytes || limitBytes < 0 {
		return Chunk{}, ErrInvalidInput
	}
	limit := limitBytes
	if limit == 0 || limit > store.limits.MaxChunkBytes {
		limit = store.limits.MaxChunkBytes
	}
	path := filepath.Join(store.operationDir(operationID), resultName)
	if err := validateSecurePath(path, false); err != nil {
		return Chunk{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return Chunk{}, err
	}
	defer file.Close()
	if offset < operation.TotalBytes && offset > 0 {
		one := []byte{0}
		if _, err := file.ReadAt(one, offset); err != nil {
			return Chunk{}, err
		}
		if one[0]&0xC0 == 0x80 {
			return Chunk{}, ErrInvalidInput
		}
	}
	remaining := operation.TotalBytes - offset
	readSize := int64(limit)
	if readSize > remaining {
		readSize = remaining
	}
	buffer := make([]byte, int(readSize))
	if readSize > 0 {
		n, readErr := file.ReadAt(buffer, offset)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return Chunk{}, readErr
		}
		buffer = buffer[:n]
	}
	end := len(buffer)
	for end > 0 && offset+int64(end) < operation.TotalBytes && !utf8.Valid(buffer[:end]) {
		end--
	}
	if end == 0 && offset < operation.TotalBytes {
		probe := make([]byte, utf8.UTFMax)
		n, readErr := file.ReadAt(probe, offset)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return Chunk{}, readErr
		}
		_, size := utf8.DecodeRune(probe[:n])
		if size <= 0 || size > store.limits.MaxChunkBytes {
			return Chunk{}, ErrInvalidInput
		}
		buffer = probe[:size]
	} else {
		buffer = buffer[:end]
	}
	next := offset + int64(len(buffer))
	return Chunk{OperationID: operationID, Status: operation.Status, OriginalIsError: operation.OriginalIsError, ErrorCode: operation.ErrorCode, TotalBytes: operation.TotalBytes, Offset: offset, NextOffset: next, Data: string(buffer), Complete: next == operation.TotalBytes}, nil
}

func (store *Store) transition(ctx context.Context, operationID string, build func(persistedRequest, stateRecord) (stateRecord, error)) (Operation, error) {
	if store == nil || !ValidOperationID(operationID) {
		return Operation{}, ErrInvalidInput
	}
	ctx = nonNilContext(ctx)
	lock, err := store.acquireControlLock(ctx)
	if err != nil {
		return Operation{}, err
	}
	defer lock.close()
	request, current, err := store.readOperationLocked(operationID)
	if err != nil {
		return Operation{}, err
	}
	next, err := build(request, current)
	if err != nil {
		return Operation{}, err
	}
	if err := store.writeStateExclusive(operationID, next); err != nil {
		return Operation{}, err
	}
	return store.operationFrom(request, next), nil
}

func (store *Store) validateVisibility(operationID string, currentAllowedDirectories []string) error {
	request, err := store.readRequest(operationID)
	if err != nil {
		return err
	}
	if len(request.Request.OriginPaths) == 0 {
		return nil
	}
	resolved, err := security.NormalizeAllowedDirs(currentAllowedDirectories)
	if err != nil || len(resolved) == 0 {
		return ErrAccessDenied
	}
	for _, origin := range request.Request.OriginPaths {
		if _, err := security.ValidatePathWithAllowedDirectories(origin, resolved, resolved); err != nil {
			return ErrAccessDenied
		}
	}
	return nil
}

func (store *Store) reconcileLostExecutorLocked(operationID string) error {
	state, err := store.latestState(operationID)
	if err != nil || state.Status.Terminal() || (state.Status != StatusRunning && state.Status != StatusStarting) || !fileExists(filepath.Join(store.operationDir(operationID), startedName)) {
		return err
	}
	now := store.now().UTC()
	if store.executorHeartbeatFresh(operationID, state, now) {
		return nil
	}
	return store.writeRecoveryTerminal(operationID, state, StatusInterrupted, "EXECUTOR_LOST", "deferred executor heartbeat was lost; operation was not rerun", now)
}

func (store *Store) operationFrom(request persistedRequest, state stateRecord) Operation {
	resultAvailable := state.Status == StatusCompleted && fileExists(filepath.Join(store.operationDir(request.OperationID), resultName))
	// The marker is the no-replay barrier, but Started is externally observable
	// only after the matching running-state transition has been persisted.
	started := state.StartedAt != nil && fileExists(filepath.Join(store.operationDir(request.OperationID), startedName))
	operation := Operation{
		OperationID:     request.OperationID,
		Tool:            request.Request.Tool,
		Status:          state.Status,
		CreatedAt:       request.CreatedAt,
		UpdatedAt:       state.UpdatedAt,
		StartedAt:       state.StartedAt,
		FinishedAt:      state.FinishedAt,
		Revision:        state.Revision,
		Started:         started,
		Exposed:         fileExists(filepath.Join(store.operationDir(request.OperationID), exposedName)),
		CancelRequested: fileExists(filepath.Join(store.operationDir(request.OperationID), cancelName)),
		ResultAvailable: resultAvailable,
		TotalBytes:      state.TotalBytes,
		Message:         state.Message,
	}
	if state.Result != nil {
		operation.OriginalIsError = state.Result.OriginalIsError
		operation.ErrorCode = state.Result.ErrorCode
	}
	return operation
}

func (store *Store) readRequest(operationID string) (persistedRequest, error) {
	if !ValidOperationID(operationID) {
		return persistedRequest{}, ErrInvalidInput
	}
	path := filepath.Join(store.operationDir(operationID), requestName)
	var request persistedRequest
	if err := readJSONBounded(path, maxRequestBytes, &request); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return persistedRequest{}, ErrNotFound
		}
		return persistedRequest{}, err
	}
	if request.OperationID != operationID || request.Request.Tool == "" || !json.Valid(request.Request.Arguments) {
		return persistedRequest{}, errors.New("deferred operation request record is invalid")
	}
	return request, nil
}

func (store *Store) latestState(operationID string) (stateRecord, error) {
	entries, err := os.ReadDir(store.operationDir(operationID))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return stateRecord{}, ErrNotFound
		}
		return stateRecord{}, err
	}
	names := make([]string, 0, maxStateRecords)
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !strings.HasPrefix(entry.Name(), "state-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 || len(names) > maxStateRecords {
		return stateRecord{}, errors.New("deferred operation state history is invalid")
	}
	sort.Strings(names)
	var state stateRecord
	if err := readJSONBounded(filepath.Join(store.operationDir(operationID), names[len(names)-1]), 64*1024, &state); err != nil {
		return stateRecord{}, err
	}
	if state.Revision == 0 || state.Revision > maxStateRecords {
		return stateRecord{}, errors.New("deferred operation state revision is invalid")
	}
	return state, nil
}

func (store *Store) writeStateExclusive(operationID string, state stateRecord) error {
	if state.Revision == 0 || state.Revision > maxStateRecords || state.Status == "" {
		return ErrInvalidInput
	}
	name := fmt.Sprintf("state-%06d.json", state.Revision)
	return writeJSONExclusive(filepath.Join(store.operationDir(operationID), name), state)
}

func (store *Store) operationDir(operationID string) string {
	return filepath.Join(store.operationsRoot, operationID)
}

func (store *Store) activeCountLocked() (int, error) {
	entries, err := os.ReadDir(store.operationsRoot)
	if err != nil {
		return 0, err
	}
	maximum := store.limits.MaxConcurrency + store.limits.MaxQueued + store.limits.MaxTerminal + 1024
	if len(entries) > maximum {
		return 0, ErrCapacity
	}
	active := 0
	for _, entry := range entries {
		if !entry.IsDir() || !ValidOperationID(entry.Name()) {
			continue
		}
		state, err := store.latestState(entry.Name())
		if err == nil && !state.Status.Terminal() {
			active++
		}
	}
	return active, nil
}

func (store *Store) purgeLocked() error {
	entries, err := os.ReadDir(store.operationsRoot)
	if err != nil {
		return err
	}
	type terminal struct {
		id       string
		finished time.Time
		bytes    int64
	}
	terminals := make([]terminal, 0)
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() || !ValidOperationID(entry.Name()) {
			continue
		}
		size, err := directorySizeBounded(store.operationDir(entry.Name()), store.limits.MaxTotalBytes+1)
		if err != nil {
			continue
		}
		total += size
		state, err := store.latestState(entry.Name())
		if err == nil && state.Status.Terminal() && state.FinishedAt != nil {
			terminals = append(terminals, terminal{id: entry.Name(), finished: *state.FinishedAt, bytes: size})
		}
	}
	sort.Slice(terminals, func(i, j int) bool {
		if terminals[i].finished.Equal(terminals[j].finished) {
			return terminals[i].id < terminals[j].id
		}
		return terminals[i].finished.Before(terminals[j].finished)
	})
	cutoff := store.now().Add(-time.Duration(store.limits.RetentionSeconds) * time.Second)
	remaining := len(terminals)
	for _, item := range terminals {
		if remaining <= store.limits.MaxTerminal && total <= store.limits.MaxTotalBytes && !item.finished.Before(cutoff) {
			continue
		}
		path := store.operationDir(item.id)
		if !security.IsPathWithinAllowedDirectories(path, []string{store.operationsRoot}) {
			return errors.New("deferred operation retention target escaped its namespace")
		}
		if err := os.RemoveAll(path); err != nil {
			return err
		}
		remaining--
		total -= item.bytes
	}
	if total > store.limits.MaxTotalBytes {
		return ErrCapacity
	}
	return nil
}

func randomOperationID() (string, error) {
	payload := make([]byte, 32)
	if _, err := rand.Read(payload); err != nil {
		return "", err
	}
	return "op_" + hex.EncodeToString(payload), nil
}

func writeJSONExclusive(path string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return writeBytesExclusive(path, payload)
}

func writeBytesExclusive(path string, payload []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		closeErr := file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
		err = errors.Join(err, closeErr)
	}()
	if err := securePath(path, false); err != nil {
		return err
	}
	if _, err := file.Write(payload); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func writeMarkerExclusive(path string) error {
	return writeBytesExclusive(path, []byte("1"))
}

func readJSONBounded(path string, maximum int64, destination any) error {
	if err := validateSecurePath(path, false); err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maximum {
		return errors.New("deferred operation record exceeds its bound")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(io.LimitReader(file, maximum+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("deferred operation record has trailing data")
	}
	return nil
}

func touch(path string) error {
	now := time.Now()
	if err := os.Chtimes(path, now, now); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := writeBytesExclusive(path, []byte("1")); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return os.Chtimes(path, now, now)
}

func fileExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func directorySizeBounded(root string, maximum int64) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("link found in deferred operation store")
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
			return ErrCapacity
		}
		return nil
	})
	return total, err
}

func boundedMessage(value string) string {
	value = strings.ToValidUTF8(strings.TrimSpace(value), "\uFFFD")
	if len(value) <= 1024 {
		return value
	}
	value = value[:1024]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
