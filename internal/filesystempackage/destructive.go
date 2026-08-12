package filesystempackage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

// BackupCaptureFunc is the persistent store's package-wide durable capture boundary.
type BackupCaptureFunc func(context.Context, []backupstore.CaptureRequest) ([]backupstore.CaptureResult, error)

// VerifiedBackupBatch is an opaque proof that every regular-file pre-state in a
// prepared destructive package was durably captured and matched the preview.
type VerifiedBackupBatch struct {
	fingerprints map[string]string
	backupIDs    []string
	verified     bool
}

// BackupIDs returns the durable manifests created for this batch in request order.
func (proof VerifiedBackupBatch) BackupIDs() []string {
	return append([]string(nil), proof.backupIDs...)
}

// CapturePreparedBackups revalidates every destructive byte source, captures
// the complete batch, verifies durable manifest evidence, then revalidates again.
func CapturePreparedBackups(ctx context.Context, prepared PreparedPackage, capture BackupCaptureFunc, treeOptions filesystem.ExactTreeOptions) (VerifiedBackupBatch, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(prepared.BackupRequirements) == 0 {
		return VerifiedBackupBatch{fingerprints: map[string]string{}, verified: true}, nil
	}
	if capture == nil {
		return VerifiedBackupBatch{}, operation.New(operation.KindInvalidInput, "persistent backup capture is required for destructive filesystem packages")
	}
	if err := revalidateDestructiveSources(ctx, prepared, treeOptions); err != nil {
		return VerifiedBackupBatch{}, err
	}
	requests := make([]backupstore.CaptureRequest, 0, len(prepared.BackupRequirements))
	for _, requirement := range prepared.BackupRequirements {
		requests = append(requests, backupstore.CaptureRequest{
			TargetPath: requirement.Path, SourceOperation: backupstore.SourceOperationFilesystemPackage,
		})
	}
	results, err := capture(ctx, requests)
	if err != nil {
		return VerifiedBackupBatch{}, err
	}
	if len(results) != len(prepared.BackupRequirements) {
		return VerifiedBackupBatch{}, operation.New(operation.KindFilesystem, "backup capture did not durably commit the complete prepared batch")
	}
	proof := VerifiedBackupBatch{
		fingerprints: make(map[string]string, len(results)),
		backupIDs:    make([]string, 0, len(results)),
		verified:     true,
	}
	for index, result := range results {
		requirement := prepared.BackupRequirements[index]
		manifest := result.Manifest
		if manifest.BackupID == "" || manifest.TargetPath != requirement.Path ||
			manifest.SourceOperation != backupstore.SourceOperationFilesystemPackage ||
			manifest.ContentFingerprint != requirement.ExpectedFingerprint || manifest.ObjectBytes != requirement.Bytes {
			return VerifiedBackupBatch{}, operation.New(operation.KindConflict, fmt.Sprintf("backup manifest %d does not match prepared destructive source", index))
		}
		proof.fingerprints[requirement.Path] = requirement.ExpectedFingerprint
		proof.backupIDs = append(proof.backupIDs, manifest.BackupID)
	}
	if err := revalidateDestructiveSources(ctx, prepared, treeOptions); err != nil {
		return VerifiedBackupBatch{}, err
	}
	return proof, nil
}

// DeletePreparedFile removes only the preview-bound regular file and requires a
// verified persistent backup proof for its exact prepared fingerprint.
func DeletePreparedFile(ctx context.Context, item PreparedOperation, proof VerifiedBackupBatch) error {
	if item.Operation.Type != OperationDeleteFile {
		return operation.New(operation.KindInvalidInput, "prepared operation is not deleteFile")
	}
	if err := requireBackupProof(item.Path.ResolvedPath, item.ExpectedResultFingerprint, proof); err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, "delete_prepared_file", item.Path.ResolvedPath, err)
	}
	if err := verifyPreparedIdentity(filepath.Dir(item.Path.ResolvedPath), item.SourceParentIdentity, "deleteFile parent"); err != nil {
		return err
	}
	matches, err := item.TargetIdentity.Matches(item.Path.ResolvedPath)
	if err != nil || !matches {
		if err == nil {
			err = operation.New(operation.KindConflict, "deleteFile target identity changed")
		}
		return err
	}
	if err := item.SourceSnapshot.Verify(item.Path.ResolvedPath); err != nil {
		return err
	}
	if err := os.Remove(item.Path.ResolvedPath); err != nil {
		return operation.WrapFilesystem("delete_prepared_file", item.Path.ResolvedPath, err)
	}
	if err := filesystem.SyncDirectory(filepath.Dir(item.Path.ResolvedPath)); err != nil {
		return operation.WrapFilesystem("sync_deleted_file_parent", filepath.Dir(item.Path.ResolvedPath), err)
	}
	if _, err := os.Lstat(item.Path.ResolvedPath); err == nil {
		return operation.New(operation.KindConflict, "deleteFile target still exists after removal")
	} else if !os.IsNotExist(err) {
		return operation.WrapFilesystem("verify_deleted_file", item.Path.ResolvedPath, err)
	}
	return nil
}

