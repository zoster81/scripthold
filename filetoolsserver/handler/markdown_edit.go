package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/marksplice"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
	"github.com/zoster81/scripthold/internal/operation"
)

const (
	MarkdownErrInvalidStructure = "invalid_structure"
	MarkdownErrSourceConflict   = "source_conflict"
)

// MarkdownEditOperation is one declarative Marksplice-backed mutation request.
// The public schema keeps operation-specific fields closed; zero values here
// exist only because Go uses one transport struct for the discriminated union.
type MarkdownEditOperation struct {
	Action         string `json:"action"`
	Subject        string `json:"subject"`
	TargetID       string `json:"targetId"`
	AnchorTargetID string `json:"anchorTargetId,omitempty"`
	Text           string `json:"text,omitempty"`
	Level          int    `json:"level,omitempty"`
	Markdown       string `json:"markdown,omitempty"`
	Position       string `json:"position,omitempty"`
	Part           string `json:"part,omitempty"`
}

// MarkdownEditInput prepares one source-bound Markdown preview and never writes
// the target file.
type MarkdownEditInput struct {
	Path         string                  `json:"path"`
	Encoding     string                  `json:"encoding,omitempty"`
	Operations   []MarkdownEditOperation `json:"operations"`
	BackupPolicy string                  `json:"backupPolicy,omitempty"`
}

// MarkdownEditOutput reports physical and semantic evidence for one prepared
// Markdown mutation.
type MarkdownEditOutput struct {
	PreviewID         string                  `json:"previewId"`
	CreatedAt         string                  `json:"createdAt"`
	ExpiresAt         string                  `json:"expiresAt"`
	Path              string                  `json:"path"`
	Operations        []MarkdownEditOperation `json:"operations"`
	TargetFingerprint string                  `json:"targetFingerprint"`
	ResultFingerprint string                  `json:"resultFingerprint"`
	Encoding          string                  `json:"encoding"`
	HasBOM            bool                    `json:"hasBOM"`
	BOMType           string                  `json:"bomType,omitempty"`
	LineEndingStyle   string                  `json:"lineEndingStyle"`
	BackupPolicy      string                  `json:"backupPolicy,omitempty"`
	Diff              string                  `json:"diff,omitempty"`
	Changed           bool                    `json:"changed"`
}

// MarkdownApplyInput is intentionally the entire markdown_apply request shape.
type MarkdownApplyInput struct {
	PreviewID string `json:"previewId"`
}

// MarkdownApplyOutput reports observed post-apply state rather than preview
// predictions.
type MarkdownApplyOutput struct {
	Path              string `json:"path"`
	TargetFingerprint string `json:"targetFingerprint"`
	ResultFingerprint string `json:"resultFingerprint"`
	ActualFingerprint string `json:"actualFingerprint,omitempty"`
	Encoding          string `json:"encoding"`
	HasBOM            bool   `json:"hasBOM"`
	BOMType           string `json:"bomType,omitempty"`
	LineEndingStyle   string `json:"lineEndingStyle"`
	BackupPolicy      string `json:"backupPolicy,omitempty"`
	BackupID          string `json:"backupId,omitempty"`
	State             string `json:"state"`
	Changed           bool   `json:"changed"`
	Applied           bool   `json:"applied"`
}

