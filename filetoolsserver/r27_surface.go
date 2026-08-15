package filetoolsserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

var r27SourceEvidenceValues = []string{
	"textual", "lexical", "structural", "scope-resolved", "project-resolved", "semantic",
}

func sourceQueryCatalogTool() *mcp.Tool {
	tool := catalogTool("source_query")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"operation", "paths"},
		"$defs": map[string]any{
			"s": r27SourceSelectorSchema(),
			"i": r27SourceIndexBindingSchema(),
		},
		"properties": map[string]any{
			"operation":        map[string]any{"enum": []string{"search", "relations", "context"}},
			"paths":            map[string]any{},
			"query":            map[string]any{},
			"mode":             map[string]any{"enum": []string{"textual", "lexical", "structural"}},
			"match":            map[string]any{"enum": []string{"exact", "prefix", "contains"}},
			"relation":         map[string]any{"enum": r27SourceRelationKinds()},
			"subject":          map[string]any{"$ref": "#/$defs/s"},
			"target":           map[string]any{"$ref": "#/$defs/s"},
			"targets":          map[string]any{"items": map[string]any{"$ref": "#/$defs/s"}},
			"budgetBytes":      map[string]any{},
			"bodyPolicy":       map[string]any{"enum": []string{"prefer", "signatures-only"}},
			"language":         map[string]any{},
			"encoding":         map[string]any{},
			"kinds":            map[string]any{},
			"includes":         map[string]any{},
			"excludes":         map[string]any{},
			"respectGitignore": map[string]any{},
			"evidence":         r27SourceEvidenceSchema(),
			"maxFiles":         map[string]any{},
			"maxResults":       map[string]any{},
			"maxNodes":         map[string]any{},
			"maxEdges":         map[string]any{},
			"maxDepth":         map[string]any{},
			"maxItems":         map[string]any{},
			"index":            map[string]any{"$ref": "#/$defs/i"},
		},
	}
	return tool
}

func r27SourceEvidenceSchema() map[string]any {
	return map[string]any{"items": map[string]any{"enum": r27SourceEvidenceValues}}
}

func r27SourceIndexBindingSchema() map[string]any {
	return map[string]any{
		"additionalProperties": false,
		"properties": map[string]any{
			"generation":  map[string]any{},
			"fingerprint": map[string]any{},
			"stalePolicy": map[string]any{"enum": []string{"reject", "allow"}},
		},
	}
}

func r27SourceSelectorSchema() map[string]any {
	return map[string]any{
		"additionalProperties": false,
		"properties": map[string]any{
			"kind":              map[string]any{"enum": []string{"symbol", "position", "path"}},
			"path":              map[string]any{},
			"symbolId":          map[string]any{},
			"sourceFingerprint": map[string]any{},
			"position": map[string]any{
				"additionalProperties": false,
				"properties":           map[string]any{"line": map[string]any{}, "column": map[string]any{}},
			},
		},
	}
}

func r27SourceRelationKinds() []string {
	return []string{
		"dependencies", "dependents", "references", "definitions", "inheritance", "implementations",
		"overrides", "callers", "callees", "trace", "impact", "cycles",
	}
}
