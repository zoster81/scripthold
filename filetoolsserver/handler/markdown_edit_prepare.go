package handler

import (
	"github.com/zoster81/marksplice"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

func prepareMarkdownEditOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Action {
	case "create":
		return prepareMarkdownCreateOperation(snapshot, op)
	case "remove":
		return prepareMarkdownRemoveOperation(snapshot, op)
	case "rename":
		return prepareMarkdownRenameOperation(snapshot, op)
	case "retarget":
		return prepareMarkdownRetargetOperation(snapshot, op)
	case "set":
		return prepareMarkdownSetOperation(snapshot, op)
	case "replace":
		return prepareMarkdownReplaceOperation(snapshot, op)
	case "sync":
		return prepareMarkdownSyncOperation(snapshot, op)
	case "add":
		return prepareMarkdownAddOperation(snapshot, op)
	case "insert":
		return prepareMarkdownInsertOperation(snapshot, op)
	case "move":
		return prepareMarkdownMoveOperation(snapshot, op)
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownCreateOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "front_matter":
		format, _ := markdownFrontMatterFormat(op.Format)
		return snapshot.PrepareAddFrontMatter(format)
	case "front_matter_field":
		return snapshot.PrepareAppendFrontMatterField([]byte(op.Key), []byte(op.Value))
	case "reference_definition":
		return snapshot.PrepareAppendReferenceDefinition([]byte(op.Label), []byte(op.Destination), []byte(op.Title))
	case "footnote_definition":
		return snapshot.PrepareAppendFootnoteDefinition([]byte(op.Label), []byte(op.Body))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownRemoveOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "front_matter":
		return snapshot.PrepareRemoveFrontMatter()
	case "inline_link":
		return snapshot.PrepareRemoveInlineLinkTitle(op.TargetID)
	case "image":
		return snapshot.PrepareRemoveImageTitle(op.TargetID)
	case "reference_definition":
		return prepareMarkdownRemoveReferenceOperation(snapshot, op)
	case "front_matter_field":
		return snapshot.PrepareRemoveFrontMatterField(op.TargetID)
	case "thematic_break":
		return snapshot.PrepareRemoveThematicBreak(op.TargetID)
	case "blockquote":
		return snapshot.PrepareRemoveBlockquote(op.TargetID)
	case "footnote_definition":
		return snapshot.PrepareRemoveFootnoteDefinition(op.TargetID)
	case "list_item":
		return snapshot.PrepareRemoveListItem(op.TargetID)
	case "paragraph":
		return snapshot.PrepareRemoveParagraph(op.TargetID)
	case "section":
		return snapshot.PrepareRemoveSection(op.TargetID)
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownRemoveReferenceOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	if op.Part == "title" {
		return snapshot.PrepareRemoveReferenceDefinitionTitle(op.TargetID)
	}
	return snapshot.PrepareRemoveReferenceDefinition(op.TargetID)
}

func prepareMarkdownRenameOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "heading":
		return snapshot.PrepareRenameHeading(op.TargetID, []byte(op.Text))
	case "reference_definition":
		return snapshot.PrepareRenameReferenceDefinition(op.TargetID, []byte(op.Text))
	case "front_matter_field":
		return snapshot.PrepareRenameFrontMatterField(op.TargetID, []byte(op.Text))
	case "footnote_definition":
		return snapshot.PrepareRenameFootnoteDefinition(op.TargetID, []byte(op.Text))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownRetargetOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	if op.Subject != "reference_occurrence" {
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
	return snapshot.PrepareRetargetReferenceOccurrence(op.TargetID, []byte(op.Text))
}

func prepareMarkdownSetOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "heading":
		return snapshot.PrepareSetHeadingLevel(op.TargetID, op.Level)
	case "task":
		return snapshot.PrepareSetTaskChecked(op.TargetID, *op.Checked)
	case "table":
		return prepareMarkdownSetTableOperation(snapshot, op)
	case "fenced_code":
		return snapshot.PrepareSetFencedBlockInfo(op.TargetID, []byte(op.Text))
	case "alert":
		kind, _ := markdownAlertKind(op.Text)
		return snapshot.PrepareSetAlertKind(op.TargetID, kind)
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownSetTableOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Part {
	case "column_alignment":
		alignment, _ := markdownTableAlignment(op.Alignment)
		return snapshot.PrepareSetTableColumnAlignment(op.TargetID, *op.Column, alignment)
	case "alignments":
		alignments := make([]marksplice.TableAlignment, len(op.Alignments))
		for index, value := range op.Alignments {
			alignments[index], _ = markdownTableAlignment(value)
		}
		return snapshot.PrepareSetTableAlignments(op.TargetID, alignments)
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownReplaceOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "inline_link", "image", "reference_definition":
		return prepareMarkdownReplaceLinkOperation(snapshot, op)
	case "list_item", "section", "fenced_code":
		return prepareMarkdownReplaceStructureOperation(snapshot, op)
	default:
		return prepareMarkdownReplaceSimpleOperation(snapshot, op)
	}
}

func prepareMarkdownReplaceSimpleOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "paragraph":
		return snapshot.PrepareReplaceParagraph(op.TargetID, []byte(op.Markdown))
	case "code_span":
		return snapshot.PrepareReplaceCodeSpan(op.TargetID, []byte(op.Text))
	case "strikethrough":
		return snapshot.PrepareReplaceStrikethrough(op.TargetID, []byte(op.Text))
	case "emphasis":
		return snapshot.PrepareReplaceEmphasis(op.TargetID, []byte(op.Text))
	case "strong":
		return snapshot.PrepareReplaceStrong(op.TargetID, []byte(op.Text))
	case "autolink":
		return snapshot.PrepareReplaceAutoLink(op.TargetID, []byte(op.Text))
	case "front_matter_field":
		return snapshot.PrepareReplaceFrontMatterValue(op.TargetID, []byte(op.Text))
	case "html_comment":
		return snapshot.PrepareReplaceHTMLComment(op.TargetID, []byte(op.Text))
	case "html_anchor":
		return snapshot.PrepareReplaceHTMLAnchor(op.TargetID, []byte(op.Text))
	case "math_expression":
		return snapshot.PrepareReplaceMathExpression(op.TargetID, []byte(op.Text))
	case "blockquote":
		return snapshot.PrepareReplaceBlockquoteContent(op.TargetID, []byte(op.Text))
	case "alert":
		return snapshot.PrepareReplaceAlertBody(op.TargetID, []byte(op.Text))
	case "footnote_definition":
		return snapshot.PrepareReplaceFootnoteDefinitionBody(op.TargetID, []byte(op.Text))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownReplaceLinkOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "inline_link":
		return prepareMarkdownReplaceInlineLinkOperation(snapshot, op)
	case "image":
		return prepareMarkdownReplaceImageOperation(snapshot, op)
	case "reference_definition":
		return prepareMarkdownReplaceReferenceOperation(snapshot, op)
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownReplaceInlineLinkOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Part {
	case "destination":
		return snapshot.PrepareReplaceInlineLinkDestination(op.TargetID, []byte(op.Text))
	case "label":
		return snapshot.PrepareReplaceInlineLinkLabel(op.TargetID, []byte(op.Text))
	case "title":
		return snapshot.PrepareReplaceInlineLinkTitle(op.TargetID, []byte(op.Text))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownReplaceImageOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Part {
	case "destination":
		return snapshot.PrepareReplaceImageDestination(op.TargetID, []byte(op.Text))
	case "alt":
		return snapshot.PrepareReplaceImageAlt(op.TargetID, []byte(op.Text))
	case "title":
		return snapshot.PrepareReplaceImageTitle(op.TargetID, []byte(op.Text))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownReplaceReferenceOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Part {
	case "destination":
		return snapshot.PrepareReplaceReferenceDefinitionDestination(op.TargetID, []byte(op.Text))
	case "title":
		return snapshot.PrepareReplaceReferenceDefinitionTitle(op.TargetID, []byte(op.Text))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownReplaceStructureOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "fenced_code":
		return snapshot.PrepareReplaceFencedCode(op.TargetID, []byte(op.Text))
	case "list_item":
		if op.Part == "subtree" {
			return snapshot.PrepareReplaceListItemSubtree(op.TargetID, []byte(op.Markdown))
		}
		return snapshot.PrepareReplaceListItem(op.TargetID, []byte(op.Markdown))
	case "section":
		if op.Part == "body" {
			return snapshot.PrepareReplaceSectionBody(op.TargetID, []byte(op.Markdown))
		}
		return snapshot.PrepareReplaceSection(op.TargetID, []byte(op.Markdown))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownSyncOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	if op.Subject != "toc" {
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
	return snapshot.PrepareSyncTOC(op.TargetID)
}

func prepareMarkdownAddOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "inline_link":
		return snapshot.PrepareAddInlineLinkTitle(op.TargetID, []byte(op.Text))
	case "image":
		return snapshot.PrepareAddImageTitle(op.TargetID, []byte(op.Text))
	case "reference_definition":
		return snapshot.PrepareAddReferenceDefinitionTitle(op.TargetID, []byte(op.Text))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownInsertOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "list_item":
		return prepareMarkdownInsertListItemOperation(snapshot, op)
	case "paragraph":
		return prepareMarkdownInsertParagraphOperation(snapshot, op)
	case "section":
		return prepareMarkdownInsertSectionOperation(snapshot, op)
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownInsertListItemOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Position {
	case "before":
		return snapshot.PrepareInsertListItemBefore(op.TargetID, []byte(op.Markdown))
	case "after":
		return snapshot.PrepareInsertListItemAfter(op.TargetID, []byte(op.Markdown))
	case "child":
		return snapshot.PrepareAppendListItemChild(op.TargetID, []byte(op.Markdown))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownInsertParagraphOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Position {
	case "before":
		return snapshot.PrepareInsertParagraphBefore(op.TargetID, []byte(op.Markdown))
	case "after":
		return snapshot.PrepareInsertParagraphAfter(op.TargetID, []byte(op.Markdown))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownInsertSectionOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Position {
	case "before":
		return snapshot.PrepareInsertSectionBefore(op.TargetID, []byte(op.Markdown))
	case "after":
		return snapshot.PrepareInsertSectionAfter(op.TargetID, []byte(op.Markdown))
	case "child":
		return snapshot.PrepareAppendSectionChild(op.TargetID, []byte(op.Markdown))
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownMoveOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	switch op.Subject {
	case "list_item":
		return prepareMarkdownMoveListItemOperation(snapshot, op)
	case "section":
		return prepareMarkdownMoveSectionOperation(snapshot, op)
	default:
		return markdownintelligence.PreparedChange{}, marksplice.ErrInvalidQuery
	}
}

func prepareMarkdownMoveListItemOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	if op.Position == "before" {
		return snapshot.PrepareMoveListItemBefore(op.TargetID, op.AnchorTargetID)
	}
	return snapshot.PrepareMoveListItemAfter(op.TargetID, op.AnchorTargetID)
}

func prepareMarkdownMoveSectionOperation(snapshot *markdownintelligence.Snapshot, op MarkdownEditOperation) (markdownintelligence.PreparedChange, error) {
	if op.Position == "before" {
		return snapshot.PrepareMoveSectionBefore(op.TargetID, op.AnchorTargetID)
	}
	return snapshot.PrepareMoveSectionAfter(op.TargetID, op.AnchorTargetID)
}