func (h *Handler) HandleMarkdownEdit(ctx context.Context, _ *mcp.CallToolRequest, input MarkdownEditInput) (*mcp.CallToolResult, MarkdownEditOutput, error) {
	if result := validateMarkdownEditInput(input); result != nil {
		return result, MarkdownEditOutput{}, nil
	}
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_edit", input.Path, err)), MarkdownEditOutput{}, nil
	}
	backupPolicy, err := h.effectivePersistentBackupPolicy(input.BackupPolicy)
	if err != nil {
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return validated.Result, MarkdownEditOutput{}, nil
	}

	identityFile, err := filesystem.OpenFileIdentity(validated.Path)
	if err != nil {
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	keepIdentity := false
	defer func() {
		if !keepIdentity {
			_ = identityFile.Close()
		}
	}()

	document, sourceData, err := h.readTextDocumentWithData(ctx, validated.Path, input.Encoding)
	if err != nil {
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	matches, err := identityFile.Matches(validated.Path)
	if err != nil || !matches {
		return errorResultWithCode(ErrCodeConflict, "Markdown target identity changed while preparing preview"), MarkdownEditOutput{}, nil
	}
	if isReadOnly(document.Mode) {
		return errorResultWithCode(ErrCodePermission, "Markdown target is read-only"), MarkdownEditOutput{}, nil
	}
	if !utf8.ValidString(document.Text) {
		return errorResultFromError(operation.Wrap(operation.KindEncoding, "markdown_edit", validated.Path, fmt.Errorf("decoded Markdown is not valid UTF-8"))), MarkdownEditOutput{}, nil
	}

	sourceUTF8 := []byte(document.Text)
	snapshot, err := markdownintelligence.Parse(sourceUTF8)
	if err != nil {
		return markdownEditErrorResult(err), MarkdownEditOutput{}, nil
	}
	preparedChanges := make([]markdownintelligence.PreparedChange, 0, len(input.Operations))
	for _, operationInput := range input.Operations {
		var preparedChange markdownintelligence.PreparedChange
		var prepareErr error
		switch {
		case operationInput.Action == "rename" && operationInput.Subject == "heading":
			preparedChange, prepareErr = snapshot.PrepareRenameHeading(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "set" && operationInput.Subject == "heading":
			preparedChange, prepareErr = snapshot.PrepareSetHeadingLevel(operationInput.TargetID, operationInput.Level)
		case operationInput.Action == "replace" && operationInput.Subject == "paragraph":
			preparedChange, prepareErr = snapshot.PrepareReplaceParagraph(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "replace" && operationInput.Subject == "list_item":
			preparedChange, prepareErr = snapshot.PrepareReplaceListItem(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "remove" && operationInput.Subject == "paragraph":
			preparedChange, prepareErr = snapshot.PrepareRemoveParagraph(operationInput.TargetID)
		case operationInput.Action == "insert" && operationInput.Subject == "paragraph" && operationInput.Position == "before":
			preparedChange, prepareErr = snapshot.PrepareInsertParagraphBefore(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "insert" && operationInput.Subject == "paragraph" && operationInput.Position == "after":
			preparedChange, prepareErr = snapshot.PrepareInsertParagraphAfter(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "insert" && operationInput.Subject == "section" && operationInput.Position == "before":
			preparedChange, prepareErr = snapshot.PrepareInsertSectionBefore(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "insert" && operationInput.Subject == "section" && operationInput.Position == "after":
			preparedChange, prepareErr = snapshot.PrepareInsertSectionAfter(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "insert" && operationInput.Subject == "section" && operationInput.Position == "child":
			preparedChange, prepareErr = snapshot.PrepareAppendSectionChild(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "remove" && operationInput.Subject == "section":
			preparedChange, prepareErr = snapshot.PrepareRemoveSection(operationInput.TargetID)
		case operationInput.Action == "replace" && operationInput.Subject == "section" && operationInput.Part == "body":
			preparedChange, prepareErr = snapshot.PrepareReplaceSectionBody(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "replace" && operationInput.Subject == "section" && operationInput.Part == "subtree":
			preparedChange, prepareErr = snapshot.PrepareReplaceSection(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "move" && operationInput.Subject == "section" && operationInput.Position == "before":
			preparedChange, prepareErr = snapshot.PrepareMoveSectionBefore(operationInput.TargetID, operationInput.AnchorTargetID)
		case operationInput.Action == "move" && operationInput.Subject == "section" && operationInput.Position == "after":
			preparedChange, prepareErr = snapshot.PrepareMoveSectionAfter(operationInput.TargetID, operationInput.AnchorTargetID)
		default:
			prepareErr = marksplice.ErrInvalidQuery
		}
		if prepareErr != nil {
			return markdownEditErrorResult(prepareErr), MarkdownEditOutput{}, nil
		}
		preparedChanges = append(preparedChanges, preparedChange)
	}
	preparedChange, err := snapshot.ComposeChanges(preparedChanges...)
	if err != nil {
		return markdownEditErrorResult(err), MarkdownEditOutput{}, nil
	}
	resultUTF8, err := preparedChange.Apply(sourceUTF8)
	if err != nil {
		return markdownEditErrorResult(err), MarkdownEditOutput{}, nil
	}
	resultData, err := markdownPhysicalResult(document, sourceData, sourceUTF8, resultUTF8)
	if err != nil {
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	if int64(len(resultData)) > h.maxFileBytes() {
		return errorResultFromError(operation.New(operation.KindLimit, fmt.Sprintf("prepared Markdown file size %d exceeds limit %d", len(resultData), h.maxFileBytes()))), MarkdownEditOutput{}, nil
	}

	targetFingerprint, err := filesystem.FingerprintRegularFileSnapshot(document.Snapshot)
	if err != nil {
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	resultFingerprint := filesystem.FingerprintRegularFileData(resultData)
	changed := !bytes.Equal(sourceData, resultData)
	if changed && persistentBackupRequired(backupPolicy) {
		if h.backupCapturePreflight == nil {
			return errorResultFromError(operation.New(operation.KindInvalidInput, "required backup preflight authority is unavailable")), MarkdownEditOutput{}, nil
		}
		if err := h.backupCapturePreflight.PreflightCaptureBatch(ctx, []backupstore.CaptureRequest{{
			TargetPath:      validated.Path,
			SourceOperation: backupstore.SourceOperationEdit,
			Pinned:          persistentBackupPinned(backupPolicy),
		}}); err != nil {
			return errorResultFromError(err), MarkdownEditOutput{}, nil
		}
	}

	prepared := preparedMarkdownEdit{
		requestedPath:     input.Path,
		resolvedPath:      validated.Path,
		resultUTF8:        resultUTF8,
		targetFingerprint: targetFingerprint,
		resultFingerprint: resultFingerprint,
		encoding:          document.Charset,
		bomType:           document.BOM.Type,
		lineEndingStyle:   document.LineEndings.Style,
		diff:              createUnifiedDiff(string(sourceUTF8), string(resultUTF8), input.Path),
		backupPolicy:      backupPolicy,
		semantic:          preparedChange,
		identityFile:      identityFile,
		hasBOM:            document.BOM.HasBOM,
		changed:           changed,
	}
	preview, err := h.markdownEditPreviews.put(prepared)
	if err != nil {
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	keepIdentity = true
	output := markdownEditOutputFromPreview(preview, input.Operations)
	text := markdownEditPreviewText(output)
	if err := h.checkMarkdownMutationResponseLimit(output, text); err != nil {
		h.markdownEditPreviews.discard(preview.id)
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func (h *Handler) HandleMarkdownApply(ctx context.Context, _ *mcp.CallToolRequest, input MarkdownApplyInput) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	preview, err := h.markdownEditPreviews.claim(input.PreviewID)
	if err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}
	prepared := preview.prepared
	defer func() {
		if prepared.identityFile != nil {
			_ = prepared.identityFile.Close()
		}
	}()
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", prepared.resolvedPath, err)), MarkdownApplyOutput{}, nil
	}

	validated := h.ValidatePath(prepared.requestedPath)
	if !validated.Ok() {
		return validated.Result, MarkdownApplyOutput{}, nil
	}
	if validated.Path != prepared.resolvedPath {
		return errorResultWithCode(ErrCodeConflict, "Markdown target path changed after preview"), MarkdownApplyOutput{}, nil
	}
	if prepared.identityFile == nil {
		return errorResultWithCode(ErrCodeConflict, "Markdown preview identity is unavailable"), MarkdownApplyOutput{}, nil
	}
	matches, err := prepared.identityFile.Matches(validated.Path)
	if err != nil || !matches {
		return errorResultWithCode(ErrCodeConflict, "Markdown target identity changed after preview"), MarkdownApplyOutput{}, nil
	}

	currentDocument, currentData, err := h.readTextDocumentWithData(ctx, validated.Path, prepared.encoding)
	if err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}
	if currentDocument.Charset != prepared.encoding || currentDocument.BOM.HasBOM != prepared.hasBOM || currentDocument.BOM.Type != prepared.bomType {
		return errorResultWithCode(ErrCodeConflict, "Markdown encoding or BOM changed after preview"), MarkdownApplyOutput{}, nil
	}
	currentFingerprint, err := filesystem.FingerprintRegularFileSnapshot(currentDocument.Snapshot)
	if err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}
	if currentFingerprint != prepared.targetFingerprint {
		return errorResultWithCode(ErrCodeConflict, "Markdown target fingerprint changed after preview"), MarkdownApplyOutput{}, nil
	}
	if prepared.changed && isReadOnly(currentDocument.Mode) {
		return errorResultWithCode(ErrCodePermission, "Markdown target became read-only after preview"), MarkdownApplyOutput{}, nil
	}
	if !utf8.ValidString(currentDocument.Text) {
		return errorResultFromError(operation.Wrap(operation.KindEncoding, "markdown_apply", validated.Path, fmt.Errorf("decoded Markdown is not valid UTF-8"))), MarkdownApplyOutput{}, nil
	}
	currentUTF8 := []byte(currentDocument.Text)
	resultUTF8, err := prepared.semantic.Apply(currentUTF8)
	if err != nil {
		return markdownEditErrorResult(err), MarkdownApplyOutput{}, nil
	}
	if !bytes.Equal(resultUTF8, prepared.resultUTF8) {
		return errorResultWithCode(ErrCodeConflict, "Marksplice result changed after preview"), MarkdownApplyOutput{}, nil
	}
	resultData, err := markdownPhysicalResult(currentDocument, currentData, currentUTF8, resultUTF8)
	if err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}
	if filesystem.FingerprintRegularFileData(resultData) != prepared.resultFingerprint {
		return errorResultWithCode(ErrCodeConflict, "prepared Markdown result no longer matches its fingerprint"), MarkdownApplyOutput{}, nil
	}

	output := markdownApplyOutput(prepared, currentFingerprint)
	worstCase := output
	worstCase.Applied = prepared.changed
	worstCase.Changed = prepared.changed
	if prepared.changed {
		worstCase.State = editApplyStateCommitted
	}
	if prepared.changed && persistentBackupRequired(prepared.backupPolicy) {
		worstCase.BackupID = "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	}
	if err := h.checkMarkdownMutationResponseLimit(worstCase, markdownApplyText(worstCase)); err != nil {
		return errorResultFromError(err), MarkdownApplyOutput{}, nil
	}

	matches, err = prepared.identityFile.Matches(validated.Path)
	if err != nil || !matches {
		return errorResultWithCode(ErrCodeConflict, "Markdown target identity changed before apply"), MarkdownApplyOutput{}, nil
	}
	if prepared.changed && persistentBackupRequired(prepared.backupPolicy) {
		if h.backupCapture == nil {
			return errorResultFromError(operation.New(operation.KindConflict, "required backup store is unavailable")), output, nil
		}
		captured, captureErr := h.backupCapture.Capture(ctx, backupstore.CaptureRequest{
			TargetPath:      validated.Path,
			SourceOperation: backupstore.SourceOperationEdit,
			Pinned:          persistentBackupPinned(prepared.backupPolicy),
		})
		if captured.Manifest.BackupID == "" {
			if captureErr == nil {
				captureErr = operation.New(operation.KindFilesystem, "required backup did not commit a manifest")
			}
			return errorResultFromError(captureErr), output, nil
		}
		output.BackupID = captured.Manifest.BackupID
		if !validMarkdownEditPreviewID(output.BackupID) || captured.Manifest.TargetPath != validated.Path ||
			captured.Manifest.SourceOperation != backupstore.SourceOperationEdit || captured.Manifest.ContentFingerprint != prepared.targetFingerprint {
			return errorResultWithCode(ErrCodeConflict, "durable backup does not match the approved Markdown pre-state"), output, nil
		}
		if captureErr != nil {
			slog.Warn("Markdown backup manifest committed but derived index refresh reported an error")
		}
		matches, err = prepared.identityFile.Matches(validated.Path)
		if err != nil || !matches {
			return errorResultWithCode(ErrCodeConflict, "Markdown target identity changed after durable backup"), output, nil
		}
	}

	if !prepared.changed {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: markdownApplyText(output)}}}, output, nil
	}
	if err := ctx.Err(); err != nil {
		return errorResultFromError(operation.Wrap(operation.KindCancelled, "markdown_apply", validated.Path, err)), output, nil
	}
	matches, err = prepared.identityFile.Matches(validated.Path)
	if err != nil || !matches {
		return errorResultWithCode(ErrCodeConflict, "Markdown target identity changed at commit boundary"), output, nil
	}
	if err := prepared.identityFile.Close(); err != nil {
		return errorResultFromError(operation.WrapFilesystem("close_markdown_preview_identity", validated.Path, err)), output, nil
	}
	prepared.identityFile = nil

	if err := h.replaceFile(validated.Path, resultData, filesystem.ReplaceOptions{Mode: currentDocument.Mode.Perm(), Expected: &currentDocument.Snapshot}); err != nil {
		return h.classifyMarkdownApplyFailure(prepared, output, errorResultFromError(fmt.Errorf("failed to write Markdown file: %w", err)))
	}
	post, err := filesystem.CaptureSnapshotWithDigest(validated.Path)
	if err != nil {
		return h.classifyMarkdownApplyFailure(prepared, output, errorResultFromError(err))
	}
	actualFingerprint, err := filesystem.FingerprintRegularFileSnapshot(post)
	if err != nil {
		return h.classifyMarkdownApplyFailure(prepared, output, errorResultFromError(err))
	}
	if actualFingerprint != prepared.resultFingerprint {
		return h.classifyMarkdownApplyFailure(prepared, output, errorResultWithCode(ErrCodeConflict, "applied Markdown file does not match the prepared result fingerprint"))
	}
	output.ActualFingerprint = actualFingerprint
	output.State = editApplyStateCommitted
	output.Changed = true
	output.Applied = true
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: markdownApplyText(output)}}}, output, nil
}

func validateMarkdownEditInput(input MarkdownEditInput) *mcp.CallToolResult {
	if input.Path == "" {
		return errorResultWithCode(ErrCodeInvalidInput, "path is required")
	}
	if len(input.Operations) == 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "at least one Markdown edit operation is required")
	}
	if len(input.Operations) > markdownintelligence.MaxEditOperations {
		return errorResultWithCode(ErrCodeLimit, fmt.Sprintf("Markdown edit operations exceed fixed limit %d", markdownintelligence.MaxEditOperations))
	}
	for _, op := range input.Operations {
		if !isLowerHexDigest(op.TargetID) {
			return errorResultWithCode(ErrCodeInvalidInput, "operations require a snapshot-bound targetId")
		}
		isSectionMove := op.Action == "move" && op.Subject == "section"
		if !isSectionMove && op.AnchorTargetID != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "anchorTargetId is only valid for move/section")
		}
		switch {
		case op.Action == "rename" && op.Subject == "heading":
			if op.Level != 0 || op.Markdown != "" || op.Position != "" || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "rename/heading accepts text only")
			}
		case op.Action == "set" && op.Subject == "heading":
			if op.Level < 1 || op.Level > 6 || op.Text != "" || op.Markdown != "" || op.Position != "" || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "set/heading requires level from 1 to 6")
			}
		case op.Action == "replace" && op.Subject == "paragraph":
			if op.Text != "" || op.Level != 0 || op.Position != "" || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "replace/paragraph accepts markdown only")
			}
		case op.Action == "replace" && op.Subject == "list_item":
			if op.Text != "" || op.Level != 0 || op.Position != "" || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "replace/list_item accepts markdown only")
			}
		case op.Action == "remove" && op.Subject == "paragraph":
			if op.Text != "" || op.Level != 0 || op.Markdown != "" || op.Position != "" || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "remove/paragraph accepts targetId only")
			}
		case op.Action == "insert" && op.Subject == "paragraph":
			if (op.Position != "before" && op.Position != "after") || op.Text != "" || op.Level != 0 || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "insert/paragraph requires position before or after and markdown")
			}
		case op.Action == "insert" && op.Subject == "section":
			if (op.Position != "before" && op.Position != "after" && op.Position != "child") || op.Text != "" || op.Level != 0 || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "insert/section requires position before, after, or child and markdown")
			}
		case op.Action == "remove" && op.Subject == "section":
			if op.Text != "" || op.Level != 0 || op.Markdown != "" || op.Position != "" || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "remove/section accepts targetId only")
			}
		case op.Action == "replace" && op.Subject == "section":
			if (op.Part != "body" && op.Part != "subtree") || op.Text != "" || op.Level != 0 || op.Position != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "replace/section requires part body or subtree and markdown")
			}
		case isSectionMove:
			if !isLowerHexDigest(op.AnchorTargetID) || (op.Position != "before" && op.Position != "after") || op.Text != "" || op.Level != 0 || op.Markdown != "" || op.Part != "" {
				return errorResultWithCode(ErrCodeInvalidInput, "move/section requires anchorTargetId and position before or after")
			}
		default:
			return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
		}
	}
	if _, err := normalizeEditBackupPolicy(input.BackupPolicy); err != nil {
		return errorResultFromError(err)
	}
	return nil
}

