package filesystem

import (
	"bytes"
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

var (
	// ErrConcurrentModification is returned when a path no longer matches the
	// snapshot captured while an operation was prepared.
	ErrConcurrentModification = operation.New(operation.KindConflict, "file changed during operation")

	// ErrDestinationExists is returned by no-replace copy and move operations.
	ErrDestinationExists = operation.New(operation.KindConflict, "destination already exists")
)

// FileSnapshot records the observable state used for optimistic concurrency
// checks. A digest is included when the caller already has the complete bytes.
type FileSnapshot struct {
	Exists  bool
	Size    int64
	ModTime time.Time
	Mode    fs.FileMode

	digest    [sha256.Size]byte
	hasDigest bool
}

// CaptureSnapshot records metadata for path. A missing path is represented by
// Exists=false and is not an error.
func CaptureSnapshot(path string) (snapshot FileSnapshot, err error) {
	defer func() {
		err = operation.WrapFilesystem("capture_snapshot", path, err)
	}()

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return FileSnapshot{}, nil
		}
		return FileSnapshot{}, err
	}
	return snapshotFromInfo(info), nil
}

// CaptureSnapshotWithData records metadata plus a digest of data. The caller
// should pass bytes read from path during operation preparation.
func CaptureSnapshotWithData(path string, data []byte) (snapshot FileSnapshot, err error) {
	defer func() {
		err = operation.WrapFilesystem("capture_snapshot_with_data", path, err)
	}()

	snapshot, err = CaptureSnapshot(path)
	if err != nil {
		return FileSnapshot{}, err
	}
	if !snapshot.Exists {
		return FileSnapshot{}, fmt.Errorf("cannot capture data snapshot for missing path: %s", path)
	}
	if snapshot.Size != int64(len(data)) {
		return FileSnapshot{}, fmt.Errorf("%w: size is %d, prepared bytes are %d", ErrConcurrentModification, snapshot.Size, len(data))
	}
	snapshot.digest = sha256.Sum256(data)
	snapshot.hasDigest = true
	return snapshot, nil
}

// CaptureSnapshotWithDigest streams path into a SHA-256 digest without loading
// the entire file into memory, then verifies that path still matches the opened
// file metadata. It is suitable for optimistic pre-execution checks.
func CaptureSnapshotWithDigest(path string) (snapshot FileSnapshot, err error) {
	defer func() {
		err = operation.WrapFilesystem("capture_snapshot_with_digest", path, err)
	}()

	file, err := os.Open(path)
	if err != nil {
		return FileSnapshot{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return FileSnapshot{}, err
	}
	if !info.Mode().IsRegular() {
		return FileSnapshot{}, fmt.Errorf("path is not a regular file: %s", path)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return FileSnapshot{}, err
	}

	snapshot = snapshotFromInfo(info)
	copy(snapshot.digest[:], hasher.Sum(nil))
	snapshot.hasDigest = true
	if err := snapshot.verifyMetadata(path); err != nil {
		return FileSnapshot{}, err
	}
	return snapshot, nil
}

func snapshotFromInfo(info fs.FileInfo) FileSnapshot {
	return FileSnapshot{
		Exists:  true,
		Size:    info.Size(),
		ModTime: info.ModTime(),
		Mode:    info.Mode(),
	}
}

// Equal reports whether two snapshots describe the same existence, metadata,
// and digest evidence. Digest-bearing snapshots compare their SHA-256 values.
func (snapshot FileSnapshot) Equal(other FileSnapshot) bool {
	if snapshot.Exists != other.Exists || snapshot.Size != other.Size ||
		!snapshot.ModTime.Equal(other.ModTime) || snapshot.Mode != other.Mode ||
		snapshot.hasDigest != other.hasDigest {
		return false
	}
	return !snapshot.hasDigest || bytes.Equal(snapshot.digest[:], other.digest[:])
}

// MatchesContentDigest reports whether a digest-bearing snapshot has the given
// byte size and SHA-256 digest. It supports bounded dry-run comparisons without
// exposing mutable snapshot internals.
func (snapshot FileSnapshot) MatchesContentDigest(size int64, digest []byte) bool {
	return snapshot.Exists && snapshot.hasDigest && snapshot.Size == size &&
		len(digest) == sha256.Size && bytes.Equal(snapshot.digest[:], digest)
}

// ContentDigest returns the captured SHA-256 digest without exposing mutable
// snapshot storage. The boolean is false when the snapshot has no digest.
func (snapshot FileSnapshot) ContentDigest() ([sha256.Size]byte, bool) {
	return snapshot.digest, snapshot.hasDigest
}

// Verify confirms that path still matches the captured state.
func (snapshot FileSnapshot) Verify(path string) (err error) {
	defer func() {
		err = operation.WrapFilesystem("verify_snapshot", path, err)
	}()

	if err := snapshot.verifyMetadata(path); err != nil {
		return err
	}
	if !snapshot.hasDigest {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if err := snapshot.verifyInfo(path, info); err != nil {
		return err
	}

	hasher := sha256.New()
	written, err := io.CopyN(hasher, file, snapshot.Size)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return fmt.Errorf("%w: content truncated for %s", ErrConcurrentModification, path)
		}
		return err
	}
	if written != snapshot.Size {
		return fmt.Errorf("%w: verified %d of %d bytes for %s", ErrConcurrentModification, written, snapshot.Size, path)
	}
	if !bytes.Equal(hasher.Sum(nil), snapshot.digest[:]) {
		return fmt.Errorf("%w: content differs for %s", ErrConcurrentModification, path)
	}
	return snapshot.verifyMetadata(path)
}

