package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

type backupVisibilitySnapshot struct {
	requested          []string
	resolved           []string
	protectedRequested []string
	protectedResolved  []string
	scope              string
}

func (h *Handler) finishBackupStoreOutput(output BackupStoreOutput, text string) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if err := h.checkBackupStoreOutputLimit(output, text); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) checkBackupStoreOutputLimit(output BackupStoreOutput, text string) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindFilesystem, "encode_backup_store_output", "", err)
	}
	if int64(len(encoded))+int64(len(text)) > h.maxOutputBytes() {
		return operation.New(operation.KindLimit, fmt.Sprintf("backup store output exceeds limit %d bytes", h.maxOutputBytes()))
	}
	return nil
}

func (h *Handler) backupVisibilitySnapshot() backupVisibilitySnapshot {
	h.mu.RLock()
	snapshot := backupVisibilitySnapshot{
		requested:          append([]string(nil), h.allowedRequestedDirs...),
		resolved:           append([]string(nil), h.allowedDirs...),
		protectedRequested: append([]string(nil), h.protectedRequestedDirs...),
		protectedResolved:  append([]string(nil), h.protectedDirs...),
	}
	h.mu.RUnlock()

	hasher := sha256.New()
	_, _ = hasher.Write([]byte("mcp-file-tools:backup-visibility-v1\x00"))
	for _, group := range [][]string{snapshot.requested, snapshot.resolved, snapshot.protectedRequested, snapshot.protectedResolved} {
		ordered := append([]string(nil), group...)
		sort.Strings(ordered)
		for _, path := range ordered {
			_, _ = hasher.Write([]byte(path))
			_, _ = hasher.Write([]byte{0})
		}
		_, _ = hasher.Write([]byte{0xff})
	}
	snapshot.scope = hex.EncodeToString(hasher.Sum(nil))
	return snapshot
}

func (snapshot backupVisibilitySnapshot) validate(path string) (string, error) {
	validated, err := security.ValidatePathWithAllowedDirectories(path, snapshot.requested, snapshot.resolved)
	if err != nil {
		return "", err
	}
	requested := path
	if absolute, absoluteErr := filepath.Abs(security.ExpandHome(path)); absoluteErr == nil {
		requested = absolute
	}
	if security.IsPathWithinAllowedDirectories(requested, snapshot.protectedRequested) ||
		security.IsPathWithinAllowedDirectories(validated, snapshot.protectedResolved) {
		return "", operation.New(operation.KindAccessDenied, "access denied - path reserved for internal storage")
	}
	return validated, nil
}

func mapBackupStoreStatus(status backupstore.StoreStatus) *BackupStoreStatusOutput {
	return &BackupStoreStatusOutput{
		FormatVersion:     status.FormatVersion,
		ManifestVersion:   status.ManifestVersion,
		IndexVersion:      status.IndexVersion,
		ObjectAlgorithm:   status.ObjectAlgorithm,
		Healthy:           status.Healthy,
		Generation:        status.Generation,
		TotalObjectBytes:  status.TotalObjectBytes,
		ObjectCount:       status.ObjectCount,
		ManifestCount:     status.ManifestCount,
		PinnedCount:       status.PinnedCount,
		OrphanObjectCount: status.OrphanObjectCount,
		StagingEntryCount: status.StagingEntryCount,
		TrashEntryCount:   status.TrashEntryCount,
		Limits: BackupStoreLimitsOutput{
			MaxTotalBytes:        status.Limits.MaxTotalBytes,
			MaxObjectBytes:       status.Limits.MaxObjectBytes,
			MaxManifests:         status.Limits.MaxManifests,
			MaxVersionsPerTarget: status.Limits.MaxVersionsPerTarget,
			MaxPinned:            status.Limits.MaxPinned,
			RetentionDays:        status.Limits.RetentionDays,
			PlanTTLSeconds:       status.Limits.PlanTTLSeconds,
		},
		Issues: mapBackupStoreIssues(status.Issues),
	}
}

func mapBackupStoreManifest(item backupstore.ManifestSummary) BackupStoreManifestItem {
	return BackupStoreManifestItem{
		BackupID:           item.BackupID,
		CreatedAt:          item.CreatedAt,
		TargetPath:         item.TargetPath,
		SourceOperation:    item.SourceOperation,
		ObjectDigest:       item.ObjectDigest,
		ObjectBytes:        item.ObjectBytes,
		ContentFingerprint: item.ContentFingerprint,
		Pinned:             item.Pinned,
		ManifestChecksum:   item.ManifestChecksum,
	}
}

func mapBackupStoreInspect(result backupstore.InspectResult) *BackupStoreInspectOutput {
	manifest := result.Manifest
	return &BackupStoreInspectOutput{
		BackupID:           manifest.BackupID,
		CreatedAt:          manifest.CreatedAt,
		TargetPath:         manifest.TargetPath,
		SourceOperation:    manifest.SourceOperation,
		ObjectAlgorithm:    manifest.ObjectAlgorithm,
		ObjectDigest:       manifest.ObjectDigest,
		ObjectBytes:        manifest.ObjectBytes,
		ContentFingerprint: manifest.ContentFingerprint,
		OriginalMode:       manifest.OriginalMode,
		OriginalModTime:    manifest.OriginalModTime,
		Label:              manifest.Label,
		Pinned:             manifest.Pinned,
		ManifestChecksum:   manifest.ManifestChecksum,
		ObjectVerified:     result.ObjectVerified,
	}
}

func mapBackupStoreAudit(report backupstore.AuditReport) *BackupStoreAuditOutput {
	return &BackupStoreAuditOutput{
		Mode:              report.Mode,
		Healthy:           report.Healthy,
		Generation:        report.Generation,
		ManifestCount:     report.ManifestCount,
		ObjectCount:       report.ObjectCount,
		ReferencedBytes:   report.ReferencedBytes,
		OrphanObjectCount: report.OrphanObjectCount,
		OrphanObjectBytes: report.OrphanObjectBytes,
		StagingEntryCount: report.StagingEntryCount,
		StagingEntryBytes: report.StagingEntryBytes,
		TrashEntryCount:   report.TrashEntryCount,
		TrashEntryBytes:   report.TrashEntryBytes,
		IndexConsistent:   report.IndexConsistent,
		Issues:            mapBackupStoreIssues(report.Issues),
	}
}

func mapBackupStoreIssues(issues []backupstore.AuditIssue) []BackupStoreAuditIssue {
	if len(issues) == 0 {
		return nil
	}
	mapped := make([]BackupStoreAuditIssue, len(issues))
	for index, issue := range issues {
		mapped[index] = BackupStoreAuditIssue{Code: issue.Code, Message: issue.Message}
	}
	return mapped
}