func markdownEditErrorResult(err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, marksplice.ErrNodeNotFound):
		return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown target was not found")
	case errors.Is(err, marksplice.ErrInvalidTargetKind):
		return markdownSemanticErrorResult(ErrCodeUnsupported, MarkdownErrUnsupportedTargetKind, "Markdown target kind does not support the requested operation")
	case errors.Is(err, marksplice.ErrInvalidReplacement):
		return markdownSemanticErrorResult(ErrCodeInvalidInput, MarkdownErrInvalidStructure, "Marksplice rejected the requested Markdown structure")
	case errors.Is(err, marksplice.ErrSourceConflict):
		return markdownSemanticErrorResult(ErrCodeConflict, MarkdownErrSourceConflict, "Markdown source changed after preparation")
	case errors.Is(err, marksplice.ErrInvalidQuery):
		return markdownSemanticErrorResult(ErrCodeInvalidInput, MarkdownErrInvalidQuery, "Markdown operation is invalid")
	default:
		return errorResultWithCode(ErrCodeOperationFailed, "Markdown operation failed")
	}
}

func markdownEditOutputFromPreview(preview *markdownEditPreview, operations []MarkdownEditOperation) MarkdownEditOutput {
	return MarkdownEditOutput{
		PreviewID:         preview.id,
		CreatedAt:         preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt:         preview.expiresAt.Format(timeRFC3339Nano),
		Path:              preview.prepared.requestedPath,
		Operations:        append([]MarkdownEditOperation(nil), operations...),
		TargetFingerprint: preview.prepared.targetFingerprint,
		ResultFingerprint: preview.prepared.resultFingerprint,
		Encoding:          preview.prepared.encoding,
		HasBOM:            preview.prepared.hasBOM,
		BOMType:           preview.prepared.bomType,
		LineEndingStyle:   preview.prepared.lineEndingStyle,
		BackupPolicy:      preview.prepared.backupPolicy,
		Diff:              preview.prepared.diff,
		Changed:           preview.prepared.changed,
	}
}

