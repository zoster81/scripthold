package filetoolsserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

const markdownReadMaxItems = 4096

func markdownReadCatalogTool() *mcp.Tool {
	tool := catalogTool("markdown_read")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           markdownReadProperties(),
		"oneOf": []any{
			markdownReadBranchSchema("inspect", []string{"action", "path", "limit"}, []string{"path", "encoding", "limit"}, nil),
			markdownReadBranchSchema("query", []string{"action", "path", "query", "limit"}, []string{"path", "encoding", "query", "limit", "kinds"}, map[string]string{"query": "nodes"}),
			markdownReadBranchSchema("query", []string{"action", "path", "query", "limit"}, []string{"path", "encoding", "query", "limit", "levels", "within"}, map[string]string{"query": "sections"}),
			markdownReadBranchSchema("query", []string{"action", "path", "query", "limit"}, []string{"path", "encoding", "query", "limit"}, map[string]string{"query": "relationships"}),
			markdownReadBranchSchema("get", []string{"action", "path", "targetId"}, []string{"path", "encoding", "targetId", "includeSource"}, nil),
			markdownReadBranchSchema("resolve", []string{"action", "path", "fragment"}, []string{"path", "encoding", "fragment"}, nil),
			markdownReadBranchSchema("validate", []string{"action", "path", "fragment"}, []string{"path", "encoding", "fragment"}, nil),
			markdownReadBranchSchema("validate", []string{"action", "path", "targetId"}, []string{"path", "encoding", "targetId"}, nil),
			markdownReadBranchSchema("generate", []string{"action", "path", "generate"}, []string{"path", "encoding", "generate"}, nil),
		},
	}
	return tool
}

func markdownReadProperties() map[string]any {
	return map[string]any{
		"action":   map[string]any{"type": "string", "enum": []string{"inspect", "query", "get", "resolve", "validate", "generate"}},
		"path":     map[string]any{"type": "string", "minLength": 1},
		"encoding": map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
		"limit":    map[string]any{"type": "integer", "minimum": 1, "maximum": markdownReadMaxItems},
		"query":    map[string]any{"type": "string", "enum": []string{"nodes", "sections", "relationships"}},
		"kinds": map[string]any{
			"type": "array", "maxItems": 64,
			"items": map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
		},
		"levels": map[string]any{
			"type": "array", "maxItems": 6,
			"items": map[string]any{"type": "integer", "minimum": 1, "maximum": 6},
		},
		"within": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"required":             []string{"start", "end"},
			"properties": map[string]any{
				"start": map[string]any{"type": "integer", "minimum": 0},
				"end":   map[string]any{"type": "integer", "minimum": 0},
			},
		},
		"targetId":      map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		"includeSource": map[string]any{"type": "boolean"},
		"fragment":      map[string]any{"type": "string", "minLength": 1, "maxLength": 2048},
		"generate":      map[string]any{"type": "string", "enum": []string{"toc", "canonical_markdown"}},
	}
}

func markdownReadBranchSchema(action string, required, legal []string, constants map[string]string) map[string]any {
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
