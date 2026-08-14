package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
)

func scanRecoveryObjects(ctx context.Context, root string, references map[string]int, bounds RecoveryBounds) (recoveryObjectScanResult, error) {
	result := recoveryObjectScanResult{
		states:          make(map[string]recoveryObjectState),
		metadataObjects: make(map[string]scannedObject),
		reasonCounts:    make(map[RecoveryRejectReason]int),
		events:          make([]string, 0),
		complete:        true,
	}
	objectRoot := filepath.Join(root, "objects", ObjectAlgorithm)
	rootInfo, err := os.Lstat(objectRoot)
	if os.IsNotExist(err) {
		result.events = append(result.events, "objects|missing")
		return result, nil
	}
	if err != nil || isLinkOrReparse(rootInfo) || !rootInfo.IsDir() || validatePathPermissions(objectRoot, true) != nil {
		result.complete = false
		result.structuralIssue = true
		result.events = append(result.events, "objects|unscannable")
		return result, nil
	}

	shards, shardOverflow, err := readDirectoryBounded(objectRoot, 256)
	if err != nil {
		return recoveryObjectScanResult{}, sanitizedFilesystemError("backup object shards cannot be inspected for recovery", err)
	}
	if shardOverflow {
		result.complete = false
		result.limited = true
		result.events = append(result.events, "objects|shard-overflow")
	}

	objectCount := 0
	var hashedBytes int64
scanShards:
	for _, shard := range shards {
		if err := recoveryContextError(ctx, "scan_backup_recovery_objects"); err != nil {
			return recoveryObjectScanResult{}, err
		}
		shardName := shard.Name()
		shardPath := filepath.Join(objectRoot, shardName)
		shardInfo, statErr := os.Lstat(shardPath)
		if statErr != nil || len(shardName) != 2 || !isLowerHex(shardName) || isLinkOrReparse(shardInfo) || !shardInfo.IsDir() ||
			validatePathPermissions(shardPath, true) != nil {
			result.reasonCounts[RecoveryRejectObjectInvalid]++
			result.events = append(result.events, "object-shard|rejected|"+opaqueRecoveryToken(shardName))
			continue
		}

		remaining := bounds.MaxObjects - objectCount
		if remaining <= 0 {
			entries, overflow, readErr := readDirectoryBounded(shardPath, 0)
			if readErr != nil {
				return recoveryObjectScanResult{}, sanitizedFilesystemError("backup object shard cannot be inspected for recovery", readErr)
			}
			if overflow || len(entries) > 0 {
				result.complete = false
				result.limited = true
				result.events = append(result.events, "objects|object-limit")
			}
			break
		}
		entries, overflow, readErr := readDirectoryBounded(shardPath, remaining)
		if readErr != nil {
			return recoveryObjectScanResult{}, sanitizedFilesystemError("backup object shard cannot be inspected for recovery", readErr)
		}
		if overflow {
			result.complete = false
			result.limited = true
			result.events = append(result.events, "objects|object-limit")
		}
		for _, entry := range entries {
			if err := recoveryContextError(ctx, "scan_backup_recovery_objects"); err != nil {
				return recoveryObjectScanResult{}, err
			}
			objectCount++
			digest := entry.Name()
			canonicalDigest := validHexIdentifier(digest) && digest[:2] == shardName
			path := filepath.Join(shardPath, digest)
			info, statErr := os.Lstat(path)
			if statErr != nil || !canonicalDigest || isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > hardMaxObjectBytes ||
				validateSingleLink(path, info) != nil || validatePathPermissions(path, false) != nil {
				if canonicalDigest {
					result.states[digest] = recoveryObjectState{digest: digest, path: path, trusted: false, reason: RecoveryRejectObjectInvalid}
					if references[digest] == 0 {
						result.reasonCounts[RecoveryRejectObjectInvalid]++
					}
					result.events = append(result.events, "object|invalid|"+digest)
				} else {
					result.reasonCounts[RecoveryRejectObjectInvalid]++
					result.events = append(result.events, "object|invalid-name|"+opaqueRecoveryToken(digest))
				}
				continue
			}

			size := info.Size()
			result.metadataObjects[digest] = scannedObject{Digest: digest, Bytes: size, References: references[digest]}
			if size > bounds.MaxBytes-hashedBytes {
				result.complete = false
				result.limited = true
				result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: false, reason: RecoveryRejectObjectInvalid}
				result.events = append(result.events, "objects|byte-limit|"+digest+"|"+strconv.FormatInt(size, 10))
				break scanShards
			}
			hashedBytes += size
			actualDigest, hashErr := hashRegularFile(ctx, path, size)
			if hashErr != nil {
				if err := recoveryContextError(ctx, "hash_backup_recovery_object"); err != nil {
					return recoveryObjectScanResult{}, err
				}
				result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: false, reason: RecoveryRejectObjectInvalid}
				if references[digest] == 0 {
					result.reasonCounts[RecoveryRejectObjectInvalid]++
				}
				result.events = append(result.events, "object|unreadable|"+digest+"|"+strconv.FormatInt(size, 10))
				continue
			}
			if actualDigest != digest || !recoveryFileIdentityStable(path, info) {
				result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: false, reason: RecoveryRejectObjectDigestMismatch}
				if references[digest] == 0 {
					result.reasonCounts[RecoveryRejectObjectDigestMismatch]++
				}
				result.events = append(result.events, "object|digest-mismatch|"+digest+"|"+strconv.FormatInt(size, 10))
				continue
			}
			result.states[digest] = recoveryObjectState{digest: digest, bytes: size, path: path, trusted: true}
			result.events = append(result.events, "object|trusted|"+digest+"|"+strconv.FormatInt(size, 10))
		}
		if overflow {
			break
		}
	}
	return result, nil
}