func markdownApplyOutput(prepared preparedMarkdownEdit, currentFingerprint string) MarkdownApplyOutput {
	return MarkdownApplyOutput{
		Path:              prepared.requestedPath,
		TargetFingerprint: prepared.targetFingerprint,
		ResultFingerprint: prepared.resultFingerprint,
		ActualFingerprint: currentFingerprint,
		Encoding:          prepared.encoding,
		HasBOM:            prepared.hasBOM,
		BOMType:           prepared.bomType,
		LineEndingStyle:   prepared.lineEndingStyle,
		BackupPolicy:      prepared.backupPolicy,
		State:             editApplyStateUnchanged,
	}
}

func markdownEditPreviewText(output MarkdownEditOutput) string {
	return fmt.Sprintf("Markdown edit preview prepared.\nPreview ID: %s\nExpires: %s\nTarget fingerprint: %s\nResult fingerprint: %s\n\n%s",
		output.PreviewID, output.ExpiresAt, output.TargetFingerprint, output.ResultFingerprint, output.Diff)
}

func markdownApplyText(output MarkdownApplyOutput) string {
	return fmt.Sprintf("Markdown preview applied.\nState: %s\nTarget fingerprint: %s\nResult fingerprint: %s\nActual fingerprint: %s",
		output.State, output.TargetFingerprint, output.ResultFingerprint, output.ActualFingerprint)
}

