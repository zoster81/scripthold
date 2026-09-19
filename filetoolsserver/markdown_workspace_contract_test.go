package filetoolsserver

import (
	"slices"
	"testing"
)

func TestMarkdownWorkspaceCatalogSchemaIsClosedReadOnlyUnion(t *testing.T) {
	tool := markdownWorkspaceCatalogTool()
	schema := markdownReadSchemaMap(t, tool.InputSchema)
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("markdown_workspace schema = %#v, want strict object", schema)
	}
	properties := markdownReadSchemaMap(t, schema["properties"])
	markdownReadAssertStringSet(t, "action enum", markdownReadSchemaMap(t, properties["action"])["enum"], []string{"inspect", "query", "repairPreview", "validate"})
	markdownReadAssertStringSet(t, "query enum", markdownReadSchemaMap(t, properties["query"])["enum"], []string{"backlinks", "edges", "outgoing", "reachable", "related"})
	markdownReadAssertStringSet(t, "backupPolicy enum", markdownReadSchemaMap(t, properties["backupPolicy"])["enum"], []string{"pinned", "required"})
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
	if !ok || len(branches) != 5 {
		t.Fatalf("markdown_workspace oneOf = %#v, want 5 closed shape variants", schema["oneOf"])
	}
	foundDocumentQueries := false
	foundRepairPreview := false
	for _, raw := range branches {
		branch := markdownReadSchemaMap(t, raw)
		branchProperties := markdownReadSchemaMap(t, branch["properties"])
		if action := markdownReadSchemaMap(t, branchProperties["action"])["const"]; action == "repairPreview" {
			foundRepairPreview = true
			required, _ := branch["required"].([]string)
			if !slices.Contains(required, "managedTocs") || branchProperties["backupPolicy"] == nil {
				t.Fatalf("repairPreview branch = %#v, want required managedTocs and optional backupPolicy", branch)
			}
		}
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
	if !foundRepairPreview {
		t.Fatalf("markdown_workspace repairPreview branch missing: %#v", branches)
	}
}
