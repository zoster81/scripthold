package filesystem

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
)

// SyncDirectory durably flushes one directory namespace where the platform
// provides that primitive; supported no-op implementations remain explicit.
func SyncDirectory(path string) (err error) {
	defer func() {
		err = operation.WrapFilesystem("sync_directory", path, err)
	}()
	return defaultMutationOps.syncDirectory(path)
}

// CreateDirectoryExactNoReplace creates exactly path and never creates parents.
// The parent namespace is synced before success is reported.
func CreateDirectoryExactNoReplace(path string, mode fs.FileMode) (err error) {
	defer func() {
		err = operation.WrapFilesystem("mkdir_exact_no_replace", path, err)
	}()
	if err := os.Mkdir(path, mode.Perm()); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ErrDestinationExists
		}
		return err
	}
	if err := defaultMutationOps.syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("directory was created but parent sync failed: %w", err)
	}
	return nil
}

// StagedFile owns one synced, identity-bound regular file in a verified staging
// directory until it is published no-replace or safely cleaned up.
type StagedFile struct {
	path       string
	identity   ObjectIdentity
	size       int64
	digest     [sha256.Size]byte
	stagingDir string
}

// StageRawFile streams exact bytes into a random restricted staging file. It
// does not inspect or mutate the eventual destination.
func StageRawFile(ctx context.Context, stagingDir string, source io.Reader, mode fs.FileMode, modTime *time.Time, expectedBytes int64) (staged *StagedFile, err error) {
	defer func() {
		err = operation.WrapFilesystem("stage_raw_file", stagingDir, err)
	}()
	if ctx == nil {
		ctx = context.Background()
	}
	if expectedBytes < 0 {
		return nil, operation.New(operation.KindInvalidInput, "expected staged byte count must not be negative")
	}
	if err := ctx.Err(); err != nil {
		return nil, operation.Wrap(operation.KindCancelled, "stage_raw_file", stagingDir, err)
	}
	stagingIdentity, err := CaptureObjectIdentity(stagingDir)
	if err != nil {
		return nil, err
	}
	if !stagingIdentity.IsDirectory() {
		return nil, operation.New(operation.KindInvalidInput, "staging path must be a real directory")
	}

	file, err := os.CreateTemp(stagingDir, ".scripthold-r24-file-*.tmp")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
		if err != nil {
			err = errors.Join(err, os.Remove(tempPath))
		}
	}()
	if err := file.Chmod(mode.Perm()); err != nil {
		return nil, err
	}
	hasher := sha256.New()
	written, err := io.CopyBuffer(io.MultiWriter(file, hasher), &contextReader{ctx: ctx, reader: source}, make([]byte, 128*1024))
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, operation.Wrap(operation.KindCancelled, "stage_raw_file", tempPath, err)
		}
		return nil, err
	}
	if written != expectedBytes {
		return nil, operation.New(operation.KindConflict, fmt.Sprintf("staged byte count %d does not match prepared size %d", written, expectedBytes))
	}
	if modTime != nil {
		if err := os.Chtimes(tempPath, *modTime, *modTime); err != nil {
			return nil, err
		}
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	identity, err := CaptureObjectIdentity(tempPath)
	if err != nil {
		return nil, err
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return &StagedFile{path: tempPath, identity: identity, size: written, digest: digest, stagingDir: stagingDir}, nil
}

// StagePreparedRegularFile copies exactly the prepared regular-file pre-state.
func StagePreparedRegularFile(ctx context.Context, sourcePath, stagingDir string, expected FileSnapshot) (*StagedFile, error) {
	if !expected.Exists || !expected.Mode.IsRegular() || !expected.hasDigest {
		return nil, operation.New(operation.KindInvalidInput, "prepared regular-file staging requires a digest-bearing regular-file snapshot")
	}
	if err := expected.verifyMetadata(sourcePath); err != nil {
		return nil, err
	}
	file, err := os.Open(sourcePath)
	if err != nil {
		return nil, operation.WrapFilesystem("open_prepared_stage_source", sourcePath, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, operation.WrapFilesystem("stat_prepared_stage_source", sourcePath, err)
	}
	if err := expected.verifyInfo(sourcePath, info); err != nil {
		return nil, err
	}
	modTime := expected.ModTime
	staged, err := StageRawFile(ctx, stagingDir, file, expected.Mode, &modTime, expected.Size)
	if err != nil {
		return nil, err
	}
	if staged.digest != expected.digest {
		cleanupErr := staged.Cleanup()
		return nil, errors.Join(operation.New(operation.KindConflict, "prepared source content changed while staging"), cleanupErr)
	}
	if err := expected.verifyMetadata(sourcePath); err != nil {
		cleanupErr := staged.Cleanup()
		return nil, errors.Join(err, cleanupErr)
	}
	return staged, nil
}

// PublishNoReplace installs the staged file at destination without replacement.
func (staged *StagedFile) PublishNoReplace(destination string) (err error) {
	if staged == nil || staged.path == "" {
		return operation.New(operation.KindInvalidInput, "staged file is not available")
	}
	defer func() {
		err = operation.WrapFilesystem("publish_staged_file_no_replace", destination, err)
	}()
	matches, err := staged.identity.Matches(staged.path)
	if err != nil || !matches {
		if err == nil {
			err = operation.New(operation.KindConflict, "staged file identity changed")
		}
		return err
	}
	snapshot, err := CaptureRegularFileSnapshotBounded(context.Background(), staged.path, max(staged.size, int64(1)))
	if err != nil {
		return err
	}
	digest, ok := snapshot.ContentDigest()
	if !ok || snapshot.Size != staged.size || digest != staged.digest {
		return operation.New(operation.KindConflict, "staged file content changed")
	}
	if _, err := os.Lstat(destination); err == nil {
		return ErrDestinationExists
	} else if !os.IsNotExist(err) {
		return err
	}
	parentIdentity, err := CaptureObjectIdentity(filepath.Dir(destination))
	if err != nil {
		return err
	}
	sameVolume, err := staged.identity.SameVolume(parentIdentity)
	if err != nil {
		return err
	}
	if !sameVolume {
		return operation.New(operation.KindUnsupported, "staged file and destination are on different filesystem volumes")
	}
	oldParent := filepath.Dir(staged.path)
	if err := movePathNoReplace(staged.path, destination); err != nil {
		if isDestinationExistsError(err) || errors.Is(err, fs.ErrExist) {
			return ErrDestinationExists
		}
		return err
	}
	staged.path = ""
	for _, directory := range uniqueDirectories(oldParent, filepath.Dir(destination)) {
		if err := defaultMutationOps.syncDirectory(directory); err != nil {
			return fmt.Errorf("published staged file but failed to sync directory %s: %w", directory, err)
		}
	}
	return nil
}

// Cleanup removes only the identity-bound staging file.
func (staged *StagedFile) Cleanup() error {
	if staged == nil || staged.path == "" {
		return nil
	}
	matches, err := staged.identity.Matches(staged.path)
	if err != nil {
		return err
	}
	if !matches {
		return operation.New(operation.KindConflict, "staged file identity changed; refusing cleanup")
	}
	if err := os.Remove(staged.path); err != nil {
		return operation.WrapFilesystem("cleanup_staged_file", staged.path, err)
	}
	staged.path = ""
	return nil
}

// StagedDirectory owns one fully prepared exact directory tree.
type StagedDirectory struct {
	path       string
	tree       ExactTree
	options    ExactTreeOptions
	stagingDir string
}

// StageExactDirectoryCopy constructs and verifies a complete staged tree before
// the final destination is published.
func StageExactDirectoryCopy(ctx context.Context, expected ExactTree, stagingDir string, options ExactTreeOptions) (staged *StagedDirectory, err error) {
	defer func() {
		err = operation.WrapFilesystem("stage_exact_directory_copy", expected.Root, err)
	}()
	if len(expected.Entries) == 0 || !expected.Entries[0].IsDirectory {
		return nil, operation.New(operation.KindInvalidInput, "prepared directory tree is empty or invalid")
	}
	if err := VerifyExactTree(ctx, expected, options); err != nil {
		return nil, err
	}
	stagingIdentity, err := CaptureObjectIdentity(stagingDir)
	if err != nil {
		return nil, err
	}
	if !stagingIdentity.IsDirectory() {
		return nil, operation.New(operation.KindInvalidInput, "staging path must be a real directory")
	}
	rootPath, err := os.MkdirTemp(stagingDir, ".scripthold-r24-dir-*")
	if err != nil {
		return nil, err
	}
	created := []string{rootPath}
	cleanupCreated := func() error {
		var cleanupErr error
		for index := len(created) - 1; index >= 0; index-- {
			if removeErr := os.Remove(created[index]); removeErr != nil && !os.IsNotExist(removeErr) {
				cleanupErr = errors.Join(cleanupErr, removeErr)
			}
		}
		return cleanupErr
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, cleanupCreated())
		}
	}()
	if err := os.Chmod(rootPath, expected.Entries[0].Mode.Perm()); err != nil {
		return nil, err
	}
	for _, entry := range expected.Entries[1:] {
		if err := ctx.Err(); err != nil {
			return nil, operation.Wrap(operation.KindCancelled, "stage_exact_directory_copy", entry.Path, err)
		}
		relative := filepath.FromSlash(entry.RelativePath)
		destination := filepath.Join(rootPath, relative)
		if entry.IsDirectory {
			if err := os.Mkdir(destination, entry.Mode.Perm()); err != nil {
				return nil, err
			}
			created = append(created, destination)
			continue
		}
		fileStage, err := StagePreparedRegularFile(ctx, entry.Path, filepath.Dir(destination), entry.Snapshot)
		if err != nil {
			return nil, err
		}
		if err := fileStage.PublishNoReplace(destination); err != nil {
			cleanupErr := fileStage.Cleanup()
			return nil, errors.Join(err, cleanupErr)
		}
		created = append(created, destination)
	}
	if err := VerifyExactTree(ctx, expected, options); err != nil {
		return nil, err
	}
	stagedTree, err := EnumerateExactTree(ctx, rootPath, options)
	if err != nil {
		return nil, err
	}
	if !ExactTreeContentEqual(expected, stagedTree) {
		return nil, operation.New(operation.KindConflict, "staged directory tree does not match prepared source tree")
	}
	if err := syncExactTreeDirectories(stagedTree); err != nil {
		return nil, err
	}
	return &StagedDirectory{path: rootPath, tree: stagedTree, options: options, stagingDir: stagingDir}, nil
}

