package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/operation"
)

// PreviewApplyInput is the complete public input for every R23 mutating apply
// tool. Unknown fields are rejected so an apply call cannot alter the approved
// operation after preview.
type PreviewApplyInput struct {
	PreviewID string `json:"previewId"`
}

func (input *PreviewApplyInput) UnmarshalJSON(data []byte) error {
	type alias PreviewApplyInput
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*input = PreviewApplyInput(decoded)
	return nil
}

// EditFilePreviewInput contains only approval-preparation fields. It cannot
// express direct edit or apply.
type EditFilePreviewInput struct {
	Action        string          `json:"action"`
	Path          string          `json:"path"`
	Edits         []EditOperation `json:"edits,omitempty"`
	Patch         string          `json:"patch,omitempty"`
	Encoding      string          `json:"encoding,omitempty"`
	ForceWritable *bool           `json:"forceWritable,omitempty"`
	BackupPolicy  string          `json:"backupPolicy,omitempty"`
}

func (h *Handler) HandleEditFilePreview(ctx context.Context, _ *mcp.CallToolRequest, input EditFilePreviewInput) (*mcp.CallToolResult, EditFileOutput, error) {
	if strings.TrimSpace(input.Action) != editActionPreview {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "edit_file action must be preview")), EditFileOutput{}, nil
	}
	return h.handleEditPreview(ctx, EditFileInput{
		Action:        editActionPreview,
		Path:          input.Path,
		Edits:         input.Edits,
		Patch:         input.Patch,
		Encoding:      input.Encoding,
		ForceWritable: input.ForceWritable,
		BackupPolicy:  input.BackupPolicy,
	})
}

func (h *Handler) HandleEditFileApply(ctx context.Context, _ *mcp.CallToolRequest, input PreviewApplyInput) (*mcp.CallToolResult, EditFileOutput, error) {
	if !validEditPreviewID(input.PreviewID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")), EditFileOutput{}, nil
	}
	return h.handleEditApply(ctx, input.PreviewID)
}

// PatchPackageReadInput deliberately omits previewId. The public patch_package
// tool can inspect, prepare, or verify, but cannot apply.
type PatchPackageReadInput struct {
	Action   string               `json:"action"`
	Manifest PatchPackageManifest `json:"manifest"`
}

func (input *PatchPackageReadInput) UnmarshalJSON(data []byte) error {
	type alias PatchPackageReadInput
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*input = PatchPackageReadInput(decoded)
	return nil
}

func (h *Handler) HandlePatchPackageRead(ctx context.Context, _ *mcp.CallToolRequest, input PatchPackageReadInput) (*mcp.CallToolResult, PatchPackageOutput, error) {
	action := strings.TrimSpace(input.Action)
	if action != patchPackageActionInspect && action != patchPackageActionDryRun && action != patchPackageActionVerify {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "patch_package action must be inspect, dryRun, or verify")), PatchPackageOutput{}, nil
	}
	targets, err := h.validatePatchPackageManifest(ctx, input.Manifest)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	switch action {
	case patchPackageActionInspect:
		return h.handlePatchPackageInspect(input.Manifest, targets)
	case patchPackageActionVerify:
		return h.handlePatchPackageVerify(ctx, input.Manifest, targets)
	default:
		return h.handlePatchPackageDryRun(ctx, input.Manifest, targets)
	}
}

func (h *Handler) HandlePatchPackageApply(ctx context.Context, _ *mcp.CallToolRequest, input PreviewApplyInput) (*mcp.CallToolResult, PatchPackageOutput, error) {
	if !validPatchPackagePreviewID(input.PreviewID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")), PatchPackageOutput{}, nil
	}
	return h.handlePatchPackageApply(ctx, input.PreviewID)
}

// BackupStoreReadInput contains only read/preview fields. Restore and GC apply
// are separate tools and therefore cannot be selected through this union.
type BackupStoreReadInput struct {
	Action        string `json:"action"`
	Cursor        string `json:"cursor,omitempty"`
	Limit         int    `json:"limit,omitempty"`
	TargetPath    string `json:"targetPath,omitempty"`
	Pinned        *bool  `json:"pinned,omitempty"`
	BackupID      string `json:"backupId,omitempty"`
	OtherBackupID string `json:"otherBackupId,omitempty"`
	AuditMode     string `json:"auditMode,omitempty"`
	MaxObjects    int    `json:"maxObjects,omitempty"`
	MaxBytes      int64  `json:"maxBytes,omitempty"`
}

