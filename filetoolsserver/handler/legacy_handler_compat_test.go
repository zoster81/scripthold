package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

// Test-only compatibility adapters preserve regression coverage for the
// pre-preview/apply handler surface without shipping those entry points in the
// production package.
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

	switch input.Action {
	case BackupStoreActionStatus,
		BackupStoreActionList,
		BackupStoreActionInspect,
		BackupStoreActionRestorePreview,
		BackupStoreActionGCDryRun,
		BackupStoreActionAudit:
		return h.HandleBackupStoreRead(ctx, nil, backupStoreReadInputFromLegacy(input))
	case BackupStoreActionRestoreApply:
		return h.handleBackupStoreRestoreApply(ctx, input.PreviewID)
	case BackupStoreActionGCApply:
		return h.handleBackupStoreGCApply(ctx, input.PreviewID)
	}

	return errorResultFromError(operation.New(operation.KindInvalidInput, "backup store action is invalid")), BackupStoreOutput{}, nil
}

func backupStoreReadInputFromLegacy(input BackupStoreInput) BackupStoreReadInput {
	return BackupStoreReadInput{
		Action:     input.Action,
		Cursor:     input.Cursor,
		Limit:      input.Limit,
		TargetPath: input.TargetPath,
		Pinned:     input.Pinned,
		BackupID:   input.BackupID,
		AuditMode:  input.AuditMode,
		MaxObjects: input.MaxObjects,
		MaxBytes:   input.MaxBytes,
	}
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

func (h *Handler) HandleConvertEncoding(ctx context.Context, _ *mcp.CallToolRequest, input ConvertEncodingInput) (*mcp.CallToolResult, ConvertEncodingOutput, error) {
	if strings.TrimSpace(input.To) == "" {
		return errorResult("target encoding (to) is required"), ConvertEncodingOutput{}, nil
	}
	targetEncoding, ok := fileEncoding.CanonicalName(input.To)
	if !ok {
		return errorResult(fmt.Sprintf("unsupported target encoding: %s. Use list_encodings to see available encodings.", input.To)), ConvertEncodingOutput{}, nil
	}
	policy, err := parseBOMPolicy(input.BOM, bomAuto)
	if err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}

	if input.Path != "" && len(input.Paths) > 0 {
		return errorResult("path and paths are mutually exclusive"), ConvertEncodingOutput{}, nil
	}
	if input.Path == "" && len(input.Paths) == 0 {
		return errorResult("path or paths is required"), ConvertEncodingOutput{}, nil
	}
	if len(input.Paths) > h.maxBatchFiles() {
		return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("paths contains %d files; configured limit is %d", len(input.Paths), h.maxBatchFiles())), ConvertEncodingOutput{}, nil
	}
	if len(input.Paths) > 0 {
		if err := h.validateConversionBatchPaths(input.Paths, input.Backup && !input.DryRun); err != nil {
			return errorResult("invalid conversion batch: " + err.Error()), ConvertEncodingOutput{}, nil
		}
	}

	if input.Path != "" {
		result, convertErr := h.convertEncodingPath(ctx, input.Path, input, targetEncoding, policy)
		if convertErr != nil {
			return errorResultFromError(convertErr), ConvertEncodingOutput{}, nil
		}
		if result.UnsupportedCount > 0 {
			return errorResultWithCode(ErrCodeEncoding, formatUnsupportedError(result)), ConvertEncodingOutput{}, nil
		}
		return &mcp.CallToolResult{}, flattenConvertResult(result, targetEncoding, input.DryRun), nil
	}

	output := ConvertEncodingOutput{
		Message:        fmt.Sprintf("Processed %d files for conversion to %s", len(input.Paths), targetEncoding),
		TargetEncoding: targetEncoding,
		DryRun:         input.DryRun,
		Results:        make([]ConvertFileResult, 0, len(input.Paths)),
	}
	errorSummary := newBoundedErrorSummary(h.maxOutputBytes())
	for _, path := range input.Paths {
		result, convertErr := h.convertEncodingPath(ctx, path, input, targetEncoding, policy)
		if convertErr != nil {
			mapped := mapOperationError(convertErr, path)
			result = ConvertFileResult{
				Path:              path,
				Error:             mapped.Message,
				ErrorCode:         mapped.BatchCode,
				EncodingErrorCode: encodingErrorCode(convertErr),
			}
		}
		if result.UnsupportedCount > 0 && result.Error == "" {
			result.Error = formatUnsupportedError(result)
			result.ErrorCode = ErrCodeEncoding
			result.EncodingErrorCode = EncodingErrorUnrepresentable
		}
		if result.Error != "" {
			output.ErrorCount++
			errorSummary.Add(fmt.Sprintf("%s: %s", path, result.Error))
		} else {
			output.SuccessCount++
		}
		output.Results = append(output.Results, result)
	}
	output.Errors = errorSummary.Items()
	output.ErrorsOmitted = errorSummary.Omitted()
	output.ErrorsTruncated = output.ErrorsOmitted > 0
	return &mcp.CallToolResult{}, output, nil
}

