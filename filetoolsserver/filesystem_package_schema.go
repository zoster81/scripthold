package filetoolsserver

import "github.com/modelcontextprotocol/go-sdk/mcp"

func filesystemPackageCatalogTool() *mcp.Tool {
	tool := catalogTool("filesystem_package")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"formatVersion", "operations"},
		"properties": map[string]any{
			"formatVersion": map[string]any{
				"type":  "string",
				"const": "filesystem-package-v1",
			},
			"operations": map[string]any{
				"type":     "array",
				"minItems": 1,
				"items": map[string]any{
					"oneOf": []any{
						filesystemPackagePathOperationSchema("mkdir", "deleteFile", "deleteDirectory"),
						filesystemPackageCreateFileOperationSchema(),
						filesystemPackageSourceDestinationOperationSchema("copyFile", "copyDirectory", "move"),
					},
				},
			},
		},
	}
	return tool
}

func filesystemPackagePathOperationSchema(operationTypes ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "path"},
		"properties": map[string]any{
			"type": filesystemPackageOperationTypeSchema(operationTypes),
			"path": map[string]any{"type": "string", "minLength": 1},
		},
	}
}

func filesystemPackageCreateFileOperationSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "path", "contentBase64"},
		"properties": map[string]any{
			"type":          map[string]any{"type": "string", "const": "createFile"},
			"path":          map[string]any{"type": "string", "minLength": 1},
			"contentBase64": map[string]any{"type": "string", "contentEncoding": "base64"},
		},
	}
}

func filesystemPackageOperationTypeSchema(operationTypes []string) map[string]any {
	if len(operationTypes) == 1 {
		return map[string]any{"type": "string", "const": operationTypes[0]}
	}
	return map[string]any{"type": "string", "enum": operationTypes}
}

func filesystemPackageSourceDestinationOperationSchema(operationTypes ...string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"type", "source", "destination"},
		"properties": map[string]any{
			"type":        filesystemPackageOperationTypeSchema(operationTypes),
			"source":      map[string]any{"type": "string", "minLength": 1},
			"destination": map[string]any{"type": "string", "minLength": 1},
		},
	}
}