// DeletePreparedDirectory removes only the exact preview-bound recursive scope,
// children before parents. It never discovers and deletes newly appeared entries.
func DeletePreparedDirectory(ctx context.Context, item PreparedOperation, proof VerifiedBackupBatch, treeOptions filesystem.ExactTreeOptions) error {
	if item.Operation.Type != OperationDeleteDirectory || item.Tree == nil {
		return operation.New(operation.KindInvalidInput, "prepared operation is not deleteDirectory")
	}
	for _, entry := range item.Tree.Entries {
		if entry.IsDirectory {
			continue
		}
		fingerprint, err := filesystem.FingerprintRegularFileSnapshot(entry.Snapshot)
		if err != nil {
			return err
		}
		if err := requireBackupProof(entry.Path, fingerprint, proof); err != nil {
			return err
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := verifyPreparedIdentity(filepath.Dir(item.Path.ResolvedPath), item.SourceParentIdentity, "deleteDirectory parent"); err != nil {
		return err
	}
	if err := filesystem.VerifyExactTree(ctx, *item.Tree, treeOptions); err != nil {
		return err
	}
	for index := len(item.Tree.Entries) - 1; index >= 0; index-- {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "delete_prepared_directory", item.Path.ResolvedPath, err)
		}
		entry := item.Tree.Entries[index]
		matches, err := entry.Identity.Matches(entry.Path)
		if err != nil || !matches {
			if err == nil {
				err = operation.New(operation.KindConflict, fmt.Sprintf("recursive delete entry identity changed: %s", entry.Path))
			}
			return err
		}
		if !entry.IsDirectory {
			if err := entry.Snapshot.Verify(entry.Path); err != nil {
				return err
			}
		}
		if err := os.Remove(entry.Path); err != nil {
			return operation.WrapFilesystem("delete_prepared_directory_entry", entry.Path, err)
		}
		parent := filepath.Dir(entry.Path)
		if _, statErr := os.Stat(parent); statErr == nil {
			if err := filesystem.SyncDirectory(parent); err != nil {
				return operation.WrapFilesystem("sync_deleted_directory_parent", parent, err)
			}
		}
	}
	if _, err := os.Lstat(item.Path.ResolvedPath); err == nil {
		return operation.New(operation.KindConflict, "deleteDirectory root still exists after exact removal")
	} else if !os.IsNotExist(err) {
		return operation.WrapFilesystem("verify_deleted_directory", item.Path.ResolvedPath, err)
	}
	return nil
}

func requireBackupProof(path, fingerprint string, proof VerifiedBackupBatch) error {
	if !proof.verified {
		return operation.New(operation.KindInvalidInput, "verified persistent backup proof is required before destructive deletion")
	}
	if fingerprint == "" {
		return operation.New(operation.KindInvalidInput, "prepared destructive fingerprint is missing")
	}
	if got, ok := proof.fingerprints[path]; !ok || got != fingerprint {
		return operation.New(operation.KindConflict, fmt.Sprintf("persistent backup proof does not cover prepared path %s", path))
	}
	return nil
}

func revalidateDestructiveSources(ctx context.Context, prepared PreparedPackage, treeOptions filesystem.ExactTreeOptions) error {
	for _, item := range prepared.Operations {
		if err := ctx.Err(); err != nil {
			return operation.Wrap(operation.KindCancelled, "revalidate_destructive_sources", "", err)
		}
		switch item.Operation.Type {
		case OperationDeleteFile:
			matches, err := item.TargetIdentity.Matches(item.Path.ResolvedPath)
			if err != nil || !matches {
				if err == nil {
					err = operation.New(operation.KindConflict, "deleteFile target identity changed before backup")
				}
				return err
			}
			if err := item.SourceSnapshot.Verify(item.Path.ResolvedPath); err != nil {
				return err
			}
		case OperationDeleteDirectory:
			if item.Tree == nil {
				return operation.New(operation.KindInvalidInput, "deleteDirectory prepared tree is missing")
			}
			if err := filesystem.VerifyExactTree(ctx, *item.Tree, treeOptions); err != nil {
				return err
			}
		}
	}
	return nil
}
