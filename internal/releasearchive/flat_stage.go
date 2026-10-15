package releasearchive

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

// CreatePrivateRawStagingDirectory creates an empty, single-owner destination
// under the locally trusted runner temporary directory. Never pass a source
// asset path or an untrusted cross-job path as parent. The returned pathname
// is for the SAME runner job only; it is not part of the release manifest.
//
// The caller must keep the directory private and single-writer until its
// authenticated contents have been consumed and safely cleaned up.
func CreatePrivateRawStagingDirectory(parent string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("raw release staging requires a Linux runner")
	}
	return createPrivateRawStagingDirectory(parent)
}

// StageVerifiedSourceFile copies a source file whose original bytes have an
// independent trusted digest to a unique physical raw-upload basename.
// Source directory and its leaf are supplied by the producing job locally,
// not by a later consumer or an untrusted manifest path. The file is created
// without replacement, verified while copied, and synced before use.
func StageVerifiedSourceFile(sourceDir, sourceName, stageDir, stageName string, expected ExpectedFile) error {
	if runtime.GOOS != "linux" {
		return errors.New("raw release staging requires a Linux runner")
	}
	return stageVerifiedSourceFile(sourceDir, sourceName, stageDir, stageName, expected)
}

// removeCreatedStagingFile refuses to unlink a different object if the
// freshly created output pathname was replaced before error recovery.
// The directory is private and single-writer; this is an identity check,
// not an atomic unlink-by-inode guarantee against a hostile same-UID racer.
func removeCreatedStagingFile(root *os.Root, name string, created os.FileInfo) error {
	if created == nil {
		return errors.New("missing identity of originally created staging file")
	}
	current, err := root.Lstat(name)
	if err != nil {
		return fmt.Errorf("inspect failed staging output before cleanup: %w", err)
	}
	if !current.Mode().IsRegular() || !os.SameFile(created, current) {
		return errors.New("staging file path was replaced; refusing unsafe cleanup")
	}
	if err := root.Remove(name); err != nil {
		return fmt.Errorf("remove created staging output: %w", err)
	}
	return nil
}

func stageVerifiedSourceFile(sourceDir, sourceName, stageDir, stageName string, expected ExpectedFile) (err error) {
	if !filepath.IsAbs(sourceDir) || !filepath.IsAbs(stageDir) ||
		!portableStagingName(sourceName) || !portableStagingName(stageName) ||
		!portableStagingName(expected.Name) ||
		expected.Size <= 0 || expected.Size > maxAssetBytes ||
		!validDigest(expected.SHA256) {
		return errors.New("invalid source, destination, or authenticated asset")
	}

	// Rooted source operations never follow an absolute or escaping symlink.
	sourceRoot, err := os.OpenRoot(sourceDir)
	if err != nil {
		return fmt.Errorf("open producer source directory: %w", err)
	}
	defer sourceRoot.Close()
	before, err := sourceRoot.Lstat(sourceName)
	if err != nil {
		return fmt.Errorf("inspect producer source: %w", err)
	}
	if !before.Mode().IsRegular() || before.Size() != expected.Size {
		return errors.New("source file is not a matching ordinary file")
	}
	source, err := sourceRoot.OpenFile(sourceName, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open producer source: %w", err)
	}
	defer source.Close()
	sourceInfo, err := source.Stat()
	if err != nil || !sameStagingInfo(before, sourceInfo) {
		return errors.New("producer source changed during opening")
	}
	if err := verifySingleLinkStagingFile(source); err != nil {
		return fmt.Errorf("producer source identity: %w", err)
	}

	initialRoot, err := os.Lstat(stageDir)
	if err != nil {
		return fmt.Errorf("inspect private staging directory: %w", err)
	}
	if !initialRoot.IsDir() || initialRoot.Mode()&os.ModeSymlink != 0 {
		return errors.New("staging destination must be a real directory")
	}
	if err := verifyPrivateStagingRoot(initialRoot); err != nil {
		return fmt.Errorf("staging destination permissions: %w", err)
	}
	destRoot, err := os.OpenRoot(stageDir)
	if err != nil {
		return fmt.Errorf("open private staging destination: %w", err)
	}
	defer destRoot.Close()
	anchored, err := destRoot.Stat(".")
	if err != nil || !os.SameFile(initialRoot, anchored) {
		return errors.New("staging destination changed during opening")
	}

	// No-replace matters even for reruns: a previous upload or an unexpected
	// process must never be silently overwritten or cleaned up as ours.
	dest, err := destRoot.OpenFile(stageName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create exclusive staging file: %w", err)
	}
	createdInfo, infoErr := dest.Stat()
	if infoErr != nil {
		_ = dest.Close()
		return fmt.Errorf("inspect exclusive staging output: %w", infoErr)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, removeCreatedStagingFile(destRoot, stageName, createdInfo))
		}
	}()
	hash := sha256.New()
	count, copyErr := io.CopyBuffer(io.MultiWriter(dest, hash),
		io.LimitReader(source, expected.Size+1), make([]byte, 64*1024))
	if copyErr != nil || count != expected.Size ||
		hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		err = errors.Join(copyErr, errors.New("source changed, exceeded length or failed original digest"))
		_ = dest.Close()
		return err
	}
	afterSource, statErr := source.Stat()
	currentSource, pathErr := sourceRoot.Lstat(sourceName)
	if statErr != nil || pathErr != nil ||
		!sameStagingInfo(before, afterSource) || !sameStagingInfo(before, currentSource) {
		_ = dest.Close()
		return errors.Join(errors.New("producer source changed while copying"), statErr, pathErr)
	}
	if linkErr := verifySingleLinkStagingFile(source); linkErr != nil {
		_ = dest.Close()
		return fmt.Errorf("source hard-link identity changed while copying: %w", linkErr)
	}
	if syncErr := dest.Sync(); syncErr != nil {
		_ = dest.Close()
		return fmt.Errorf("sync staged file: %w", syncErr)
	}
	destInfo, statErr := dest.Stat()
	if statErr != nil {
		_ = dest.Close()
		return statErr
	}
	if !destInfo.Mode().IsRegular() || destInfo.Size() != expected.Size ||
		(runtime.GOOS == "linux" && destInfo.Mode().Perm() != 0o600) {
		_ = dest.Close()
		return errors.New("staged file has unexpected type, length, or permissions")
	}
	if linkErr := verifySingleLinkStagingFile(dest); linkErr != nil {
		_ = dest.Close()
		return linkErr
	}
	if err := dest.Close(); err != nil {
		return fmt.Errorf("close staged file: %w", err)
	}
	afterStage, err := destRoot.Lstat(stageName)
	if err != nil || !sameStagingInfo(destInfo, afterStage) {
		return errors.New("staged file path was replaced")
	}
	currentRoot, err := os.Lstat(stageDir)
	if err != nil || !os.SameFile(initialRoot, currentRoot) ||
		!currentRoot.IsDir() || verifyPrivateStagingRoot(currentRoot) != nil {
		return errors.New("private staging directory changed during copy")
	}
	if runtime.GOOS == "linux" {
		directory, openErr := destRoot.Open(".")
		if openErr != nil {
			return fmt.Errorf("open staging root for sync: %w", openErr)
		}
		syncErr := directory.Sync()
		closeErr := directory.Close()
		if syncErr != nil || closeErr != nil {
			return fmt.Errorf("sync private staging directory: %w",
				errors.Join(syncErr, closeErr))
		}
	}
	return nil
}