func (input *BackupStoreReadInput) UnmarshalJSON(data []byte) error {
	type alias BackupStoreReadInput
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*input = BackupStoreReadInput(decoded)
	return nil
}

func (h *Handler) HandleBackupStoreRead(ctx context.Context, _ *mcp.CallToolRequest, input BackupStoreReadInput) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if err := validateBackupStoreReadInput(input); err != nil {
		return errorResultFromError(err), BackupStoreOutput{}, nil
	}
	visibility := h.backupVisibilitySnapshot()
	switch input.Action {
	case BackupStoreActionStatus:
		return h.handleBackupStoreStatusRead(ctx)
	case BackupStoreActionList:
		return h.handleBackupStoreListRead(ctx, input, visibility)
	case BackupStoreActionHistory:
		if h.backupStore == nil {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store is not configured")), BackupStoreOutput{}, nil
		}
		return h.handleBackupStoreHistory(ctx, input, visibility)
	case BackupStoreActionInspect:
		return h.handleBackupStoreInspectRead(ctx, input, visibility)
	case BackupStoreActionCompare:
		if h.backupStore == nil {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store is not configured")), BackupStoreOutput{}, nil
		}
		return h.handleBackupStoreCompare(ctx, input, visibility)
	case BackupStoreActionAudit:
		return h.handleBackupStoreAuditRead(ctx, input)
	case BackupStoreActionRestorePreview:
		if h.backupStore == nil {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store is not configured")), BackupStoreOutput{}, nil
		}
		return h.handleBackupStoreRestorePreview(ctx, input.BackupID, visibility)
	case BackupStoreActionGCDryRun:
		if h.backupStore == nil {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store is not configured")), BackupStoreOutput{}, nil
		}
		return h.handleBackupStoreGCDryRun(ctx)
	default:
		return errorResultFromError(operation.New(operation.KindInvalidInput, "backup_store action is not read-only")), BackupStoreOutput{}, nil
	}
}

func validateBackupStoreReadInput(input BackupStoreReadInput) error {
	hasListFields := input.Cursor != "" || input.Limit != 0 || input.TargetPath != "" || input.Pinned != nil
	hasBackupFields := input.BackupID != "" || input.OtherBackupID != ""
	hasAuditFields := input.AuditMode != "" || input.MaxObjects != 0 || input.MaxBytes != 0
	switch input.Action {
	case BackupStoreActionStatus, BackupStoreActionGCDryRun:
		if hasListFields || hasBackupFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, input.Action+" accepts only action")
		}
	case BackupStoreActionList:
		if hasBackupFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "list accepts only cursor, limit, targetPath, and pinned")
		}
	case BackupStoreActionHistory:
		if input.TargetPath == "" || hasBackupFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "history requires targetPath and accepts only cursor, limit, targetPath, and pinned")
		}
	case BackupStoreActionInspect, BackupStoreActionRestorePreview:
		if input.BackupID == "" || input.OtherBackupID != "" || hasListFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, input.Action+" requires backupId and accepts no other fields")
		}
	case BackupStoreActionCompare:
		if input.BackupID == "" || hasListFields || hasAuditFields {
			return operation.New(operation.KindInvalidInput, "compare requires backupId and accepts optional otherBackupId")
		}
	case BackupStoreActionAudit:
		if hasListFields || hasBackupFields {
			return operation.New(operation.KindInvalidInput, "audit accepts only auditMode, maxObjects, and maxBytes")
		}
		if input.AuditMode != "" && input.AuditMode != string(backupstore.AuditQuick) && input.AuditMode != string(backupstore.AuditFull) {
			return operation.New(operation.KindInvalidInput, "auditMode must be quick or full")
		}
	default:
		return operation.New(operation.KindInvalidInput, "backup_store action must be status, list, history, inspect, compare, audit, restorePreview, or gcDryRun")
	}
	return nil
}

func (h *Handler) HandleBackupRestoreApply(ctx context.Context, _ *mcp.CallToolRequest, input PreviewApplyInput) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if !validRestorePreviewID(input.PreviewID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")), BackupStoreOutput{}, nil
	}
	return h.handleBackupStoreRestoreApply(ctx, input.PreviewID)
}

func (h *Handler) HandleBackupGCApply(ctx context.Context, _ *mcp.CallToolRequest, input PreviewApplyInput) (*mcp.CallToolResult, BackupStoreOutput, error) {
	if !validGCPreviewID(input.PreviewID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")), BackupStoreOutput{}, nil
	}
	return h.handleBackupStoreGCApply(ctx, input.PreviewID)
}
