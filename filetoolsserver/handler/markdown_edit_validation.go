package handler

import (
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

type markdownEditFieldMask uint32

const (
	markdownFieldAnchor markdownEditFieldMask = 1 << iota
	markdownFieldText
	markdownFieldFormat
	markdownFieldKey
	markdownFieldValue
	markdownFieldLabel
	markdownFieldDestination
	markdownFieldTitle
	markdownFieldBody
	markdownFieldColumn
	markdownFieldAlignment
	markdownFieldAlignments
	markdownFieldLevel
	markdownFieldMarkdown
	markdownFieldPosition
	markdownFieldPart
	markdownFieldChecked
)

const markdownConstructionFields = markdownFieldFormat | markdownFieldKey | markdownFieldValue |
	markdownFieldLabel | markdownFieldDestination | markdownFieldTitle | markdownFieldBody

var markdownRemoveTargetOnlyMessages = map[string]string{
	"front_matter_field":  "remove/front_matter_field accepts targetId only",
	"thematic_break":      "remove/thematic_break accepts targetId only",
	"blockquote":          "remove/blockquote accepts targetId only",
	"footnote_definition": "remove/footnote_definition accepts targetId only",
	"list_item":           "remove/list_item accepts targetId only",
	"paragraph":           "remove/paragraph accepts targetId only",
	"section":             "remove/section accepts targetId only",
}

var markdownReplaceRequiredTextMessages = map[string]string{
	"autolink":            "replace/autolink accepts text only",
	"front_matter_field":  "replace/front_matter_field requires non-empty text",
	"html_comment":        "raw HTML replacement requires non-empty text",
	"html_anchor":         "raw HTML replacement requires non-empty text",
	"math_expression":     "replace/math_expression requires non-empty text",
	"blockquote":          "replace/blockquote requires non-empty text",
	"alert":               "replace/alert requires non-empty text",
	"footnote_definition": "replace/footnote_definition requires non-empty text",
}

type markdownPartTextRule struct {
	parts   []string
	message string
}

var markdownReplacePartTextRules = map[string]markdownPartTextRule{
	"inline_link": {
		parts:   []string{"destination", "label", "title"},
		message: "replace/inline_link requires part destination, label, or title and text",
	},
	"image": {
		parts:   []string{"destination", "alt", "title"},
		message: "replace/image requires part destination, alt, or title and text",
	},
	"reference_definition": {
		parts:   []string{"destination", "title"},
		message: "replace/reference_definition requires part destination or title and text",
	},
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
	if result := validateMarkdownEditComposition(input.Operations); result != nil {
		return result
	}
	for _, op := range input.Operations {
		if result := validateMarkdownEditOperation(op); result != nil {
			return result
		}
	}
	if _, err := normalizeEditBackupPolicy(input.BackupPolicy); err != nil {
		return errorResultFromError(err)
	}
	return nil
}

func validateMarkdownEditComposition(operations []MarkdownEditOperation) *mcp.CallToolResult {
	if len(operations) == 1 {
		return nil
	}
	for _, op := range operations {
		if op.Action == "sync" && op.Subject == "toc" {
			return errorResultWithCode(ErrCodeInvalidInput, "sync/toc must be the only Markdown edit operation")
		}
	}
	return nil
}

func validateMarkdownEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if result := validateMarkdownEditOperationCommon(op); result != nil {
		return result
	}
	switch op.Action {
	case "create":
		return validateMarkdownCreateEditOperation(op)
	case "remove":
		return validateMarkdownRemoveEditOperation(op)
	case "rename":
		return validateMarkdownRenameEditOperation(op)
	case "set":
		return validateMarkdownSetEditOperation(op)
	case "sync":
		return validateMarkdownSyncEditOperation(op)
	case "replace":
		return validateMarkdownReplaceEditOperation(op)
	case "add":
		return validateMarkdownAddEditOperation(op)
	case "insert":
		return validateMarkdownInsertEditOperation(op)
	case "move":
		return validateMarkdownMoveEditOperation(op)
	case "retarget":
		return validateMarkdownRetargetEditOperation(op)
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
}

func validateMarkdownEditOperationCommon(op MarkdownEditOperation) *mcp.CallToolResult {
	fields := markdownEditFields(op)
	targetless := markdownEditOperationTargetless(op)
	if targetless {
		if op.TargetID != "" {
			return errorResultWithCode(ErrCodeInvalidInput, "document-level operations do not accept targetId")
		}
	} else {
		if !isLowerHexDigest(op.TargetID) {
			return errorResultWithCode(ErrCodeInvalidInput, "operations require a snapshot-bound targetId")
		}
		if fields&markdownConstructionFields != 0 {
			return errorResultWithCode(ErrCodeInvalidInput, "construction fields are only valid for create operations")
		}
	}
	if !markdownEditOperationMove(op) && fields&markdownFieldAnchor != 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "anchorTargetId is only valid for move/section or move/list_item")
	}
	if !markdownEditOperationTaskSet(op) && fields&markdownFieldChecked != 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "checked is only valid for set/task")
	}
	if !markdownEditOperationTableAlignment(op) && fields&(markdownFieldColumn|markdownFieldAlignment|markdownFieldAlignments) != 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "table alignment fields are only valid for set/table alignment operations")
	}
	return nil
}