func (snapshot FileSnapshot) verifyMetadata(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			if snapshot.Exists {
				return fmt.Errorf("%w: path disappeared: %s", ErrConcurrentModification, path)
			}
			return nil
		}
		return err
	}
	return snapshot.verifyInfo(path, info)
}

func (snapshot FileSnapshot) verifyInfo(path string, info fs.FileInfo) error {
	if !snapshot.Exists {
		return fmt.Errorf("%w: path appeared: %s", ErrConcurrentModification, path)
	}
	if info.Size() != snapshot.Size || !info.ModTime().Equal(snapshot.ModTime) || info.Mode() != snapshot.Mode {
		return fmt.Errorf("%w: metadata differs for %s", ErrConcurrentModification, path)
	}
	return nil
}

// RefreshMetadata updates metadata from path while retaining an existing digest.
// It is used when an operation intentionally changes permissions before commit.
func (snapshot FileSnapshot) RefreshMetadata(path string) (current FileSnapshot, err error) {
	defer func() {
		err = operation.WrapFilesystem("refresh_snapshot", path, err)
	}()

	current, err = CaptureSnapshot(path)
	if err != nil {
		return FileSnapshot{}, err
	}
	if !current.Exists {
		return FileSnapshot{}, fmt.Errorf("%w: path disappeared: %s", ErrConcurrentModification, path)
	}
	current.digest = snapshot.digest
	current.hasDigest = snapshot.hasDigest
	return current, nil
}

// ReplaceOptions controls a durable atomic replacement.
type ReplaceOptions struct {
	Mode          fs.FileMode
	ModTime       *time.Time
	Expected      *FileSnapshot
	BackupPath    string
	SkipIdentical bool
}

type mutationOps struct {
	copyStream          func(destination io.Writer, source io.Reader) (int64, error)
	replacePath         func(source, destination string) error
	installNoReplace    func(source, destination string) error
	moveNoReplace       func(source, destination string) error
	syncDirectory       func(path string) error
	syncFile            func(file *os.File) error
	remove              func(path string) error
	isDestinationExists func(error) bool
}

var defaultMutationOps = mutationOps{
	copyStream:          io.Copy,
	replacePath:         replacePath,
	installNoReplace:    installPathNoReplace,
	moveNoReplace:       movePathNoReplace,
	syncDirectory:       syncDirectory,
	syncFile:            func(file *os.File) error { return file.Sync() },
	remove:              os.Remove,
	isDestinationExists: isDestinationExistsError,
}

