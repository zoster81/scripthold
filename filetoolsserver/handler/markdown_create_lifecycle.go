package handler

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/marksplice"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
	"github.com/zoster81/scripthold/internal/operation"
)

type markdownCreateTargetBinding struct {
	resolvedPath   string
	parentPath     string
	expectedTarget filesystem.FileSnapshot
	parentIdentity filesystem.ObjectIdentity
}

type markdownCreatePhysical struct {
	canonical         []byte
	data              []byte
	resultFingerprint string
	encoding          string
	bomType           string
	lineEndingStyle   string
	hasBOM            bool
}

func (h *Handler) HandleMarkdownCreate(ctx context.Context, _ *mcp.CallToolRequest, input MarkdownCreateInput) (*mcp.CallToolResult, MarkdownCreateOutput, error) {
	if input.Path == "" {
		return errorResultWithCode(ErrCodeInvalidInput, "path is required"), MarkdownCreateOutput{}, nil
	}
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_create", input.Path, err)), MarkdownCreateOutput{}, nil
	}
	binding, err := h.prepareMarkdownCreateTarget(input.Path)
	if err != nil {
		return errorResultFromError(err), MarkdownCreateOutput{}, nil
	}
	physical, err := h.prepareMarkdownCreatePhysical(input)
	if err != nil {
		if errors.Is(err, marksplice.ErrInvalidConstruction) {
			return markdownEditErrorResult(err), MarkdownCreateOutput{}, nil
		}
		return errorResultFromError(err), MarkdownCreateOutput{}, nil
	}
	if err := revalidateMarkdownCreateBinding(binding); err != nil {
		return errorResultFromError(err), MarkdownCreateOutput{}, nil
	}

	prepared := preparedMarkdownCreate{
		requestedPath:     input.Path,
		resolvedPath:      binding.resolvedPath,
		parentPath:        binding.parentPath,
		resultData:        physical.data,
		resultFingerprint: physical.resultFingerprint,
		encoding:          physical.encoding,
		bomType:           physical.bomType,
		lineEndingStyle:   physical.lineEndingStyle,
		expectedTarget:    binding.expectedTarget,
		parentIdentity:    binding.parentIdentity,
		hasBOM:            physical.hasBOM,
	}
	preview, err := h.markdownPreviews.putCreate(prepared)
	if err != nil {
		return errorResultFromError(err), MarkdownCreateOutput{}, nil
	}
	output := markdownCreateOutputFromPreview(preview, physical.canonical)
	text := markdownCreatePreviewText(output)
	if err := h.checkMarkdownMutationResponseLimit(output, text); err != nil {
		h.markdownPreviews.discard(preview.id)
		return errorResultFromError(err), MarkdownCreateOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) prepareMarkdownCreateTarget(requestedPath string) (markdownCreateTargetBinding, error) {
	resolvedPath, err := h.validatePath(requestedPath)
	if err != nil {
		return markdownCreateTargetBinding{}, err
	}
	expectedTarget, err := filesystem.CaptureSnapshot(resolvedPath)
	if err != nil {
		return markdownCreateTargetBinding{}, err
	}
	if expectedTarget.Exists {
		return markdownCreateTargetBinding{}, operation.New(operation.KindConflict, "Markdown create target already exists")
	}
	parentPath := filepath.Dir(resolvedPath)
	parentIdentity, err := filesystem.CaptureObjectIdentity(parentPath)
	if err != nil {
		return markdownCreateTargetBinding{}, err
	}
	if !parentIdentity.IsDirectory() {
		return markdownCreateTargetBinding{}, operation.New(operation.KindInvalidInput, "Markdown create parent must be a directory")
	}
	return markdownCreateTargetBinding{
		resolvedPath: resolvedPath, parentPath: parentPath,
		expectedTarget: expectedTarget, parentIdentity: parentIdentity,
	}, nil
}

