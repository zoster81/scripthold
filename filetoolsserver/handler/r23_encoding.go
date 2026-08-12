package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
)

type ConvertEncodingPreviewInput struct {
	Path         string   `json:"path,omitempty"`
	Paths        []string `json:"paths,omitempty"`
	From         string   `json:"from,omitempty"`
	To           string   `json:"to"`
	Backup       bool     `json:"backup,omitempty"`
	BOM          string   `json:"bom,omitempty"`
	DryRun       bool     `json:"dryRun"`
	BackupPolicy string   `json:"backupPolicy,omitempty"`
}

func (input *ConvertEncodingPreviewInput) UnmarshalJSON(data []byte) error {
	type alias ConvertEncodingPreviewInput
	var decoded alias
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	*input = ConvertEncodingPreviewInput(decoded)
	return nil
}

func (h *Handler) HandleConvertEncodingPreview(ctx context.Context, _ *mcp.CallToolRequest, input ConvertEncodingPreviewInput) (*mcp.CallToolResult, ConvertEncodingOutput, error) {
	if !input.DryRun {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "convert_encoding requires dryRun=true; use convert_encoding_apply with the returned previewId to mutate")), ConvertEncodingOutput{}, nil
	}
	if strings.TrimSpace(input.To) == "" {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "target encoding (to) is required")), ConvertEncodingOutput{}, nil
	}
	targetEncoding, ok := fileEncoding.CanonicalName(input.To)
	if !ok {
		return errorResultWithCode(ErrCodeEncoding, fmt.Sprintf("unsupported target encoding: %s. Use list_encodings to see available encodings.", input.To)), ConvertEncodingOutput{}, nil
	}
	policy, err := parseBOMPolicy(input.BOM, bomAuto)
	if err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}
	backupPolicy, err := h.effectivePersistentBackupPolicy(input.BackupPolicy)
	if err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}
	paths, err := h.conversionPreviewPaths(input)
	if err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}
	if err := h.validateConversionBatchPaths(paths, input.Backup); err != nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "invalid conversion target set: "+err.Error())), ConvertEncodingOutput{}, nil
	}

	prepared := preparedByteMutation{
		kind:         byteMutationKindEncoding,
		action:       "convert",
		backupPolicy: backupPolicy,
		targets:      make([]preparedByteMutationTarget, 0, len(paths)),
	}
	keepPrepared := false
	defer func() {
		if !keepPrepared {
			prepared.close()
		}
	}()
	results := make([]ConvertFileResult, 0, len(paths))
	for _, path := range paths {
		target, result, prepareErr := h.prepareEncodingTarget(ctx, path, input, targetEncoding, policy)
		if prepareErr != nil {
			return errorResultFromError(prepareErr), ConvertEncodingOutput{}, nil
		}
		prepared.targets = append(prepared.targets, target)
		results = append(results, result)
	}
	if err := h.preflightRequiredMutationBackups(ctx, &prepared, backupstore.SourceOperationConvertEncoding); err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}
	preview, err := h.byteMutationPreviews.put(prepared)
	if err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}
	keepPrepared = true

	output := conversionPreviewOutput(preview, targetEncoding, string(policy), results)
	text := fmt.Sprintf("Encoding conversion preview prepared for %d target(s).\nPreview ID: %s\nExpires: %s", len(results), preview.id, output.ExpiresAt)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) conversionPreviewPaths(input ConvertEncodingPreviewInput) ([]string, error) {
	if input.Path != "" && len(input.Paths) > 0 {
		return nil, operation.New(operation.KindInvalidInput, "path and paths are mutually exclusive")
	}
	if input.Path == "" && len(input.Paths) == 0 {
		return nil, operation.New(operation.KindInvalidInput, "path or paths is required")
	}
	if len(input.Paths) > h.maxBatchFiles() {
		return nil, operation.New(operation.KindLimit, fmt.Sprintf("paths contains %d files; configured limit is %d", len(input.Paths), h.maxBatchFiles()))
	}
	if input.Path != "" {
		return []string{input.Path}, nil
	}
	return append([]string(nil), input.Paths...), nil
}

