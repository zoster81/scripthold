package filetoolsserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

const (
	markdownCreateMaxElements = markdownintelligence.MaxCreateElements
	markdownCreateMaxDepth    = markdownintelligence.MaxCreateDepth
)

func markdownCreateCatalogTool() *mcp.Tool {
	tool := catalogTool("markdown_create")
	tool.InputSchema = markdownCreateInputSchema()
	return tool
}

func markdownCreateInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []string{"path"},
		"properties": map[string]any{
			"path":        map[string]any{"type": "string", "minLength": 1},
			"encoding":    map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
			"bom":         map[string]any{"type": "string", "enum": []string{"auto", "always", "never"}},
			"frontMatter": map[string]any{"$ref": "#/$defs/frontMatter"},
			"blocks": map[string]any{
				"type":     "array",
				"maxItems": markdownCreateMaxElements,
				"items":    map[string]any{"$ref": "#/$defs/block"},
			},
		},
		"$defs": map[string]any{
			"frontMatterField": markdownCreateObject([]string{"key", "value"}, map[string]any{
				"key":   map[string]any{"type": "string", "minLength": 1},
				"value": map[string]any{"type": "string", "minLength": 1},
			}),
			"frontMatter": markdownCreateObject([]string{"format", "fields"}, map[string]any{
				"format": map[string]any{"type": "string", "enum": []string{"yaml", "toml"}},
				"fields": map[string]any{
					"type":     "array",
					"maxItems": markdownCreateMaxElements,
					"items":    map[string]any{"$ref": "#/$defs/frontMatterField"},
				},
			}),
			"inlineContent": map[string]any{
				"type": "array", "minItems": 1, "maxItems": markdownCreateMaxElements,
				"items": map[string]any{"$ref": "#/$defs/inline"},
			},
			"childBlocks": map[string]any{
				"type": "array", "minItems": 1, "maxItems": markdownCreateMaxElements,
				"items": map[string]any{"$ref": "#/$defs/block"},
			},
			"blockDepth": map[string]any{"type": "integer", "minimum": 1, "maximum": markdownCreateMaxDepth},
			"alertKind":  map[string]any{"type": "string", "enum": []string{"note", "tip", "important", "warning", "caution"}},
			"inline":     markdownCreateInlineSchema(),
			"listItem": markdownCreateObject([]string{"markdown", "depth"}, map[string]any{
				"markdown": map[string]any{"type": "string", "minLength": 1},
				"depth":    map[string]any{"type": "integer", "minimum": 0, "maximum": markdownCreateMaxDepth - 1},
			}),
			"taskListItem": markdownCreateObject([]string{"markdown", "checked", "depth"}, map[string]any{
				"markdown": map[string]any{"type": "string", "minLength": 1},
				"checked":  map[string]any{"type": "boolean"},
				"depth":    map[string]any{"type": "integer", "minimum": 0, "maximum": markdownCreateMaxDepth - 1},
			}),
			"block": markdownCreateBlockSchema(),
		},
	}
}

func markdownCreateInlineSchema() map[string]any {
	children := map[string]any{"$ref": "#/$defs/inlineContent"}
	branches := []any{
		markdownCreateTaggedObjects([]string{"text", "code"}, []string{"text"}, map[string]any{"text": map[string]any{"type": "string"}}),
		markdownCreateTaggedObjects([]string{"emphasis", "strong", "strikethrough"}, []string{"children"}, map[string]any{"children": children}),
		markdownCreateTaggedObjects([]string{"link", "image"}, []string{"destination", "children"}, map[string]any{
			"destination": map[string]any{"type": "string", "minLength": 1},
			"title":       map[string]any{"type": "string", "minLength": 1},
			"children":    children,
		}),
		markdownCreateTaggedObjects([]string{"autolink", "bare_autolink"}, []string{"value"}, map[string]any{
			"value": map[string]any{"type": "string"},
		}),
		markdownCreateTaggedObjects([]string{"reference_link", "reference_image", "forward_reference_link", "forward_reference_image"}, []string{"reference", "children"}, map[string]any{
			"reference": map[string]any{"type": "string", "minLength": 1},
			"children":  children,
		}),
		markdownCreateTaggedObjects([]string{"collapsed_reference_link", "collapsed_reference_image", "shortcut_reference_link", "shortcut_reference_image"}, []string{"children"}, map[string]any{"children": children}),
		markdownCreateTaggedObject("footnote_reference", []string{"label"}, map[string]any{"label": map[string]any{"type": "string", "minLength": 1}}),
		markdownCreateTaggedObjects([]string{"math", "math_backtick"}, []string{"payload"}, map[string]any{"payload": map[string]any{"type": "string"}}),
	}
	return map[string]any{"oneOf": branches}
}

