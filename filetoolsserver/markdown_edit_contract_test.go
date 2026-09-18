package filetoolsserver

import (
	"reflect"
	"sort"
	"testing"

	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

func TestMarkdownEditCatalogSchemaIsClosedForOperationUnion(t *testing.T) {
	tool := markdownEditCatalogTool()
	schema := markdownReadSchemaMap(t, tool.InputSchema)
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("markdown_edit schema = %#v, want strict object", schema)
	}
	markdownReadAssertStringSet(t, "markdown_edit required", schema["required"], []string{"operations", "path"})

	properties := markdownReadSchemaMap(t, schema["properties"])
	gotProperties := make([]string, 0, len(properties))
	for name := range properties {
		gotProperties = append(gotProperties, name)
	}
	sort.Strings(gotProperties)
	wantProperties := []string{"backupPolicy", "encoding", "operations", "path"}
	if !reflect.DeepEqual(gotProperties, wantProperties) {
		t.Fatalf("markdown_edit properties = %v, want %v", gotProperties, wantProperties)
	}

	operations := markdownReadSchemaMap(t, properties["operations"])
	if operations["type"] != "array" || operations["minItems"] != 1 || operations["maxItems"] != markdownintelligence.MaxEditOperations {
		t.Fatalf("operations schema = %#v, want 1..%d operations", operations, markdownintelligence.MaxEditOperations)
	}
	items := markdownReadSchemaMap(t, operations["items"])
	oneOf, ok := items["oneOf"].([]any)
	if !ok || len(oneOf) != 24 {
		t.Fatalf("operation union = %#v, want twenty-four schema branches covering forty-four closed forms", items["oneOf"])
	}

	rename := markdownEditOperationBranch(t, oneOf, "rename", "heading", "")
	if rename["type"] != "object" || rename["additionalProperties"] != false {
		t.Fatalf("rename schema = %#v, want strict object", rename)
	}
	markdownReadAssertStringSet(t, "rename required", rename["required"], []string{"action", "subject", "targetId", "text"})
	renameProperties := markdownReadSchemaMap(t, rename["properties"])
	if markdownReadSchemaMap(t, renameProperties["action"])["const"] != "rename" || markdownReadSchemaMap(t, renameProperties["subject"])["const"] != "heading" {
		t.Fatalf("rename discriminators = %#v", renameProperties)
	}
	if markdownReadSchemaMap(t, renameProperties["targetId"])["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatalf("rename targetId schema = %#v", renameProperties["targetId"])
	}
	if _, ok := renameProperties["level"]; ok {
		t.Fatalf("rename schema unexpectedly accepts level: %#v", renameProperties)
	}

	renameReferenceDefinition := markdownEditOperationBranch(t, oneOf, "rename", "reference_definition", "")
	markdownReadAssertStringSet(t, "reference/front matter rename required", renameReferenceDefinition["required"], []string{"action", "subject", "targetId", "text"})
	renameReferenceDefinitionProperties := markdownReadSchemaMap(t, renameReferenceDefinition["properties"])
	if markdownReadSchemaMap(t, renameReferenceDefinitionProperties["action"])["const"] != "rename" {
		t.Fatalf("reference/front matter rename action = %#v", renameReferenceDefinitionProperties)
	}
	if subjects := markdownReadSchemaMap(t, renameReferenceDefinitionProperties["subject"])["enum"]; !reflect.DeepEqual(subjects, []string{"reference_definition", "front_matter_field"}) {
		t.Fatalf("reference/front matter rename subjects = %#v", subjects)
	}
	renameReferenceText := markdownReadSchemaMap(t, renameReferenceDefinitionProperties["text"])
	if renameReferenceText["type"] != "string" || renameReferenceText["minLength"] != 1 || len(renameReferenceDefinitionProperties) != 4 {
		t.Fatalf("reference/front matter rename properties = %#v", renameReferenceDefinitionProperties)
	}
	renameFrontMatterField := markdownEditOperationBranch(t, oneOf, "rename", "front_matter_field", "")
	if !reflect.DeepEqual(renameFrontMatterField, renameReferenceDefinition) {
		t.Fatalf("front matter rename branch = %#v, want shared reference/front matter branch", renameFrontMatterField)
	}

	setLevel := markdownEditOperationBranch(t, oneOf, "set", "heading", "")
	if setLevel["type"] != "object" || setLevel["additionalProperties"] != false {
		t.Fatalf("set schema = %#v, want strict object", setLevel)
	}
	markdownReadAssertStringSet(t, "set required", setLevel["required"], []string{"action", "level", "subject", "targetId"})
	setProperties := markdownReadSchemaMap(t, setLevel["properties"])
	if markdownReadSchemaMap(t, setProperties["action"])["const"] != "set" || markdownReadSchemaMap(t, setProperties["subject"])["const"] != "heading" {
		t.Fatalf("set discriminators = %#v", setProperties)
	}
	level := markdownReadSchemaMap(t, setProperties["level"])
	if level["type"] != "integer" || level["minimum"] != 1 || level["maximum"] != 6 {
		t.Fatalf("level schema = %#v, want integer 1..6", level)
	}
	if _, ok := setProperties["text"]; ok {
		t.Fatalf("set schema unexpectedly accepts text: %#v", setProperties)
	}

	replaceParagraph := markdownEditOperationBranch(t, oneOf, "replace", "paragraph", "")
	if replaceParagraph["type"] != "object" || replaceParagraph["additionalProperties"] != false {
		t.Fatalf("paragraph replace schema = %#v, want strict object", replaceParagraph)
	}
	markdownReadAssertStringSet(t, "paragraph replace required", replaceParagraph["required"], []string{"action", "markdown", "subject", "targetId"})
	replaceProperties := markdownReadSchemaMap(t, replaceParagraph["properties"])
	if markdownReadSchemaMap(t, replaceProperties["action"])["const"] != "replace" || markdownReadSchemaMap(t, replaceProperties["subject"])["const"] != "paragraph" {
		t.Fatalf("paragraph replace discriminators = %#v", replaceProperties)
	}
	markdown := markdownReadSchemaMap(t, replaceProperties["markdown"])
	if markdown["type"] != "string" {
		t.Fatalf("paragraph markdown schema = %#v, want string", markdown)
	}
	if _, ok := replaceProperties["text"]; ok {
		t.Fatalf("paragraph replace schema unexpectedly accepts text: %#v", replaceProperties)
	}
	if _, ok := replaceProperties["level"]; ok {
		t.Fatalf("paragraph replace schema unexpectedly accepts level: %#v", replaceProperties)
	}

	removeParagraph := markdownEditOperationBranch(t, oneOf, "remove", "paragraph", "")
	if removeParagraph["type"] != "object" || removeParagraph["additionalProperties"] != false {
		t.Fatalf("target-only remove schema = %#v, want strict object", removeParagraph)
	}
	markdownReadAssertStringSet(t, "target-only remove required", removeParagraph["required"], []string{"action", "subject", "targetId"})
	removeProperties := markdownReadSchemaMap(t, removeParagraph["properties"])
	if markdownReadSchemaMap(t, removeProperties["action"])["const"] != "remove" {
		t.Fatalf("target-only remove action = %#v", removeProperties)
	}
	if subjects := markdownReadSchemaMap(t, removeProperties["subject"])["enum"]; !reflect.DeepEqual(subjects, []string{"paragraph", "section", "list_item", "reference_definition", "front_matter_field", "thematic_break"}) {
		t.Fatalf("target-only remove subjects = %#v", subjects)
	}
	if len(removeProperties) != 3 {
		t.Fatalf("target-only remove properties = %#v, want only action/subject/targetId", removeProperties)
	}
	if markdownReadSchemaMap(t, removeProperties["targetId"])["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatalf("target-only remove targetId schema = %#v", removeProperties["targetId"])
	}

	insertParagraph := markdownEditOperationBranch(t, oneOf, "insert", "paragraph", "")
	if insertParagraph["type"] != "object" || insertParagraph["additionalProperties"] != false {
		t.Fatalf("paragraph insert schema = %#v, want strict object", insertParagraph)
	}
	markdownReadAssertStringSet(t, "paragraph insert required", insertParagraph["required"], []string{"action", "markdown", "position", "subject", "targetId"})
	insertProperties := markdownReadSchemaMap(t, insertParagraph["properties"])
	if markdownReadSchemaMap(t, insertProperties["action"])["const"] != "insert" {
		t.Fatalf("insert action discriminator = %#v", insertProperties)
	}
	if markdownReadSchemaMap(t, insertProperties["subject"])["const"] != "paragraph" {
		t.Fatalf("paragraph insert subject schema = %#v", insertProperties["subject"])
	}
	position := markdownReadSchemaMap(t, insertProperties["position"])
	if !reflect.DeepEqual(position["enum"], []string{"before", "after"}) {
		t.Fatalf("paragraph insert position schema = %#v, want before/after enum", position)
	}
	if markdownReadSchemaMap(t, insertProperties["markdown"])["type"] != "string" {
		t.Fatalf("paragraph insert markdown schema = %#v", insertProperties["markdown"])
	}
	if markdownReadSchemaMap(t, insertProperties["targetId"])["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatalf("paragraph insert targetId schema = %#v", insertProperties["targetId"])
	}
	if len(insertProperties) != 5 {
		t.Fatalf("paragraph insert properties = %#v, want only action/subject/targetId/position/markdown", insertProperties)
	}

	insertListItem := markdownEditOperationBranch(t, oneOf, "insert", "list_item", "")
	markdownReadAssertStringSet(t, "list item insert required", insertListItem["required"], []string{"action", "markdown", "position", "subject", "targetId"})
	insertListItemProperties := markdownReadSchemaMap(t, insertListItem["properties"])
	if markdownReadSchemaMap(t, insertListItemProperties["action"])["const"] != "insert" || markdownReadSchemaMap(t, insertListItemProperties["subject"])["const"] != "list_item" {
		t.Fatalf("list item insert discriminators = %#v", insertListItemProperties)
	}
	if position := markdownReadSchemaMap(t, insertListItemProperties["position"]); !reflect.DeepEqual(position["enum"], []string{"before", "after", "child"}) {
		t.Fatalf("list item insert position schema = %#v, want before/after/child enum", position)
	}

	setTask := markdownEditOperationBranch(t, oneOf, "set", "task", "")
	if setTask["type"] != "object" || setTask["additionalProperties"] != false {
		t.Fatalf("task set schema = %#v, want strict object", setTask)
	}
	markdownReadAssertStringSet(t, "task set required", setTask["required"], []string{"action", "checked", "subject", "targetId"})
	setTaskProperties := markdownReadSchemaMap(t, setTask["properties"])
	if markdownReadSchemaMap(t, setTaskProperties["action"])["const"] != "set" || markdownReadSchemaMap(t, setTaskProperties["subject"])["const"] != "task" {
		t.Fatalf("task set discriminators = %#v", setTaskProperties)
	}
	if checked := markdownReadSchemaMap(t, setTaskProperties["checked"]); checked["type"] != "boolean" {
		t.Fatalf("task checked schema = %#v, want boolean", checked)
	}
	if len(setTaskProperties) != 4 {
		t.Fatalf("task set properties = %#v, want only action/subject/targetId/checked", setTaskProperties)
	}

	replaceCodeSpan := markdownEditOperationBranch(t, oneOf, "replace", "code_span", "")
	if replaceCodeSpan["type"] != "object" || replaceCodeSpan["additionalProperties"] != false {
		t.Fatalf("code span replace schema = %#v, want strict object", replaceCodeSpan)
	}
	markdownReadAssertStringSet(t, "code span replace required", replaceCodeSpan["required"], []string{"action", "subject", "targetId", "text"})
	replaceCodeSpanProperties := markdownReadSchemaMap(t, replaceCodeSpan["properties"])
	if markdownReadSchemaMap(t, replaceCodeSpanProperties["action"])["const"] != "replace" {
		t.Fatalf("simple inline replace action = %#v", replaceCodeSpanProperties)
	}
	if subjects := markdownReadSchemaMap(t, replaceCodeSpanProperties["subject"])["enum"]; !reflect.DeepEqual(subjects, []string{"code_span", "emphasis", "strong", "strikethrough"}) {
		t.Fatalf("simple inline replace subjects = %#v", subjects)
	}
	if markdownReadSchemaMap(t, replaceCodeSpanProperties["text"])["type"] != "string" || len(replaceCodeSpanProperties) != 4 {
		t.Fatalf("code span replace properties = %#v", replaceCodeSpanProperties)
	}

	replaceFencedBody := markdownEditOperationBranch(t, oneOf, "replace", "fenced_code", "body")
	markdownReadAssertStringSet(t, "fenced body replace required", replaceFencedBody["required"], []string{"action", "part", "subject", "targetId", "text"})
	replaceFencedBodyProperties := markdownReadSchemaMap(t, replaceFencedBody["properties"])
	if markdownReadSchemaMap(t, replaceFencedBodyProperties["part"])["const"] != "body" {
		t.Fatalf("fenced body part schema = %#v", replaceFencedBodyProperties["part"])
	}
	fencedBodyText := markdownReadSchemaMap(t, replaceFencedBodyProperties["text"])
	if fencedBodyText["type"] != "string" || fencedBodyText["minLength"] != 1 || len(replaceFencedBodyProperties) != 5 {
		t.Fatalf("fenced body replace properties = %#v", replaceFencedBodyProperties)
	}

	setFencedInfo := markdownEditOperationBranch(t, oneOf, "set", "fenced_code", "info")
	markdownReadAssertStringSet(t, "fenced info set required", setFencedInfo["required"], []string{"action", "part", "subject", "targetId", "text"})
	setFencedInfoProperties := markdownReadSchemaMap(t, setFencedInfo["properties"])
	if markdownReadSchemaMap(t, setFencedInfoProperties["part"])["const"] != "info" {
		t.Fatalf("fenced info part schema = %#v", setFencedInfoProperties["part"])
	}
	fencedInfoText := markdownReadSchemaMap(t, setFencedInfoProperties["text"])
	if fencedInfoText["type"] != "string" || fencedInfoText["minLength"] != nil || len(setFencedInfoProperties) != 5 {
		t.Fatalf("fenced info set properties = %#v", setFencedInfoProperties)
	}

	replaceInlineLink := markdownEditOperationBranch(t, oneOf, "replace", "inline_link", "destination")
	markdownReadAssertStringSet(t, "inline link replace required", replaceInlineLink["required"], []string{"action", "part", "subject", "targetId", "text"})
	replaceInlineLinkProperties := markdownReadSchemaMap(t, replaceInlineLink["properties"])
	if part := markdownReadSchemaMap(t, replaceInlineLinkProperties["part"]); !reflect.DeepEqual(part["enum"], []string{"destination", "label", "title"}) {
		t.Fatalf("inline link replace part schema = %#v, want destination/label/title enum", part)
	}
	inlineLinkText := markdownReadSchemaMap(t, replaceInlineLinkProperties["text"])
	if inlineLinkText["type"] != "string" || inlineLinkText["minLength"] != 1 || len(replaceInlineLinkProperties) != 5 {
		t.Fatalf("inline link replace properties = %#v", replaceInlineLinkProperties)
	}

	replaceImage := markdownEditOperationBranch(t, oneOf, "replace", "image", "alt")
	markdownReadAssertStringSet(t, "image replace required", replaceImage["required"], []string{"action", "part", "subject", "targetId", "text"})
	replaceImageProperties := markdownReadSchemaMap(t, replaceImage["properties"])
	if part := markdownReadSchemaMap(t, replaceImageProperties["part"]); !reflect.DeepEqual(part["enum"], []string{"destination", "alt", "title"}) {
		t.Fatalf("image replace part schema = %#v, want destination/alt/title enum", part)
	}
	imageText := markdownReadSchemaMap(t, replaceImageProperties["text"])
	if imageText["type"] != "string" || imageText["minLength"] != 1 || len(replaceImageProperties) != 5 {
		t.Fatalf("image replace properties = %#v", replaceImageProperties)
	}

	replaceAutoLink := markdownEditOperationBranch(t, oneOf, "replace", "autolink", "")
	markdownReadAssertStringSet(t, "simple text replace required", replaceAutoLink["required"], []string{"action", "subject", "targetId", "text"})
	replaceAutoLinkProperties := markdownReadSchemaMap(t, replaceAutoLink["properties"])
	if subjects := markdownReadSchemaMap(t, replaceAutoLinkProperties["subject"])["enum"]; !reflect.DeepEqual(subjects, []string{"autolink", "front_matter_field", "html_comment", "html_anchor", "math_expression"}) {
		t.Fatalf("simple text replace subjects = %#v", subjects)
	}
	autoLinkText := markdownReadSchemaMap(t, replaceAutoLinkProperties["text"])
	if autoLinkText["type"] != "string" || autoLinkText["minLength"] != 1 || len(replaceAutoLinkProperties) != 4 {
		t.Fatalf("simple text replace properties = %#v", replaceAutoLinkProperties)
	}

	for _, subject := range []string{"front_matter_field", "html_comment", "html_anchor", "math_expression"} {
		branch := markdownEditOperationBranch(t, oneOf, "replace", subject, "")
		if !reflect.DeepEqual(branch, replaceAutoLink) {
			t.Fatalf("%s replace branch = %#v, want shared simple-text replace branch", subject, branch)
		}
	}

	replaceReferenceDefinition := markdownEditOperationBranch(t, oneOf, "replace", "reference_definition", "destination")
	markdownReadAssertStringSet(t, "reference definition replace required", replaceReferenceDefinition["required"], []string{"action", "part", "subject", "targetId", "text"})
	replaceReferenceDefinitionProperties := markdownReadSchemaMap(t, replaceReferenceDefinition["properties"])
	if part := markdownReadSchemaMap(t, replaceReferenceDefinitionProperties["part"]); !reflect.DeepEqual(part["enum"], []string{"destination", "title"}) {
		t.Fatalf("reference definition replace part schema = %#v, want destination/title enum", part)
	}
	referenceDefinitionText := markdownReadSchemaMap(t, replaceReferenceDefinitionProperties["text"])
	if referenceDefinitionText["type"] != "string" || referenceDefinitionText["minLength"] != 1 || len(replaceReferenceDefinitionProperties) != 5 {
		t.Fatalf("reference definition replace properties = %#v", replaceReferenceDefinitionProperties)
	}

	addReferenceDefinitionTitle := markdownEditOperationBranch(t, oneOf, "add", "reference_definition", "title")
	markdownReadAssertStringSet(t, "add reference definition title required", addReferenceDefinitionTitle["required"], []string{"action", "part", "subject", "targetId", "text"})
	addReferenceDefinitionTitleProperties := markdownReadSchemaMap(t, addReferenceDefinitionTitle["properties"])
	if markdownReadSchemaMap(t, addReferenceDefinitionTitleProperties["part"])["const"] != "title" {
		t.Fatalf("add reference definition title part = %#v", addReferenceDefinitionTitleProperties["part"])
	}
	addReferenceDefinitionTitleText := markdownReadSchemaMap(t, addReferenceDefinitionTitleProperties["text"])
	if addReferenceDefinitionTitleText["type"] != "string" || addReferenceDefinitionTitleText["minLength"] != 1 || len(addReferenceDefinitionTitleProperties) != 5 {
		t.Fatalf("add reference definition title properties = %#v", addReferenceDefinitionTitleProperties)
	}

	removeReferenceDefinitionTitle := markdownEditOperationBranch(t, oneOf, "remove", "reference_definition", "title")
	markdownReadAssertStringSet(t, "remove reference definition title required", removeReferenceDefinitionTitle["required"], []string{"action", "part", "subject", "targetId"})
	removeReferenceDefinitionTitleProperties := markdownReadSchemaMap(t, removeReferenceDefinitionTitle["properties"])
	if markdownReadSchemaMap(t, removeReferenceDefinitionTitleProperties["part"])["const"] != "title" || len(removeReferenceDefinitionTitleProperties) != 4 {
		t.Fatalf("remove reference definition title properties = %#v", removeReferenceDefinitionTitleProperties)
	}

	removeReferenceDefinition := markdownEditOperationBranch(t, oneOf, "remove", "reference_definition", "")
	if !reflect.DeepEqual(removeReferenceDefinition, removeParagraph) {
		t.Fatalf("reference definition remove branch = %#v, want shared target-only remove branch", removeReferenceDefinition)
	}
	removeFrontMatterField := markdownEditOperationBranch(t, oneOf, "remove", "front_matter_field", "")
	if !reflect.DeepEqual(removeFrontMatterField, removeParagraph) {
		t.Fatalf("front matter remove branch = %#v, want shared target-only remove branch", removeFrontMatterField)
	}
	removeThematicBreak := markdownEditOperationBranch(t, oneOf, "remove", "thematic_break", "")
	if !reflect.DeepEqual(removeThematicBreak, removeParagraph) {
		t.Fatalf("thematic break remove branch = %#v, want shared target-only remove branch", removeThematicBreak)
	}

	addLinkTitle := markdownEditOperationBranch(t, oneOf, "add", "inline_link", "title")
	markdownReadAssertStringSet(t, "add title required", addLinkTitle["required"], []string{"action", "part", "subject", "targetId", "text"})
	addTitleProperties := markdownReadSchemaMap(t, addLinkTitle["properties"])
	if subjects := markdownReadSchemaMap(t, addTitleProperties["subject"])["enum"]; !reflect.DeepEqual(subjects, []string{"inline_link", "image"}) {
		t.Fatalf("add title subjects = %#v", subjects)
	}
	if markdownReadSchemaMap(t, addTitleProperties["part"])["const"] != "title" {
		t.Fatalf("add title part = %#v", addTitleProperties["part"])
	}
	addTitleText := markdownReadSchemaMap(t, addTitleProperties["text"])
	if addTitleText["type"] != "string" || addTitleText["minLength"] != 1 || len(addTitleProperties) != 5 {
		t.Fatalf("add title properties = %#v", addTitleProperties)
	}

	removeImageTitle := markdownEditOperationBranch(t, oneOf, "remove", "image", "title")
	markdownReadAssertStringSet(t, "remove title required", removeImageTitle["required"], []string{"action", "part", "subject", "targetId"})
	removeTitleProperties := markdownReadSchemaMap(t, removeImageTitle["properties"])
	if subjects := markdownReadSchemaMap(t, removeTitleProperties["subject"])["enum"]; !reflect.DeepEqual(subjects, []string{"inline_link", "image"}) {
		t.Fatalf("remove title subjects = %#v", subjects)
	}
	if markdownReadSchemaMap(t, removeTitleProperties["part"])["const"] != "title" || len(removeTitleProperties) != 4 {
		t.Fatalf("remove title properties = %#v", removeTitleProperties)
	}

	insertSection := markdownEditOperationBranch(t, oneOf, "insert", "section", "")
	if insertSection["type"] != "object" || insertSection["additionalProperties"] != false {
		t.Fatalf("section insert schema = %#v, want strict object", insertSection)
	}
	markdownReadAssertStringSet(t, "section insert required", insertSection["required"], []string{"action", "markdown", "position", "subject", "targetId"})
	insertSectionProperties := markdownReadSchemaMap(t, insertSection["properties"])
	if markdownReadSchemaMap(t, insertSectionProperties["action"])["const"] != "insert" || markdownReadSchemaMap(t, insertSectionProperties["subject"])["const"] != "section" {
		t.Fatalf("section insert discriminators = %#v", insertSectionProperties)
	}
	if position := markdownReadSchemaMap(t, insertSectionProperties["position"]); !reflect.DeepEqual(position["enum"], []string{"before", "after", "child"}) {
		t.Fatalf("section insert position schema = %#v, want before/after/child enum", position)
	}
	if markdownReadSchemaMap(t, insertSectionProperties["markdown"])["type"] != "string" || len(insertSectionProperties) != 5 {
		t.Fatalf("section insert properties = %#v", insertSectionProperties)
	}

	removeSection := markdownEditOperationBranch(t, oneOf, "remove", "section", "")
	if !reflect.DeepEqual(removeSection, removeParagraph) {
		t.Fatalf("section remove branch = %#v, want shared target-only remove branch", removeSection)
	}

	replaceSectionBody := markdownEditOperationBranch(t, oneOf, "replace", "section", "body")
	if replaceSectionBody["type"] != "object" || replaceSectionBody["additionalProperties"] != false {
		t.Fatalf("section body replace schema = %#v, want strict object", replaceSectionBody)
	}
	markdownReadAssertStringSet(t, "section body replace required", replaceSectionBody["required"], []string{"action", "markdown", "part", "subject", "targetId"})
	replaceSectionBodyProperties := markdownReadSchemaMap(t, replaceSectionBody["properties"])
	if markdownReadSchemaMap(t, replaceSectionBodyProperties["action"])["const"] != "replace" || markdownReadSchemaMap(t, replaceSectionBodyProperties["subject"])["const"] != "section" {
		t.Fatalf("section replace discriminators = %#v", replaceSectionBodyProperties)
	}
	if part := markdownReadSchemaMap(t, replaceSectionBodyProperties["part"]); !reflect.DeepEqual(part["enum"], []string{"body", "subtree"}) {
		t.Fatalf("section replace part schema = %#v, want body/subtree enum", part)
	}
	if markdownReadSchemaMap(t, replaceSectionBodyProperties["markdown"])["type"] != "string" {
		t.Fatalf("section body replace markdown schema = %#v", replaceSectionBodyProperties["markdown"])
	}
	if len(replaceSectionBodyProperties) != 5 {
		t.Fatalf("section body replace properties = %#v, want only action/subject/targetId/part/markdown", replaceSectionBodyProperties)
	}

	moveSection := markdownEditOperationBranch(t, oneOf, "move", "section", "")
	if moveSection["type"] != "object" || moveSection["additionalProperties"] != false {
		t.Fatalf("section move schema = %#v, want strict object", moveSection)
	}
	markdownReadAssertStringSet(t, "section move required", moveSection["required"], []string{"action", "anchorTargetId", "position", "subject", "targetId"})
	moveSectionProperties := markdownReadSchemaMap(t, moveSection["properties"])
	if markdownReadSchemaMap(t, moveSectionProperties["action"])["const"] != "move" {
		t.Fatalf("move action discriminator = %#v", moveSectionProperties)
	}
	if subject := markdownReadSchemaMap(t, moveSectionProperties["subject"]); !reflect.DeepEqual(subject["enum"], []string{"section", "list_item"}) {
		t.Fatalf("move subject schema = %#v, want section/list_item enum", subject)
	}
	if position := markdownReadSchemaMap(t, moveSectionProperties["position"]); !reflect.DeepEqual(position["enum"], []string{"before", "after"}) {
		t.Fatalf("section move position schema = %#v, want before/after enum", position)
	}
	if markdownReadSchemaMap(t, moveSectionProperties["anchorTargetId"])["pattern"] != "^[0-9a-f]{64}$" || len(moveSectionProperties) != 5 {
		t.Fatalf("section move properties = %#v", moveSectionProperties)
	}

	replaceListItem := markdownEditOperationBranch(t, oneOf, "replace", "list_item", "")
	if replaceListItem["type"] != "object" || replaceListItem["additionalProperties"] != false {
		t.Fatalf("list item replace schema = %#v, want strict object", replaceListItem)
	}
	markdownReadAssertStringSet(t, "list item replace required", replaceListItem["required"], []string{"action", "markdown", "subject", "targetId"})
	replaceListItemProperties := markdownReadSchemaMap(t, replaceListItem["properties"])
	if markdownReadSchemaMap(t, replaceListItemProperties["action"])["const"] != "replace" || markdownReadSchemaMap(t, replaceListItemProperties["subject"])["const"] != "list_item" {
		t.Fatalf("list item replace discriminators = %#v", replaceListItemProperties)
	}
	if markdownReadSchemaMap(t, replaceListItemProperties["markdown"])["type"] != "string" || len(replaceListItemProperties) != 4 {
		t.Fatalf("list item replace properties = %#v", replaceListItemProperties)
	}

	replaceListItemSubtree := markdownEditOperationBranch(t, oneOf, "replace", "list_item", "subtree")
	if replaceListItemSubtree["type"] != "object" || replaceListItemSubtree["additionalProperties"] != false {
		t.Fatalf("list item subtree replace schema = %#v, want strict object", replaceListItemSubtree)
	}
	markdownReadAssertStringSet(t, "list item subtree replace required", replaceListItemSubtree["required"], []string{"action", "markdown", "part", "subject", "targetId"})
	replaceListItemSubtreeProperties := markdownReadSchemaMap(t, replaceListItemSubtree["properties"])
	if markdownReadSchemaMap(t, replaceListItemSubtreeProperties["action"])["const"] != "replace" || markdownReadSchemaMap(t, replaceListItemSubtreeProperties["subject"])["const"] != "list_item" || markdownReadSchemaMap(t, replaceListItemSubtreeProperties["part"])["const"] != "subtree" {
		t.Fatalf("list item subtree replace discriminators = %#v", replaceListItemSubtreeProperties)
	}
	if markdownReadSchemaMap(t, replaceListItemSubtreeProperties["markdown"])["type"] != "string" || len(replaceListItemSubtreeProperties) != 5 {
		t.Fatalf("list item subtree replace properties = %#v", replaceListItemSubtreeProperties)
	}

	removeListItem := markdownEditOperationBranch(t, oneOf, "remove", "list_item", "")
	if !reflect.DeepEqual(removeListItem, removeParagraph) {
		t.Fatalf("list item remove branch = %#v, want shared target-only remove branch", removeListItem)
	}

}

