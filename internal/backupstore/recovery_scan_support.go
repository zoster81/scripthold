package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

type recoveryRootScanResult struct {
	unknownCount int
	layoutIssue  bool
	limited      bool
	complete     bool
	events       []string
}

type recoveryResidueScanResult struct {
	count    int
	bytes    int64
	issue    bool
	complete bool
	events   []string
}

func inspectRecoveryDescriptor(root string) recoveryDescriptorSnapshot {
	path := filepath.Join(root, "store.json")
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return recoveryDescriptorSnapshot{state: "missing"}
	}
	if err != nil {
		return recoveryDescriptorSnapshot{state: "unreadable"}
	}
	if isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxDescriptorBytes {
		return recoveryDescriptorSnapshot{state: "invalid-metadata"}
	}
	if err := validateSingleLink(path, info); err != nil {
		return recoveryDescriptorSnapshot{state: "invalid-link-state"}
	}
	if err := validatePathPermissions(path, false); err != nil {
		return recoveryDescriptorSnapshot{state: "invalid-permissions"}
	}
	data, ok := readStableRecoveryRegularFile(path, info, maxDescriptorBytes)
	if !ok {
		return recoveryDescriptorSnapshot{state: "unstable"}
	}
	digest := sha256.Sum256(data)
	fingerprint := hex.EncodeToString(digest[:])
	var descriptor Descriptor
	if err := decodeStrictRecoveryJSON(data, maxDescriptorBytes, &descriptor); err != nil {
		return recoveryDescriptorSnapshot{fingerprint: fingerprint, state: "invalid-json"}
	}
	if err := validateDescriptor(descriptor); err != nil {
		return recoveryDescriptorSnapshot{fingerprint: fingerprint, state: "invalid-schema"}
	}
	return recoveryDescriptorSnapshot{
		descriptor:  descriptor,
		valid:       true,
		fingerprint: fingerprint,
		state:       "valid",
	}
}

func readStableRecoveryRegularFile(path string, expected os.FileInfo, maxBytes int) ([]byte, bool) {
	if expected == nil || maxBytes < 0 || expected.Size() < 0 || expected.Size() > int64(maxBytes) {
		return nil, false
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	data, readErr := io.ReadAll(io.LimitReader(file, int64(maxBytes)+1))
	openedInfo, statErr := file.Stat()
	closeErr := file.Close()
	if readErr != nil || statErr != nil || closeErr != nil || openedInfo == nil ||
		!openedInfo.Mode().IsRegular() || isLinkOrReparse(openedInfo) || !os.SameFile(expected, openedInfo) ||
		openedInfo.Size() != expected.Size() || len(data) > maxBytes || int64(len(data)) != openedInfo.Size() {
		return nil, false
	}
	return data, true
}

func scanRecoveryRootEntries(ctx context.Context, root string, bounds RecoveryBounds) (recoveryRootScanResult, error) {
	result := recoveryRootScanResult{complete: true, events: make([]string, 0, len(expectedRootEntries)+4)}
	limit := len(expectedRootEntries) + bounds.MaxObjects
	entries, overflow, err := readDirectoryBounded(root, limit)
	if err != nil {
		return recoveryRootScanResult{}, sanitizedFilesystemError("backup store root cannot be inspected for recovery", err)
	}
	if overflow {
		result.complete = false
		result.limited = true
		result.events = append(result.events, "root|overflow")
	}
	present := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if err := recoveryContextError(ctx, "scan_backup_recovery_root"); err != nil {
			return recoveryRootScanResult{}, err
		}
		name := entry.Name()
		present[name] = struct{}{}
		if _, expected := expectedRootEntries[name]; expected {
			continue
		}
		result.unknownCount++
		result.layoutIssue = true
		result.events = append(result.events, "root|unknown|"+opaqueRecoveryToken(name))
	}

	for _, name := range []string{"store.json", "store.lock", "objects", "manifests", "index", "staging", "trash"} {
		if _, exists := present[name]; !exists {
			result.layoutIssue = true
			result.events = append(result.events, "root|missing|"+name)
			if name == "manifests" {
				result.complete = false
			}
			continue
		}
		path := filepath.Join(root, name)
		info, statErr := os.Lstat(path)
		if statErr != nil || isLinkOrReparse(info) {
			result.layoutIssue = true
			result.events = append(result.events, "root|invalid|"+name)
			if name == "manifests" || name == "objects" {
				result.complete = false
			}
			continue
		}
		wantDirectory := name != "store.json" && name != "store.lock"
		if wantDirectory != info.IsDir() || (!wantDirectory && !info.Mode().IsRegular()) {
			result.layoutIssue = true
			result.events = append(result.events, "root|invalid-type|"+name)
			if name == "manifests" || name == "objects" {
				result.complete = false
			}
			continue
		}
		if err := validatePathPermissions(path, wantDirectory); err != nil {
			result.layoutIssue = true
			result.events = append(result.events, "root|invalid-permissions|"+name)
			if name == "manifests" || name == "objects" {
				result.complete = false
			}
		}
	}
	return result, nil
}

