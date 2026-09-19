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
	managedTOCs := markdownReadSchemaMap(t, properties["managedTocs"])
	managedTOCItem := markdownReadSchemaMap(t, managedTOCs["items"])
	if managedTOCs["type"] != "array" || managedTOCItem["additionalProperties"] != false {
		t.Fatalf("managedTocs schema = %#v, want bounded array of strict objects", managedTOCs)
	}
	branches, ok := schema["oneOf"].([]any)
	if !ok || len(branches) != 4 {
		t.Fatalf("markdown_workspace oneOf = %#v, want 4 closed shape variants", schema["oneOf"])
	}
	foundDocumentQueries := false
	for _, raw := range branches {
		branch := markdownReadSchemaMap(t, raw)
		branchProperties := markdownReadSchemaMap(t, branch["properties"])
		query, ok := branchProperties["query"]
		if !ok {
			continue
		}
		querySchema := markdownReadSchemaMap(t, query)
		values, ok := querySchema["enum"].([]string)
		if ok && len(values) == 4 {
			markdownReadAssertStringSet(t, "document query enum", values, []string{"backlinks", "outgoing", "reachable", "related"})
			foundDocumentQueries = true
		}
	}
	if !foundDocumentQueries {
		t.Fatalf("markdown_workspace document-query branch missing: %#v", branches)
	}
}
