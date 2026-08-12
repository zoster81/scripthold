package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	manageBOMActionDetect       = "detect"
	manageBOMActionAddPreview   = "addPreview"
	manageBOMActionStripPreview = "stripPreview"
)

type ManageBOMReadInput struct {
	Path         string `json:"path"`
	Action       string `json:"action"`
	Encoding     string `json:"encoding,omitempty"`
	BackupPolicy string `json:"backupPolicy,omitempty"`
}

func (input *ManageBOMReadInput) UnmarshalJSON(data []byte) error {
	type alias ManageBOMReadInput
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*input = ManageBOMReadInput(decoded)
	return nil
}

func (h *Handler) HandleManageBOMRead(ctx context.Context, _ *mcp.CallToolRequest, input ManageBOMReadInput) (*mcp.CallToolResult, ManageBomOutput, error) {
	switch strings.TrimSpace(input.Action) {
	case manageBOMActionDetect:
		if input.Encoding != "" || input.BackupPolicy != "" {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "detect accepts only path and action")), ManageBomOutput{}, nil
		}
		return h.handleManageBOMDetect(input.Path)
	case manageBOMActionAddPreview, manageBOMActionStripPreview:
		return h.handleManageBOMPreview(ctx, input)
	default:
		return errorResultFromError(operation.New(operation.KindInvalidInput, "action must be detect, addPreview, or stripPreview")), ManageBomOutput{}, nil
	}
}

func (h *Handler) handleManageBOMDetect(requestedPath string) (*mcp.CallToolResult, ManageBomOutput, error) {
	validation := h.ValidatePath(requestedPath)
	if !validation.Ok() {
		return validation.Result, ManageBomOutput{}, nil
	}
	session, err := filesystem.OpenReadSession(validation.Path)
	if err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}
	defer session.Close()
	detected, hasBOM, err := detectBOMPrefix(session)
	if err != nil {
		return errorResultFromError(operation.WrapFilesystem("detect_bom", validation.Path, err)), ManageBomOutput{}, nil
	}
	output := ManageBomOutput{Action: manageBOMActionDetect, TargetPath: requestedPath, HasBOM: hasBOM}
	if !hasBOM {
		output.Message = "No BOM detected"
		return &mcp.CallToolResult{}, output, nil
	}
	output.BOMType = detected.Charset
	output.BOMBytes = fileEncoding.BOMSize(detected.Charset)
	output.Message = fmt.Sprintf("BOM detected: %s (%d bytes)", output.BOMType, output.BOMBytes)
	return &mcp.CallToolResult{}, output, nil
}

func (h *Handler) handleManageBOMPreview(ctx context.Context, input ManageBOMReadInput) (*mcp.CallToolResult, ManageBomOutput, error) {
	backupPolicy, err := h.effectivePersistentBackupPolicy(input.BackupPolicy)
	if err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}
	target, err := h.readExactMutationTarget(ctx, input.Path)
	if err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}
	prepared := preparedByteMutation{
		kind:         byteMutationKindBOM,
		action:       strings.TrimSpace(input.Action),
		backupPolicy: backupPolicy,
		targets:      []preparedByteMutationTarget{target},
	}
	keepPrepared := false
	defer func() {
		if !keepPrepared {
			prepared.close()
		}
	}()

	current := prepared.targets[0].data
	detected, hasBOM := fileEncoding.DetectBOM(current)
	bomSize := 0
	if hasBOM {
		bomSize = fileEncoding.BOMSize(detected.Charset)
	}
	result := append([]byte(nil), current...)
	resultHasBOM := hasBOM
	resultBOMType := ""
	resultBOMBytes := 0
	if hasBOM {
		resultBOMType = detected.Charset
		resultBOMBytes = bomSize
	}

	switch prepared.action {
	case manageBOMActionAddPreview:
		if strings.TrimSpace(input.Encoding) == "" {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "encoding is required for addPreview")), ManageBomOutput{}, nil
		}
		bom := fileEncoding.BOMBytesFor(input.Encoding)
		if len(bom) == 0 {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "encoding must be utf-8, utf-16-le, utf-16-be, utf-32-le, or utf-32-be")), ManageBomOutput{}, nil
		}
		if hasBOM {
			return errorResultFromError(operation.New(operation.KindConflict, fmt.Sprintf("file already has a %s BOM", detected.Charset))), ManageBomOutput{}, nil
		}
		if int64(len(current)) > h.maxFileBytes()-int64(len(bom)) {
			return errorResultFromError(operation.New(operation.KindLimit, "BOM result exceeds the configured file limit")), ManageBomOutput{}, nil
		}
		result = make([]byte, 0, len(bom)+len(current))
		result = append(result, bom...)
		result = append(result, current...)
		resultHasBOM = true
		resultBOMType = canonicalBOMEncoding(input.Encoding)
		resultBOMBytes = len(bom)
	case manageBOMActionStripPreview:
		if input.Encoding != "" {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "encoding is not accepted for stripPreview")), ManageBomOutput{}, nil
		}
		if hasBOM {
			result = append([]byte(nil), current[bomSize:]...)
			resultHasBOM = false
			resultBOMType = detected.Charset
			resultBOMBytes = bomSize
		}
	}

	prepared.targets[0].data = result
	prepared.targets[0].resultFingerprint = filesystem.FingerprintRegularFileData(result)
	prepared.targets[0].changed = prepared.targets[0].resultFingerprint != prepared.targets[0].targetFingerprint
	prepared.targets[0].hasBOM = resultHasBOM
	prepared.targets[0].bomType = resultBOMType
	if err := h.preflightRequiredMutationBackups(ctx, &prepared, backupstore.SourceOperationManageBOM); err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}

	preview, err := h.byteMutationPreviews.put(prepared)
	if err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}
	keepPrepared = true
	output := ManageBomOutput{
		Message:           "BOM mutation preview prepared",
		Action:            prepared.action,
		PreviewID:         preview.id,
		CreatedAt:         preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt:         preview.expiresAt.Format(timeRFC3339Nano),
		TargetPath:        input.Path,
		TargetFingerprint: prepared.targets[0].targetFingerprint,
		ResultFingerprint: prepared.targets[0].resultFingerprint,
		BackupPolicy:      backupPolicy,
		HasBOM:            resultHasBOM,
		BOMType:           resultBOMType,
		BOMBytes:          resultBOMBytes,
		Changed:           prepared.targets[0].changed,
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("BOM preview prepared.\nPreview ID: %s\nTarget fingerprint: %s\nResult fingerprint: %s", output.PreviewID, output.TargetFingerprint, output.ResultFingerprint)}}}, output, nil
}

