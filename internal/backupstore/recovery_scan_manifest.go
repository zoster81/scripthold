package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
)

func scanRecoveryManifests(ctx context.Context, root string, descriptor recoveryDescriptorSnapshot, bounds RecoveryBounds) (recoveryManifestScanResult, error) {
	result := recoveryManifestScanResult{
		candidates:      make([]recoveryManifestCandidate, 0),
		indexManifests:  make([]Manifest, 0),
		references:      make(map[string]int),
		reasonCounts:    make(map[RecoveryRejectReason]int),
		events:          make([]string, 0),
		complete:        true,
		rejectedRecords: make([]RecoveryRejectedRecord, 0),
	}
	manifestRoot := filepath.Join(root, "manifests")
	rootInfo, err := os.Lstat(manifestRoot)
	if os.IsNotExist(err) {
		result.events = append(result.events, "manifests|missing")
		return result, nil
	}
	if err != nil || isLinkOrReparse(rootInfo) || !rootInfo.IsDir() || validatePathPermissions(manifestRoot, true) != nil {
		result.complete = false
		result.structuralIssue = true
		result.events = append(result.events, "manifests|unscannable")
		return result, nil
	}

	entries, overflow, err := readDirectoryBounded(manifestRoot, bounds.MaxManifests)
	if err != nil {
		return recoveryManifestScanResult{}, sanitizedFilesystemError("backup manifests cannot be inspected for recovery", err)
	}
	if overflow {
		result.complete = false
		result.limited = true
		result.events = append(result.events, "manifests|overflow")
	}

	byBackupID := make(map[string][]recoveryManifestCandidate)
	for _, entry := range entries {
		if err := recoveryContextError(ctx, "scan_backup_recovery_manifests"); err != nil {
			return recoveryManifestScanResult{}, err
		}
		nameToken := opaqueRecoveryToken(entry.Name())
		path := filepath.Join(manifestRoot, entry.Name())
		info, statErr := os.Lstat(path)
		if statErr != nil || isLinkOrReparse(info) || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > maxManifestBytes ||
			validateSingleLink(path, info) != nil || validatePathPermissions(path, false) != nil {
			rejectUnidentifiedRecoveryManifest(&result, nameToken, "metadata")
			continue
		}
		data, ok := readStableRecoveryRegularFile(path, info, maxManifestBytes)
		if !ok || !recoveryFileIdentityStable(path, info) {
			rejectUnidentifiedRecoveryManifest(&result, nameToken, "unstable")
			continue
		}
		rawDigest := stableRecoveryBytesDigest(data)
		if !descriptor.valid {
			rejectUnidentifiedRecoveryManifest(&result, rawDigest, "descriptor-untrusted")
			continue
		}
		var manifest Manifest
		if err := decodeStrictRecoveryJSON(data, maxManifestBytes, &manifest); err != nil || validateManifest(manifest, descriptor.descriptor) != nil {
			rejectUnidentifiedRecoveryManifest(&result, rawDigest, "invalid")
			continue
		}
		canonicalFilename := entry.Name() == filepath.Base(manifestPath(root, manifest.BackupID))
		candidate := recoveryManifestCandidate{manifest: manifest, path: path, canonicalFilename: canonicalFilename}
		byBackupID[manifest.BackupID] = append(byBackupID[manifest.BackupID], candidate)
		filenameState := "canonical"
		if !canonicalFilename {
			filenameState = "noncanonical"
		}
		result.events = append(result.events, "manifest|valid|"+filenameState+"|"+manifest.BackupID+"|"+manifest.ManifestChecksum+"|"+manifest.ObjectDigest+"|"+strconv.FormatInt(manifest.ObjectBytes, 10))
	}

	backupIDs := make([]string, 0, len(byBackupID))
	for backupID := range byBackupID {
		backupIDs = append(backupIDs, backupID)
	}
	sort.Strings(backupIDs)
	for _, backupID := range backupIDs {
		group := byBackupID[backupID]
		canonical := make([]recoveryManifestCandidate, 0, 1)
		nonCanonicalCount := 0
		for _, candidate := range group {
			if candidate.canonicalFilename {
				canonical = append(canonical, candidate)
			} else {
				nonCanonicalCount++
			}
		}
		if nonCanonicalCount > 0 {
			result.reasonCounts[RecoveryRejectManifestInvalid] += nonCanonicalCount
			result.events = append(result.events, "manifest|noncanonical-evidence|"+backupID+"|count="+strconv.Itoa(nonCanonicalCount))
		}
		if len(canonical) == 0 {
			result.rejectedRecordCount++
			result.rejectedRecords = append(result.rejectedRecords, RecoveryRejectedRecord{BackupID: backupID, Reason: RecoveryRejectManifestInvalid})
			result.events = append(result.events, "manifest|missing-canonical|"+backupID)
			continue
		}
		chosen := canonical[0]
		ambiguous := false
		for index := 1; index < len(canonical); index++ {
			if !reflect.DeepEqual(canonical[index].manifest, chosen.manifest) {
				ambiguous = true
				break
			}
		}
		if ambiguous {
			result.rejectedRecordCount++
			result.rejectedRecords = append(result.rejectedRecords, RecoveryRejectedRecord{BackupID: backupID, Reason: RecoveryRejectManifestInvalid})
			result.events = append(result.events, "manifest|ambiguous|"+backupID)
			continue
		}
		result.candidates = append(result.candidates, chosen)
		result.indexManifests = append(result.indexManifests, chosen.manifest)
		result.references[chosen.manifest.ObjectDigest]++
	}
	return result, nil
}

func rejectUnidentifiedRecoveryManifest(result *recoveryManifestScanResult, evidenceToken, detail string) {
	if result == nil {
		return
	}
	result.rejectedRecordCount++
	result.reasonCounts[RecoveryRejectManifestInvalid]++
	result.events = append(result.events, "manifest|rejected|"+detail+"|"+evidenceToken)
}