// PublishNoReplace installs the complete staged directory as one namespace move.
func (staged *StagedDirectory) PublishNoReplace(destination string) (err error) {
	if staged == nil || staged.path == "" {
		return operation.New(operation.KindInvalidInput, "staged directory is not available")
	}
	defer func() {
		err = operation.WrapFilesystem("publish_staged_directory_no_replace", destination, err)
	}()
	if err := VerifyExactTree(context.Background(), staged.tree, staged.options); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return ErrDestinationExists
	} else if !os.IsNotExist(err) {
		return err
	}
	rootIdentity := staged.tree.Entries[0].Identity
	parentIdentity, err := CaptureObjectIdentity(filepath.Dir(destination))
	if err != nil {
		return err
	}
	sameVolume, err := rootIdentity.SameVolume(parentIdentity)
	if err != nil {
		return err
	}
	if !sameVolume {
		return operation.New(operation.KindUnsupported, "staged directory and destination are on different filesystem volumes")
	}
	oldParent := filepath.Dir(staged.path)
	if err := movePathNoReplace(staged.path, destination); err != nil {
		if isDestinationExistsError(err) || errors.Is(err, fs.ErrExist) {
			return ErrDestinationExists
		}
		return err
	}
	staged.path = ""
	for _, directory := range uniqueDirectories(oldParent, filepath.Dir(destination)) {
		if err := defaultMutationOps.syncDirectory(directory); err != nil {
			return fmt.Errorf("published staged directory but failed to sync directory %s: %w", directory, err)
		}
	}
	return nil
}

