package filetoolsserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

var sourceQueryEvidenceValues = []string{
	"textual", "lexical", "structural", "scope-resolved", "project-resolved", "semantic",
}

func sourceQueryCatalogTool() *mcp.Tool {
	tool := catalogTool("source_query")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"operation", "paths"},
		"$defs": map[string]any{
			"s": sourceQuerySelectorSchema(),
			"i": sourceQueryIndexBindingSchema(),
		},
		"properties": map[string]any{
			"operation":        map[string]any{"enum": []string{"search", "relations", "context"}},
			"paths":            map[string]any{},
			"query":            map[string]any{},
			"mode":             map[string]any{"enum": []string{"textual", "lexical", "structural"}},
			"match":            map[string]any{"enum": []string{"exact", "prefix", "contains"}},
			"relation":         map[string]any{"enum": sourceQueryRelationKinds()},
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
			"evidence":         sourceQueryEvidenceSchema(),
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

func sourceQueryEvidenceSchema() map[string]any {
	return map[string]any{"items": map[string]any{"enum": sourceQueryEvidenceValues}}
}

func sourceQueryIndexBindingSchema() map[string]any {
	return map[string]any{
		"additionalProperties": false,
		"properties": map[string]any{
			"generation":  map[string]any{},
			"fingerprint": map[string]any{},
			"stalePolicy": map[string]any{"enum": []string{"reject", "allow"}},
		},
	}
}

func sourceQuerySelectorSchema() map[string]any {
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

func sourceQueryRelationKinds() []string {
	return []string{
		"dependencies", "dependents", "references", "definitions", "inheritance", "implementations",
		"overrides", "callers", "callees", "trace", "impact", "cycles",
	}
}
