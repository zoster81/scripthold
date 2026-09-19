package handler

import (
	"bytes"
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
	"github.com/zoster81/scripthold/internal/operation"
)

type markdownEditSource struct {
	path         string
	identityFile *filesystem.FileIdentity
	document     textDocument
	sourceData   []byte
	sourceUTF8   []byte
}

func (h *Handler) openMarkdownEditSource(ctx context.Context, input MarkdownEditInput) (markdownEditSource, *mcp.CallToolResult) {
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return markdownEditSource{}, validated.Result
	}
	identityFile, err := filesystem.OpenFileIdentity(validated.Path)
	if err != nil {
		return markdownEditSource{}, errorResultFromError(err)
	}
	source := markdownEditSource{path: validated.Path, identityFile: identityFile}
	document, sourceData, err := h.readTextDocumentWithData(ctx, validated.Path, input.Encoding)
	if err != nil {
		_ = identityFile.Close()
		return markdownEditSource{}, errorResultFromError(err)
	}
	matches, err := identityFile.Matches(validated.Path)
	if err != nil || !matches {
		_ = identityFile.Close()
		return markdownEditSource{}, errorResultWithCode(ErrCodeConflict, "Markdown target identity changed while preparing preview")
	}
	if isReadOnly(document.Mode) {
		_ = identityFile.Close()
		return markdownEditSource{}, errorResultWithCode(ErrCodePermission, "Markdown target is read-only")
	}
	if !utf8.ValidString(document.Text) {
		_ = identityFile.Close()
		err := operation.Wrap(operation.KindEncoding, "markdown_edit", validated.Path, fmt.Errorf("decoded Markdown is not valid UTF-8"))
		return markdownEditSource{}, errorResultFromError(err)
	}
	source.document = document
	source.sourceData = sourceData
	source.sourceUTF8 = []byte(document.Text)
	return source, nil
}

func prepareMarkdownEditSemantic(sourceUTF8 []byte, operations []MarkdownEditOperation) (markdownintelligence.PreparedChange, []byte, *mcp.CallToolResult) {
	snapshot, err := markdownintelligence.Parse(sourceUTF8)
	if err != nil {
		return markdownintelligence.PreparedChange{}, nil, markdownEditErrorResult(err)
	}
	preparedChanges := make([]markdownintelligence.PreparedChange, 0, len(operations))
	for _, operationInput := range operations {
		preparedChange, prepareErr := prepareMarkdownEditOperation(snapshot, operationInput)
		if prepareErr != nil {
			return markdownintelligence.PreparedChange{}, nil, markdownEditErrorResult(prepareErr)
		}
		preparedChanges = append(preparedChanges, preparedChange)
	}
	preparedChange, err := snapshot.ComposeChanges(preparedChanges...)
	if err != nil {
		return markdownintelligence.PreparedChange{}, nil, markdownEditErrorResult(err)
	}
	resultUTF8, err := preparedChange.Apply(sourceUTF8)
	if err != nil {
		return markdownintelligence.PreparedChange{}, nil, markdownEditErrorResult(err)
	}
	return preparedChange, resultUTF8, nil
}

func (h *Handler) buildPreparedMarkdownEdit(ctx context.Context, input MarkdownEditInput, backupPolicy string, source markdownEditSource, semantic markdownintelligence.PreparedChange, resultUTF8 []byte) (preparedMarkdownEdit, *mcp.CallToolResult) {
	resultData, err := markdownPhysicalResult(source.document, source.sourceData, source.sourceUTF8, resultUTF8)
	if err != nil {
		return preparedMarkdownEdit{}, errorResultFromError(err)
	}
	if int64(len(resultData)) > h.maxFileBytes() {
		err := operation.New(operation.KindLimit, fmt.Sprintf("prepared Markdown file size %d exceeds limit %d", len(resultData), h.maxFileBytes()))
		return preparedMarkdownEdit{}, errorResultFromError(err)
	}
	targetFingerprint, err := filesystem.FingerprintRegularFileSnapshot(source.document.Snapshot)
	if err != nil {
		return preparedMarkdownEdit{}, errorResultFromError(err)
	}
	resultFingerprint := filesystem.FingerprintRegularFileData(resultData)
	changed := !bytes.Equal(source.sourceData, resultData)
	if failure := h.preflightMarkdownEditBackup(ctx, source.path, backupPolicy, changed); failure != nil {
		return preparedMarkdownEdit{}, failure
	}
	return preparedMarkdownEdit{
		requestedPath:     input.Path,
		resolvedPath:      source.path,
		resultUTF8:        resultUTF8,
		targetFingerprint: targetFingerprint,
		resultFingerprint: resultFingerprint,
		encoding:          source.document.Charset,
		bomType:           source.document.BOM.Type,
		lineEndingStyle:   source.document.LineEndings.Style,
		diff:              createUnifiedDiff(string(source.sourceUTF8), string(resultUTF8), input.Path),
		backupPolicy:      backupPolicy,
		semantic:          semantic,
		identityFile:      source.identityFile,
		hasBOM:            source.document.BOM.HasBOM,
		changed:           changed,
	}, nil
}

func (h *Handler) preflightMarkdownEditBackup(ctx context.Context, path, backupPolicy string, changed bool) *mcp.CallToolResult {
	if !changed || !persistentBackupRequired(backupPolicy) {
		return nil
	}
	if h.backupCapturePreflight == nil {
		return errorResultFromError(operation.New(operation.KindInvalidInput, "required backup preflight authority is unavailable"))
	}
	err := h.backupCapturePreflight.PreflightCaptureBatch(ctx, []backupstore.CaptureRequest{{
		TargetPath:      path,
		SourceOperation: backupstore.SourceOperationEdit,
		Pinned:          persistentBackupPinned(backupPolicy),
	}})
	if err != nil {
		return errorResultFromError(err)
	}
	return nil
}
