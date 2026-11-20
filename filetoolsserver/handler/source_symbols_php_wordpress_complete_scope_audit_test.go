package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// WordPress source-first inventory: top-level functions, classes, class
// properties, and class methods from the three completely read original files.
// Function-local static declarations and global statements are not class fields.
func TestPHPWordPressOriginalCompleteSupportedDeclarationInventory(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original PHP project corpus not configured")
	}
	project := filepath.Join(corpus, "php54-wordpress-audit-selected")
	type decl struct {
		file                       string
		line                       int
		name, kind, native, parent string
	}
	expected := []decl{
		{"src/wp-includes/plugin.php", 122, "add_filter", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 174, "apply_filters", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 229, "apply_filters_ref_array", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 287, "has_filter", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 319, "remove_filter", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 347, "remove_all_filters", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 370, "current_filter", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 397, "doing_filter", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 417, "did_filter", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 447, "add_action", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 488, "do_action", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 543, "do_action_ref_array", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 599, "has_action", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 623, "remove_action", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 637, "remove_all_actions", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 648, "current_action", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 672, "doing_action", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 686, "did_action", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 723, "apply_filters_deprecated", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 750, "do_action_deprecated", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 776, "plugin_basename", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 813, "wp_register_plugin_realpath", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 846, "plugin_dir_path", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 858, "plugin_dir_url", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 880, "register_activation_hook", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 903, "register_deactivation_hook", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 934, "register_uninstall_hook", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 972, "_wp_call_all_hook", "function", "function-definition", ""},
		{"src/wp-includes/plugin.php", 1002, "_wp_filter_build_unique_id", "function", "function-definition", ""},
		{"src/wp-includes/class-wp-hook.php", 27, "WP_Hook", "class", "class", ""},
		{"src/wp-includes/class-wp-hook.php", 36, "$callbacks", "property", "property", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 44, "$priorities", "property", "property", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 52, "$iterations", "property", "property", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 60, "$current_priority", "property", "property", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 68, "$nesting_level", "property", "property", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 76, "$doing_action", "property", "property", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 91, "add_filter", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 130, "resort_active_iterations", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 205, "remove_filter", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 252, "has_filter", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 283, "has_filters", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 300, "remove_all_filters", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 328, "apply_filters", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 375, "do_action", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 392, "do_all_hook", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 416, "current_priority", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 454, "build_preinitialized_hooks", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 492, "offsetExists", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 508, "offsetGet", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 524, "offsetSet", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 544, "offsetUnset", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 560, "current", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 575, "next", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 589, "key", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 603, "valid", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-hook.php", 615, "rewind", "method", "method-definition", "WP_Hook"},
		{"src/wp-includes/class-wp-error.php", 19, "WP_Error", "class", "class", ""},
		{"src/wp-includes/class-wp-error.php", 26, "$errors", "property", "property", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 34, "$error_data", "property", "property", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 42, "$additional_data", "property", "property", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 61, "WP_Error", "constructor", "constructor-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 76, "get_error_codes", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 91, "get_error_code", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 110, "get_error_messages", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 136, "get_error_message", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 155, "get_error_data", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 170, "has_errors", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 184, "add", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 214, "add_data", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 234, "get_all_error_data", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 263, "remove", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 277, "merge_from", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 289, "export_to", "method", "method-definition", "WP_Error"},
		{"src/wp-includes/class-wp-error.php", 302, "copy_errors", "method", "method-definition", "WP_Error"},
	}
	files := []string{"src/wp-includes/plugin.php", "src/wp-includes/class-wp-hook.php", "src/wp-includes/class-wp-error.php"}
	original := map[string][]string{}
	paths := make([]string, 0, len(files))
	for _, rel := range files {
		full := filepath.Join(project, filepath.FromSlash(rel))
		requireAdditionalOriginalFile(t, full)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 1<<20 {
			t.Fatalf("original oversized: %s", rel)
		}
		original[rel] = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		paths = append(paths, full)
	}
	oracle := make(map[string]decl, len(expected))
	for _, d := range expected {
		lines, ok := original[d.file]
		sourceName := d.name
		if d.kind == "constructor" {
			// The PHP analyzer uses the declaring class as the normalized constructor name.
			sourceName = "__construct"
		}
		if !ok || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], sourceName) {
			t.Fatalf("bad original oracle %s:%d %s", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, duplicate := oracle[key]; duplicate {
			t.Fatalf("duplicate original declaration location: %s", key)
		}
		oracle[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "php", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: len(paths), MaxSymbols: 128,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(files) {
		t.Fatalf("incomplete WordPress analysis err=%v failed=%+v out=%+v", err, failed, out)
	}
	actual := make(map[string]bool, len(out.Symbols))
	for _, s := range out.Symbols {
		rel, err := filepath.Rel(project, s.Path)
		if err != nil {
			t.Errorf("unrelated symbol path %s", s.Path)
			continue
		}
		key := filepath.ToSlash(rel) + ":" + strconv.Itoa(s.NameRange.Start.Line)
		if actual[key] {
			t.Errorf("duplicate result at %s", key)
		}
		actual[key] = true
		d, ok := oracle[key]
		if !ok {
			t.Errorf("invented or local-scoped symbol %s %s %s owner=%s", key, s.Kind, s.Name, s.ParentQualifiedName)
			continue
		}
		if string(s.Kind) != d.kind || s.Name != d.name || s.NativeKind != d.native || s.ParentQualifiedName != d.parent {
			t.Errorf("source mismatch %s got %s %s native=%s owner=%s; want %+v", key, s.Kind, s.Name, s.NativeKind, s.ParentQualifiedName, d)
		}
	}
	for key, d := range oracle {
		if !actual[key] {
			t.Errorf("missing original declaration %s %s", key, d.name)
		}
	}
	if len(out.Symbols) != len(expected) {
		t.Errorf("indexed %d vs %d independently inventoried original declarations", len(out.Symbols), len(expected))
	}
	t.Logf("WordPress source-first: three full original files and %d supported package/class declarations verified", len(expected))
}