// ReplaceFile stages data beside path, syncs it, optionally commits a backup of
// the original, atomically replaces path, and syncs the containing directory.
func ReplaceFile(path string, data []byte, options ReplaceOptions) (err error) {
	defer func() {
		err = operation.WrapFilesystem("replace_file", path, err)
	}()
	return replaceFile(path, data, options, defaultMutationOps)
}

// StagedReplacement owns a synced same-directory temporary file until Commit
// installs it or Cleanup removes it. The fields are intentionally private so
// callers cannot bypass the commit and cleanup invariants.
type StagedReplacement struct {
	targetPath string
	tempPath   string
	size       int64
	digest     [sha256.Size]byte
	ops        mutationOps
}

// StageReplacement streams source into a synced same-directory temporary file.
// It does not inspect or modify the current target. A zero permission mode uses
// the historical safe default 0600.
func StageReplacement(target string, source io.Reader, mode fs.FileMode, modTime *time.Time) (staged *StagedReplacement, err error) {
	defer func() {
		err = operation.WrapFilesystem("stage_replacement", target, err)
	}()
	return stageReplacement(target, source, mode, modTime, true, defaultMutationOps)
}

// StageReplacementExactMode preserves mode exactly, including permission mode
// 0000. It is intended for restoring already-authorized immutable metadata.
func StageReplacementExactMode(target string, source io.Reader, mode fs.FileMode, modTime *time.Time) (staged *StagedReplacement, err error) {
	defer func() {
		err = operation.WrapFilesystem("stage_replacement_exact_mode", target, err)
	}()
	return stageReplacement(target, source, mode, modTime, false, defaultMutationOps)
}

func stageReplacement(target string, source io.Reader, mode fs.FileMode, modTime *time.Time, defaultZeroMode bool, ops mutationOps) (*StagedReplacement, error) {
	if defaultZeroMode && mode.Perm() == 0 {
		mode = 0600
	}
	tempPath, size, digest, err := stageReader(target, source, mode, modTime, ops)
	if err != nil {
		return nil, err
	}
	return &StagedReplacement{
		targetPath: target,
		tempPath:   tempPath,
		size:       size,
		digest:     digest,
		ops:        ops,
	}, nil
}