func (h *Handler) prepareMarkdownCreatePhysical(input MarkdownCreateInput) (markdownCreatePhysical, error) {
	canonical, err := markdownintelligence.BuildDocument(
		markdownCreateDocument(input),
		markdownintelligence.CreateLimits{MaxTextBytes: h.maxFileBytes()},
	)
	if err != nil {
		return markdownCreatePhysical{}, err
	}
	encodingName, err := h.resolveMarkdownCreateEncoding(input.Encoding)
	if err != nil {
		return markdownCreatePhysical{}, err
	}
	policy, err := parseBOMPolicy(input.BOM, bomAuto)
	if err != nil {
		return markdownCreatePhysical{}, err
	}
	if policy == bomPreserve {
		return markdownCreatePhysical{}, operation.New(operation.KindInvalidInput, "markdown_create BOM policy must be auto, always, or never")
	}
	resultData, err := encodeTextDocument(textDocument{Charset: encodingName}, string(canonical), policy)
	if err != nil {
		return markdownCreatePhysical{}, err
	}
	if int64(len(resultData)) > h.maxFileBytes() {
		return markdownCreatePhysical{}, operation.New(operation.KindLimit, fmt.Sprintf("prepared Markdown file size %d exceeds limit %d", len(resultData), h.maxFileBytes()))
	}
	hasBOM, bomType := outputBOMMetadata(resultData)
	return markdownCreatePhysical{
		canonical:         canonical,
		data:              resultData,
		resultFingerprint: filesystem.FingerprintRegularFileData(resultData),
		encoding:          encodingName,
		bomType:           bomType,
		lineEndingStyle:   DetectLineEndings(canonical).Style,
		hasBOM:            hasBOM,
	}, nil
}

func revalidateMarkdownCreateBinding(binding markdownCreateTargetBinding) error {
	matches, err := binding.parentIdentity.Matches(binding.parentPath)
	if err != nil {
		return err
	}
	if !matches {
		return operation.New(operation.KindConflict, "Markdown create parent identity changed")
	}
	if err := binding.expectedTarget.Verify(binding.resolvedPath); err != nil {
		return operation.New(operation.KindConflict, "Markdown create target is no longer absent")
	}
	return nil
}

func (h *Handler) resolveMarkdownCreateEncoding(inputEncoding string) (string, error) {
	if inputEncoding != "" {
		canonical, ok := fileEncoding.CanonicalName(inputEncoding)
		if !ok {
			return "", fmt.Errorf("%w: %s. Use list_encodings to see available encodings", ErrEncodingUnsupported, strings.ToLower(strings.TrimSpace(inputEncoding)))
		}
		return canonical, nil
	}
	canonical, ok := fileEncoding.CanonicalName(h.config.DefaultEncoding)
	if !ok {
		return "", fmt.Errorf("%w: configured default encoding %s is not registered", ErrEncodingUnsupported, h.config.DefaultEncoding)
	}
	return canonical, nil
}

func (h *Handler) handleMarkdownCreateApply(ctx context.Context, preview *markdownPreview) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	if preview == nil || preview.kind != markdownPreviewCreate || preview.create == nil {
		return errorResultWithCode(ErrCodeConflict, "Markdown preview is not a create preview"), MarkdownApplyOutput{}, nil
	}
	prepared := *preview.create
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", prepared.resolvedPath, err)), MarkdownApplyOutput{}, nil
	}
	resolvedPath, err := h.validateMarkdownCreateApply(prepared)
	if err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}
	output := markdownCreateApplyOutput(prepared)
	worstCase := output
	worstCase.ActualFingerprint = prepared.resultFingerprint
	worstCase.State = editApplyStateCommitted
	worstCase.Changed = true
	worstCase.Applied = true
	if err := h.checkMarkdownMutationResponseLimit(worstCase, markdownApplyText(worstCase)); err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}
	return h.commitMarkdownCreate(ctx, prepared, resolvedPath, output)
}

func (h *Handler) validateMarkdownCreateApply(prepared preparedMarkdownCreate) (string, error) {
	resolvedPath, err := h.validatePath(prepared.requestedPath)
	if err != nil {
		return "", err
	}
	if resolvedPath != prepared.resolvedPath || filepath.Dir(resolvedPath) != prepared.parentPath {
		return "", operation.New(operation.KindConflict, "Markdown create target path changed after preview")
	}
	binding := markdownCreateTargetBinding{
		resolvedPath: resolvedPath, parentPath: prepared.parentPath,
		expectedTarget: prepared.expectedTarget, parentIdentity: prepared.parentIdentity,
	}
	if err := revalidateMarkdownCreateBinding(binding); err != nil {
		return "", err
	}
	if filesystem.FingerprintRegularFileData(prepared.resultData) != prepared.resultFingerprint {
		return "", operation.New(operation.KindConflict, "prepared Markdown create result no longer matches its fingerprint")
	}
	return resolvedPath, nil
}

func (h *Handler) commitMarkdownCreate(ctx context.Context, prepared preparedMarkdownCreate, resolvedPath string, output MarkdownApplyOutput) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", resolvedPath, err)), output, nil
	}
	binding := markdownCreateTargetBinding{
		resolvedPath: resolvedPath, parentPath: prepared.parentPath,
		expectedTarget: prepared.expectedTarget, parentIdentity: prepared.parentIdentity,
	}
	if err := revalidateMarkdownCreateBinding(binding); err != nil {
		return errorResultFromError(err), output, nil
	}
	if err := h.replaceFile(resolvedPath, prepared.resultData, filesystem.ReplaceOptions{Mode: DefaultFileMode, Expected: &prepared.expectedTarget}); err != nil {
		if errors.Is(err, filesystem.ErrConcurrentModification) || errors.Is(err, filesystem.ErrDestinationExists) {
			return errorResultFromError(err), output, nil
		}
		return h.classifyMarkdownCreateApplyFailure(prepared, output, errorResultFromError(fmt.Errorf("failed to create Markdown file: %w", err)))
	}
	return h.verifyMarkdownCreateCommit(ctx, prepared, resolvedPath, output)
}