// Cleanup removes only the exact verified staging tree; any unexpected entry or
// identity/content change makes cleanup fail closed without broadening scope.
func (staged *StagedDirectory) Cleanup() error {
	if staged == nil || staged.path == "" {
		return nil
	}
	if err := VerifyExactTree(context.Background(), staged.tree, staged.options); err != nil {
		return err
	}
	for index := len(staged.tree.Entries) - 1; index >= 0; index-- {
		entry := staged.tree.Entries[index]
		matches, err := entry.Identity.Matches(entry.Path)
		if err != nil {
			return err
		}
		if !matches {
			return operation.New(operation.KindConflict, fmt.Sprintf("staging entry identity changed: %s", entry.Path))
		}
		if err := os.Remove(entry.Path); err != nil {
			return operation.WrapFilesystem("cleanup_staged_directory", entry.Path, err)
		}
	}
	if err := defaultMutationOps.syncDirectory(staged.stagingDir); err != nil {
		return err
	}
	staged.path = ""
	return nil
}

func syncExactTreeDirectories(tree ExactTree) error {
	for index := len(tree.Entries) - 1; index >= 0; index-- {
		entry := tree.Entries[index]
		if !entry.IsDirectory {
			continue
		}
		if err := defaultMutationOps.syncDirectory(entry.Path); err != nil {
			return operation.WrapFilesystem("sync_staged_directory", entry.Path, err)
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}
