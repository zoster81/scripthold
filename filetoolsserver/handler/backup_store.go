package handler

import (
	"context"
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

// HandleBackupStore is retained only as a package-level compatibility bridge
// for pre-R23 management/restore/GC regression coverage. It is not registered
// as an MCP tool.
// Deprecated: MCP callers use HandleBackupStoreRead plus dedicated apply handlers.
func (h *Handler) HandleBackupStore(ctx context.Context, _ *mcp.CallToolRequest, input BackupStoreInput) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if err := validateBackupStoreInput(input); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	if h.backupStore == nil {
		if input.Action != BackupStoreActionStatus {
			err := operation.New(operation.KindInvalidInput, "backup store is not configured")
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		output := BackupStoreOutput{
			Action:  BackupStoreActionStatus,
			Enabled: false,
			State:   BackupStoreStateDisabled,
		}
		return h.finishBackupStoreOutput(output, "Persistent backup store is disabled.")
	}

	visibility := h.backupVisibilitySnapshot()
	switch input.Action {
	case BackupStoreActionStatus:
		status, err := h.backupStore.Status(ctx)
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		state := BackupStoreStateReady
		if !status.Healthy {
			state = BackupStoreStateDegraded
		}
		mappedStatus := mapBackupStoreStatus(status)
		if h.config != nil {
			mappedStatus.DefaultPolicy = h.config.Backup.DefaultPolicy
		}
		output := BackupStoreOutput{
			Action:  input.Action,
			Enabled: true,
			State:   state,
			Status:  mappedStatus,
		}
		return h.finishBackupStoreOutput(output, fmt.Sprintf("Backup store status: %s.", state))

	case BackupStoreActionList:
		validatedTarget := ""
		if input.TargetPath != "" {
			var err error
			validatedTarget, err = visibility.validate(input.TargetPath)
			if err != nil {
				return errorResultFromError(err), BackupStoreOutput{}, nil
			}
		}
		visibilityCache := make(map[string]bool)
		listed, err := h.backupStore.List(ctx, backupstore.ListOptions{
			Cursor:          input.Cursor,
			Limit:           input.Limit,
			TargetPath:      validatedTarget,
			Pinned:          input.Pinned,
			VisibilityScope: visibility.scope,
			TargetVisible: func(path string) bool {
				if visible, exists := visibilityCache[path]; exists {
					return visible
				}
				_, visibilityErr := visibility.validate(path)
				visible := visibilityErr == nil
				visibilityCache[path] = visible
				return visible
			},
		})
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		output := BackupStoreOutput{
			Action:     input.Action,
			Enabled:    true,
			State:      BackupStoreStateReady,
			Generation: listed.Generation,
			NextCursor: listed.NextCursor,
			Items:      make([]BackupStoreManifestItem, len(listed.Items)),
		}
		for index, item := range listed.Items {
			output.Items[index] = mapBackupStoreManifest(item)
		}
		return h.finishBackupStoreOutput(output, fmt.Sprintf("Listed %d backup records.", len(output.Items)))

	case BackupStoreActionInspect:
		validatedTarget := ""
		inspected, err := h.backupStore.Inspect(ctx, input.BackupID, backupstore.InspectOptions{
			AuthorizeTarget: func(path string) error {
				var authorizationErr error
				validatedTarget, authorizationErr = visibility.validate(path)
				return authorizationErr
			},
		})
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		inspected.Manifest.TargetPath = validatedTarget
		output := BackupStoreOutput{
			Action:   input.Action,
			Enabled:  true,
			State:    BackupStoreStateReady,
			Manifest: mapBackupStoreInspect(inspected),
		}
		return h.finishBackupStoreOutput(output, "Backup metadata and object integrity verified.")

	case BackupStoreActionRestorePreview:
		return h.handleBackupStoreRestorePreview(ctx, input.BackupID, visibility)

	case BackupStoreActionRestoreApply:
		return h.handleBackupStoreRestoreApply(ctx, input.PreviewID)

	case BackupStoreActionGCDryRun:
		return h.handleBackupStoreGCDryRun(ctx)

	case BackupStoreActionGCApply:
		return h.handleBackupStoreGCApply(ctx, input.PreviewID)

	case BackupStoreActionAudit:
		audit, err := h.backupStore.Audit(ctx, backupstore.AuditOptions{
			Mode:       backupstore.AuditMode(input.AuditMode),
			MaxObjects: input.MaxObjects,
			MaxBytes:   input.MaxBytes,
		})
		if err != nil {
			return errorResultFromError(err), BackupStoreOutput{}, nil
		}
		state := BackupStoreStateReady
		if !audit.Healthy {
			state = BackupStoreStateDegraded
		}
		output := BackupStoreOutput{
			Action:  input.Action,
			Enabled: true,
			State:   state,
			Audit:   mapBackupStoreAudit(audit),
		}
		return h.finishBackupStoreOutput(output, fmt.Sprintf("Backup store %s audit completed: %s.", audit.Mode, state))
	}

	return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store action is invalid")), BackupStoreOutput{}, nil
}

func validateBackupStoreInput(input BackupStoreInput) error {
	hasListFields := input.Cursor != "" || input.Limit != 0 || input.TargetPath != "" || input.Pinned != nil
	hasInspectFields := input.BackupID != ""
	hasPreviewFields := input.PreviewID != ""
	hasAuditFields := input.AuditMode != "" || input.MaxObjects != 0 || input.MaxBytes != 0

	switch input.Action {
	case BackupStoreActionStatus:
		if hasListFields || hasInspectFields || hasPreviewFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "status accepts only action")
		}
	case BackupStoreActionList:
		if hasInspectFields || hasPreviewFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "list accepts only cursor, limit, targetPath, and pinned")
		}
	case BackupStoreActionInspect:
		if input.BackupID == "" {
			return operation.New(operation.KindInvalidInput, "inspect requires backupId")
		}
		if hasListFields || hasPreviewFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "inspect accepts only backupId")
		}
	case BackupStoreActionRestorePreview:
		if input.BackupID == "" {
			return operation.New(operation.KindInvalidInput, "restorePreview requires backupId")
		}
		if hasListFields || hasPreviewFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "restorePreview accepts only backupId")
		}
	case BackupStoreActionRestoreApply:
		if !validRestorePreviewID(input.PreviewID) {
			return operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
		}
		if hasListFields || hasInspectFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "restoreApply accepts only previewId")
		}
	case BackupStoreActionGCDryRun:
		if hasListFields || hasInspectFields || hasPreviewFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "gcDryRun accepts only action")
		}
	case BackupStoreActionGCApply:
		if !validGCPreviewID(input.PreviewID) {
			return operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")
		}
		if hasListFields || hasInspectFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "gcApply accepts only previewId")
		}
	case BackupStoreActionAudit:
		if hasListFields || hasInspectFields || hasPreviewFields {
			return operation.New(operation.KindInvalidInput, "audit accepts only auditMode, maxObjects, and maxBytes")
		}
		if input.AuditMode != "" && input.AuditMode != string(backupstore.AuditQuick) && input.AuditMode != string(backupstore.AuditFull) {
			return operation.New(operation.KindInvalidInput, "auditMode must be quick or full")
		}
	default:
		return operation.New(operation.KindInvalidInput, "action must be status, list, inspect, audit, restorePreview, restoreApply, gcDryRun, or gcApply")
	}
	return nil
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
