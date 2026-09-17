package filetoolsserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

func markdownEditCatalogTool() *mcp.Tool {
	tool := catalogTool("markdown_edit")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"path", "operations"},
		"properties": map[string]any{
			"path":     map[string]any{"type": "string", "minLength": 1},
			"encoding": map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
			"operations": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": 1,
				"items": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"action", "subject", "targetId", "text"},
					"properties": map[string]any{
						"action":   map[string]any{"const": "rename"},
						"subject":  map[string]any{"const": "heading"},
						"targetId": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
						"text":     map[string]any{"type": "string"},
					},
				},
			},
			"backupPolicy": map[string]any{"type": "string", "enum": []string{"required", "pinned"}},
		},
	}
	return tool
}

func markdownApplyCatalogTool() *mcp.Tool {
	tool := catalogTool("markdown_apply")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"previewId"},
		"properties": map[string]any{
			"previewId": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		},
	}
	return tool
}