func markdownEditOperationTargetless(op MarkdownEditOperation) bool {
	if op.Action == "remove" && op.Subject == "front_matter" {
		return true
	}
	if op.Action != "create" {
		return false
	}
	switch op.Subject {
	case "front_matter", "front_matter_field", "reference_definition", "footnote_definition":
		return true
	default:
		return false
	}
}

func markdownEditOperationMove(op MarkdownEditOperation) bool {
	return op.Action == "move" && (op.Subject == "section" || op.Subject == "list_item")
}

func markdownEditOperationTaskSet(op MarkdownEditOperation) bool {
	return op.Action == "set" && op.Subject == "task"
}

func markdownEditOperationTableAlignment(op MarkdownEditOperation) bool {
	return op.Action == "set" && op.Subject == "table" && (op.Part == "column_alignment" || op.Part == "alignments")
}

func markdownEditFields(op MarkdownEditOperation) markdownEditFieldMask {
	values := []struct {
		mask    markdownEditFieldMask
		present bool
	}{
		{markdownFieldAnchor, op.AnchorTargetID != ""},
		{markdownFieldText, op.Text != ""},
		{markdownFieldFormat, op.Format != ""},
		{markdownFieldKey, op.Key != ""},
		{markdownFieldValue, op.Value != ""},
		{markdownFieldLabel, op.Label != ""},
		{markdownFieldDestination, op.Destination != ""},
		{markdownFieldTitle, op.Title != ""},
		{markdownFieldBody, op.Body != ""},
		{markdownFieldColumn, op.Column != nil},
		{markdownFieldAlignment, op.Alignment != ""},
		{markdownFieldAlignments, len(op.Alignments) != 0},
		{markdownFieldLevel, op.Level != 0},
		{markdownFieldMarkdown, op.Markdown != ""},
		{markdownFieldPosition, op.Position != ""},
		{markdownFieldPart, op.Part != ""},
		{markdownFieldChecked, op.Checked != nil},
	}
	var fields markdownEditFieldMask
	for _, value := range values {
		if value.present {
			fields |= value.mask
		}
	}
	return fields
}

func markdownEditHasOnly(op MarkdownEditOperation, allowed markdownEditFieldMask) bool {
	return markdownEditFields(op)&^allowed == 0
}

func markdownEditRequiresOnly(op MarkdownEditOperation, allowed markdownEditFieldMask, required ...string) bool {
	for _, value := range required {
		if value == "" {
			return false
		}
	}
	return markdownEditHasOnly(op, allowed)
}

func validateMarkdownCreateEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	switch op.Subject {
	case "front_matter":
		if _, ok := markdownFrontMatterFormat(op.Format); !ok {
			return errorResultWithCode(ErrCodeInvalidInput, "create/front_matter requires format yaml or toml")
		}
		if !markdownEditHasOnly(op, markdownFieldFormat) {
			return errorResultWithCode(ErrCodeInvalidInput, "create/front_matter requires format yaml or toml")
		}
	case "front_matter_field":
		if !markdownEditRequiresOnly(op, markdownFieldKey|markdownFieldValue, op.Key, op.Value) {
			return errorResultWithCode(ErrCodeInvalidInput, "create/front_matter_field requires key and value")
		}
	case "reference_definition":
		if !markdownEditRequiresOnly(op, markdownFieldLabel|markdownFieldDestination|markdownFieldTitle, op.Label, op.Destination) {
			return errorResultWithCode(ErrCodeInvalidInput, "create/reference_definition requires label and destination; title is optional")
		}
	case "footnote_definition":
		if !markdownEditRequiresOnly(op, markdownFieldLabel|markdownFieldBody, op.Label, op.Body) {
			return errorResultWithCode(ErrCodeInvalidInput, "create/footnote_definition requires label and body")
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	return nil
}

func validateMarkdownRemoveEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if op.Subject == "front_matter" {
		if !markdownEditHasOnly(op, 0) {
			return errorResultWithCode(ErrCodeInvalidInput, "remove/front_matter accepts no operation fields")
		}
		return nil
	}
	if (op.Subject == "inline_link" || op.Subject == "image") && op.Part == "title" {
		if !markdownEditHasOnly(op, markdownFieldPart) {
			return errorResultWithCode(ErrCodeInvalidInput, "remove title accepts targetId and part title only")
		}
		return nil
	}
	if op.Subject == "reference_definition" {
		if op.Part == "title" {
			if !markdownEditHasOnly(op, markdownFieldPart) {
				return errorResultWithCode(ErrCodeInvalidInput, "remove/reference_definition title accepts targetId and part title only")
			}
			return nil
		}
		if !markdownEditHasOnly(op, 0) {
			return errorResultWithCode(ErrCodeInvalidInput, "remove/reference_definition accepts targetId only")
		}
		return nil
	}
	message, ok := markdownRemoveTargetOnlyMessages[op.Subject]
	if !ok {
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	if !markdownEditHasOnly(op, 0) {
		return errorResultWithCode(ErrCodeInvalidInput, message)
	}
	return nil
}

func validateMarkdownRenameEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	switch op.Subject {
	case "heading":
		if !markdownEditHasOnly(op, markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "rename/heading accepts text only")
		}
	case "reference_definition":
		if op.Text == "" || !markdownEditHasOnly(op, markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "rename/reference_definition requires non-empty text")
		}
	case "front_matter_field":
		if op.Text == "" || !markdownEditHasOnly(op, markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "rename/front_matter_field requires non-empty text")
		}
	case "footnote_definition":
		if op.Text == "" || !markdownEditHasOnly(op, markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "rename/footnote_definition requires non-empty text")
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	return nil
}

func validateMarkdownSetEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	switch op.Subject {
	case "heading":
		if op.Level < 1 || op.Level > 6 || !markdownEditHasOnly(op, markdownFieldLevel) {
			return errorResultWithCode(ErrCodeInvalidInput, "set/heading requires level from 1 to 6")
		}
	case "task":
		if op.Checked == nil || !markdownEditHasOnly(op, markdownFieldChecked) {
			return errorResultWithCode(ErrCodeInvalidInput, "set/task requires checked")
		}
	case "table":
		return validateMarkdownSetTableEditOperation(op)
	case "fenced_code":
		if op.Part != "info" || !markdownEditHasOnly(op, markdownFieldPart|markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "set/fenced_code requires part info and text")
		}
	case "alert":
		if _, ok := markdownAlertKind(op.Text); !ok || !markdownEditHasOnly(op, markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "set/alert requires text note, tip, important, warning, or caution")
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	return nil
}

func validateMarkdownSetTableEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	switch op.Part {
	case "column_alignment":
		if op.Column == nil || *op.Column < 0 || op.Alignment == "" ||
			!markdownEditHasOnly(op, markdownFieldPart|markdownFieldColumn|markdownFieldAlignment) {
			return errorResultWithCode(ErrCodeInvalidInput, "set/table column_alignment requires zero-based column and alignment")
		}
		if _, ok := markdownTableAlignment(op.Alignment); !ok {
			return errorResultWithCode(ErrCodeInvalidInput, "table alignment must be default, left, right, or center")
		}
	case "alignments":
		if len(op.Alignments) == 0 || !markdownEditHasOnly(op, markdownFieldPart|markdownFieldAlignments) {
			return errorResultWithCode(ErrCodeInvalidInput, "set/table alignments requires a non-empty alignment vector")
		}
		for _, alignment := range op.Alignments {
			if _, ok := markdownTableAlignment(alignment); !ok {
				return errorResultWithCode(ErrCodeInvalidInput, "table alignments must contain only default, left, right, or center")
			}
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	return nil
}

func validateMarkdownSyncEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if op.Subject != "toc" {
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	if !markdownEditHasOnly(op, 0) {
		return errorResultWithCode(ErrCodeInvalidInput, "sync/toc accepts targetId only")
	}
	return nil
}

func validateMarkdownReplaceEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if result, handled := validateMarkdownReplaceTextRule(op); handled {
		return result
	}
	switch op.Subject {
	case "paragraph":
		if !markdownEditHasOnly(op, markdownFieldMarkdown) {
			return errorResultWithCode(ErrCodeInvalidInput, "replace/paragraph accepts markdown only")
		}
	case "code_span", "strikethrough", "emphasis", "strong":
		if !markdownEditHasOnly(op, markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "simple inline replacement accepts text only")
		}
	case "fenced_code":
		return validateMarkdownReplaceFencedCodeOperation(op)
	case "list_item":
		return validateMarkdownReplaceListItemOperation(op)
	case "section":
		return validateMarkdownReplaceSectionOperation(op)
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	return nil
}

