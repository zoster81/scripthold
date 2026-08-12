package handler

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
)

// handleBackupStoreStatusRead uses only the read-only backup management
// authority. It intentionally does not share a dispatcher with restore or GC
// application.
func (h *Handler) handleBackupStoreStatusRead(ctx context.Context) (*mcp.CallToolResult, BackupStoreOutput, error) {
	defaultPolicy := ""
	if h.config != nil {
		defaultPolicy = h.config.Backup.DefaultPolicy
	}
	if h.backupStore == nil {
		output := BackupStoreOutput{
			Action:  BackupStoreActionStatus,
			Enabled: false,
			State:   BackupStoreStateDisabled,
			Status:  &BackupStoreStatusOutput{DefaultPolicy: defaultPolicy},
		}
		return h.finishBackupStoreOutput(output, "Persistent backup store is disabled.")
	}
	status, err := h.backupStore.Status(ctx)
	if err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	state := BackupStoreStateReady
	if !status.Healthy {
		state = BackupStoreStateDegraded
	}
	mappedStatus := mapBackupStoreStatus(status)
	mappedStatus.DefaultPolicy = defaultPolicy
	output := BackupStoreOutput{
		Action:  BackupStoreActionStatus,
		Enabled: true,
		State:   state,
		Status:  mappedStatus,
	}
	return h.finishBackupStoreOutput(output, fmt.Sprintf("Backup store status: %s.", state))
}

func (h *Handler) handleBackupStoreListRead(ctx context.Context, input BackupStoreReadInput, visibility backupVisibilitySnapshot) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupStore == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store is not configured")), BackupStoreOutput{}, nil
	}
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
		Action:     BackupStoreActionList,
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
}

func (h *Handler) handleBackupStoreInspectRead(ctx context.Context, input BackupStoreReadInput, visibility backupVisibilitySnapshot) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupStore == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store is not configured")), BackupStoreOutput{}, nil
	}
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
		Action:   BackupStoreActionInspect,
		Enabled:  true,
		State:    BackupStoreStateReady,
		Manifest: mapBackupStoreInspect(inspected),
	}
	return h.finishBackupStoreOutput(output, "Backup metadata and object integrity verified.")
}

func (h *Handler) handleBackupStoreAuditRead(ctx context.Context, input BackupStoreReadInput) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if h.backupStore == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store is not configured")), BackupStoreOutput{}, nil
	}
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
		Action:  BackupStoreActionAudit,
		Enabled: true,
		State:   state,
		Audit:   mapBackupStoreAudit(audit),
	}
	return h.finishBackupStoreOutput(output, fmt.Sprintf("Backup store %s audit completed: %s.", audit.Mode, state))
}
