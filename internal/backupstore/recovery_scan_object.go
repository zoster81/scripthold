package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
)

type recoveryObjectScanner struct {
	ctx         context.Context
	references  map[string]int
	bounds      RecoveryBounds
	result      recoveryObjectScanResult
	objectCount int
	hashedBytes int64
}

func newRecoveryObjectScanner(ctx context.Context, references map[string]int, bounds RecoveryBounds) *recoveryObjectScanner {
	return &recoveryObjectScanner{
		ctx:        ctx,
		references: references,
		bounds:     bounds,
		result: recoveryObjectScanResult{
			states:          make(map[string]recoveryObjectState),
			metadataObjects: make(map[string]scannedObject),
			reasonCounts:    make(map[RecoveryRejectReason]int),
			events:          make([]string, 0),
			complete:        true,
		},
	}
}

func scanRecoveryObjects(ctx context.Context, root string, references map[string]int, bounds RecoveryBounds) (recoveryObjectScanResult, error) {
	scanner := newRecoveryObjectScanner(ctx, references, bounds)
	objectRoot := filepath.Join(root, "objects", ObjectAlgorithm)
	rootInfo, err := os.Lstat(objectRoot)
	if os.IsNotExist(err) {
		scanner.result.events = append(scanner.result.events, "objects|missing")
		return scanner.result, nil
	}
	if err != nil || isLinkOrReparse(rootInfo) || !rootInfo.IsDir() || validatePathPermissions(objectRoot, true) != nil {
		scanner.result.complete = false
		scanner.result.structuralIssue = true
		scanner.result.events = append(scanner.result.events, "objects|unscannable")
		return scanner.result, nil
	}

	shards, shardOverflow, err := readDirectoryBounded(objectRoot, 256)
	if err != nil {
		return recoveryObjectScanResult{}, sanitizedFilesystemError("backup object shards cannot be inspected for recovery", err)
	}
	if shardOverflow {
		scanner.result.complete = false
		scanner.result.limited = true
		scanner.result.events = append(scanner.result.events, "objects|shard-overflow")
	}

	for _, shard := range shards {
		stop, scanErr := scanner.scanShard(objectRoot, shard)
		if scanErr != nil {
			return recoveryObjectScanResult{}, scanErr
		}
		if stop {
			break
		}
	}
	return scanner.result, nil
}

func (scanner *recoveryObjectScanner) scanShard(objectRoot string, shard os.DirEntry) (bool, error) {
	if err := recoveryContextError(scanner.ctx, "scan_backup_recovery_objects"); err != nil {
		return false, err
	}
	shardName := shard.Name()
	shardPath := filepath.Join(objectRoot, shardName)
	shardInfo, statErr := os.Lstat(shardPath)
	if statErr != nil || len(shardName) != 2 || !isLowerHex(shardName) || isLinkOrReparse(shardInfo) || !shardInfo.IsDir() ||
		validatePathPermissions(shardPath, true) != nil {
		scanner.result.reasonCounts[RecoveryRejectObjectInvalid]++
		scanner.result.events = append(scanner.result.events, "object-shard|rejected|"+opaqueRecoveryToken(shardName))
		return false, nil
	}

	remaining := scanner.bounds.MaxObjects - scanner.objectCount
	if remaining <= 0 {
		entries, overflow, readErr := readDirectoryBounded(shardPath, 0)
		if readErr != nil {
			return false, sanitizedFilesystemError("backup object shard cannot be inspected for recovery", readErr)
		}
		if overflow || len(entries) > 0 {
			scanner.markObjectLimit()
		}
		return true, nil
	}

	entries, overflow, readErr := readDirectoryBounded(shardPath, remaining)
	if readErr != nil {
		return false, sanitizedFilesystemError("backup object shard cannot be inspected for recovery", readErr)
	}
	if overflow {
		scanner.markObjectLimit()
	}
	for _, entry := range entries {
		stop, scanErr := scanner.scanObject(shardName, shardPath, entry)
		if scanErr != nil {
			return false, scanErr
		}
		if stop {
			return true, nil
		}
	}
	return overflow, nil
}

func (scanner *recoveryObjectScanner) markObjectLimit() {
	scanner.result.complete = false
	scanner.result.limited = true
	scanner.result.events = append(scanner.result.events, "objects|object-limit")
}

func (scanner *recoveryObjectScanner) scanObject(shardName, shardPath string, entry os.DirEntry) (bool, error) {
	if err := recoveryContextError(scanner.ctx, "scan_backup_recovery_objects"); err != nil {
		return false, err
	}
	scanner.objectCount++
	digest := entry.Name()
	canonicalDigest := validHexIdentifier(digest) && digest[:2] == shardName
	path := filepath.Join(shardPath, digest)
	info, statErr := os.Lstat(path)
	if statErr != nil || !canonicalDigest || isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > hardMaxObjectBytes ||
		validateSingleLink(path, info) != nil || validatePathPermissions(path, false) != nil {
		scanner.rejectInvalidObject(digest, path, canonicalDigest)
		return false, nil
	}

	size := info.Size()
	scanner.result.metadataObjects[digest] = scannedObject{Digest: digest, Bytes: size, References: scanner.references[digest]}
	if size > scanner.bounds.MaxBytes-scanner.hashedBytes {
		scanner.result.complete = false
		scanner.result.limited = true
		scanner.result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: false, reason: RecoveryRejectObjectInvalid}
		scanner.result.events = append(scanner.result.events, "objects|byte-limit|"+digest+"|"+strconv.FormatInt(size, 10))
		return true, nil
	}

	scanner.hashedBytes += size
	actualDigest, hashErr := hashRegularFile(scanner.ctx, path, size)
	if hashErr != nil {
		if err := recoveryContextError(scanner.ctx, "hash_backup_recovery_object"); err != nil {
			return false, err
		}
		scanner.result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: false, reason: RecoveryRejectObjectInvalid}
		if scanner.references[digest] == 0 {
			scanner.result.reasonCounts[RecoveryRejectObjectInvalid]++
		}
		scanner.result.events = append(scanner.result.events, "object|unreadable|"+digest+"|"+strconv.FormatInt(size, 10))
		return false, nil
	}
	if actualDigest != digest || !recoveryFileIdentityStable(path, info) {
		scanner.result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: false, reason: RecoveryRejectObjectDigestMismatch}
		if scanner.references[digest] == 0 {
			scanner.result.reasonCounts[RecoveryRejectObjectDigestMismatch]++
		}
		scanner.result.events = append(scanner.result.events, "object|digest-mismatch|"+digest+"|"+strconv.FormatInt(size, 10))
		return false, nil
	}
	scanner.result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: true}
	scanner.result.events = append(scanner.result.events, "object|trusted|"+digest+"|"+strconv.FormatInt(size, 10))
	return false, nil
}

func (scanner *recoveryObjectScanner) rejectInvalidObject(digest, path string, canonicalDigest bool) {
	if canonicalDigest {
		scanner.result.states[digest] = recoveryObjectState{digest: digest, path: path, trusted: false, reason: RecoveryRejectObjectInvalid}
		if scanner.references[digest] == 0 {
			scanner.result.reasonCounts[RecoveryRejectObjectInvalid]++
		}
		scanner.result.events = append(scanner.result.events, "object|invalid|"+digest)
		return
	}
	scanner.result.reasonCounts[RecoveryRejectObjectInvalid]++
	scanner.result.events = append(scanner.result.events, "object|invalid-name|"+opaqueRecoveryToken(digest))
}