func validateMarkdownReplaceTextRule(op MarkdownEditOperation) (*mcp.CallToolResult, bool) {
	if message, ok := markdownReplaceRequiredTextMessages[op.Subject]; ok {
		if !markdownEditRequiresOnly(op, markdownFieldText, op.Text) {
			return errorResultWithCode(ErrCodeInvalidInput, message), true
		}
		return nil, true
	}
	rule, ok := markdownReplacePartTextRules[op.Subject]
	if !ok {
		return nil, false
	}
	if !markdownAllowedPart(op.Part, rule.parts...) {
		return errorResultWithCode(ErrCodeInvalidInput, rule.message), true
	}
	if !markdownEditRequiresOnly(op, markdownFieldPart|markdownFieldText, op.Text) {
		return errorResultWithCode(ErrCodeInvalidInput, rule.message), true
	}
	return nil, true
}

func validateMarkdownReplaceFencedCodeOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if op.Part != "body" {
		return errorResultWithCode(ErrCodeInvalidInput, "replace/fenced_code requires part body and non-empty text")
	}
	if !markdownEditRequiresOnly(op, markdownFieldPart|markdownFieldText, op.Text) {
		return errorResultWithCode(ErrCodeInvalidInput, "replace/fenced_code requires part body and non-empty text")
	}
	return nil
}

func validateMarkdownReplaceSectionOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if !markdownAllowedPart(op.Part, "body", "subtree") {
		return errorResultWithCode(ErrCodeInvalidInput, "replace/section requires part body or subtree and markdown")
	}
	if !markdownEditHasOnly(op, markdownFieldPart|markdownFieldMarkdown) {
		return errorResultWithCode(ErrCodeInvalidInput, "replace/section requires part body or subtree and markdown")
	}
	return nil
}

func validateMarkdownReplaceListItemOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if op.Part == "subtree" {
		if !markdownEditHasOnly(op, markdownFieldPart|markdownFieldMarkdown) {
			return errorResultWithCode(ErrCodeInvalidInput, "replace/list_item subtree accepts part subtree and markdown only")
		}
		return nil
	}
	if !markdownEditHasOnly(op, markdownFieldMarkdown) {
		return errorResultWithCode(ErrCodeInvalidInput, "replace/list_item accepts markdown only")
	}
	return nil
}

func validateMarkdownAddEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if op.Subject == "inline_link" || op.Subject == "image" {
		if op.Part != "title" || op.Text == "" || !markdownEditHasOnly(op, markdownFieldPart|markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "add title requires part title and text")
		}
		return nil
	}
	if op.Subject == "reference_definition" {
		if op.Part != "title" || op.Text == "" || !markdownEditHasOnly(op, markdownFieldPart|markdownFieldText) {
			return errorResultWithCode(ErrCodeInvalidInput, "add/reference_definition requires part title and text")
		}
		return nil
	}
	return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
}

func validateMarkdownInsertEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	switch op.Subject {
	case "list_item":
		if !markdownAllowedPart(op.Position, "before", "after", "child") ||
			!markdownEditHasOnly(op, markdownFieldPosition|markdownFieldMarkdown) {
			return errorResultWithCode(ErrCodeInvalidInput, "insert/list_item requires position before, after, or child and markdown")
		}
	case "paragraph":
		if !markdownAllowedPart(op.Position, "before", "after") ||
			!markdownEditHasOnly(op, markdownFieldPosition|markdownFieldMarkdown) {
			return errorResultWithCode(ErrCodeInvalidInput, "insert/paragraph requires position before or after and markdown")
		}
	case "section":
		if !markdownAllowedPart(op.Position, "before", "after", "child") ||
			!markdownEditHasOnly(op, markdownFieldPosition|markdownFieldMarkdown) {
			return errorResultWithCode(ErrCodeInvalidInput, "insert/section requires position before, after, or child and markdown")
		}
	default:
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	return nil
}

func validateMarkdownMoveEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if op.Subject != "section" && op.Subject != "list_item" {
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	if !isLowerHexDigest(op.AnchorTargetID) || !markdownAllowedPart(op.Position, "before", "after") ||
		!markdownEditHasOnly(op, markdownFieldAnchor|markdownFieldPosition) {
		return errorResultWithCode(ErrCodeInvalidInput, "move requires anchorTargetId and position before or after")
	}
	return nil
}

func validateMarkdownRetargetEditOperation(op MarkdownEditOperation) *mcp.CallToolResult {
	if op.Subject != "reference_occurrence" {
		return errorResultWithCode(ErrCodeInvalidInput, "unsupported Markdown edit operation")
	}
	if op.Text == "" || !markdownEditHasOnly(op, markdownFieldText) {
		return errorResultWithCode(ErrCodeInvalidInput, "retarget/reference_occurrence requires a relationship targetId and non-empty text")
	}
	return nil
}

func markdownAllowedPart(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
