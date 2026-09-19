package filetoolsserver

import "github.com/zoster81/scripthold/internal/markdownintelligence"

const (
	markdownCreateMaxElements = markdownintelligence.MaxCreateElements
	markdownCreateMaxDepth    = markdownintelligence.MaxCreateDepth
)

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
				"value": map[string]any{"type": "string"},
			}),
			"frontMatter": markdownCreateObject([]string{"format", "fields"}, map[string]any{
				"format": map[string]any{"type": "string", "enum": []string{"yaml", "toml"}},
				"fields": map[string]any{
					"type":     "array",
					"maxItems": markdownCreateMaxElements,
					"items":    map[string]any{"$ref": "#/$defs/frontMatterField"},
				},
			}),
			"inline": markdownCreateInlineSchema(),
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
	children := map[string]any{
		"type":     "array",
		"minItems": 1,
		"maxItems": markdownCreateMaxElements,
		"items":    map[string]any{"$ref": "#/$defs/inline"},
	}
	branches := []any{
		markdownCreateTaggedObject("text", []string{"text"}, map[string]any{"text": map[string]any{"type": "string"}}),
		markdownCreateTaggedObject("code", []string{"text"}, map[string]any{"text": map[string]any{"type": "string"}}),
	}
	for _, kind := range []string{"emphasis", "strong", "strikethrough"} {
		branches = append(branches, markdownCreateTaggedObject(kind, []string{"children"}, map[string]any{"children": children}))
	}
	for _, kind := range []string{"link", "image"} {
		branches = append(branches, markdownCreateTaggedObject(kind, []string{"destination", "children"}, map[string]any{
			"destination": map[string]any{"type": "string", "minLength": 1},
			"title":       map[string]any{"type": "string", "minLength": 1},
			"children":    children,
		}))
	}
	for _, kind := range []string{"autolink", "bare_autolink"} {
		branches = append(branches, markdownCreateTaggedObject(kind, []string{"value"}, map[string]any{
			"value": map[string]any{"type": "string", "minLength": 1},
		}))
	}
	for _, kind := range []string{"reference_link", "reference_image", "forward_reference_link", "forward_reference_image"} {
		branches = append(branches, markdownCreateTaggedObject(kind, []string{"reference", "children"}, map[string]any{
			"reference": map[string]any{"type": "string", "minLength": 1},
			"children":  children,
		}))
	}
	for _, kind := range []string{"collapsed_reference_link", "collapsed_reference_image", "shortcut_reference_link", "shortcut_reference_image"} {
		branches = append(branches, markdownCreateTaggedObject(kind, []string{"children"}, map[string]any{"children": children}))
	}
	branches = append(branches,
		markdownCreateTaggedObject("footnote_reference", []string{"label"}, map[string]any{"label": map[string]any{"type": "string", "minLength": 1}}),
		markdownCreateTaggedObject("math", []string{"payload"}, map[string]any{"payload": map[string]any{"type": "string"}}),
		markdownCreateTaggedObject("math_backtick", []string{"payload"}, map[string]any{"payload": map[string]any{"type": "string"}}),
	)
	return map[string]any{"oneOf": branches}
}

func markdownCreateBlockSchema() map[string]any {
	inlineContent := map[string]any{
		"type":     "array",
		"minItems": 1,
		"maxItems": markdownCreateMaxElements,
		"items":    map[string]any{"$ref": "#/$defs/inline"},
	}
	childBlocks := map[string]any{
		"type":     "array",
		"minItems": 1,
		"maxItems": markdownCreateMaxElements,
		"items":    map[string]any{"$ref": "#/$defs/block"},
	}
	depth := map[string]any{"type": "integer", "minimum": 1, "maximum": markdownCreateMaxDepth}
	alertKind := map[string]any{"type": "string", "enum": []string{"note", "tip", "important", "warning", "caution"}}
	branches := []any{
		markdownCreateTaggedObject("heading", []string{"level", "content"}, map[string]any{
			"level": map[string]any{"type": "integer", "minimum": 1, "maximum": 6}, "content": inlineContent,
		}),
		markdownCreateTaggedObject("heading", []string{"level", "rawMarkdown"}, map[string]any{
			"level": map[string]any{"type": "integer", "minimum": 1, "maximum": 6}, "rawMarkdown": map[string]any{"type": "string", "minLength": 1},
		}),
		markdownCreateTaggedObject("paragraph", []string{"content"}, map[string]any{"content": inlineContent}),
		markdownCreateTaggedObject("paragraph", []string{"rawMarkdown"}, map[string]any{"rawMarkdown": map[string]any{"type": "string", "minLength": 1}}),
		markdownCreateTaggedObject("thematic_break", nil, nil),
		markdownCreateTaggedObject("blockquote", []string{"depth", "content"}, map[string]any{"depth": depth, "content": inlineContent}),
		markdownCreateTaggedObject("blockquote", []string{"depth", "rawMarkdown"}, map[string]any{"depth": depth, "rawMarkdown": map[string]any{"type": "string", "minLength": 1}}),
		markdownCreateTaggedObject("blockquote", []string{"depth", "blocks"}, map[string]any{"depth": depth, "blocks": childBlocks}),
		markdownCreateTaggedObject("alert", []string{"kind", "content"}, map[string]any{"kind": alertKind, "content": inlineContent}),
		markdownCreateTaggedObject("alert", []string{"kind", "rawMarkdown"}, map[string]any{"kind": alertKind, "rawMarkdown": map[string]any{"type": "string", "minLength": 1}}),
		markdownCreateTaggedObject("alert", []string{"kind", "blocks"}, map[string]any{"kind": alertKind, "blocks": childBlocks}),
		markdownCreateTaggedObject("list", []string{"ordered", "items"}, map[string]any{
			"ordered": map[string]any{"type": "boolean"},
			"items":   map[string]any{"type": "array", "minItems": 1, "maxItems": markdownCreateMaxElements, "items": map[string]any{"$ref": "#/$defs/listItem"}},
		}),
		markdownCreateTaggedObject("task_list", []string{"ordered", "items"}, map[string]any{
			"ordered": map[string]any{"type": "boolean"},
			"items":   map[string]any{"type": "array", "minItems": 1, "maxItems": markdownCreateMaxElements, "items": map[string]any{"$ref": "#/$defs/taskListItem"}},
		}),
		markdownCreateTaggedObject("fenced_code", []string{"content"}, map[string]any{
			"content": map[string]any{"type": "string"}, "info": map[string]any{"type": "string"},
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
	allRequired := append([]string{"type"}, required...)
	allProperties := map[string]any{"type": map[string]any{"const": kind}}
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
