package handler

import (
	"bytes"
	"context"
	"errors"
	"os"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

// preparedExistingFileReplacement is a non-owning view of one approved
// replacement of an existing regular file. The preview that created the view
// retains ownership of resultData and the file-identity slot.
type preparedExistingFileReplacement struct {
	requestedPath     string
	resolvedPath      string
	resultData        []byte
	targetFingerprint string
	resultFingerprint string
	identitySlot      **filesystem.FileIdentity
	changed           bool
	forceWritable     bool
}

type existingFileReplacementOps struct {
	stage   func(context.Context, string, []byte, os.FileMode) (*filesystem.StagedReplacement, error)
	commit  func(int, *filesystem.StagedReplacement, filesystem.ReplaceOptions) (bool, error)
	cleanup func(*filesystem.StagedReplacement) error
}

type existingFileReplacementBatch struct {
	replacements []preparedExistingFileReplacement
	staged       []*filesystem.StagedReplacement
	ops          existingFileReplacementOps
}

func preparedPatchPackageReplacement(target *preparedPatchPackageTarget) preparedExistingFileReplacement {
	return preparedExistingFileReplacement{
		requestedPath:     target.requestedPath,
		resolvedPath:      target.resolvedPath,
		resultData:        target.prepared.data,
		targetFingerprint: target.prepared.targetFingerprint,
		resultFingerprint: target.prepared.resultFingerprint,
		identitySlot:      &target.prepared.identityFile,
		changed:           target.prepared.changed,
		forceWritable:     target.prepared.forceWritable,
	}
}

func (replacement preparedExistingFileReplacement) identity() *filesystem.FileIdentity {
	if replacement.identitySlot == nil {
		return nil
	}
	return *replacement.identitySlot
}

func (replacement preparedExistingFileReplacement) closeIdentity() error {
	identity := replacement.identity()
	if identity == nil {
		return nil
	}
	if err := identity.Close(); err != nil {
		return err
	}
	*replacement.identitySlot = nil
	return nil
}

func stageExistingFileReplacement(ctx context.Context, path string, data []byte, mode os.FileMode) (*filesystem.StagedReplacement, error) {
	return filesystem.StageReplacement(path, textstream.WithContext(ctx, bytes.NewReader(data)), mode, nil)
}

func commitExistingFileReplacement(_ int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
	return staged.Commit(options)
}

func (h *Handler) stageExistingFileReplacementBatch(
	ctx context.Context,
	replacements []preparedExistingFileReplacement,
	modes []os.FileMode,
	ops existingFileReplacementOps,
	stageOperation string,
	cleanupOperation string,
) (*existingFileReplacementBatch, error) {
	if len(replacements) != len(modes) {
		return nil, operation.New(operation.KindInvalidInput, "existing-file replacement modes do not match targets")
	}
	if ops.stage == nil || ops.commit == nil || ops.cleanup == nil {
		return nil, operation.New(operation.KindInvalidInput, "existing-file replacement operations are incomplete")
	}
	batch := &existingFileReplacementBatch{
		replacements: append([]preparedExistingFileReplacement(nil), replacements...),
		staged:       make([]*filesystem.StagedReplacement, len(replacements)),
		ops:          ops,
	}
	for index, replacement := range replacements {
		if !replacement.changed {
			continue
		}
		mode := modes[index]
		if isReadOnly(mode) {
			mode |= 0o200
		}
		staged, err := ops.stage(ctx, replacement.resolvedPath, replacement.resultData, mode)
		if err == nil {
			batch.staged[index] = staged
			continue
		}
		if ctx.Err() != nil {
			err = operation.Wrap(operation.KindCancelled, stageOperation, replacement.resolvedPath, ctx.Err())
		}
		cleanupErr := batch.cleanup(cleanupOperation)
		if cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
		return nil, err
	}
	return batch, nil
}

func (batch *existingFileReplacementBatch) cleanup(operationName string) error {
	if batch == nil {
		return nil
	}
	var cleanupErrors []error
	for index, staged := range batch.staged {
		if staged == nil {
			continue
		}
		if err := batch.ops.cleanup(staged); err != nil {
			cleanupErrors = append(cleanupErrors, operation.WrapFilesystem(operationName, "", err))
		}
		batch.staged[index] = nil
	}
	return errors.Join(cleanupErrors...)
}

func (h *Handler) commitExistingFileReplacementBatchTarget(
	ctx context.Context,
	batch *existingFileReplacementBatch,
	index int,
	expected filesystem.FileSnapshot,
	mismatchMessage string,
) (string, error) {
	if batch == nil || index < 0 || index >= len(batch.replacements) || len(batch.staged) != len(batch.replacements) {
		return "", operation.New(operation.KindInvalidInput, "existing-file replacement batch target is invalid")
	}
	replacement := batch.replacements[index]
	if !replacement.changed || batch.staged[index] == nil {
		return "", operation.New(operation.KindConflict, "existing-file replacement target is not staged")
	}
	if _, err := batch.ops.commit(index, batch.staged[index], filesystem.ReplaceOptions{Expected: &expected}); err != nil {
		return "", err
	}
	batch.staged[index] = nil
	return h.verifyExistingFileReplacementCommitted(ctx, replacement, mismatchMessage)
}