func createPrivateRawStagingDirectory(parent string) (stage string, err error) {
	if parent == "" || !filepath.IsAbs(parent) {
		return "", errors.New("staging parent must be an absolute trusted runner directory")
	}
	initial, err := os.Stat(parent)
	if err != nil {
		return "", fmt.Errorf("inspect trusted staging parent: %w", err)
	}
	if !initial.IsDir() {
		return "", errors.New("staging parent is not a directory")
	}
	parentRoot, err := os.OpenRoot(parent)
	if err != nil {
		return "", fmt.Errorf("anchor trusted staging parent: %w", err)
	}
	defer parentRoot.Close()
	anchoredParent, err := parentRoot.Stat(".")
	if err != nil {
		return "", fmt.Errorf("stat anchored staging parent: %w", err)
	}
	// Other legitimate staging runs can create siblings concurrently; parent
	// size and modification time are not directory-identity attributes.
	if !os.SameFile(initial, anchoredParent) || !anchoredParent.IsDir() ||
		initial.Mode() != anchoredParent.Mode() {
		return "", errors.New("trusted staging parent identity or mode changed")
	}

	for range 4 {
		// Root.Mkdir performs an exclusive directory creation relative to the
		// opened parent, even if a parent pathname is renamed meanwhile.
		name := "scripthold-raw-" + rand.Text()
		if err = parentRoot.Mkdir(name, 0o700); errors.Is(err, os.ErrExist) {
			continue
		} else if err != nil {
			return "", fmt.Errorf("create private staging root: %w", err)
		}
		complete := filepath.Join(parent, name)
		// The directory is still empty, so failed checks can safely remove
		// only this freshly created entry via the anchored parent descriptor.
		defer func() {
			if err != nil {
				err = errors.Join(err, parentRoot.Remove(name))
			}
		}()
		created, statErr := parentRoot.Lstat(name)
		if statErr != nil {
			return "", fmt.Errorf("inspect new staging root: %w", statErr)
		}
		if !created.IsDir() || created.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("new staging root is not a real directory")
		}
		if checkErr := verifyPrivateStagingRoot(created); checkErr != nil {
			return "", fmt.Errorf("staging root permissions: %w", checkErr)
		}
		physical, statErr := os.Lstat(complete)
		if statErr != nil {
			return "", fmt.Errorf("recheck new staging root path: %w", statErr)
		}
		if !sameStagingInfo(created, physical) {
			return "", errors.New("new staging root path changed")
		}
		lastParent, statErr := os.Stat(parent)
		if statErr != nil {
			return "", fmt.Errorf("recheck trusted staging parent: %w", statErr)
		}
		// Creating a child legitimately changes its parent's mtime and size.
		// Recheck the directory object and permissions, not volatile metadata.
		if !os.SameFile(initial, lastParent) || !lastParent.IsDir() ||
			initial.Mode() != lastParent.Mode() {
			return "", errors.New("trusted staging parent identity or mode changed")
		}
		anchored, openErr := parentRoot.OpenRoot(name)
		if openErr != nil {
			return "", fmt.Errorf("open newly created staging root: %w", openErr)
		}
		rootInfo, infoErr := anchored.Stat(".")
		closeErr := anchored.Close()
		if infoErr != nil || closeErr != nil {
			return "", fmt.Errorf("inspect anchored new staging root: %w",
				errors.Join(infoErr, closeErr))
		}
		if !sameStagingInfo(created, rootInfo) {
			return "", errors.New("staging root identity mismatch")
		}
		return complete, nil
	}
	return "", errors.New("private staging directory creation encountered repeated collisions")
}