func (h *Handler) checkMarkdownMutationResponseLimit(output any, text string) error {
	encoded, err := json.Marshal(output)
	if err != nil {
		return operation.Wrap(operation.KindUnknown, "encode_markdown_mutation_output", "", err)
	}
	if int64(len(encoded))+int64(len(text)) > h.maxOutputBytes() {
		return operation.New(operation.KindLimit, fmt.Sprintf("Markdown mutation output exceeds limit %d bytes", h.maxOutputBytes()))
	}
	return nil
}

func (h *Handler) classifyMarkdownApplyFailure(prepared preparedMarkdownEdit, output MarkdownApplyOutput, failure *mcp.CallToolResult) (*mcp.CallToolResult, MarkdownApplyOutput, error) {
	output.Applied = false
	output.State = editApplyStateUnknown
	output.ActualFingerprint = ""
	output.Changed = false

	classificationCtx, cancel := context.WithTimeout(context.Background(), editApplyClassificationTimeout)
	defer cancel()
	validation := h.ValidatePath(prepared.requestedPath)
	if validation.Ok() && validation.Path == prepared.resolvedPath && classificationCtx.Err() == nil {
		post, err := filesystem.CaptureRegularFileSnapshotBounded(classificationCtx, validation.Path, h.maxFileBytes())
		if err == nil {
			if actual, fingerprintErr := filesystem.FingerprintRegularFileSnapshot(post); fingerprintErr == nil {
				output.ActualFingerprint = actual
				switch actual {
				case prepared.targetFingerprint:
					output.State = editApplyStateUnchanged
				case prepared.resultFingerprint:
					output.State = editApplyStateCommitted
					output.Changed = prepared.changed
				default:
					output.State = editApplyStateUnknown
					output.Changed = true
				}
			}
		}
	}
	if output.State == editApplyStateUnchanged {
		return failure, output, nil
	}
	message := "Markdown apply failed after the target may have changed"
	if failure != nil && len(failure.Content) > 0 {
		if content, ok := failure.Content[0].(*mcp.TextContent); ok && content.Text != "" {
			message = content.Text
		}
	}
	return errorResultWithCode(ErrCodePartialCommit, message), output, nil
}
