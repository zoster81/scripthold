package backupstore

import (
	"context"
	"os"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

// ReadSource exposes only verified read operations over one immutable backup
// object. Mutation-capable staging is deliberately available only through Store.
type ReadSource struct {
	source *RestoreSource
}

// OpenReadSource opens one verified immutable backup source without exposing a
// staging method to read-only callers.
func (store *Store) OpenReadSource(ctx context.Context, backupID string, options RestoreSourceOptions) (*ReadSource, error) {
	source, err := store.OpenRestoreSource(ctx, backupID, options)
	if err != nil {
		return nil, err
	}
	return &ReadSource{source: source}, nil
}

func (source *ReadSource) Manifest() Manifest {
	if source == nil || source.source == nil {
		return Manifest{}
	}
	return source.source.Manifest()
}

func (source *ReadSource) Verify(ctx context.Context) error {
	if source == nil || source.source == nil {
		return operation.New(operation.KindConflict, "backup source is unavailable")
	}
	return source.source.Verify(ctx)
}

func (source *ReadSource) ReadAll(ctx context.Context, maxBytes int64) ([]byte, error) {
	if source == nil || source.source == nil {
		return nil, operation.New(operation.KindConflict, "backup source is unavailable")
	}
	return source.source.ReadAll(ctx, maxBytes)
}

func (source *ReadSource) Close() error {
	if source == nil || source.source == nil {
		return nil
	}
	err := source.source.Close()
	source.source = nil
	return err
}

// StageReadSource is intentionally a Store mutation authority. ReadSource does
// not expose staging, so a read-only handler cannot create target-adjacent state.
func (store *Store) StageReadSource(ctx context.Context, source *ReadSource, target string, mode os.FileMode, modTime *time.Time) (*filesystem.StagedReplacement, error) {
	if store == nil || source == nil || source.source == nil || source.source.store != store {
		return nil, operation.New(operation.KindConflict, "backup source staging authority is unavailable")
	}
	return source.source.Stage(ctx, target, mode, modTime)
}
