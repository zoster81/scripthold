package filesystempackage

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

// RevalidatePrepared verifies the complete preview-bound authorization, object,
// parent, recursive-scope, and missing-destination evidence without mutation.
func (planner *Planner) RevalidatePrepared(ctx context.Context, prepared PreparedPackage) error {
	if planner == nil {
		return operation.New(operation.KindInvalidInput, "filesystem package planner is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	allowed := planner.allowedDirs()
	if len(allowed) == 0 {
		return operation.New(operation.KindAccessDenied, "no resolved allowed directories are available")
	}
	treeOptions := filesystem.ExactTreeOptions{
		ResolvedAllowedDirs: allowed,
		MaxEntries:          planner.limits.MaxRecursiveEntries,
		MaxDepth:            planner.limits.MaxRecursiveDepth,
		MaxFileBytes:        planner.limits.MaxFileBytes,
		MaxAggregateBytes:   planner.limits.MaxAggregateBytes,
	}
	for _, item := range prepared.Operations {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "revalidate_filesystem_package", "", err)
		}
		var err error
		switch item.Operation.Type {
		case OperationMkdir, OperationCreateFile:
			err = planner.revalidateMissingTarget(item.Operation.Path, item.Path, item.NearestAncestorIdentity)
		case OperationCopyFile:
			err = planner.revalidateRegularSource(item.Operation.Source, item.Source, item.SourceIdentity, item.SourceParentIdentity, item.SourceSnapshot)
			if err == nil {
				err = planner.revalidateMissingTarget(item.Operation.Destination, item.Destination, item.NearestAncestorIdentity)
			}
		case OperationCopyDirectory:
			err = planner.revalidateExistingSource(item.Operation.Source, item.Source, item.SourceIdentity, item.SourceParentIdentity)
			if err == nil && item.Tree == nil {
				err = operation.New(operation.KindConflict, "prepared copyDirectory tree is missing")
			}
			if err == nil {
				err = filesystem.VerifyExactTree(ctx, *item.Tree, treeOptions)
			}
			if err == nil {
				err = planner.revalidateMissingTarget(item.Operation.Destination, item.Destination, item.NearestAncestorIdentity)
			}
		case OperationMove:
			err = planner.revalidateExistingSource(item.Operation.Source, item.Source, item.SourceIdentity, item.SourceParentIdentity)
			if err == nil && !item.SourceIdentity.IsDirectory() {
				err = item.SourceSnapshot.Verify(item.Source.ResolvedPath)
			}
			if err == nil {
				err = planner.revalidateMissingTarget(item.Operation.Destination, item.Destination, item.NearestAncestorIdentity)
			}
			if err == nil {
				var same bool
				same, err = item.SourceIdentity.SameVolume(item.NearestAncestorIdentity)
				if err == nil && !same {
					err = operation.New(operation.KindUnsupported, "move source and destination are no longer on the same filesystem volume")
				}
			}
		case OperationDeleteFile:
			err = planner.revalidateRegularSource(item.Operation.Path, item.Path, item.TargetIdentity, item.SourceParentIdentity, item.SourceSnapshot)
		case OperationDeleteDirectory:
			err = planner.revalidateExistingSource(item.Operation.Path, item.Path, item.TargetIdentity, item.SourceParentIdentity)
			if err == nil && item.Tree == nil {
				err = operation.New(operation.KindConflict, "prepared deleteDirectory tree is missing")
			}
			if err == nil {
				err = filesystem.VerifyExactTree(ctx, *item.Tree, treeOptions)
			}
		default:
			err = operation.New(operation.KindInvalidInput, "prepared filesystem package contains an unknown operation")
		}
		if err != nil {
			kind := operation.KindOf(err)
			if kind != operation.KindCancelled && kind != operation.KindUnsupported && kind != operation.KindAccessDenied && kind != operation.KindSymlinkEscape && kind != operation.KindLimit {
				kind = operation.KindConflict
			}
			return operation.Wrap(kind, fmt.Sprintf("revalidate_operation_%d", item.Index), preparedOperationPath(item), err)
		}
	}
	return nil
}

func (planner *Planner) revalidateMissingTarget(requested string, expected security.PathEvidence, expectedAncestor filesystem.ObjectIdentity) error {
	current, err := planner.authorize(requested)
	if err != nil {
		return err
	}
	if current.Exists {
		return operation.New(operation.KindConflict, fmt.Sprintf("prepared destination appeared: %s", current.ResolvedPath))
	}
	if !security.PathsEqual(current.ResolvedPath, expected.ResolvedPath) ||
		!security.PathsEqual(current.NearestExistingPath, expected.NearestExistingPath) {
		return operation.New(operation.KindConflict, "prepared missing-path resolution changed")
	}
	if err := verifyPreparedIdentity(expected.NearestExistingPath, expectedAncestor, "nearest existing destination ancestor"); err != nil {
		return err
	}
	return nil
}

func (planner *Planner) revalidateExistingSource(requested string, expected security.PathEvidence, expectedIdentity, expectedParent filesystem.ObjectIdentity) error {
	current, err := planner.authorize(requested)
	if err != nil {
		return err
	}
	if !current.Exists || !security.PathsEqual(current.ResolvedPath, expected.ResolvedPath) {
		return operation.New(operation.KindConflict, "prepared source resolution or existence changed")
	}
	if err := verifyPreparedIdentity(expected.ResolvedPath, expectedIdentity, "source"); err != nil {
		return err
	}
	if expectedParent.StableKey() != "" {
		if err := verifyPreparedIdentity(filepath.Dir(expected.ResolvedPath), expectedParent, "source parent"); err != nil {
			return err
		}
	}
	return nil
}

func (planner *Planner) revalidateRegularSource(requested string, expected security.PathEvidence, expectedIdentity, expectedParent filesystem.ObjectIdentity, snapshot filesystem.FileSnapshot) error {
	if err := planner.revalidateExistingSource(requested, expected, expectedIdentity, expectedParent); err != nil {
		return err
	}
	return snapshot.Verify(expected.ResolvedPath)
}

func verifyPreparedIdentity(path string, expected filesystem.ObjectIdentity, label string) error {
	if expected.StableKey() == "" {
		return operation.New(operation.KindConflict, fmt.Sprintf("prepared %s identity is missing", label))
	}
	matches, err := expected.Matches(path)
	if err != nil {
		return err
	}
	if !matches {
		return operation.New(operation.KindConflict, fmt.Sprintf("prepared %s identity changed: %s", label, path))
	}
	return nil
}

func preparedOperationPath(item PreparedOperation) string {
	if item.Path.ResolvedPath != "" {
		return item.Path.ResolvedPath
	}
	if item.Destination.ResolvedPath != "" {
		return item.Destination.ResolvedPath
	}
	return item.Source.ResolvedPath
}
