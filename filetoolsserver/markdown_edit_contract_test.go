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
	if !ok || len(oneOf) != 15 {
		t.Fatalf("operation union = %#v, want fifteen schema branches covering nineteen closed forms", items["oneOf"])
	}

	rename := markdownReadSchemaMap(t, oneOf[0])
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

	setLevel := markdownReadSchemaMap(t, oneOf[1])
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

	replaceParagraph := markdownReadSchemaMap(t, oneOf[2])
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

	removeParagraph := markdownReadSchemaMap(t, oneOf[3])
	if removeParagraph["type"] != "object" || removeParagraph["additionalProperties"] != false {
		t.Fatalf("paragraph remove schema = %#v, want strict object", removeParagraph)
	}
	markdownReadAssertStringSet(t, "paragraph remove required", removeParagraph["required"], []string{"action", "subject", "targetId"})
	removeProperties := markdownReadSchemaMap(t, removeParagraph["properties"])
	if markdownReadSchemaMap(t, removeProperties["action"])["const"] != "remove" || markdownReadSchemaMap(t, removeProperties["subject"])["const"] != "paragraph" {
		t.Fatalf("paragraph remove discriminators = %#v", removeProperties)
	}
	if len(removeProperties) != 3 {
		t.Fatalf("paragraph remove properties = %#v, want only action/subject/targetId", removeProperties)
	}
	if markdownReadSchemaMap(t, removeProperties["targetId"])["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatalf("paragraph remove targetId schema = %#v", removeProperties["targetId"])
	}

	insertParagraph := markdownReadSchemaMap(t, oneOf[4])
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

	insertListItem := markdownReadSchemaMap(t, oneOf[12])
	markdownReadAssertStringSet(t, "list item insert required", insertListItem["required"], []string{"action", "markdown", "position", "subject", "targetId"})
	insertListItemProperties := markdownReadSchemaMap(t, insertListItem["properties"])
	if markdownReadSchemaMap(t, insertListItemProperties["action"])["const"] != "insert" || markdownReadSchemaMap(t, insertListItemProperties["subject"])["const"] != "list_item" {
		t.Fatalf("list item insert discriminators = %#v", insertListItemProperties)
	}
	if position := markdownReadSchemaMap(t, insertListItemProperties["position"]); !reflect.DeepEqual(position["enum"], []string{"before", "after", "child"}) {
		t.Fatalf("list item insert position schema = %#v, want before/after/child enum", position)
	}

	setTask := markdownReadSchemaMap(t, oneOf[13])
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

	replaceCodeSpan := markdownReadSchemaMap(t, oneOf[14])
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

	insertSection := markdownReadSchemaMap(t, oneOf[5])
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

	removeSection := markdownReadSchemaMap(t, oneOf[6])
	if removeSection["type"] != "object" || removeSection["additionalProperties"] != false {
		t.Fatalf("section remove schema = %#v, want strict object", removeSection)
	}
	markdownReadAssertStringSet(t, "section remove required", removeSection["required"], []string{"action", "subject", "targetId"})
	removeSectionProperties := markdownReadSchemaMap(t, removeSection["properties"])
	if markdownReadSchemaMap(t, removeSectionProperties["action"])["const"] != "remove" || markdownReadSchemaMap(t, removeSectionProperties["subject"])["const"] != "section" {
		t.Fatalf("section remove discriminators = %#v", removeSectionProperties)
	}
	if len(removeSectionProperties) != 3 {
		t.Fatalf("section remove properties = %#v, want only action/subject/targetId", removeSectionProperties)
	}
	if markdownReadSchemaMap(t, removeSectionProperties["targetId"])["pattern"] != "^[0-9a-f]{64}$" {
		t.Fatalf("section remove targetId schema = %#v", removeSectionProperties["targetId"])
	}

	replaceSectionBody := markdownReadSchemaMap(t, oneOf[7])
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

	moveSection := markdownReadSchemaMap(t, oneOf[8])
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

	replaceListItem := markdownReadSchemaMap(t, oneOf[9])
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

	replaceListItemSubtree := markdownReadSchemaMap(t, oneOf[10])
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

	removeListItem := markdownReadSchemaMap(t, oneOf[11])
	if removeListItem["type"] != "object" || removeListItem["additionalProperties"] != false {
		t.Fatalf("list item remove schema = %#v, want strict object", removeListItem)
	}
	markdownReadAssertStringSet(t, "list item remove required", removeListItem["required"], []string{"action", "subject", "targetId"})
	removeListItemProperties := markdownReadSchemaMap(t, removeListItem["properties"])
	if markdownReadSchemaMap(t, removeListItemProperties["action"])["const"] != "remove" || markdownReadSchemaMap(t, removeListItemProperties["subject"])["const"] != "list_item" || len(removeListItemProperties) != 3 {
		t.Fatalf("list item remove properties = %#v", removeListItemProperties)
	}

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