func markdownEditOperationBranch(t *testing.T, branches []any, action, subject, part string) map[string]any {
	t.Helper()
	var match map[string]any
	for _, raw := range branches {
		branch := markdownReadSchemaMap(t, raw)
		properties := markdownReadSchemaMap(t, branch["properties"])
		actionSchema := markdownReadSchemaMap(t, properties["action"])
		if actionSchema["const"] != action || !markdownEditSchemaStringMatches(t, properties["subject"], subject) {
			continue
		}
		partSchema, hasPart := properties["part"]
		if part == "" {
			if hasPart {
				continue
			}
		} else if !hasPart || !markdownEditSchemaStringMatches(t, partSchema, part) {
			continue
		}
		if match != nil {
			t.Fatalf("duplicate markdown_edit operation branches for action=%q subject=%q part=%q", action, subject, part)
		}
		match = branch
	}
	if match == nil {
		t.Fatalf("missing markdown_edit operation branch for action=%q subject=%q part=%q", action, subject, part)
	}
	return match
}

func markdownEditSchemaStringMatches(t *testing.T, value any, want string) bool {
	t.Helper()
	schema := markdownReadSchemaMap(t, value)
	if schema["const"] == want {
		return true
	}
	values, ok := schema["enum"].([]string)
	if !ok {
		return false
	}
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestMarkdownApplyCatalogSchemaAcceptsOnlyPreviewID(t *testing.T) {
	tool := markdownApplyCatalogTool()
	schema := markdownReadSchemaMap(t, tool.InputSchema)
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("markdown_apply schema = %#v, want strict object", schema)
	}
	markdownReadAssertStringSet(t, "markdown_apply required", schema["required"], []string{"previewId"})
	properties := markdownReadSchemaMap(t, schema["properties"])
	if len(properties) != 1 {
		t.Fatalf("markdown_apply properties = %#v, want only previewId", properties)
	}
	previewID := markdownReadSchemaMap(t, properties["previewId"])
	if previewID["type"] != "string" || previewID["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatalf("previewId schema = %#v", previewID)
	}
}
