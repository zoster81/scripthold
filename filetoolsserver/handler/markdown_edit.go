package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/marksplice"
	"github.com/zoster81/scripthold/internal/filesystem"
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
	Path              string                        `json:"path"`
	TargetFingerprint string                        `json:"targetFingerprint"`
	ResultFingerprint string                        `json:"resultFingerprint"`
	ActualFingerprint string                        `json:"actualFingerprint,omitempty"`
	Encoding          string                        `json:"encoding"`
	HasBOM            bool                          `json:"hasBOM"`
	BOMType           string                        `json:"bomType,omitempty"`
	LineEndingStyle   string                        `json:"lineEndingStyle"`
	BackupPolicy      string                        `json:"backupPolicy,omitempty"`
	BackupID          string                        `json:"backupId,omitempty"`
	State             string                        `json:"state"`
	Changed           bool                          `json:"changed"`
	Applied           bool                          `json:"applied"`
	Workspace         *MarkdownWorkspaceApplyOutput `json:"workspace,omitempty"`
}

// MarkdownWorkspaceApplyDocumentOutput reports observed state for one workspace
// repair target.
type MarkdownWorkspaceApplyDocumentOutput struct {
	Document          string `json:"document"`
	Path              string `json:"path"`
	TargetFingerprint string `json:"targetFingerprint"`
	ResultFingerprint string `json:"resultFingerprint"`
	ActualFingerprint string `json:"actualFingerprint,omitempty"`
	Encoding          string `json:"encoding"`
	HasBOM            bool   `json:"hasBOM"`
	BOMType           string `json:"bomType,omitempty"`
	LineEndingStyle   string `json:"lineEndingStyle"`
	BackupID          string `json:"backupId,omitempty"`
	State             string `json:"state"`
	Changed           bool   `json:"changed"`
	Applied           bool   `json:"applied"`
	ErrorCode         string `json:"errorCode,omitempty"`
	Error             string `json:"error,omitempty"`
}

// MarkdownWorkspaceApplyOutput reports observed multi-file apply state.
type MarkdownWorkspaceApplyOutput struct {
	BackupPolicy   string                                 `json:"backupPolicy,omitempty"`
	TotalTargets   int                                    `json:"totalTargets"`
	CommittedCount int                                    `json:"committedCount"`
	UnchangedCount int                                    `json:"unchangedCount"`
	UnknownCount   int                                    `json:"unknownCount"`
	BackupCount    int                                    `json:"backupCount"`
	PartialCommit  bool                                   `json:"partialCommit"`
	FailedIndex    *int                                   `json:"failedIndex,omitempty"`
	FailedDocument string                                 `json:"failedDocument,omitempty"`
	FailureCode    string                                 `json:"failureCode,omitempty"`
	FailureMessage string                                 `json:"failureMessage,omitempty"`
	Results        []MarkdownWorkspaceApplyDocumentOutput `json:"results"`
}

func (output MarkdownApplyOutput) MarshalJSON() ([]byte, error) {
	if output.Workspace != nil {
		return json.Marshal(struct {
			Workspace *MarkdownWorkspaceApplyOutput `json:"workspace"`
		}{Workspace: output.Workspace})
	}
	type markdownApplyOutputAlias MarkdownApplyOutput
	return json.Marshal(markdownApplyOutputAlias(output))
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
	source, failure := h.openMarkdownEditSource(ctx, input)
	if failure != nil {
		return failure, MarkdownEditOutput{}, nil
	}
	keepIdentity := false
	defer func() {
		if !keepIdentity {
			_ = source.identityFile.Close()
		}
	}()

	semantic, resultUTF8, failure := prepareMarkdownEditSemantic(source.sourceUTF8, input.Operations)
	if failure != nil {
		return failure, MarkdownEditOutput{}, nil
	}
	prepared, failure := h.buildPreparedMarkdownEdit(ctx, input, backupPolicy, source, semantic, resultUTF8)
	if failure != nil {
		return failure, MarkdownEditOutput{}, nil
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