func (h *Handler) verifyMarkdownCreateCommit(ctx context.Context, prepared preparedMarkdownCreate, resolvedPath string, output MarkdownApplyOutput) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	post, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, resolvedPath, h.maxFileBytes())
	if err != nil {
		return h.classifyMarkdownCreateApplyFailure(prepared, output, errorResultFromError(err))
	}
	actualFingerprint, err := filesystem.FingerprintRegularFileSnapshot(post)
	if err != nil {
		return h.classifyMarkdownCreateApplyFailure(prepared, output, errorResultFromError(err))
	}
	if actualFingerprint != prepared.resultFingerprint {
		return h.classifyMarkdownCreateApplyFailure(prepared, output, errorResultWithCode(ErrCodeConflict, "created Markdown file does not match the prepared result fingerprint"))
	}
	output.ActualFingerprint = actualFingerprint
	output.State = editApplyStateCommitted
	output.Changed = true
	output.Applied = true
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: markdownApplyText(output)}}}, output, nil
}

func markdownCreateApplyOutput(prepared preparedMarkdownCreate) MarkdownApplyOutput {
	return MarkdownApplyOutput{
		Path: prepared.requestedPath, ResultFingerprint: prepared.resultFingerprint,
		Encoding: prepared.encoding, HasBOM: prepared.hasBOM, BOMType: prepared.bomType,
		LineEndingStyle: prepared.lineEndingStyle, State: editApplyStateUnknown,
	}
}

func (h *Handler) classifyMarkdownCreateApplyFailure(prepared preparedMarkdownCreate, output MarkdownApplyOutput, failure *mcp.CallToolResult) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	output.Applied = false
	output.Changed = false
	output.State = editApplyStateUnknown
	output.ActualFingerprint = ""

	classificationCtx, cancel := context.WithTimeout(context.Background(), editApplyClassificationTimeout)
	defer cancel()
	validation := h.ValidatePath(prepared.requestedPath)
	if validation.Ok() && validation.Path == prepared.resolvedPath && classificationCtx.Err() == nil {
		post, err := filesystem.CaptureRegularFileSnapshotBounded(classificationCtx, validation.Path, h.maxFileBytes())
		if err == nil {
			if !post.Exists {
				output.State = editApplyStateUnchanged
			} else if actual, fingerprintErr := filesystem.FingerprintRegularFileSnapshot(post); fingerprintErr == nil {
				output.ActualFingerprint = actual
				output.Changed = true
				if actual == prepared.resultFingerprint {
					output.State = editApplyStateCommitted
				}
			}
		}
	}
	if output.State == editApplyStateUnchanged {
		return failure, output, nil
	}
	message := "Markdown create apply failed after the target may have changed"
	if failure != nil && len(failure.Content) > 0 {
		if content, ok := failure.Content[0].(*mcp.TextContent); ok && content.Text != "" {
			message = content.Text
		}
	}
	return errorResultWithCode(ErrCodePartialCommit, message), output, nil
}

func markdownCreateOutputFromPreview(preview *markdownPreview, canonical []byte) MarkdownCreateOutput {
	if preview == nil || preview.create == nil {
		return MarkdownCreateOutput{}
	}
	prepared := preview.create
	return MarkdownCreateOutput{
		PreviewID: preview.id, CreatedAt: preview.createdAt.Format(timeRFC3339Nano), ExpiresAt: preview.expiresAt.Format(timeRFC3339Nano),
		Path: prepared.requestedPath, ResultFingerprint: prepared.resultFingerprint, Encoding: prepared.encoding,
		HasBOM: prepared.hasBOM, BOMType: prepared.bomType, LineEndingStyle: prepared.lineEndingStyle,
		SizeBytes: int64(len(prepared.resultData)), Markdown: string(canonical),
	}
}

func markdownCreatePreviewText(output MarkdownCreateOutput) string {
	return fmt.Sprintf("Markdown create preview prepared.\nPreview ID: %s\nExpires: %s\nResult fingerprint: %s\nSize: %d bytes",
		output.PreviewID, output.ExpiresAt, output.ResultFingerprint, output.SizeBytes)
}
