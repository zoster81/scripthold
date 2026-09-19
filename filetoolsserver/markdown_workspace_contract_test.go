package filetoolsserver

import "testing"

func TestMarkdownWorkspaceCatalogSchemaIsClosedReadOnlyUnion(t *testing.T) {
	tool := markdownWorkspaceCatalogTool()
	schema := markdownReadSchemaMap(t, tool.InputSchema)
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("markdown_workspace schema = %#v, want strict object", schema)
	}
	properties := markdownReadSchemaMap(t, schema["properties"])
	markdownReadAssertStringSet(t, "action enum", markdownReadSchemaMap(t, properties["action"])["enum"], []string{"inspect", "query", "validate"})
	markdownReadAssertStringSet(t, "query enum", markdownReadSchemaMap(t, properties["query"])["enum"], []string{"backlinks", "edges", "outgoing", "reachable", "related"})
	discovery := markdownReadSchemaMap(t, properties["discovery"])
	if discovery["type"] != "object" || discovery["additionalProperties"] != false {
		t.Fatalf("discovery schema = %#v, want strict object", discovery)
	}
	branches, ok := schema["oneOf"].([]any)
	if !ok || len(branches) != 7 {
		t.Fatalf("markdown_workspace oneOf = %#v, want 7 closed variants", schema["oneOf"])
	}
}
