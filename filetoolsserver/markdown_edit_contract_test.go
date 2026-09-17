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
	if !ok || len(oneOf) != 5 {
		t.Fatalf("operation union = %#v, want five closed variants", items["oneOf"])
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
	if markdownReadSchemaMap(t, insertProperties["action"])["const"] != "insert" || markdownReadSchemaMap(t, insertProperties["subject"])["const"] != "paragraph" {
		t.Fatalf("paragraph insert discriminators = %#v", insertProperties)
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
