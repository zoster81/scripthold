package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Direct Vue Options watch callbacks are distinct authored edit targets.
// Original components from independent projects exercise watched state and
// neighboring methods without requiring any JavaScript to run.
func TestVueOptionsDirectWatchShorthandNavigation(t *testing.T) {
	check := func(t *testing.T, source, watchName string, line int, neighbor string, forbidden []string) {
		t.Helper()
		result, err := (VueAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("original.vue", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated {
			t.Fatalf("unexpected truncation: %+v", result.Analysis)
		}
		watched, adjacent := 0, false
		for _, sym := range result.Analysis.Symbols {
			if sym.Name == watchName && sym.NativeKind == "vue-options-watch" {
				if sym.Kind != SymbolKindMethod || sym.NameRange.Start.Line != line ||
					sym.BodyRange == nil || sym.ID == "" || sym.RegionID == "" {
					t.Errorf("invalid watch declaration: %+v", sym)
				}
				watched++
			}
			if sym.Name == neighbor && sym.NativeKind == "vue-options-method" {
				adjacent = true
			}
			for _, bad := range forbidden {
				if sym.Name == bad && sym.NativeKind == "vue-options-watch" {
					t.Fatalf("invented watcher %q: %+v", bad, sym)
				}
			}
		}
		if watched != 1 || !adjacent {
			t.Fatalf("watcher %q count=%d neighbor %q=%t; symbols=%v",
				watchName, watched, neighbor, adjacent, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	const fixture = "<template><div>watch: { fake() {} }</div></template>\n" +
		"<script>\n" +
		"const unrelated = { watch: { decoy() {} } };\n" +
		"const prose = \"export default { watch: { ghost() {} } }\";\n" +
		"export default {\n" +
		"  props: { config: { watch: { nested() {} } } },\n" +
		"  watch: {\n" +
		"    show(newVal) { return newVal },\n" +
		"    options: { handler() { return false } },\n" +
		"    'nested.path'(value) { return value },\n" +
		"  },\n" +
		"  methods: { save(value) { return value } },\n" +
		"}\n</script>\n"
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(fixture, "\n", tc.eol), "show", 8, "save",
				[]string{"fake", "decoy", "ghost", "nested", "handler"})
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_WATCH_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, file, anchor, watcher string
		line                        int
		neighbor                    string
	}{
		{"captcha", "captcha-App.vue", "show(newVal) {", "show", 33, "captchaCallback"},
		{"color-wheel", "color-picker.vue", "value(newVal, oldVal) {", "value", 93, "setColor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, tc.anchor) != 1 || strings.Count(source, "watch: {") != 1 {
				t.Fatalf("original watch oracle changed: %s", tc.name)
			}
			check(t, source, tc.watcher, tc.line, tc.neighbor, nil)
		})
	}
}