func scanRecoveryResidue(ctx context.Context, root string, maxEntries int) (recoveryResidueScanResult, error) {
	result := recoveryResidueScanResult{complete: true, events: make([]string, 0, 4)}
	for _, name := range []string{"staging", "trash"} {
		path := filepath.Join(root, name)
		info, statErr := os.Lstat(path)
		if os.IsNotExist(statErr) {
			result.issue = true
			result.events = append(result.events, "residue|"+name+"|missing")
			continue
		}
		if statErr != nil || isLinkOrReparse(info) || !info.IsDir() || validatePathPermissions(path, true) != nil {
			result.issue = true
			result.complete = false
			result.events = append(result.events, "residue|"+name+"|invalid")
			continue
		}
		limited := false
		issue := false
		addIssue := func(code, _ string) {
			issue = true
			if code == AuditIssueLimit {
				limited = true
			}
		}
		count, bytes, scanErr := scanResidualDirectory(ctx, path, maxEntries, addIssue)
		if scanErr != nil {
			return recoveryResidueScanResult{}, scanErr
		}
		result.count += count
		if !addNonNegativeInt64(&result.bytes, bytes) {
			result.complete = false
			limited = true
		}
		result.issue = result.issue || issue
		if limited {
			result.complete = false
		}
		result.events = append(result.events,
			"residue|"+name+"|count="+strconv.Itoa(count)+"|bytes="+strconv.FormatInt(bytes, 10)+"|limited="+strconv.FormatBool(limited),
		)
	}
	return result, nil
}

func recoveryContextError(ctx context.Context, operationName string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return operation.Wrap(operation.KindCancelled, operationName, "", err)
	}
	return nil
}

func opaqueRecoveryToken(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func stableRecoveryBytesDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func recoveryFileIdentityStable(path string, expected os.FileInfo) bool {
	if expected == nil {
		return false
	}
	current, err := os.Lstat(path)
	return err == nil && current != nil && !isLinkOrReparse(current) && os.SameFile(expected, current) &&
		current.Mode() == expected.Mode() && current.Size() == expected.Size() && current.ModTime().Equal(expected.ModTime()) &&
		recoveryPlatformFileIdentityStable(expected, current)
}

type recoveryOwnedRegularFile struct {
	info     fs.FileInfo
	identity *filesystem.FileIdentity
}

func captureRecoveryOwnedRegularFile(file *os.File) (recoveryOwnedRegularFile, error) {
	if file == nil {
		return recoveryOwnedRegularFile{}, fs.ErrInvalid
	}
	info, err := file.Stat()
	if err != nil || info == nil || isLinkOrReparse(info) || !info.Mode().IsRegular() {
		if err == nil {
			err = fs.ErrInvalid
		}
		return recoveryOwnedRegularFile{}, err
	}
	identity, err := filesystem.OpenFileIdentity(file.Name())
	if err != nil {
		return recoveryOwnedRegularFile{}, err
	}
	current, statErr := os.Lstat(file.Name())
	matches, matchErr := identity.Matches(file.Name())
	if statErr != nil || current == nil || isLinkOrReparse(current) || !current.Mode().IsRegular() ||
		!os.SameFile(info, current) || matchErr != nil || !matches {
		_ = identity.Close()
		if statErr != nil {
			return recoveryOwnedRegularFile{}, statErr
		}
		if matchErr != nil {
			return recoveryOwnedRegularFile{}, matchErr
		}
		return recoveryOwnedRegularFile{}, fs.ErrInvalid
	}
	return recoveryOwnedRegularFile{info: info, identity: identity}, nil
}

func recoveryOwnedRegularFileStable(path string, expected recoveryOwnedRegularFile) bool {
	if path == "" || expected.info == nil || expected.identity == nil {
		return false
	}
	current, err := os.Lstat(path)
	if err != nil || current == nil || isLinkOrReparse(current) || !current.Mode().IsRegular() {
		return false
	}
	matches, err := expected.identity.Matches(path)
	return err == nil && matches
}

func closeRecoveryOwnedRegularFile(expected *recoveryOwnedRegularFile) {
	if expected == nil || expected.identity == nil {
		return
	}
	_ = expected.identity.Close()
	expected.identity = nil
}

func removeRecoveryRegularFileIfOwned(path string, expected recoveryOwnedRegularFile) bool {
	if !recoveryOwnedRegularFileStable(path, expected) {
		return false
	}
	if err := os.Remove(path); err != nil {
		return false
	}
	_ = syncDirectory(filepath.Dir(path))
	return true
}

func writeRecoveryStagingData(directory, pattern string, data []byte) (path string, identity recoveryOwnedRegularFile, err error) {
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", recoveryOwnedRegularFile{}, err
	}
	path = file.Name()
	closed := false
	defer func() {
		if !closed {
			if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) {
				err = errors.Join(err, closeErr)
			}
		}
		if err != nil {
			removeRecoveryRegularFileIfOwned(path, identity)
			closeRecoveryOwnedRegularFile(&identity)
			path = ""
			identity = recoveryOwnedRegularFile{}
		}
	}()

	identity, err = captureRecoveryOwnedRegularFile(file)
	if err != nil {
		return "", recoveryOwnedRegularFile{}, err
	}
	if err = validateSingleLink(path, identity.info); err != nil {
		return "", recoveryOwnedRegularFile{}, err
	}
	if err = restrictPathPermissions(path, false); err != nil {
		return "", recoveryOwnedRegularFile{}, err
	}
	if err = writeAndSync(file, data); err != nil {
		return "", recoveryOwnedRegularFile{}, err
	}
	if err = file.Close(); err != nil {
		closed = true
		return "", recoveryOwnedRegularFile{}, err
	}
	closed = true
	current, statErr := os.Lstat(path)
	if statErr != nil || current == nil || isLinkOrReparse(current) || !current.Mode().IsRegular() ||
		!recoveryOwnedRegularFileStable(path, identity) || current.Size() != int64(len(data)) {
		if statErr != nil {
			err = statErr
		} else {
			err = errors.New("recovery staging identity changed during persistence")
		}
		return "", recoveryOwnedRegularFile{}, err
	}
	if err = validateSingleLink(path, current); err != nil {
		return "", recoveryOwnedRegularFile{}, err
	}
	if err = validatePathPermissions(path, false); err != nil {
		return "", recoveryOwnedRegularFile{}, err
	}
	identity.info = current
	return path, identity, nil
}
