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
			markdownWorkspaceBranchSchema("query", []string{"action", "root", "discovery", "query", "document", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "query", "document"}, map[string]string{"query": "outgoing"}),
			markdownWorkspaceBranchSchema("query", []string{"action", "root", "discovery", "query", "document", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "query", "document"}, map[string]string{"query": "backlinks"}),
			markdownWorkspaceBranchSchema("query", []string{"action", "root", "discovery", "query", "document", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "query", "document"}, map[string]string{"query": "reachable"}),
			markdownWorkspaceBranchSchema("query", []string{"action", "root", "discovery", "query", "document", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "query", "document"}, map[string]string{"query": "related"}),
			markdownWorkspaceBranchSchema("validate", []string{"action", "root", "discovery", "limit"}, []string{"root", "discovery", "maxDocuments", "maxRelationships", "maxBytes", "maxDepth", "limit", "roots"}, nil),
		},
	}
	return tool
}

func markdownWorkspaceProperties() map[string]any {
	return map[string]any{
		"action": map[string]any{"type": "string", "enum": []string{"inspect", "query", "validate"}},
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
	}
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