func markdownCreateBlockSchema() map[string]any {
	inlineContent := map[string]any{"$ref": "#/$defs/inlineContent"}
	childBlocks := map[string]any{"$ref": "#/$defs/childBlocks"}
	depth := map[string]any{"$ref": "#/$defs/blockDepth"}
	alertKind := map[string]any{"$ref": "#/$defs/alertKind"}
	branches := []any{
		markdownCreateTaggedAlternatives("heading", []string{"level"}, map[string]any{
			"level":   map[string]any{"type": "integer", "minimum": 1, "maximum": 6},
			"content": inlineContent, "rawMarkdown": map[string]any{"type": "string", "minLength": 1},
		}, []string{"content"}, []string{"rawMarkdown"}),
		markdownCreateTaggedAlternatives("paragraph", nil, map[string]any{
			"content": inlineContent, "rawMarkdown": map[string]any{"type": "string", "minLength": 1},
		}, []string{"content"}, []string{"rawMarkdown"}),
		markdownCreateTaggedObject("thematic_break", nil, nil),
		markdownCreateTaggedAlternatives("blockquote", []string{"depth"}, map[string]any{
			"depth": depth, "content": inlineContent, "rawMarkdown": map[string]any{"type": "string", "minLength": 1}, "blocks": childBlocks,
		}, []string{"content"}, []string{"rawMarkdown"}, []string{"blocks"}),
		markdownCreateTaggedAlternatives("alert", []string{"kind"}, map[string]any{
			"kind": alertKind, "content": inlineContent, "rawMarkdown": map[string]any{"type": "string", "minLength": 1}, "blocks": childBlocks,
		}, []string{"content"}, []string{"rawMarkdown"}, []string{"blocks"}),
		markdownCreateTaggedObject("list", []string{"ordered", "items"}, map[string]any{
			"ordered": map[string]any{"type": "boolean"},
			"items":   map[string]any{"type": "array", "minItems": 1, "maxItems": markdownCreateMaxElements, "items": map[string]any{"$ref": "#/$defs/listItem"}},
		}),
		markdownCreateTaggedObject("task_list", []string{"ordered", "items"}, map[string]any{
			"ordered": map[string]any{"type": "boolean"},
			"items":   map[string]any{"type": "array", "minItems": 1, "maxItems": markdownCreateMaxElements, "items": map[string]any{"$ref": "#/$defs/taskListItem"}},
		}),
		markdownCreateTaggedObject("fenced_code", []string{"code"}, map[string]any{
			"code": map[string]any{"type": "string"}, "info": map[string]any{"type": "string"},
		}),
		markdownCreateTaggedObject("reference_definition", []string{"label", "destination"}, map[string]any{
			"label": map[string]any{"type": "string", "minLength": 1}, "destination": map[string]any{"type": "string", "minLength": 1},
			"title": map[string]any{"type": "string", "minLength": 1}, "deferred": map[string]any{"type": "boolean"},
		}),
		markdownCreateTaggedObject("footnote_definition", []string{"label", "body"}, map[string]any{
			"label": map[string]any{"type": "string", "minLength": 1}, "body": map[string]any{"type": "string", "minLength": 1},
			"deferred": map[string]any{"type": "boolean"},
		}),
		markdownCreateTaggedObject("math_block", []string{"payload"}, map[string]any{"payload": map[string]any{"type": "string"}}),
		markdownCreateTaggedObject("table", []string{"header"}, map[string]any{
			"header": map[string]any{"type": "array", "minItems": 1, "maxItems": markdownCreateMaxElements, "items": map[string]any{"type": "string"}},
			"rows": map[string]any{"type": "array", "maxItems": markdownCreateMaxElements, "items": map[string]any{
				"type": "array", "maxItems": markdownCreateMaxElements, "items": map[string]any{"type": "string"},
			}},
			"alignments": map[string]any{"type": "array", "maxItems": markdownCreateMaxElements, "items": map[string]any{
				"type": "string", "enum": []string{"default", "left", "right", "center"},
			}},
		}),
	}
	return map[string]any{"oneOf": branches}
}

func markdownCreateTaggedObject(kind string, required []string, properties map[string]any) map[string]any {
	return markdownCreateTaggedObjects([]string{kind}, required, properties)
}

func markdownCreateTaggedAlternatives(kind string, required []string, properties map[string]any, alternatives ...[]string) map[string]any {
	schema := markdownCreateTaggedObject(kind, required, properties)
	oneOf := make([]any, 0, len(alternatives))
	for _, alternative := range alternatives {
		oneOf = append(oneOf, map[string]any{"required": alternative})
	}
	schema["oneOf"] = oneOf
	return schema
}

func markdownCreateTaggedObjects(kinds []string, required []string, properties map[string]any) map[string]any {
	allRequired := append([]string{"type"}, required...)
	typeSchema := map[string]any{"enum": kinds}
	if len(kinds) == 1 {
		typeSchema = map[string]any{"const": kinds[0]}
	}
	allProperties := map[string]any{"type": typeSchema}
	for name, schema := range properties {
		allProperties[name] = schema
	}
	return markdownCreateObject(allRequired, allProperties)
}

func markdownCreateObject(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           properties,
	}
}