// Commit verifies expected state, optionally suppresses a byte-identical
// replacement, and installs the staged file through the durable mutation path.
// A staged replacement is single-use.
func (staged *StagedReplacement) Commit(options ReplaceOptions) (changed bool, err error) {
	if staged == nil || staged.tempPath == "" {
		return false, errors.New("staged replacement is not available")
	}
	defer func() {
		if cleanupErr := staged.Cleanup(); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()

	if options.SkipIdentical && options.Expected != nil && options.Expected.hasDigest &&
		options.Expected.Size == staged.size && bytes.Equal(options.Expected.digest[:], staged.digest[:]) {
		if err := options.Expected.Verify(staged.targetPath); err != nil {
			return false, err
		}
		return false, nil
	}

	if err := commitStagedReplacement(staged.targetPath, staged.tempPath, options, staged.ops); err != nil {
		return false, err
	}
	staged.tempPath = ""
	return true, nil
}

// MatchesContentDigest reports whether the staged replacement contains exactly
// the expected byte count and raw SHA-256 digest without exposing its path.
func (staged *StagedReplacement) MatchesContentDigest(size int64, digest [sha256.Size]byte) bool {
	return staged != nil && staged.tempPath != "" && staged.size == size && staged.digest == digest
}

// Cleanup removes an uncommitted staged replacement. It is safe to call more
// than once and after a successful Commit.
func (staged *StagedReplacement) Cleanup() error {
	if staged == nil || staged.tempPath == "" {
		return nil
	}
	err := cleanupMutationPath(staged.tempPath, staged.ops)
	if err == nil {
		staged.tempPath = ""
	}
	return err
}

func replaceFile(path string, data []byte, options ReplaceOptions, ops mutationOps) error {
	if options.Mode.Perm() == 0 {
		options.Mode = 0600
	}
	targetTemp, err := stageBytes(path, data, options.Mode, options.ModTime, ops)
	if err != nil {
		return err
	}
	return commitStagedReplacement(path, targetTemp, options, ops)
}

func commitStagedReplacement(path, targetTemp string, options ReplaceOptions, ops mutationOps) (err error) {
	if options.BackupPath != "" && filepath.Clean(options.BackupPath) == filepath.Clean(path) {
		return errors.New("backup path must differ from target path")
	}

	var backupTemp string
	var previousBackupTemp string
	defer func() {
		err = errors.Join(err,
			cleanupMutationPath(targetTemp, ops),
			cleanupMutationPath(backupTemp, ops),
			cleanupMutationPath(previousBackupTemp, ops),
		)
	}()

	var previousBackup FileSnapshot
	backupCommitted := false
	rollbackBackup := func() error {
		if !backupCommitted {
			return nil
		}
		var rollbackErr error
		if previousBackup.Exists {
			recoveryPath := previousBackupTemp
			rollbackErr = ops.replacePath(previousBackupTemp, options.BackupPath)
			if rollbackErr == nil {
				previousBackupTemp = ""
			} else {
				// Preserve the staged previous backup for manual recovery instead
				// of deleting the last known-good copy during deferred cleanup.
				previousBackupTemp = ""
				rollbackErr = fmt.Errorf("failed to restore previous backup; recovery copy preserved at %s: %w", recoveryPath, rollbackErr)
			}
		} else {
			rollbackErr = removeIfExists(options.BackupPath, ops)
		}
		if syncErr := ops.syncDirectory(filepath.Dir(options.BackupPath)); syncErr != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("failed to sync backup directory during rollback: %w", syncErr))
		}
		return rollbackErr
	}

	if options.BackupPath != "" {
		if options.Expected == nil || !options.Expected.Exists {
			return errors.New("backup requires an existing expected target snapshot")
		}
		backupTemp, err = stageFileCopy(path, options.BackupPath, options.Expected, ops)
		if err != nil {
			return fmt.Errorf("failed to stage backup: %w", err)
		}

		previousBackup, err = CaptureSnapshot(options.BackupPath)
		if err != nil {
			return fmt.Errorf("failed to inspect existing backup: %w", err)
		}
		if previousBackup.Exists {
			previousBackupTemp, err = stageFileCopy(options.BackupPath, options.BackupPath, &previousBackup, ops)
			if err != nil {
				return fmt.Errorf("failed to preserve existing backup: %w", err)
			}
		}

		if err = previousBackup.Verify(options.BackupPath); err != nil {
			return fmt.Errorf("failed to verify existing backup before commit: %w", err)
		}
		if err = ops.replacePath(backupTemp, options.BackupPath); err != nil {
			return fmt.Errorf("failed to commit backup: %w", err)
		}
		backupTemp = ""
		backupCommitted = true
		if err = ops.syncDirectory(filepath.Dir(options.BackupPath)); err != nil {
			return errors.Join(fmt.Errorf("failed to sync backup directory: %w", err), rollbackBackup())
		}
	}

	if options.Expected != nil {
		if err = options.Expected.Verify(path); err != nil {
			return errors.Join(err, rollbackBackup())
		}
	}

	commitTarget := ops.replacePath
	if options.Expected != nil && !options.Expected.Exists {
		commitTarget = ops.installNoReplace
	}
	if err = commitStagedTargetWithRetry(path, targetTemp, options.Expected, commitTarget); err != nil {
		if options.Expected != nil && !options.Expected.Exists && ops.isDestinationExists(err) {
			err = fmt.Errorf("%w: path appeared before commit: %s", ErrConcurrentModification, path)
		}
		return errors.Join(fmt.Errorf("failed to commit target: %w", err), rollbackBackup())
	}
	targetTemp = ""
	if err = ops.syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("target was replaced but its directory could not be synced: %w", err)
	}

	// The newly committed backup is now authoritative; the staged previous
	// backup is no longer needed.
	backupCommitted = false
	return nil
}

const (
	atomicReplaceRetryWindow = 750 * time.Millisecond
	atomicReplaceRetryDelay  = 25 * time.Millisecond
)

