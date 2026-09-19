package handler

import (
	"bytes"
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

type markdownWorkspaceRepairApplyTarget struct {
	document    textDocument
	replacement preparedExistingFileReplacement
}

func (h *Handler) prepareMarkdownWorkspaceRepairApplyTargets(ctx context.Context, prepared *preparedMarkdownWorkspaceRepair) ([]markdownWorkspaceRepairApplyTarget, *mcp.CallToolResult) {
	if prepared == nil || len(prepared.targets) == 0 {
		return nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair preview is unavailable")
	}
	rootValidation := h.ValidatePath(prepared.root)
	if !rootValidation.Ok() {
		return nil, rootValidation.Result
	}
	if rootValidation.Path != prepared.root {
		return nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace root changed after preview")
	}

	plans := make([]markdownWorkspaceRepairApplyTarget, len(prepared.targets))
	for index := range prepared.targets {
		if err := ctx.Err(); err != nil {
			return nil, errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", prepared.root, err))
		}
		plan, failure := h.prepareMarkdownWorkspaceRepairApplyTarget(ctx, prepared.root, &prepared.targets[index])
		if failure != nil {
			return nil, failure
		}
		plans[index] = plan
	}
	return plans, nil
}

func (h *Handler) prepareMarkdownWorkspaceRepairApplyTarget(ctx context.Context, root string, target *preparedMarkdownWorkspaceRepairTarget) (markdownWorkspaceRepairApplyTarget, *mcp.CallToolResult) {
	path, failure := h.revalidateMarkdownWorkspaceRepairApplyBinding(root, target)
	if failure != nil {
		return markdownWorkspaceRepairApplyTarget{}, failure
	}
	document, sourceData, failure := h.readMarkdownWorkspaceRepairApplySource(ctx, target, path)
	if failure != nil {
		return markdownWorkspaceRepairApplyTarget{}, failure
	}
	if failure := materializeMarkdownWorkspaceRepairApplyResult(target, document, sourceData); failure != nil {
		return markdownWorkspaceRepairApplyTarget{}, failure
	}
	return markdownWorkspaceRepairApplyTarget{
		document:    document,
		replacement: preparedMarkdownWorkspaceRepairReplacement(target),
	}, nil
}

func (h *Handler) revalidateMarkdownWorkspaceRepairApplyBinding(root string, target *preparedMarkdownWorkspaceRepairTarget) (string, *mcp.CallToolResult) {
	if target == nil || target.identityFile == nil {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity is unavailable")
	}
	validation := h.ValidatePath(target.requestedPath)
	if !validation.Ok() {
		return "", validation.Result
	}
	if validation.Path != target.resolvedPath || !security.IsPathWithinAllowedDirectories(validation.Path, []string{root}) {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target binding changed after preview")
	}
	matches, err := target.identityFile.Matches(validation.Path)
	if err != nil || !matches {
		return "", errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity changed after preview")
	}
	return validation.Path, nil
}

func (h *Handler) readMarkdownWorkspaceRepairApplySource(ctx context.Context, target *preparedMarkdownWorkspaceRepairTarget, path string) (textDocument, []byte, *mcp.CallToolResult) {
	document, sourceData, err := h.readTextDocumentWithData(ctx, path, target.encoding)
	if err != nil {
		return textDocument{}, nil, errorResultFromError(err)
	}
	matches, err := target.identityFile.Matches(path)
	if err != nil || !matches {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target identity changed while revalidating")
	}
	if document.Charset != target.encoding || document.BOM.HasBOM != target.hasBOM || document.BOM.Type != target.bomType {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair encoding or BOM changed after preview")
	}
	if document.LineEndings.Style != target.lineEndingStyle {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair line endings changed after preview")
	}
	currentFingerprint, err := filesystem.FingerprintRegularFileSnapshot(document.Snapshot)
	if err != nil {
		return textDocument{}, nil, errorResultFromError(err)
	}
	if currentFingerprint != target.targetFingerprint {
		return textDocument{}, nil, errorResultWithCode(ErrCodeConflict, "Markdown workspace repair target fingerprint changed after preview")
	}
	if target.changed && isReadOnly(document.Mode) {
		return textDocument{}, nil, errorResultWithCode(ErrCodePermission, "Markdown workspace repair target became read-only after preview")
	}
	if !utf8.ValidString(document.Text) {
		err := operation.Wrap(operation.KindEncoding, "markdown_apply", path, fmt.Errorf("decoded Markdown is not valid UTF-8"))
		return textDocument{}, nil, errorResultFromError(err)
	}
	return document, sourceData, nil
}

func materializeMarkdownWorkspaceRepairApplyResult(target *preparedMarkdownWorkspaceRepairTarget, document textDocument, sourceData []byte) *mcp.CallToolResult {
	sourceUTF8 := []byte(document.Text)
	resultUTF8, err := target.change.Apply(sourceUTF8)
	if err != nil {
		return markdownEditErrorResult(err)
	}
	resultData, err := markdownPhysicalResult(document, sourceData, sourceUTF8, resultUTF8)
	if err != nil {
		return errorResultFromError(err)
	}
	if !bytes.Equal(resultData, target.resultData) || filesystem.FingerprintRegularFileData(resultData) != target.resultFingerprint {
		return errorResultWithCode(ErrCodeConflict, "prepared Markdown workspace repair result changed after preview")
	}
	if changed := !bytes.Equal(sourceData, resultData); changed != target.changed {
		return errorResultWithCode(ErrCodeConflict, "prepared Markdown workspace repair change state changed after preview")
	}
	return nil
}
