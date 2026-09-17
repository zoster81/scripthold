package filetoolsserver

import (
	"reflect"
	"sort"
	"testing"

	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

func TestMarkdownEditCatalogSchemaIsClosedForHeadingOperationUnion(t *testing.T) {
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
	if !ok || len(oneOf) != 2 {
		t.Fatalf("operation union = %#v, want two closed variants", items["oneOf"])
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
