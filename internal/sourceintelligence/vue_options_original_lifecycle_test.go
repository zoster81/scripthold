package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Direct lifecycle callbacks in exported Vue Options objects are authored
// source targets; decoys in template text and nested objects are not.
func TestVueOptionsLifecycleSourceNavigation(t *testing.T) {
	check := func(t *testing.T, source string, want map[string]int, forbidden []string) {
		t.Helper()
		result, err := (VueAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("lifecycle.vue", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated {
			t.Fatalf("unexpected scan truncation: %+v", result.Analysis)
		}
		found := map[string]int{}
		for _, sym := range result.Analysis.Symbols {
			if sym.NativeKind != "vue-options-lifecycle" {
				continue
			}
			for _, bad := range forbidden {
				if sym.Name == bad {
					t.Fatalf("invented lifecycle declaration %q: %+v", bad, sym)
				}
			}
			expected, ok := want[sym.Name]
			if !ok {
				t.Fatalf("invented unrelated lifecycle declaration: %+v", sym)
			}
			if sym.Kind != SymbolKindMethod || sym.NameRange.Start.Line != expected ||
				sym.BodyRange == nil || sym.ID == "" || sym.RegionID == "" {
				t.Errorf("invalid lifecycle source identity: %+v", sym)
			}
			found[sym.Name]++
		}
		for name := range want {
			if found[name] != 1 {
				t.Fatalf("lifecycle %q count=%d; known names=%v", name, found[name], sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	}
	const fixture = "<template><p>mounted() { phantom() {} }</p></template>\n" +
		"<script>\n" +
		"const decoy = { created() { return false } };\n" +
		"const prose = \"mounted() { fake() {} }\";\n" +
		"export default {\n" +
		"  props: { config: { mounted() { return false } } },\n" +
		"  created() { this.ready = true },\n" +
		"  mounted() { this.start() },\n" +
		"  methods: { start() { return 1 } },\n" +
		"}\n</script>\n"
	for _, eol := range []string{"LF", "CRLF"} {
		t.Run("portable-"+eol, func(t *testing.T) {
			source := fixture
			if eol == "CRLF" {
				source = strings.ReplaceAll(source, "\n", "\r\n")
			}
			check(t, source, map[string]int{"created": 7, "mounted": 8},
				[]string{"phantom", "decoy", "fake", "config"})
		})
	}
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_SORTGROUPCITY_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, file, hook, oracle string
		line                     int
	}{
		{"vue2-elm", "vue2-elm-home.vue", "mounted", "mounted(){", 57},
		{"vue-weather", "vue-weather-citylist.vue", "created", "created() {", 143},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			lines := strings.Split(source, "\n")
			if tc.line < 1 || tc.line > len(lines) || strings.TrimSpace(lines[tc.line-1]) != tc.oracle ||
				!strings.Contains(source, "export default {") {
				t.Fatalf("original lifecycle source oracle changed: %s", tc.name)
			}
			for _, eol := range []string{"LF", "CRLF"} {
				t.Run(eol, func(t *testing.T) {
					current := source
					if eol == "CRLF" {
						current = strings.ReplaceAll(source, "\n", "\r\n")
					}
					check(t, current, map[string]int{tc.hook: tc.line}, nil)
				})
			}
		})
	}
}
