package releasearchive

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
)

// VerifyFlatDirectory authenticates a set of raw, already downloaded Actions
// artifacts inside a private directory without following the asset paths
// outside the opened directory. The trusted caller supplies the complete
// allowlisted inventory, an owner-controlled staging root, and a single-writer
// lifetime. This does not download, publish, rename, or create release assets.
//
// The official artifact download action writes by pathname. A malicious
// concurrent writer running as this process's user can still mutate bytes
// between this check and later publication; this function does not promise
// immutability of files after it returns.
func VerifyFlatDirectory(path string, files []ExpectedFile) error {
	if runtime.GOOS != "linux" {
		return errors.New("raw Actions release staging is only supported on a Linux runner")
	}
	return verifyFlatDirectory(path, files)
}

// verifyFlatDirectory remains directly testable on Windows to exercise its
// real directory and hard-link behavior without authorizing publication there.
func verifyFlatDirectory(path string, files []ExpectedFile) error {
	if len(files) == 0 || len(files) > maxFiles {
		return errors.New("invalid raw staging file count")
	}
	expected := make(map[string]ExpectedFile, len(files))
	folded := make(map[string]bool, len(files))
	for _, file := range files {
		if !portableStagingName(file.Name) ||
			file.Size <= 0 || file.Size > maxAssetBytes || !validDigest(file.SHA256) {
			return fmt.Errorf("invalid expected raw release file %q", file.Name)
		}
		key := strings.ToLower(file.Name)
		if _, duplicate := expected[file.Name]; duplicate || folded[key] {
			return fmt.Errorf("duplicate raw release filename %q", file.Name)
		}
		folded[key] = true
		expected[file.Name] = file
	}
	initialRoot, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect staging root: %w", err)
	}
	if !initialRoot.IsDir() || initialRoot.Mode()&os.ModeSymlink != 0 {
		return errors.New("staging root must be a real directory")
	}
	if err := verifyPrivateStagingRoot(initialRoot); err != nil {
		return fmt.Errorf("staging root is not private: %w", err)
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return fmt.Errorf("open anchored staging root: %w", err)
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil {
		return fmt.Errorf("stat anchored staging root: %w", err)
	}
	if !os.SameFile(initialRoot, rootInfo) {
		return errors.New("staging root changed before verification")
	}
	directory, err := root.Open(".")
	if err != nil {
		return fmt.Errorf("open staging directory listing: %w", err)
	}
	// Reading only one entry above the declared total bounds a hostile
	// directory listing without allocating memory for an unbounded directory.
	items, readErr := directory.ReadDir(len(files) + 1)
	closeErr := directory.Close()
	if (readErr != nil && !errors.Is(readErr, io.EOF)) || closeErr != nil {
		return fmt.Errorf("enumerate staging directory: %w", errors.Join(readErr, closeErr))
	}
	if len(items) != len(files) {
		return errors.New("staging directory file count mismatch")
	}
	for _, item := range items {
		if _, ok := expected[item.Name()]; !ok {
			return fmt.Errorf("unexpected staging directory entry %q", item.Name())
		}
	}
	for _, file := range files {
		if err := verifyFlatMember(root, file); err != nil {
			return fmt.Errorf("verify staging file %q: %w", file.Name, err)
		}
	}
	currentRoot, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("recheck staging root: %w", err)
	}
	if !sameStagingInfo(initialRoot, currentRoot) {
		return errors.New("staging root changed during verification")
	}
	return nil
}

func portableStagingName(name string) bool {
	if !safeFlatName(name) || len(name) > 240 || strings.HasSuffix(name, ".") {
		return false
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	switch stem {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return false
	}
	return true
}

func verifyFlatMember(root *os.Root, expected ExpectedFile) error {
	before, err := root.Lstat(expected.Name)
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() || before.Size() != expected.Size {
		return errors.New("staging file is not a matching ordinary file")
	}
	file, err := root.OpenFile(expected.Name, os.O_RDONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || !sameStagingInfo(before, opened) {
		return errors.New("opened staging file differs from the directory entry")
	}
	if err := verifySingleLinkStagingFile(file); err != nil {
		return err
	}
	if runtime.GOOS == "linux" && opened.Mode().Perm()&0o022 != 0 {
		return errors.New("staging file is writable by another user or group")
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, expected.Size+1))
	if err != nil {
		return err
	}
	if n != expected.Size {
		return errors.New("staging file length mismatch")
	}
	if hex.EncodeToString(hash.Sum(nil)) != expected.SHA256 {
		return errors.New("staging file digest mismatch")
	}
	after, err := file.Stat()
	if err != nil {
		return fmt.Errorf("recheck opened staging file: %w", err)
	}
	if !sameStagingInfo(opened, after) {
		return errors.New("opened staging file changed while reading")
	}
	leafNow, err := root.Lstat(expected.Name)
	if err != nil {
		return fmt.Errorf("recheck staged file path: %w", err)
	}
	if !sameStagingInfo(opened, leafNow) {
		return errors.New("staging file path changed while reading")
	}
	return nil
}

func sameStagingInfo(a, b os.FileInfo) bool {
	return a != nil && b != nil && os.SameFile(a, b) &&
		a.Size() == b.Size() && a.Mode() == b.Mode() && a.ModTime().Equal(b.ModTime())
}
