package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	Action         string   `json:"action"`
	Subject        string   `json:"subject"`
	TargetID       string   `json:"targetId,omitempty"`
	AnchorTargetID string   `json:"anchorTargetId,omitempty"`
	Text           string   `json:"text,omitempty"`
	Format         string   `json:"format,omitempty"`
	Key            string   `json:"key,omitempty"`
	Value          string   `json:"value,omitempty"`
	Label          string   `json:"label,omitempty"`
	Destination    string   `json:"destination,omitempty"`
	Title          string   `json:"title,omitempty"`
	Body           string   `json:"body,omitempty"`
	Column         *int     `json:"column,omitempty"`
	Alignment      string   `json:"alignment,omitempty"`
	Alignments     []string `json:"alignments,omitempty"`
	Level          int      `json:"level,omitempty"`
	Markdown       string   `json:"markdown,omitempty"`
	Position       string   `json:"position,omitempty"`
	Part           string   `json:"part,omitempty"`
	Checked        *bool    `json:"checked,omitempty"`
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
		case operationInput.Action == "create" && operationInput.Subject == "front_matter":
			format, _ := markdownFrontMatterFormat(operationInput.Format)
			preparedChange, prepareErr = snapshot.PrepareAddFrontMatter(format)
		case operationInput.Action == "remove" && operationInput.Subject == "front_matter":
			preparedChange, prepareErr = snapshot.PrepareRemoveFrontMatter()
		case operationInput.Action == "create" && operationInput.Subject == "front_matter_field":
			preparedChange, prepareErr = snapshot.PrepareAppendFrontMatterField([]byte(operationInput.Key), []byte(operationInput.Value))
		case operationInput.Action == "create" && operationInput.Subject == "reference_definition":
			preparedChange, prepareErr = snapshot.PrepareAppendReferenceDefinition([]byte(operationInput.Label), []byte(operationInput.Destination), []byte(operationInput.Title))
		case operationInput.Action == "create" && operationInput.Subject == "footnote_definition":
			preparedChange, prepareErr = snapshot.PrepareAppendFootnoteDefinition([]byte(operationInput.Label), []byte(operationInput.Body))
		case operationInput.Action == "rename" && operationInput.Subject == "heading":
			preparedChange, prepareErr = snapshot.PrepareRenameHeading(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "rename" && operationInput.Subject == "reference_definition":
			preparedChange, prepareErr = snapshot.PrepareRenameReferenceDefinition(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "retarget" && operationInput.Subject == "reference_occurrence":
			preparedChange, prepareErr = snapshot.PrepareRetargetReferenceOccurrence(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "rename" && operationInput.Subject == "front_matter_field":
			preparedChange, prepareErr = snapshot.PrepareRenameFrontMatterField(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "rename" && operationInput.Subject == "footnote_definition":
			preparedChange, prepareErr = snapshot.PrepareRenameFootnoteDefinition(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "set" && operationInput.Subject == "heading":
			preparedChange, prepareErr = snapshot.PrepareSetHeadingLevel(operationInput.TargetID, operationInput.Level)
		case operationInput.Action == "set" && operationInput.Subject == "task":
			preparedChange, prepareErr = snapshot.PrepareSetTaskChecked(operationInput.TargetID, *operationInput.Checked)
		case operationInput.Action == "set" && operationInput.Subject == "table" && operationInput.Part == "column_alignment":
			alignment, _ := markdownTableAlignment(operationInput.Alignment)
			preparedChange, prepareErr = snapshot.PrepareSetTableColumnAlignment(operationInput.TargetID, *operationInput.Column, alignment)
		case operationInput.Action == "set" && operationInput.Subject == "table" && operationInput.Part == "alignments":
			alignments := make([]marksplice.TableAlignment, len(operationInput.Alignments))
			for index, value := range operationInput.Alignments {
				alignments[index], _ = markdownTableAlignment(value)
			}
			preparedChange, prepareErr = snapshot.PrepareSetTableAlignments(operationInput.TargetID, alignments)
		case operationInput.Action == "replace" && operationInput.Subject == "paragraph":
			preparedChange, prepareErr = snapshot.PrepareReplaceParagraph(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "replace" && operationInput.Subject == "code_span":
			preparedChange, prepareErr = snapshot.PrepareReplaceCodeSpan(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "strikethrough":
			preparedChange, prepareErr = snapshot.PrepareReplaceStrikethrough(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "emphasis":
			preparedChange, prepareErr = snapshot.PrepareReplaceEmphasis(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "strong":
			preparedChange, prepareErr = snapshot.PrepareReplaceStrong(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "fenced_code" && operationInput.Part == "body":
			preparedChange, prepareErr = snapshot.PrepareReplaceFencedCode(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "set" && operationInput.Subject == "fenced_code" && operationInput.Part == "info":
			preparedChange, prepareErr = snapshot.PrepareSetFencedBlockInfo(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "inline_link" && operationInput.Part == "destination":
			preparedChange, prepareErr = snapshot.PrepareReplaceInlineLinkDestination(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "inline_link" && operationInput.Part == "label":
			preparedChange, prepareErr = snapshot.PrepareReplaceInlineLinkLabel(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "inline_link" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareReplaceInlineLinkTitle(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "add" && operationInput.Subject == "inline_link" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareAddInlineLinkTitle(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "remove" && operationInput.Subject == "inline_link" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareRemoveInlineLinkTitle(operationInput.TargetID)
		case operationInput.Action == "replace" && operationInput.Subject == "image" && operationInput.Part == "destination":
			preparedChange, prepareErr = snapshot.PrepareReplaceImageDestination(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "image" && operationInput.Part == "alt":
			preparedChange, prepareErr = snapshot.PrepareReplaceImageAlt(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "image" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareReplaceImageTitle(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "add" && operationInput.Subject == "image" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareAddImageTitle(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "remove" && operationInput.Subject == "image" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareRemoveImageTitle(operationInput.TargetID)
		case operationInput.Action == "replace" && operationInput.Subject == "autolink":
			preparedChange, prepareErr = snapshot.PrepareReplaceAutoLink(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "front_matter_field":
			preparedChange, prepareErr = snapshot.PrepareReplaceFrontMatterValue(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "html_comment":
			preparedChange, prepareErr = snapshot.PrepareReplaceHTMLComment(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "html_anchor":
			preparedChange, prepareErr = snapshot.PrepareReplaceHTMLAnchor(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "math_expression":
			preparedChange, prepareErr = snapshot.PrepareReplaceMathExpression(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "sync" && operationInput.Subject == "toc":
			preparedChange, prepareErr = snapshot.PrepareSyncTOC(operationInput.TargetID)
		case operationInput.Action == "set" && operationInput.Subject == "alert":
			kind, _ := markdownAlertKind(operationInput.Text)
			preparedChange, prepareErr = snapshot.PrepareSetAlertKind(operationInput.TargetID, kind)
		case operationInput.Action == "replace" && operationInput.Subject == "blockquote":
			preparedChange, prepareErr = snapshot.PrepareReplaceBlockquoteContent(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "alert":
			preparedChange, prepareErr = snapshot.PrepareReplaceAlertBody(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "footnote_definition":
			preparedChange, prepareErr = snapshot.PrepareReplaceFootnoteDefinitionBody(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "remove" && operationInput.Subject == "front_matter_field":
			preparedChange, prepareErr = snapshot.PrepareRemoveFrontMatterField(operationInput.TargetID)
		case operationInput.Action == "remove" && operationInput.Subject == "thematic_break":
			preparedChange, prepareErr = snapshot.PrepareRemoveThematicBreak(operationInput.TargetID)
		case operationInput.Action == "remove" && operationInput.Subject == "blockquote":
			preparedChange, prepareErr = snapshot.PrepareRemoveBlockquote(operationInput.TargetID)
		case operationInput.Action == "remove" && operationInput.Subject == "footnote_definition":
			preparedChange, prepareErr = snapshot.PrepareRemoveFootnoteDefinition(operationInput.TargetID)
		case operationInput.Action == "replace" && operationInput.Subject == "reference_definition" && operationInput.Part == "destination":
			preparedChange, prepareErr = snapshot.PrepareReplaceReferenceDefinitionDestination(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "replace" && operationInput.Subject == "reference_definition" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareReplaceReferenceDefinitionTitle(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "add" && operationInput.Subject == "reference_definition" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareAddReferenceDefinitionTitle(operationInput.TargetID, []byte(operationInput.Text))
		case operationInput.Action == "remove" && operationInput.Subject == "reference_definition" && operationInput.Part == "title":
			preparedChange, prepareErr = snapshot.PrepareRemoveReferenceDefinitionTitle(operationInput.TargetID)
		case operationInput.Action == "remove" && operationInput.Subject == "reference_definition":
			preparedChange, prepareErr = snapshot.PrepareRemoveReferenceDefinition(operationInput.TargetID)
		case operationInput.Action == "replace" && operationInput.Subject == "list_item" && operationInput.Part == "subtree":
			preparedChange, prepareErr = snapshot.PrepareReplaceListItemSubtree(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "replace" && operationInput.Subject == "list_item":
			preparedChange, prepareErr = snapshot.PrepareReplaceListItem(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "insert" && operationInput.Subject == "list_item" && operationInput.Position == "before":
			preparedChange, prepareErr = snapshot.PrepareInsertListItemBefore(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "insert" && operationInput.Subject == "list_item" && operationInput.Position == "after":
			preparedChange, prepareErr = snapshot.PrepareInsertListItemAfter(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "insert" && operationInput.Subject == "list_item" && operationInput.Position == "child":
			preparedChange, prepareErr = snapshot.PrepareAppendListItemChild(operationInput.TargetID, []byte(operationInput.Markdown))
		case operationInput.Action == "move" && operationInput.Subject == "list_item" && operationInput.Position == "before":
			preparedChange, prepareErr = snapshot.PrepareMoveListItemBefore(operationInput.TargetID, operationInput.AnchorTargetID)
		case operationInput.Action == "move" && operationInput.Subject == "list_item" && operationInput.Position == "after":
			preparedChange, prepareErr = snapshot.PrepareMoveListItemAfter(operationInput.TargetID, operationInput.AnchorTargetID)
		case operationInput.Action == "remove" && operationInput.Subject == "list_item":
			preparedChange, prepareErr = snapshot.PrepareRemoveListItem(operationInput.TargetID)
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
	preview, err := h.markdownPreviews.putEdit(prepared)
	if err != nil {
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	keepIdentity = true
	output := markdownEditOutputFromPreview(preview, input.Operations)
	text := markdownEditPreviewText(output)
	if err := h.checkMarkdownMutationResponseLimit(output, text); err != nil {
		h.markdownPreviews.discard(preview.id)
		return errorResultFromError(err), MarkdownEditOutput{}, nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, output, nil
}

func markdownTableAlignment(value string) (marksplice.TableAlignment, bool) {
	switch value {
	case "default":
		return marksplice.TableAlignmentDefault, true
	case "left":
		return marksplice.TableAlignmentLeft, true
	case "right":
		return marksplice.TableAlignmentRight, true
	case "center":
		return marksplice.TableAlignmentCenter, true
	default:
		return marksplice.TableAlignmentDefault, false
	}
}

func markdownFrontMatterFormat(value string) (marksplice.FrontMatterFormat, bool) {
	switch value {
	case "yaml":
		return marksplice.FrontMatterFormatYAML, true
	case "toml":
		return marksplice.FrontMatterFormatTOML, true
	default:
		return marksplice.FrontMatterFormatUnknown, false
	}
}

func markdownAlertKind(value string) (marksplice.AlertKind, bool) {
	switch value {
	case "note":
		return marksplice.AlertKindNote, true
	case "tip":
		return marksplice.AlertKindTip, true
	case "important":
		return marksplice.AlertKindImportant, true
	case "warning":
		return marksplice.AlertKindWarning, true
	case "caution":
		return marksplice.AlertKindCaution, true
	default:
		return marksplice.AlertKindUnknown, false
	}
}

func markdownEditErrorResult(err error) *mcp.CallToolResult {
	switch {
	case errors.Is(err, marksplice.ErrNodeNotFound):
		return markdownSemanticErrorResult(ErrCodeNotFound, MarkdownErrTargetNotFound, "Markdown target was not found")
	case errors.Is(err, marksplice.ErrInvalidTargetKind):
		return markdownSemanticErrorResult(ErrCodeUnsupported, MarkdownErrUnsupportedTargetKind, "Markdown target kind does not support the requested operation")
	case errors.Is(err, marksplice.ErrInvalidReplacement), errors.Is(err, marksplice.ErrInvalidConstruction):
		return markdownSemanticErrorResult(ErrCodeInvalidInput, MarkdownErrInvalidStructure, "Marksplice rejected the requested Markdown structure")
	case errors.Is(err, marksplice.ErrSourceConflict):
		return markdownSemanticErrorResult(ErrCodeConflict, MarkdownErrSourceConflict, "Markdown source changed after preparation")
	case errors.Is(err, marksplice.ErrInvalidQuery):
		return markdownSemanticErrorResult(ErrCodeInvalidInput, MarkdownErrInvalidQuery, "Markdown operation is invalid")
	default:
		return errorResultWithCode(ErrCodeOperationFailed, "Markdown operation failed")
	}
}

func markdownEditOutputFromPreview(preview *markdownPreview, operations []MarkdownEditOperation) MarkdownEditOutput {
	if preview == nil || preview.edit == nil {
		return MarkdownEditOutput{}
	}
	prepared := preview.edit
	return MarkdownEditOutput{
		PreviewID:         preview.id,
		CreatedAt:         preview.createdAt.Format(timeRFC3339Nano),
		ExpiresAt:         preview.expiresAt.Format(timeRFC3339Nano),
		Path:              prepared.requestedPath,
		Operations:        append([]MarkdownEditOperation(nil), operations...),
		TargetFingerprint: prepared.targetFingerprint,
		ResultFingerprint: prepared.resultFingerprint,
		Encoding:          prepared.encoding,
		HasBOM:            prepared.hasBOM,
		BOMType:           prepared.bomType,
		LineEndingStyle:   prepared.lineEndingStyle,
		BackupPolicy:      prepared.backupPolicy,
		Diff:              prepared.diff,
		Changed:           prepared.changed,
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
