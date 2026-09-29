//go:build linux || darwin

package filesystem

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	maxExecutableXattrListBytes  = 64 << 10
	maxExecutableXattrValueBytes = 64 << 10
	maxExecutableXattrTotalBytes = 256 << 10
	maxExecutableXattrCount      = 128
)

type executableMetadata struct {
	mode   os.FileMode
	uid    int
	gid    int
	xattrs map[string][]byte
}

func prepareExecutableReplacementCandidate(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) error {
	before, err := captureExecutableMetadata(targetPath, targetIdentity)
	if err != nil {
		return err
	}
	if err := applyExecutableMetadata(candidatePath, candidateIdentity, before); err != nil {
		return err
	}
	after, err := captureExecutableMetadata(targetPath, targetIdentity)
	if err != nil {
		return err
	}
	if !executableMetadataEqual(before, after) {
		return errors.New("target executable metadata changed during replacement preparation")
	}
	prepared, err := captureExecutableMetadata(candidatePath, candidateIdentity)
	if err != nil {
		return err
	}
	if !executableMetadataEqual(before, prepared) {
		return errors.New("candidate executable metadata does not match target")
	}
	file, err := OpenVerifiedSingleLinkFile(candidatePath, candidateIdentity)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func captureExecutableMetadata(path string, expected ObjectIdentity) (executableMetadata, error) {
	file, err := OpenVerifiedSingleLinkFile(path, expected)
	if err != nil {
		return executableMetadata{}, err
	}
	info, err := file.Stat()
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		return executableMetadata{}, errors.Join(err, closeErr)
	}
	if !info.Mode().IsRegular() {
		return executableMetadata{}, errors.New("executable metadata source is not a regular file")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return executableMetadata{}, errors.New("target executable has no execute permission bits")
	}
	if info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return executableMetadata{}, errors.New("privilege-bearing executable mode is unsupported")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat == nil {
		return executableMetadata{}, errors.New("executable ownership metadata is unavailable")
	}
	attrs, err := readExecutableXattrs(path)
	if err != nil {
		return executableMetadata{}, err
	}
	return executableMetadata{
		mode:   info.Mode().Perm(),
		uid:    int(stat.Uid),
		gid:    int(stat.Gid),
		xattrs: attrs,
	}, nil
}

func applyExecutableMetadata(path string, expected ObjectIdentity, metadata executableMetadata) error {
	matches, err := expected.Matches(path)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New("candidate identity changed before metadata preparation")
	}
	if err := os.Chown(path, metadata.uid, metadata.gid); err != nil {
		return fmt.Errorf("preserve executable ownership: %w", err)
	}
	if err := os.Chmod(path, metadata.mode); err != nil {
		return fmt.Errorf("preserve executable mode: %w", err)
	}
	existing, err := readExecutableXattrs(path)
	if err != nil {
		return err
	}
	for name := range existing {
		if err := unix.Removexattr(path, name); err != nil {
			return fmt.Errorf("remove candidate xattr %q: %w", name, err)
		}
	}
	names := make([]string, 0, len(metadata.xattrs))
	for name := range metadata.xattrs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := unix.Setxattr(path, name, metadata.xattrs[name], 0); err != nil {
			return fmt.Errorf("preserve executable xattr %q: %w", name, err)
		}
	}
	return nil
}

