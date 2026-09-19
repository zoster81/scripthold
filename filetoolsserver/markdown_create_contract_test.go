package filetoolsserver

import "testing"

func TestMarkdownCreateInputSchemaIsClosedRecursiveDocumentModel(t *testing.T) {
	schema := markdownCreateInputSchema()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("top-level schema is not closed: %#v", schema)
	}
	required, ok := schema["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "path" {
		t.Fatalf("required = %#v, want [path]", schema["required"])
	}
	properties := schema["properties"].(map[string]any)
	for _, name := range []string{"path", "encoding", "bom", "frontMatter", "blocks"} {
		if properties[name] == nil {
			t.Fatalf("missing top-level property %q", name)
		}
	}

	defs := schema["$defs"].(map[string]any)
	for _, name := range []string{"frontMatterField", "frontMatter", "inline", "listItem", "taskListItem", "block"} {
		if defs[name] == nil {
			t.Fatalf("missing $defs entry %q", name)
		}
	}

	inlineKinds := schemaTaggedKinds(t, defs["inline"].(map[string]any))
	for _, kind := range []string{
		"text", "code", "emphasis", "strong", "strikethrough", "link", "image",
		"autolink", "bare_autolink", "reference_link", "reference_image",
		"forward_reference_link", "forward_reference_image", "collapsed_reference_link",
		"collapsed_reference_image", "shortcut_reference_link", "shortcut_reference_image",
		"footnote_reference", "math", "math_backtick",
	} {
		if !inlineKinds[kind] {
			t.Fatalf("inline kind %q missing", kind)
		}
	}

	blockKinds := schemaTaggedKinds(t, defs["block"].(map[string]any))
	for _, kind := range []string{
		"heading", "paragraph", "thematic_break", "blockquote", "alert", "list",
		"task_list", "fenced_code", "reference_definition", "footnote_definition",
		"math_block", "table",
	} {
		if !blockKinds[kind] {
			t.Fatalf("block kind %q missing", kind)
		}
	}

	blockBranches := defs["block"].(map[string]any)["oneOf"].([]any)
	foundRecursiveBlocks := false
	for _, raw := range blockBranches {
		branch := raw.(map[string]any)
		props := branch["properties"].(map[string]any)
		blocks, ok := props["blocks"].(map[string]any)
		if !ok {
			continue
		}
		items := blocks["items"].(map[string]any)
		if items["$ref"] == "#/$defs/block" {
			foundRecursiveBlocks = true
			break
		}
	}
	if !foundRecursiveBlocks {
		t.Fatal("recursive block $ref missing")
	}

	inlineBranches := defs["inline"].(map[string]any)["oneOf"].([]any)
	foundRecursiveInline := false
	for _, raw := range inlineBranches {
		branch := raw.(map[string]any)
		props := branch["properties"].(map[string]any)
		children, ok := props["children"].(map[string]any)
		if !ok {
			continue
		}
		items := children["items"].(map[string]any)
		if items["$ref"] == "#/$defs/inline" {
			foundRecursiveInline = true
			break
		}
	}
	if !foundRecursiveInline {
		t.Fatal("recursive inline $ref missing")
	}
}

func schemaTaggedKinds(t *testing.T, schema map[string]any) map[string]bool {
	t.Helper()
	branches, ok := schema["oneOf"].([]any)
	if !ok || len(branches) == 0 {
		t.Fatalf("oneOf = %#v", schema["oneOf"])
	}
	kinds := make(map[string]bool)
	for _, raw := range branches {
		branch := raw.(map[string]any)
		if branch["additionalProperties"] != false {
			t.Fatalf("branch is not closed: %#v", branch)
		}
		props := branch["properties"].(map[string]any)
		typeSchema := props["type"].(map[string]any)
		kind, _ := typeSchema["const"].(string)
		if kind == "" {
			t.Fatalf("branch type const missing: %#v", branch)
		}
		kinds[kind] = true
	}
	return kinds
}