func commitStagedTargetWithRetry(path, stagedPath string, expected *FileSnapshot, commit func(string, string) error) error {
	err := commit(stagedPath, path)
	if err == nil || expected == nil || !expected.Exists || !isRetryableAtomicReplaceError(err) {
		return err
	}

	deadline := time.Now().Add(atomicReplaceRetryWindow)
	lastErr := err
	for time.Now().Before(deadline) {
		time.Sleep(atomicReplaceRetryDelay)
		if verifyErr := expected.Verify(path); verifyErr != nil {
			if isRetryableAtomicReplaceError(verifyErr) {
				lastErr = verifyErr
				continue
			}
			return fmt.Errorf("target changed while waiting to retry atomic replacement: %w", verifyErr)
		}
		lastErr = commit(stagedPath, path)
		if lastErr == nil || !isRetryableAtomicReplaceError(lastErr) {
			return lastErr
		}
	}
	return lastErr
}

// CopyFile copies a regular file through the same durable staging layer and
// atomically fails when the destination already exists.
func CopyFile(source, destination string) (err error) {
	defer func() {
		err = operation.WrapFilesystem("copy_file", destination, err)
	}()

	sourceSnapshot, err := CaptureSnapshot(source)
	if err != nil {
		return err
	}
	if !sourceSnapshot.Exists {
		return fs.ErrNotExist
	}
	if !sourceSnapshot.Mode.IsRegular() {
		return fmt.Errorf("source is not a regular file: %s", source)
	}
	destinationSnapshot, err := CaptureSnapshot(destination)
	if err != nil {
		return err
	}
	if destinationSnapshot.Exists {
		return ErrDestinationExists
	}

	tempPath, err := stageFileCopy(source, destination, &sourceSnapshot, defaultMutationOps)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, cleanupMutationPath(tempPath, defaultMutationOps))
	}()

	if err = destinationSnapshot.Verify(destination); err != nil {
		if errors.Is(err, ErrConcurrentModification) {
			return ErrDestinationExists
		}
		return err
	}
	if err = defaultMutationOps.installNoReplace(tempPath, destination); err != nil {
		if defaultMutationOps.isDestinationExists(err) {
			return ErrDestinationExists
		}
		return fmt.Errorf("failed to install copied file: %w", err)
	}
	tempPath = ""
	if err = defaultMutationOps.syncDirectory(filepath.Dir(destination)); err != nil {
		return fmt.Errorf("copied file was installed but its directory could not be synced: %w", err)
	}
	return nil
}

