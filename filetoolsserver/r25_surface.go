package filetoolsserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

const (
	r25SourceMaxInputPaths = 256
	r25SourceMaxFiles      = 4096
	r25SourceMaxSymbols    = 100000
	r25SourceMaxShowBytes  = 8 * 1024 * 1024
)

func sourceSymbolsCatalogTool() *mcp.Tool {
	tool := catalogTool("source_symbols")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           sourceSymbolsProperties(),
		"oneOf": []any{
			sourceSymbolsBranchSchema("outline", []string{"operation", "paths"}, []string{"paths", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "maxSymbols", "maxFiles"}),
			sourceSymbolsBranchSchema("digest", []string{"operation", "paths"}, []string{"paths", "language", "encoding", "includes", "excludes", "respectGitignore", "maxFiles"}),
			sourceSymbolsBranchSchema("find", []string{"operation", "paths", "query"}, []string{"paths", "query", "match", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "maxSymbols", "maxFiles"}),
			sourceSymbolsBranchSchema("show", []string{"operation", "path", "symbolId", "sourceFingerprint", "language", "encoding"}, []string{"path", "symbolId", "sourceFingerprint", "language", "encoding", "maxBytes"}),
		},
	}
	return tool
}

func sourceSymbolsProperties() map[string]any {
	return map[string]any{
		"operation": map[string]any{"type": "string", "enum": []string{"outline", "digest", "find", "show"}},
		"paths": map[string]any{
			"type": "array", "minItems": 1, "maxItems": r25SourceMaxInputPaths,
			"items": map[string]any{"type": "string", "minLength": 1},
		},
		"path":              map[string]any{"type": "string", "minLength": 1},
		"query":             map[string]any{"type": "string", "minLength": 1, "maxLength": 512},
		"match":             map[string]any{"type": "string", "enum": []string{"exact", "prefix", "qualified"}},
		"language":          sourceSymbolsLanguageSchema(),
		"encoding":          sourceSymbolsEncodingSchema(),
		"kinds":             sourceSymbolsKindsSchema(),
		"includes":          sourceSymbolsPatternsSchema(),
		"excludes":          sourceSymbolsPatternsSchema(),
		"respectGitignore":  map[string]any{"type": "boolean"},
		"includeSignatures": map[string]any{"type": "boolean"},
		"maxSymbols":        sourceSymbolsBoundedIntegerSchema(r25SourceMaxSymbols),
		"maxFiles":          sourceSymbolsBoundedIntegerSchema(r25SourceMaxFiles),
		"symbolId":          map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		"sourceFingerprint": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		"maxBytes":          sourceSymbolsBoundedIntegerSchema(r25SourceMaxShowBytes),
	}
}

func sourceSymbolsBranchSchema(operation string, required, legal []string) map[string]any {
	properties := make(map[string]any, len(legal)+1)
	properties["operation"] = map[string]any{"const": operation}
	for _, name := range legal {
		properties[name] = map[string]any{}
	}
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}

func sourceSymbolsBoundedIntegerSchema(maximum int) map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": maximum}
}

func sourceSymbolsLanguageSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
}

func sourceSymbolsEncodingSchema() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 64}
}

func sourceSymbolsPatternsSchema() map[string]any {
	return map[string]any{
		"type": "array", "maxItems": 64,
		"items": map[string]any{"type": "string", "minLength": 1, "maxLength": 512},
	}
}

func sourceSymbolsKindsSchema() map[string]any {
	return map[string]any{
		"type": "array", "maxItems": 32,
		"items": map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
	}
}