func (h *Handler) HandleManageBOMApply(ctx context.Context, _ *mcp.CallToolRequest, input PreviewApplyInput) (*mcp.CallToolResult, ManageBomOutput, error) {
	if !validByteMutationPreviewID(input.PreviewID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")), ManageBomOutput{}, nil
	}
	preview, err := h.byteMutationPreviews.claim(input.PreviewID, byteMutationKindBOM)
	if err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}
	prepared := &preview.prepared
	defer prepared.close()
	target := &prepared.targets[0]
	output := ManageBomOutput{
		Message:           "BOM mutation capability consumed",
		Action:            prepared.action,
		TargetPath:        target.requestedPath,
		TargetFingerprint: target.targetFingerprint,
		ResultFingerprint: target.resultFingerprint,
		BackupPolicy:      prepared.backupPolicy,
		HasBOM:            target.hasBOM,
		BOMType:           target.bomType,
		Changed:           target.changed,
	}
	if filesystem.FingerprintRegularFileData(target.data) != target.resultFingerprint {
		return errorResultWithCode(ErrCodeConflict, "prepared BOM result no longer matches its fingerprint"), output, nil
	}
	current, err := h.revalidateExactMutationTarget(ctx, target)
	if err != nil {
		return errorResultFromError(err), output, nil
	}
	if !target.changed {
		output.Message = "BOM mutation is a no-op; no backup or write was performed"
		return &mcp.CallToolResult{}, output, nil
	}
	if prepared.backupPolicy == editBackupPolicyRequired {
		output.BackupID, err = h.captureRequiredMutationBackup(ctx, target, backupstore.SourceOperationManageBOM)
		if err != nil {
			return errorResultFromError(err), output, nil
		}
		current, err = h.revalidateExactMutationTarget(ctx, target)
		if err != nil {
			return errorResultFromError(err), output, nil
		}
	}
	if err := target.identityFile.Close(); err != nil {
		return errorResultFromError(operation.WrapFilesystem("close_bom_preview_identity", target.resolvedPath, err)), output, nil
	}
	target.identityFile = nil
	if err := h.replaceFile(target.resolvedPath, target.data, filesystem.ReplaceOptions{Mode: current.Mode.Perm(), Expected: &current}); err != nil {
		return errorResultFromError(err), output, nil
	}
	post, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, target.resolvedPath, h.maxFileBytes())
	if err != nil {
		return errorResultFromError(err), output, nil
	}
	actual, err := filesystem.FingerprintRegularFileSnapshot(post)
	if err != nil {
		return errorResultFromError(err), output, nil
	}
	if actual != target.resultFingerprint {
		return errorResultWithCode(ErrCodeConflict, "applied BOM result does not match the prepared fingerprint"), output, nil
	}
	output.Applied = true
	output.Message = "BOM mutation applied"
	return &mcp.CallToolResult{}, output, nil
}
