package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
)

// HandleBackupDelete performs one explicit backup deletion. Pinned manifests
// are intentionally accepted here because automatic retention and GC never
// select them; this dedicated destructive tool is the explicit override.
func (h *Handler) HandleBackupDelete(ctx context.Context, _ *mcp.CallToolRequest, input BackupDeleteInput) (*mcp.CallToolResult, BackupDeleteOutput, error) {
	if !validRestorePreviewID(input.BackupID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backupId must be 64 hexadecimal characters")), BackupDeleteOutput{}, nil
	}
	if h.backupStore == nil || h.backupDeleter == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store does not provide explicit deletion authority")), BackupDeleteOutput{}, nil
	}
	visibility := h.backupVisibilitySnapshot()
	inspected, err := h.backupStore.Inspect(ctx, input.BackupID, backupstore.InspectOptions{})
	if err != nil {
		return errorResultFromError(err), BackupDeleteOutput{}, nil
	}
	if _, err := visibility.validate(inspected.Manifest.TargetPath); err != nil {
		return errorResultFromError(err), BackupDeleteOutput{}, nil
	}
	worst := BackupDeleteOutput{
		BackupID:        input.BackupID,
		Pinned:          false,
		ManifestRemoved: false,
		ObjectRemoved:   false,
		BytesReclaimed:  inspected.Manifest.ObjectBytes,
		Generation:      strings.Repeat("f", 64),
	}
	if err := h.checkBackupDeleteOutputLimit(worst, backupDeleteText(worst)); err != nil {
		return errorResultFromError(err), BackupDeleteOutput{}, nil
	}

	deleted, err := h.backupDeleter.DeleteBackup(ctx, input.BackupID)
	output := BackupDeleteOutput{
		BackupID:        deleted.BackupID,
		Pinned:          deleted.Pinned,
		ManifestRemoved: deleted.ManifestRemoved,
		ObjectRemoved:   deleted.ObjectRemoved,
		BytesReclaimed:  deleted.BytesReclaimed,
		Generation:      deleted.Generation,
	}
	if err != nil {
		return errorResultFromError(err), output, nil
	}
	text := backupDeleteText(output)
	if limitErr := h.checkBackupDeleteOutputLimit(output, text); limitErr != nil {
		// Preflight above makes this defensive path unreachable for a stable
		// response schema, but preserve real durable evidence if it ever fires.
		return errorResultFromError(limitErr), output, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func backupDeleteText(output BackupDeleteOutput) string {
	return fmt.Sprintf("Backup explicitly deleted.\nBackup ID: %s\nPinned: %t\nManifest removed: %t\nObject removed: %t\nBytes reclaimed: %d",
		output.BackupID, output.Pinned, output.ManifestRemoved, output.ObjectRemoved, output.BytesReclaimed)
}

func (h *Handler) checkBackupDeleteOutputLimit(output BackupDeleteOutput, text string) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindFilesystem, "encode_backup_delete_output", "", err)
	}
	if int64(len(encoded))+int64(len(text)) > h.maxOutputBytes() {
		return operation.New(operation.KindLimit, fmt.Sprintf("backup delete output exceeds limit %d bytes", h.maxOutputBytes()))
	}
	return nil
}