func flattenConvertResult(result ConvertFileResult, target string, dryRun bool) ConvertEncodingOutput {
	return ConvertEncodingOutput{
		Message:        result.Message,
		SourceEncoding: result.SourceEncoding,
		TargetEncoding: target,
		BackupPath:     result.BackupPath,
		BOMPolicy:      result.BOMPolicy,
		HasBOM:         result.HasBOM,
		BOMType:        result.BOMType,
		Changed:        result.Changed,
		DryRun:         dryRun,
	}
}

func (h *Handler) convertEncodingPath(ctx context.Context, requestedPath string, input ConvertEncodingInput, targetEncoding string, policy bomPolicy) (ConvertFileResult, error) {
	result := ConvertFileResult{Path: requestedPath, BOMPolicy: string(policy)}
	validated := h.ValidatePath(requestedPath)
	if !validated.Ok() {
		return result, validated.Err
	}

	preview, err := h.openDecodedTextStream(ctx, validated.Path, input.From)
	if err != nil {
		return result, err
	}
	result.SourceEncoding = preview.Charset
	targetBOM, err := documentBOMBytes(textDocument{Charset: targetEncoding, BOM: preview.BOM}, policy)
	if err != nil {
		_ = preview.Close()
		return result, err
	}
	unsupported, unsupportedCount, err := inspectUnsupportedCharacters(ctx, preview.Reader, targetEncoding)
	if err != nil {
		_ = preview.Close()
		return result, err
	}
	previewSnapshot, err := preview.Finish()
	if err != nil {
		_ = preview.Close()
		return result, err
	}
	if err := preview.Close(); err != nil {
		return result, err
	}
	result.Unsupported = unsupported
	result.UnsupportedCount = unsupportedCount
	result.HasBOM = len(targetBOM) > 0
	if result.HasBOM {
		result.BOMType = canonicalBOMEncoding(targetEncoding)
	}
	sourceCanonical, _ := fileEncoding.CanonicalName(result.SourceEncoding)
	result.Changed = sourceCanonical != targetEncoding || !bytes.Equal(preview.BOM.Bytes, targetBOM)
	if unsupportedCount > 0 {
		return result, nil
	}
	if input.DryRun {
		changed, compareErr := h.previewEncodingChange(ctx, validated.Path, result.SourceEncoding, targetEncoding, targetBOM, previewSnapshot)
		if compareErr != nil {
			return result, compareErr
		}
		result.Changed = changed
		if changed {
			result.Message = fmt.Sprintf("Dry run: %s would change when converted from %s to %s", requestedPath, result.SourceEncoding, targetEncoding)
		} else {
			result.Message = fmt.Sprintf("Dry run: %s is byte-identical under the requested encoding and BOM state", requestedPath)
		}
		return result, nil
	}

	stream, err := h.openDecodedTextStream(ctx, validated.Path, result.SourceEncoding)
	if err != nil {
		return result, err
	}
	defer stream.Close()
	encoded, err := fileEncoding.NewEncoderReader(stream.Reader, targetEncoding)
	if err != nil {
		return result, err
	}
	outputReader := io.MultiReader(bytes.NewReader(targetBOM), &encodingOutputReader{reader: encoded, target: targetEncoding})
	staged, err := filesystem.StageReplacement(validated.Path, outputReader, stream.Mode.Perm(), nil)
	if err != nil {
		return result, err
	}
	defer staged.Cleanup()

	snapshot, err := stream.Finish()
	if err != nil {
		return result, err
	}
	if err := stream.Close(); err != nil {
		return result, err
	}

	backupPath := ""
	if input.Backup {
		backup := h.ValidatePath(validated.Path + ".bak")
		if !backup.Ok() {
			return result, backup.Err
		}
		backupPath = backup.Path
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	commit := h.ValidatePath(requestedPath)
	if !commit.Ok() {
		return result, commit.Err
	}
	if commit.Path != validated.Path {
		return result, operation.Wrap(operation.KindConflict, "convert_encoding", requestedPath, fmt.Errorf("path changed while preparing encoding conversion"))
	}
	changed, err := staged.Commit(filesystem.ReplaceOptions{
		Expected:      &snapshot,
		BackupPath:    backupPath,
		SkipIdentical: true,
	})
	if err != nil {
		return result, err
	}
	result.Changed = changed
	if changed {
		result.BackupPath = backupPath
		result.Message = fmt.Sprintf("Successfully converted %s from %s to %s (BOM: %s)", requestedPath, result.SourceEncoding, targetEncoding, policy)
		if backupPath != "" {
			result.Message += fmt.Sprintf(" (backup: %s)", backupPath)
		}
	} else {
		result.Message = fmt.Sprintf("No conversion needed for %s: target bytes are unchanged", requestedPath)
	}
	return result, nil
}

func (h *Handler) previewEncodingChange(ctx context.Context, path, sourceEncoding, targetEncoding string, targetBOM []byte, expected filesystem.FileSnapshot) (bool, error) {
	stream, err := h.openDecodedTextStream(ctx, path, sourceEncoding)
	if err != nil {
		return false, err
	}
	defer stream.Close()

	encoded, err := fileEncoding.NewEncoderReader(stream.Reader, targetEncoding)
	if err != nil {
		return false, err
	}
	hasher := sha256.New()
	if _, err := hasher.Write(targetBOM); err != nil {
		return false, err
	}
	written, err := io.Copy(hasher, encoded)
	if err != nil {
		if operation.KindOf(err) == operation.KindCancelled {
			return false, err
		}
		return false, operation.Wrap(operation.KindEncodingOutput, "preview_encoding", path, err)
	}
	current, err := stream.Finish()
	if err != nil {
		return false, err
	}
	if !expected.Equal(current) {
		return false, operation.Wrap(operation.KindConflict, "preview_encoding", path, fmt.Errorf("file changed during conversion preview"))
	}
	targetSize := int64(len(targetBOM)) + written
	return !expected.MatchesContentDigest(targetSize, hasher.Sum(nil)), nil
}

func (h *Handler) HandleEditFile(ctx context.Context, _ *mcp.CallToolRequest, input EditFileInput) (*mcp.CallToolResult, EditFileOutput, error) {
	action, err := validateEditActionInput(input)
	if err != nil {
		return errorResultFromError(err), EditFileOutput{}, nil
	}
	switch action {
	case editActionPreview:
		return h.handleEditPreview(ctx, input)
	case editActionApply:
		return h.handleEditApply(ctx, input.PreviewID)
	default:
		return h.handleDirectEdit(ctx, input)
	}
}

func (h *Handler) HandleManageBom(ctx context.Context, _ *mcp.CallToolRequest, input ManageBomInput) (*mcp.CallToolResult, ManageBomOutput, error) {
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return validated.Result, ManageBomOutput{}, nil
	}

	action := strings.ToLower(input.Action)
	if action != "detect" && action != "strip" && action != "add" {
		return errorResult("action must be \"detect\", \"strip\", or \"add\""), ManageBomOutput{}, nil
	}

	session, err := filesystem.OpenReadSession(validated.Path)
	if err != nil {
		return errorResultFromError(err), ManageBomOutput{}, nil
	}
	defer session.Close()

	detected, hasBOM, err := detectBOMPrefix(session)
	if err != nil {
		return errorResult(fmt.Sprintf("failed to inspect BOM: %v", err)), ManageBomOutput{}, nil
	}
	bomSize := 0
	if hasBOM {
		bomSize = fileEncoding.BOMSize(detected.Charset)
	}

	switch action {
	case "detect":
		if !hasBOM {
			return &mcp.CallToolResult{}, ManageBomOutput{Message: "No BOM detected"}, nil
		}
		return &mcp.CallToolResult{}, ManageBomOutput{
			Message:  fmt.Sprintf("BOM detected: %s (%d bytes)", detected.Charset, bomSize),
			HasBOM:   true,
			BOMType:  detected.Charset,
			BOMBytes: bomSize,
		}, nil
	case "strip":
		if !hasBOM {
			return &mcp.CallToolResult{}, ManageBomOutput{Message: "No BOM to strip"}, nil
		}
		if err := session.Start(int64(bomSize)); err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		staged, err := filesystem.StageReplacement(validated.Path, textstream.WithContext(ctx, session), session.Mode().Perm(), nil)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to stage BOM removal: %v", err)), ManageBomOutput{}, nil
		}
		defer staged.Cleanup()
		snapshot, err := session.Finish()
		if err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		if err := session.Close(); err != nil {
			return errorResult(fmt.Sprintf("failed to close source file before commit: %v", err)), ManageBomOutput{}, nil
		}
		if result := h.commitBOMReplacement(input.Path, validated.Path, staged, snapshot); result != nil {
			return result, ManageBomOutput{}, nil
		}
		return &mcp.CallToolResult{}, ManageBomOutput{
			Message:  fmt.Sprintf("Stripped %s BOM (%d bytes) from %s", detected.Charset, bomSize, input.Path),
			BOMType:  detected.Charset,
			BOMBytes: bomSize,
			Changed:  true,
		}, nil
	case "add":
		if input.Encoding == "" {
			return errorResult("encoding is required for add action"), ManageBomOutput{}, nil
		}
		bom := fileEncoding.BOMBytesFor(input.Encoding)
		if len(bom) == 0 {
			return errorResult("encoding must be utf-8, utf-16-le, utf-16-be, utf-32-le, or utf-32-be"), ManageBomOutput{}, nil
		}
		if hasBOM {
			return errorResult(fmt.Sprintf("file already has a %s BOM", detected.Charset)), ManageBomOutput{}, nil
		}
		if err := session.Start(0); err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		staged, err := filesystem.StageReplacement(validated.Path, io.MultiReader(bytes.NewReader(bom), textstream.WithContext(ctx, session)), session.Mode().Perm(), nil)
		if err != nil {
			return errorResult(fmt.Sprintf("failed to stage BOM addition: %v", err)), ManageBomOutput{}, nil
		}
		defer staged.Cleanup()
		snapshot, err := session.Finish()
		if err != nil {
			return errorResultFromError(err), ManageBomOutput{}, nil
		}
		if err := session.Close(); err != nil {
			return errorResult(fmt.Sprintf("failed to close source file before commit: %v", err)), ManageBomOutput{}, nil
		}
		if result := h.commitBOMReplacement(input.Path, validated.Path, staged, snapshot); result != nil {
			return result, ManageBomOutput{}, nil
		}
		charset := canonicalBOMEncoding(input.Encoding)
		return &mcp.CallToolResult{}, ManageBomOutput{
			Message:  fmt.Sprintf("Added %s BOM (%d bytes) to %s", charset, len(bom), input.Path),
			HasBOM:   true,
			BOMType:  charset,
			BOMBytes: len(bom),
			Changed:  true,
		}, nil
	}

	return errorResult("unsupported BOM action"), ManageBomOutput{}, nil
}

func (h *Handler) commitBOMReplacement(inputPath, preparedPath string, staged *filesystem.StagedReplacement, snapshot filesystem.FileSnapshot) *mcp.CallToolResult {
	commit := h.ValidatePath(inputPath)
	if !commit.Ok() {
		return commit.Result
	}
	if commit.Path != preparedPath {
		return errorResult("path changed while preparing BOM mutation")
	}
	if _, err := staged.Commit(filesystem.ReplaceOptions{Expected: &snapshot}); err != nil {
		return errorResult(fmt.Sprintf("failed to write file: %v", err))
	}
	return nil
}

func (h *Handler) HandlePatchPackage(ctx context.Context, _ *mcp.CallToolRequest, input PatchPackageInput) (*mcp.CallToolResult, PatchPackageOutput, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	action, err := validatePatchPackageActionInput(input)
	if err != nil {
		return errorResultFromError(err), PatchPackageOutput{}, nil
	}
	if action == patchPackageActionApply {
		return h.handlePatchPackageApply(ctx, input.PreviewID)
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