func readExecutableXattrs(path string) (map[string][]byte, error) {
	size, err := unix.Listxattr(path, nil)
	if err != nil {
		return nil, fmt.Errorf("list executable xattrs: %w", err)
	}
	if size < 0 || size > maxExecutableXattrListBytes {
		return nil, errors.New("executable xattr name list exceeds limit")
	}
	if size == 0 {
		return map[string][]byte{}, nil
	}
	buffer := make([]byte, size)
	n, err := unix.Listxattr(path, buffer)
	if err != nil {
		return nil, fmt.Errorf("read executable xattr names: %w", err)
	}
	if n != size {
		return nil, errors.New("executable xattr list changed while reading")
	}
	names, err := parseExecutableXattrNames(buffer[:n])
	if err != nil {
		return nil, err
	}
	if len(names) > maxExecutableXattrCount {
		return nil, errors.New("executable xattr count exceeds limit")
	}
	result := make(map[string][]byte, len(names))
	total := 0
	for _, name := range names {
		if _, exists := result[name]; exists {
			return nil, fmt.Errorf("duplicate executable xattr %q", name)
		}
		if name == "security.capability" {
			return nil, errors.New("linux executable capabilities are unsupported")
		}
		// XNU stores file-system extended ACL/security metadata under this
		// dedicated security attribute. It is deliberately rejected rather
		// than treated as an ordinary xattr.
		if name == "com.apple.system.Security" {
			return nil, errors.New("macOS extended ACL/security metadata is unsupported")
		}
		valueSize, err := unix.Getxattr(path, name, nil)
		if err != nil {
			return nil, fmt.Errorf("size executable xattr %q: %w", name, err)
		}
		if valueSize < 0 || valueSize > maxExecutableXattrValueBytes ||
			total+valueSize > maxExecutableXattrTotalBytes {
			return nil, errors.New("executable xattr values exceed limit")
		}
		value := make([]byte, valueSize)
		if valueSize > 0 {
			read, err := unix.Getxattr(path, name, value)
			if err != nil {
				return nil, fmt.Errorf("read executable xattr %q: %w", name, err)
			}
			if read != valueSize {
				return nil, fmt.Errorf("executable xattr %q changed while reading", name)
			}
		}
		total += valueSize
		result[name] = value
	}
	return result, nil
}

func parseExecutableXattrNames(buffer []byte) ([]string, error) {
	var names []string
	for len(buffer) > 0 {
		index := bytes.IndexByte(buffer, 0)
		if index < 0 {
			return nil, errors.New("executable xattr name list is not NUL terminated")
		}
		name := string(buffer[:index])
		if name == "" || strings.IndexByte(name, 0) >= 0 {
			return nil, errors.New("executable xattr name is invalid")
		}
		names = append(names, name)
		buffer = buffer[index+1:]
	}
	return names, nil
}

func executableMetadataEqual(left, right executableMetadata) bool {
	if left.mode != right.mode || left.uid != right.uid || left.gid != right.gid ||
		len(left.xattrs) != len(right.xattrs) {
		return false
	}
	for name, value := range left.xattrs {
		if !bytes.Equal(value, right.xattrs[name]) {
			return false
		}
	}
	return true
}

func commitExecutableReplacementCandidate(
	targetPath string,
	targetIdentity ObjectIdentity,
	candidatePath string,
	candidateIdentity ObjectIdentity,
) error {
	targetMatches, err := targetIdentity.Matches(targetPath)
	if err != nil {
		return err
	}
	if !targetMatches {
		return errors.New("target executable identity changed before commit")
	}
	candidateMatches, err := candidateIdentity.Matches(candidatePath)
	if err != nil {
		return err
	}
	if !candidateMatches {
		return errors.New("candidate executable identity changed before commit")
	}
	targetMetadata, err := captureExecutableMetadata(targetPath, targetIdentity)
	if err != nil {
		return err
	}
	candidateMetadata, err := captureExecutableMetadata(candidatePath, candidateIdentity)
	if err != nil {
		return err
	}
	if !executableMetadataEqual(targetMetadata, candidateMetadata) {
		return errors.New("candidate executable metadata drifted before commit")
	}
	if err := os.Rename(candidatePath, targetPath); err != nil {
		return err
	}
	if err := syncDirectory(filepath.Dir(targetPath)); err != nil {
		return fmt.Errorf("sync executable parent directory: %w", err)
	}
	matchesInstalled, err := candidateIdentity.Matches(targetPath)
	if err != nil {
		return err
	}
	if !matchesInstalled {
		return errors.New("installed executable identity does not match consumed candidate")
	}
	return nil
}
