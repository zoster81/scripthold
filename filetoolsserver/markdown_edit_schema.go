package filetoolsserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

func markdownEditCatalogTool() *mcp.Tool {
	tool := catalogTool("markdown_edit")
	tool.InputSchema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"path", "operations"},
		"$defs": map[string]any{
			"targetId": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$"},
		},
		"properties": map[string]any{
			"path":     map[string]any{"type": "string", "minLength": 1},
			"encoding": map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
			"operations": map[string]any{
				"type":     "array",
				"minItems": 1,
				"maxItems": markdownintelligence.MaxEditOperations,
				"items": map[string]any{
					"oneOf": []any{
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "format"},
							"properties": map[string]any{
								"action":  map[string]any{"const": "create"},
								"subject": map[string]any{"const": "front_matter"},
								"format":  map[string]any{"type": "string", "enum": []string{"yaml", "toml"}},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject"},
							"properties": map[string]any{
								"action":  map[string]any{"const": "remove"},
								"subject": map[string]any{"const": "front_matter"},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "key", "value"},
							"properties": map[string]any{
								"action":  map[string]any{"const": "create"},
								"subject": map[string]any{"const": "front_matter_field"},
								"key":     map[string]any{"type": "string", "minLength": 1},
								"value":   map[string]any{"type": "string", "minLength": 1},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "label", "destination"},
							"properties": map[string]any{
								"action":      map[string]any{"const": "create"},
								"subject":     map[string]any{"const": "reference_definition"},
								"label":       map[string]any{"type": "string", "minLength": 1},
								"destination": map[string]any{"type": "string", "minLength": 1},
								"title":       map[string]any{"type": "string", "minLength": 1},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "label", "body"},
							"properties": map[string]any{
								"action":  map[string]any{"const": "create"},
								"subject": map[string]any{"const": "footnote_definition"},
								"label":   map[string]any{"type": "string", "minLength": 1},
								"body":    map[string]any{"type": "string", "minLength": 1},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "column", "alignment"},
							"properties": map[string]any{
								"action":    map[string]any{"const": "set"},
								"subject":   map[string]any{"const": "table"},
								"targetId":  map[string]any{"$ref": "#/$defs/targetId"},
								"part":      map[string]any{"const": "column_alignment"},
								"column":    map[string]any{"type": "integer", "minimum": 0},
								"alignment": map[string]any{"type": "string", "enum": []string{"default", "left", "right", "center"}},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "alignments"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "set"},
								"subject":  map[string]any{"const": "table"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "alignments"},
								"alignments": map[string]any{
									"type":     "array",
									"minItems": 1,
									"items":    map[string]any{"type": "string", "enum": []string{"default", "left", "right", "center"}},
								},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "rename"},
								"subject":  map[string]any{"const": "heading"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"text":     map[string]any{"type": "string"},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "rename"},
								"subject":  map[string]any{"type": "string", "enum": []string{"reference_definition", "front_matter_field", "footnote_definition"}},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "level"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "set"},
								"subject":  map[string]any{"const": "heading"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"level":    map[string]any{"type": "integer", "minimum": 1, "maximum": 6},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "sync"},
								"subject":  map[string]any{"const": "toc"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
							},
						},
						map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "markdown"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "paragraph"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"markdown": map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "remove"},
								"subject":  map[string]any{"type": "string", "enum": []string{"paragraph", "section", "list_item", "reference_definition", "front_matter_field", "thematic_break", "blockquote", "footnote_definition"}},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "position", "markdown"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "insert"},
								"subject":  map[string]any{"const": "paragraph"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"position": map[string]any{"type": "string", "enum": []string{"before", "after"}},
								"markdown": map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "position", "markdown"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "insert"},
								"subject":  map[string]any{"const": "section"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"position": map[string]any{"type": "string", "enum": []string{"before", "after", "child"}},
								"markdown": map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "markdown"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "section"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"type": "string", "enum": []string{"body", "subtree"}},
								"markdown": map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "anchorTargetId", "position"},
							"properties": map[string]any{
								"action":         map[string]any{"const": "move"},
								"subject":        map[string]any{"type": "string", "enum": []string{"section", "list_item"}},
								"targetId":       map[string]any{"$ref": "#/$defs/targetId"},
								"anchorTargetId": map[string]any{"$ref": "#/$defs/targetId"},
								"position":       map[string]any{"type": "string", "enum": []string{"before", "after"}},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "set"},
								"subject":  map[string]any{"const": "alert"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"text":     map[string]any{"type": "string", "enum": []string{"note", "tip", "important", "warning", "caution"}},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "markdown"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "list_item"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"markdown": map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "markdown"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "list_item"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "subtree"},
								"markdown": map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "position", "markdown"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "insert"},
								"subject":  map[string]any{"const": "list_item"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"position": map[string]any{"type": "string", "enum": []string{"before", "after", "child"}},
								"markdown": map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "checked"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "set"},
								"subject":  map[string]any{"const": "task"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"checked":  map[string]any{"type": "boolean"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"type": "string", "enum": []string{"code_span", "emphasis", "strong", "strikethrough"}},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"text":     map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "fenced_code"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "body"},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "set"},
								"subject":  map[string]any{"const": "fenced_code"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "info"},
								"text":     map[string]any{"type": "string"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "inline_link"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"type": "string", "enum": []string{"destination", "label", "title"}},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "image"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"type": "string", "enum": []string{"destination", "alt", "title"}},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"type": "string", "enum": []string{"autolink", "front_matter_field", "html_comment", "html_anchor", "math_expression", "blockquote", "alert", "footnote_definition"}},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "replace"},
								"subject":  map[string]any{"const": "reference_definition"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"type": "string", "enum": []string{"destination", "title"}},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "add"},
								"subject":  map[string]any{"const": "reference_definition"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "title"},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "remove"},
								"subject":  map[string]any{"const": "reference_definition"},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "title"},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part", "text"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "add"},
								"subject":  map[string]any{"type": "string", "enum": []string{"inline_link", "image"}},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "title"},
								"text":     map[string]any{"type": "string", "minLength": 1},
							},
						}, map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []string{"action", "subject", "targetId", "part"},
							"properties": map[string]any{
								"action":   map[string]any{"const": "remove"},
								"subject":  map[string]any{"type": "string", "enum": []string{"inline_link", "image"}},
								"targetId": map[string]any{"$ref": "#/$defs/targetId"},
								"part":     map[string]any{"const": "title"},
							},
						},
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