// MoveNoReplace moves a file or directory without replacing an existing
// destination. Platform-specific implementations use native exclusion flags.
func MoveNoReplace(source, destination string) (err error) {
	defer func() {
		err = operation.WrapFilesystem("move_no_replace", destination, err)
	}()

	if _, err := os.Stat(source); err != nil {
		return err
	}
	if _, err := os.Lstat(destination); err == nil {
		return ErrDestinationExists
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := defaultMutationOps.moveNoReplace(source, destination); err != nil {
		if defaultMutationOps.isDestinationExists(err) {
			return ErrDestinationExists
		}
		return err
	}
	var syncErr error
	for _, dir := range uniqueDirectories(filepath.Dir(source), filepath.Dir(destination)) {
		if err := defaultMutationOps.syncDirectory(dir); err != nil {
			syncErr = errors.Join(syncErr, fmt.Errorf("failed to sync directory %s: %w", dir, err))
		}
	}
	return syncErr
}

// RemoveFile verifies the optional snapshot, removes path, and syncs its parent.
func RemoveFile(path string, expected *FileSnapshot) (err error) {
	defer func() {
		err = operation.WrapFilesystem("remove_file", path, err)
	}()

	if expected != nil {
		if err := expected.Verify(path); err != nil {
			return err
		}
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := defaultMutationOps.syncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("file was removed but its directory could not be synced: %w", err)
	}
	return nil
}

func stageBytes(target string, data []byte, mode fs.FileMode, modTime *time.Time, ops mutationOps) (string, error) {
	path, _, _, err := stageReader(target, bytes.NewReader(data), mode, modTime, ops)
	return path, err
}

func stageReader(target string, source io.Reader, mode fs.FileMode, modTime *time.Time, ops mutationOps) (path string, size int64, digest [sha256.Size]byte, err error) {
	file, err := os.CreateTemp(filepath.Dir(target), mutationTempPattern(target))
	if err != nil {
		return "", 0, digest, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := file.Name()
	path = tempPath
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
		if err != nil {
			err = errors.Join(err, cleanupMutationPath(tempPath, ops))
			path = ""
			size = 0
			digest = [sha256.Size]byte{}
		}
	}()

	if err = file.Chmod(mode.Perm()); err != nil {
		return "", 0, digest, fmt.Errorf("failed to set temp file mode: %w", err)
	}
	hasher := sha256.New()
	copyStream := ops.copyStream
	if copyStream == nil {
		copyStream = io.Copy
	}
	size, err = copyStream(io.MultiWriter(file, hasher), source)
	if err != nil {
		return "", 0, digest, fmt.Errorf("failed to write temp file: %w", err)
	}
	copy(digest[:], hasher.Sum(nil))
	if modTime != nil {
		if err = os.Chtimes(path, *modTime, *modTime); err != nil {
			return "", 0, digest, fmt.Errorf("failed to set temp file time: %w", err)
		}
	}
	if err = ops.syncFile(file); err != nil {
		return "", 0, digest, fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err = file.Close(); err != nil {
		return "", 0, digest, fmt.Errorf("failed to close temp file: %w", err)
	}
	return path, size, digest, nil
}

func stageFileCopy(source, destinationHint string, expected *FileSnapshot, ops mutationOps) (path string, err error) {
	if expected != nil {
		if err := expected.verifyMetadata(source); err != nil {
			return "", err
		}
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return "", err
	}
	defer sourceFile.Close()
	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return "", err
	}
	if !sourceInfo.Mode().IsRegular() {
		return "", fmt.Errorf("source is not a regular file: %s", source)
	}
	if expected != nil {
		if err := expected.verifyInfo(source, sourceInfo); err != nil {
			return "", err
		}
	}

	tempFile, err := os.CreateTemp(filepath.Dir(destinationHint), mutationTempPattern(destinationHint))
	if err != nil {
		return "", err
	}
	tempPath := tempFile.Name()
	path = tempPath
	defer func() {
		if closeErr := tempFile.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
			err = errors.Join(err, closeErr)
		}
		if err != nil {
			err = errors.Join(err, cleanupMutationPath(tempPath, ops))
			path = ""
		}
	}()

	if err = tempFile.Chmod(sourceInfo.Mode().Perm()); err != nil {
		return "", err
	}
	var writer io.Writer = tempFile
	var hasher = sha256.New()
	if expected != nil && expected.hasDigest {
		writer = io.MultiWriter(tempFile, hasher)
	}
	if _, err = io.Copy(writer, sourceFile); err != nil {
		return "", err
	}
	if expected != nil && expected.hasDigest && !bytes.Equal(hasher.Sum(nil), expected.digest[:]) {
		return "", fmt.Errorf("%w: content changed while copying %s", ErrConcurrentModification, source)
	}
	if err = os.Chtimes(path, sourceInfo.ModTime(), sourceInfo.ModTime()); err != nil {
		return "", err
	}
	if err = ops.syncFile(tempFile); err != nil {
		return "", err
	}
	if err = tempFile.Close(); err != nil {
		return "", err
	}
	if expected != nil {
		if err = expected.verifyMetadata(source); err != nil {
			return "", err
		}
	}
	return path, nil
}

func mutationTempPattern(path string) string {
	return "." + filepath.Base(path) + ".*.tmp"
}

func cleanupMutationPath(path string, ops mutationOps) error {
	if path == "" {
		return nil
	}
	if err := ops.remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to clean mutation artifact %s: %w", path, err)
	}
	return nil
}

func removeIfExists(path string, ops mutationOps) error {
	if err := ops.remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func uniqueDirectories(paths ...string) []string {
	seen := make(map[string]struct{}, len(paths))
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}