func (h *Handler) prepareEncodingTarget(ctx context.Context, requestedPath string, input ConvertEncodingPreviewInput, targetEncoding string, policy bomPolicy) (preparedByteMutationTarget, ConvertFileResult, error) {
	target, err := h.readExactMutationTarget(ctx, requestedPath)
	if err != nil {
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	keepTarget := false
	defer func() {
		if !keepTarget && target.identityFile != nil {
			_ = target.identityFile.Close()
			target.identityFile = nil
		}
	}()

	stream, err := h.openDecodedTextStream(ctx, target.resolvedPath, input.From)
	if err != nil {
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	sourceEncoding := stream.Charset
	sourceBOM := append([]byte(nil), stream.BOM.Bytes...)
	targetBOM, err := documentBOMBytes(textDocument{Charset: targetEncoding, BOM: stream.BOM}, policy)
	if err != nil {
		_ = stream.Close()
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	unsupported, unsupportedCount, err := inspectUnsupportedCharacters(ctx, stream.Reader, targetEncoding)
	if err != nil {
		_ = stream.Close()
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	streamSnapshot, err := stream.Finish()
	if err != nil {
		_ = stream.Close()
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	if err := stream.Close(); err != nil {
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	streamFingerprint, err := filesystem.FingerprintRegularFileSnapshot(streamSnapshot)
	if err != nil {
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	if streamFingerprint != target.targetFingerprint {
		return preparedByteMutationTarget{}, ConvertFileResult{}, operation.New(operation.KindConflict, "file changed while preparing encoding conversion")
	}
	if unsupportedCount > 0 {
		unsupportedResult := ConvertFileResult{Path: requestedPath, SourceEncoding: sourceEncoding, Unsupported: unsupported, UnsupportedCount: unsupportedCount}
		return preparedByteMutationTarget{}, ConvertFileResult{}, operation.New(operation.KindEncodingOutput, formatUnsupportedError(unsupportedResult))
	}
	if len(sourceBOM) > len(target.data) || !bytes.Equal(target.data[:len(sourceBOM)], sourceBOM) {
		return preparedByteMutationTarget{}, ConvertFileResult{}, operation.New(operation.KindConflict, "source BOM changed while preparing encoding conversion")
	}
	decoded, err := fileEncoding.NewDecoderReader(bytes.NewReader(target.data[len(sourceBOM):]), sourceEncoding)
	if err != nil {
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	encoded, err := fileEncoding.NewEncoderReader(decoded, targetEncoding)
	if err != nil {
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	reader := io.MultiReader(bytes.NewReader(targetBOM), &encodingOutputReader{reader: encoded, target: targetEncoding})
	resultLimit := h.maxByteMutationPreviewBytes()
	readLimit := resultLimit
	if readLimit < math.MaxInt64 {
		readLimit++
	}
	resultBytes, err := io.ReadAll(io.LimitReader(reader, readLimit))
	if err != nil {
		return preparedByteMutationTarget{}, ConvertFileResult{}, err
	}
	if int64(len(resultBytes)) > resultLimit {
		return preparedByteMutationTarget{}, ConvertFileResult{}, operation.New(operation.KindLimit, fmt.Sprintf("converted result for %s exceeds the preview byte limit %d", requestedPath, resultLimit))
	}

	target.data = resultBytes
	target.resultFingerprint = filesystem.FingerprintRegularFileData(resultBytes)
	target.changed = target.resultFingerprint != target.targetFingerprint
	target.sourceEncoding = sourceEncoding
	target.targetEncoding = targetEncoding
	target.bomPolicy = string(policy)
	target.hasBOM = len(targetBOM) > 0
	if target.hasBOM {
		target.bomType = canonicalBOMEncoding(targetEncoding)
	}
	if input.Backup {
		backup := h.ValidatePath(target.resolvedPath + ".bak")
		if !backup.Ok() {
			return preparedByteMutationTarget{}, ConvertFileResult{}, backup.Err
		}
		target.adjacentBackupPath = backup.Path
	}
	result := convertResultFromPreparedTarget(target)
	if target.changed {
		result.Message = fmt.Sprintf("Dry run: %s would change when converted from %s to %s", requestedPath, sourceEncoding, targetEncoding)
	} else {
		result.Message = fmt.Sprintf("Dry run: %s is byte-identical under the requested encoding and BOM state", requestedPath)
	}
	keepTarget = true
	return target, result, nil
}

func conversionPreviewOutput(preview *byteMutationPreview, targetEncoding, policy string, results []ConvertFileResult) ConvertEncodingOutput {
	output := ConvertEncodingOutput{
		Message:        fmt.Sprintf("Prepared %d file(s) for conversion to %s", len(results), targetEncoding),
		PreviewID:      preview.id,
		CreatedAt:      preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt:      preview.expiresAt.Format(timeRFC3339Nano),
		BackupPolicy:   preview.prepared.backupPolicy,
		TargetEncoding: targetEncoding,
		BOMPolicy:      policy,
		DryRun:         true,
		Results:        results,
		SuccessCount:   len(results),
	}
	if len(results) == 1 {
		result := results[0]
		output.SourceEncoding = result.SourceEncoding
		output.TargetFingerprint = result.TargetFingerprint
		output.ResultFingerprint = result.ResultFingerprint
		output.BackupPath = result.BackupPath
		output.HasBOM = result.HasBOM
		output.BOMType = result.BOMType
		output.Changed = result.Changed
	}
	return output
}

func convertResultFromPreparedTarget(target preparedByteMutationTarget) ConvertFileResult {
	return ConvertFileResult{
		Path:              target.requestedPath,
		TargetFingerprint: target.targetFingerprint,
		ResultFingerprint: target.resultFingerprint,
		SourceEncoding:    target.sourceEncoding,
		Changed:           target.changed,
		BackupPath:        target.adjacentBackupPath,
		BOMPolicy:         target.bomPolicy,
		HasBOM:            target.hasBOM,
		BOMType:           target.bomType,
	}
}

func (h *Handler) HandleConvertEncodingApply(ctx context.Context, _ *mcp.CallToolRequest, input PreviewApplyInput) (*mcp.CallToolResult, ConvertEncodingOutput, error) {
	if !validByteMutationPreviewID(input.PreviewID) {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "previewId must be 64 hexadecimal characters")), ConvertEncodingOutput{}, nil
	}
	preview, err := h.byteMutationPreviews.claim(input.PreviewID, byteMutationKindEncoding)
	if err != nil {
		return errorResultFromError(err), ConvertEncodingOutput{}, nil
	}
	prepared := &preview.prepared
	defer prepared.close()
	output := conversionApplyOutput(prepared)

	currents := make([]filesystem.FileSnapshot, len(prepared.targets))
	for index := range prepared.targets {
		target := &prepared.targets[index]
		if filesystem.FingerprintRegularFileData(target.data) != target.resultFingerprint {
			return errorResultWithCode(ErrCodeConflict, fmt.Sprintf("prepared conversion result %d no longer matches its fingerprint", index)), output, nil
		}
		current, revalidateErr := h.revalidateExactMutationTarget(ctx, target)
		if revalidateErr != nil {
			return errorResultFromError(revalidateErr), output, nil
		}
		currents[index] = current
	}

	if prepared.backupPolicy == editBackupPolicyRequired {
		backupIDs, backupErr := h.captureRequiredMutationBackups(ctx, prepared, backupstore.SourceOperationConvertEncoding)
		for index := range backupIDs {
			if backupIDs[index] != "" {
				output.Results[index].BackupID = backupIDs[index]
			}
		}
		if backupErr != nil {
			return errorResultFromError(backupErr), output, nil
		}
		for index := range prepared.targets {
			current, revalidateErr := h.revalidateExactMutationTarget(ctx, &prepared.targets[index])
			if revalidateErr != nil {
				return errorResultFromError(revalidateErr), output, nil
			}
			currents[index] = current
		}
	}

	for index := range prepared.targets {
		target := &prepared.targets[index]
		if !target.changed {
			output.Results[index].Message = "No conversion needed; no backup or write was performed"
			continue
		}
		if err := ctx.Err(); err != nil {
			failure := operation.Wrap(operation.KindCancelled, "apply_encoding_conversion", target.resolvedPath, err)
			return conversionApplyFailure(output, index, failure)
		}
		current, revalidateErr := h.revalidateExactMutationTarget(ctx, target)
		if revalidateErr != nil {
			return conversionApplyFailure(output, index, revalidateErr)
		}
		currents[index] = current
		if target.identityFile == nil {
			return conversionApplyFailure(output, index, operation.New(operation.KindConflict, "conversion target identity is unavailable"))
		}
		if err := target.identityFile.Close(); err != nil {
			return conversionApplyFailure(output, index, operation.WrapFilesystem("close_encoding_preview_identity", target.resolvedPath, err))
		}
		target.identityFile = nil
		if err := h.replaceFile(target.resolvedPath, target.data, filesystem.ReplaceOptions{
			Mode:       current.Mode.Perm(),
			Expected:   &current,
			BackupPath: target.adjacentBackupPath,
		}); err != nil {
			return conversionApplyFailure(output, index, err)
		}
		postLimit := int64(len(target.data))
		if postLimit <= 0 {
			postLimit = 1
		}
		post, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, target.resolvedPath, postLimit)
		if err != nil {
			return conversionApplyFailure(output, index, err)
		}
		actual, err := filesystem.FingerprintRegularFileSnapshot(post)
		if err != nil {
			return conversionApplyFailure(output, index, err)
		}
		if actual != target.resultFingerprint {
			return conversionApplyFailure(output, index, operation.New(operation.KindConflict, "applied conversion does not match the prepared fingerprint"))
		}
		output.Results[index].Applied = true
		output.Results[index].Message = "Encoding conversion applied"
		output.CommittedCount++
	}
	output.Applied = output.CommittedCount > 0
	if len(output.Results) == 1 {
		output.BackupID = output.Results[0].BackupID
		output.BackupPath = output.Results[0].BackupPath
	}
	output.Message = fmt.Sprintf("Applied %d encoding conversion(s)", output.CommittedCount)
	return &mcp.CallToolResult{}, output, nil
}

func conversionApplyOutput(prepared *preparedByteMutation) ConvertEncodingOutput {
	results := make([]ConvertFileResult, len(prepared.targets))
	for index := range prepared.targets {
		results[index] = convertResultFromPreparedTarget(prepared.targets[index])
	}
	output := ConvertEncodingOutput{
		Message:      "Encoding conversion capability consumed",
		BackupPolicy: prepared.backupPolicy,
		Results:      results,
		SuccessCount: len(results),
	}
	if len(results) > 0 {
		output.TargetEncoding = prepared.targets[0].targetEncoding
		output.BOMPolicy = prepared.targets[0].bomPolicy
	}
	if len(results) == 1 {
		result := results[0]
		output.SourceEncoding = result.SourceEncoding
		output.TargetFingerprint = result.TargetFingerprint
		output.ResultFingerprint = result.ResultFingerprint
		output.BackupPath = result.BackupPath
		output.HasBOM = result.HasBOM
		output.BOMType = result.BOMType
		output.Changed = result.Changed
	}
	return output
}

func conversionApplyFailure(output ConvertEncodingOutput, index int, cause error) (*mcp.CallToolResult, ConvertEncodingOutput, error) {
	output.PartialCommit = output.CommittedCount > 0
	output.Applied = output.PartialCommit
	if index >= 0 && index < len(output.Results) {
		mapping := mapOperationError(cause, output.Results[index].Path)
		output.Results[index].Error = mapping.Message
		output.Results[index].ErrorCode = mapping.BatchCode
		if output.PartialCommit {
			return errorResultWithCode(ErrCodePartialCommit, fmt.Sprintf("encoding conversion partially committed %d target(s); target %d failed: %s", output.CommittedCount, index, mapping.Message)), output, nil
		}
		return errorResultWithCode(mapping.BatchCode, mapping.Message), output, nil
	}
	return errorResultFromError(cause), output, nil
}
