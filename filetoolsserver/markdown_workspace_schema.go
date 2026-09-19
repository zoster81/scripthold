package filetoolsserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
)

const markdownWorkspaceMaxItems = 4096

func markdownWorkspaceCatalogTool() *mcp.Tool {
	tool := catalogTool("markdown_workspace")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           markdownWorkspaceProperties(),
		"oneOf": []any{
			markdownWorkspaceBranchSchema("inspect", []string{"action", "root", "discovery", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit"}, nil),
			markdownWorkspaceBranchSchema("query", []string{"action", "root", "discovery", "query", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "query"}, map[string]string{"query": "edges"}),
			markdownWorkspaceDocumentQueryBranchSchema(),
			markdownWorkspaceBranchSchema("validate", []string{"action", "root", "discovery", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "roots", "managedTocs"}, nil),
			markdownWorkspaceBranchSchema("repairPreview", []string{"action", "root", "discovery", "limit", "managedTocs"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "managedTocs", "backupPolicy"}, nil),
		},
	}
	return tool
}

func markdownWorkspaceProperties() map[string]any {
	return map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{"inspect", "query", "validate", "repairPreview"}},
		"root":   map[string]any{"type": "string", "minLength": 1},
		"discovery": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"mode": map[string]any{"type": "string", "enum": []string{"scan", "follow"}},
				"entries": map[string]any{
					"type": "array", "minItems": 1, "maxItems": config.HardMarkdownWorkspaceMaxDocuments,
					"items": map[string]any{"type": "string", "minLength": 1},
				},
			},
			"oneOf": []any{
				map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"mode"},
					"properties":           map[string]any{"mode": map[string]any{"const": "scan"}},
				},
				map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"mode", "entries"},
					"properties": map[string]any{
						"mode":    map[string]any{"const": "follow"},
						"entries": map[string]any{},
					},
				},
			},
		},
		"maxDocuments":     map[string]any{"type": "integer", "minimum": 1, "maximum": config.HardMarkdownWorkspaceMaxDocuments},
		"maxRelationships": map[string]any{"type": "integer", "minimum": 1, "maximum": config.HardMarkdownWorkspaceMaxRelationships},
		"maxBytes":         map[string]any{"type": "integer", "minimum": 1, "maximum": config.HardMaxFilesystemAggregateBytes},
		"maxDepth":         map[string]any{"type": "integer", "minimum": 0, "maximum": config.HardMaxFilesystemRecursiveDepth},
		"limit":            map[string]any{"type": "integer", "minimum": 1, "maximum": markdownWorkspaceMaxItems},
		"query":            map[string]any{"type": "string", "enum": []string{"edges", "outgoing", "backlinks", "reachable", "related"}},
		"document":         map[string]any{"type": "string", "minLength": 1},
		"roots": map[string]any{
			"type": "array", "maxItems": config.HardMarkdownWorkspaceMaxDocuments,
			"items": map[string]any{"type": "string", "minLength": 1},
		},
		"backupPolicy": map[string]any{"type": "string", "enum": []string{"required", "pinned"}},
		"managedTocs": map[string]any{
			"type": "array", "maxItems": markdownWorkspaceMaxItems,
			"items": map[string]any{
				"type": "object", "additionalProperties": false,
				"required": []string{"document", "fragment"},
				"properties": map[string]any{
					"document": map[string]any{"type": "string", "minLength": 1},
					"fragment": map[string]any{"type": "string", "minLength": 1},
				},
			},
		},
	}
}

func markdownWorkspaceDocumentQueryBranchSchema() map[string]any {
	schema := markdownWorkspaceBranchSchema(
		"query",
		[]string{"action", "root", "discovery", "query", "document", "limit"},
		[]string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "query", "document"},
		nil,
	)
	properties := schema["properties"].(map[string]any)
	properties["query"] = map[string]any{"type": "string", "enum": []string{"outgoing", "backlinks", "reachable", "related"}}
	return schema
}

func markdownWorkspaceBranchSchema(action string, required, legal []string, constants map[string]string) map[string]any {
	properties := make(map[string]any, len(legal)+1)
	properties["action"] = map[string]any{"const": action}
	for _, name := range legal {
		properties[name] = map[string]any{}
	}
	for name, value := range constants {
		properties[name] = map[string]any{"const": value}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}
