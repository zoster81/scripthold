package filetoolsserver

import (
	"reflect"
	"sort"
	"testing"
)

func TestMarkdownReadCatalogSchemaIsClosedActionUnion(t *testing.T) {
	tool := markdownReadCatalogTool()
	schema := markdownReadSchemaMap(t, tool.InputSchema)
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("markdown_read schema = %#v, want strict object", schema)
	}

	properties := markdownReadSchemaMap(t, schema["properties"])
	wantProperties := []string{"action", "encoding", "fragment", "generate", "includeSource", "kinds", "levels", "limit", "path", "query", "targetId", "within"}
	gotProperties := make([]string, 0, len(properties))
	for name := range properties {
		gotProperties = append(gotProperties, name)
	}
	sort.Strings(gotProperties)
	if !reflect.DeepEqual(gotProperties, wantProperties) {
		t.Fatalf("markdown_read properties = %v, want %v", gotProperties, wantProperties)
	}
	markdownReadAssertStringSet(t, "action enum", markdownReadSchemaMap(t, properties["action"])["enum"], []string{"generate", "get", "inspect", "query", "resolve", "validate"})

	branches, ok := schema["oneOf"].([]any)
	if !ok || len(branches) != 9 {
		t.Fatalf("markdown_read oneOf = %#v, want 9 closed variants", schema["oneOf"])
	}

	wantVariants := map[string][]string{
		"inspect":             {"action", "limit", "path"},
		"query:nodes":         {"action", "limit", "path", "query"},
		"query:sections":      {"action", "limit", "path", "query"},
		"query:relationships": {"action", "limit", "path", "query"},
		"get":                 {"action", "path", "targetId"},
		"resolve":             {"action", "fragment", "path"},
		"validate:fragment":   {"action", "fragment", "path"},
		"validate:targetId":   {"action", "path", "targetId"},
		"generate":            {"action", "generate", "path"},
	}
	seen := map[string]bool{}
	for _, raw := range branches {
		branch := markdownReadSchemaMap(t, raw)
		if branch["type"] != "object" || branch["additionalProperties"] != false {
			t.Fatalf("markdown_read branch = %#v, want strict object", branch)
		}
		branchProps := markdownReadSchemaMap(t, branch["properties"])
		action := markdownReadSchemaMap(t, branchProps["action"])["const"]
		key, ok := action.(string)
		if !ok {
			t.Fatalf("branch action const = %#v", action)
		}
		if query, exists := branchProps["query"]; exists {
			if value, hasConst := markdownReadSchemaMap(t, query)["const"].(string); hasConst {
				key += ":" + value
			}
		}
		if key == "validate" {
			if _, exists := branchProps["fragment"]; exists {
				key += ":fragment"
			} else {
				key += ":targetId"
			}
		}
		wantRequired, exists := wantVariants[key]
		if !exists {
			t.Fatalf("unexpected markdown_read branch %q: %#v", key, branch)
		}
		markdownReadAssertStringSet(t, key+" required", branch["required"], wantRequired)
		seen[key] = true
	}
	for key := range wantVariants {
		if !seen[key] {
			t.Fatalf("missing markdown_read branch %q", key)
		}
	}

	within := markdownReadSchemaMap(t, properties["within"])
	if within["type"] != "object" || within["additionalProperties"] != false {
		t.Fatalf("within schema = %#v, want strict object", within)
	}
	markdownReadAssertStringSet(t, "within required", within["required"], []string{"end", "start"})
}

func markdownReadSchemaMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("schema value = %#v, want map[string]any", value)
	}
	return result
}

func markdownReadAssertStringSet(t *testing.T, label string, value any, want []string) {
	t.Helper()
	var got []string
	switch typed := value.(type) {
	case []string:
		got = append(got, typed...)
	case []any:
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				t.Fatalf("%s item = %#v, want string", label, item)
			}
			got = append(got, text)
		}
	default:
		t.Fatalf("%s = %#v, want string slice", label, value)
	}
	sort.Strings(got)
	want = append([]string(nil), want...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
