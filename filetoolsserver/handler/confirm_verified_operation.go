package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/operation"
)

const maxConfirmationNoteBytes = 256

// ConfirmVerifiedOperationInput is the closed request for a prepared mutation.
type ConfirmVerifiedOperationInput struct {
	PreviewID string `json:"previewId"`
	Note      string `json:"note,omitempty"`
}

func (input *ConfirmVerifiedOperationInput) UnmarshalJSON(data []byte) error {
	type alias ConfirmVerifiedOperationInput
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("confirmation request must contain exactly one object")
	}
	if !validConfirmationOwnerID(decoded.PreviewID) {
		return errors.New("previewId must be 64 lowercase hexadecimal characters")
	}
	if len(decoded.Note) > maxConfirmationNoteBytes || !utf8.ValidString(decoded.Note) {
		return errors.New("note must be valid UTF-8 and at most 256 bytes")
	}
	*input = ConfirmVerifiedOperationInput(decoded)
	return nil
}

// HandleConfirmVerifiedOperation looks up an owner but never claims a preview.
// The original operation-specific store performs the one-shot atomic claim.
func (h *Handler) HandleConfirmVerifiedOperation(ctx context.Context, req *mcp.CallToolRequest, input ConfirmVerifiedOperationInput) (*mcp.CallToolResult, map[string]any, error) {
	if !validConfirmationOwnerID(input.PreviewID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "previewId must be 64 lowercase hexadecimal characters")), nil, nil
	}
	if len(input.Note) > maxConfirmationNoteBytes || !utf8.ValidString(input.Note) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "note exceeds 256 UTF-8 bytes")), nil, nil
	}
	owner, ok := h.confirmOwners.resolve(input.PreviewID)
	if !ok {
		return errorResultFromError(operation.New(operation.KindConflict, "prepared operation is unavailable, expired, evicted, or already consumed")), nil, nil
	}
	previewInput := PreviewApplyInput{PreviewID: input.PreviewID}
	switch owner {
	case "edit_file":
		result, output, err := h.HandleEditFileApply(ctx, req, previewInput)
		return confirmationResult("edit_file", result, output, err)
	case "patch_package":
		result, output, err := h.HandlePatchPackageApply(ctx, req, previewInput)
		return confirmationResult("patch_package", result, output, err)
	case "filesystem_package":
		result, output, err := h.HandleFilesystemPackageApply(ctx, req, previewInput)
		return confirmationResult("filesystem_package", result, output, err)
	case "markdown":
		result, output, err := h.HandleMarkdownApply(ctx, req, MarkdownApplyInput{PreviewID: input.PreviewID})
		return confirmationResult("markdown", result, output, err)
	case "convert_encoding":
		result, output, err := h.HandleConvertEncodingApply(ctx, req, previewInput)
		return confirmationResult("convert_encoding", result, output, err)
	case "manage_bom":
		result, output, err := h.HandleManageBOMApply(ctx, req, previewInput)
		return confirmationResult("manage_bom", result, output, err)
	case "backup_restore":
		result, output, err := h.HandleBackupRestoreApply(ctx, req, previewInput)
		return confirmationResult("backup_restore", result, output, err)
	case "backup_gc":
		result, output, err := h.HandleBackupGCApply(ctx, req, previewInput)
		return confirmationResult("backup_gc", result, output, err)
	default:
		return errorResultFromError(operation.New(operation.KindConflict, "prepared operation owner is unavailable")), nil, nil
	}
}

func confirmationResult[T any](kind string, result *mcp.CallToolResult, output T, callErr error) (*mcp.CallToolResult, map[string]any, error) {
	// Preserve the owning engine's JSON structure and MCP error metadata.
	projected := map[string]any{"operationKind": kind}
	encoded, err := json.Marshal(output)
	if err != nil {
		return result, projected, errors.Join(callErr, err)
	}
	var fields map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&fields); err != nil {
		return result, projected, errors.Join(callErr, err)
	}
	for key, value := range fields {
		projected[key] = value
	}
	return result, projected, callErr
}
