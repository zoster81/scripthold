package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Vue Options shorthand data() functions initialize component state and need
// a source-owned declaration without exposing fields from returned objects.
func TestVueOptionsDirectDataNavigation(t *testing.T) {
	check := func(t *testing.T, source string, line int, neighbor string, want bool) {
		t.Helper()
		analyzed, err := (VueAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("original.vue", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		if analyzed.Analysis.Truncated {
			t.Fatalf("unexpected truncation: %+v", analyzed.Analysis)
		}
		dataCount, neighborSeen := 0, false
		for _, sym := range analyzed.Analysis.Symbols {
			if sym.Name == "data" && sym.NativeKind == "vue-options-data" {
				dataCount++
				if sym.Kind != SymbolKindMethod || sym.NameRange.Start.Line != line ||
					sym.ID == "" || sym.RegionID == "" || sym.BodyRange == nil ||
					sym.DeclarationRange.Start.Line != line {
					t.Errorf("invalid Vue data() source location/identity: %+v", sym)
				}
			}
			if sym.Name == neighbor && sym.NativeKind == "vue-options-computed" {
				neighborSeen = true
			}
		}
		expected := 0
		if want {
			expected = 1
		}
		if dataCount != expected {
			t.Fatalf("data() declarations=%d, want %d; symbols=%v", dataCount, expected, sortedSymbolQualifiedNames(analyzed.Analysis.Symbols))
		}
		if neighbor != "" && !neighborSeen {
			t.Fatalf("neighboring computed getter %q missing", neighbor)
		}
	}

	const fixture = "<template><div>data(){ fake(){} }</div></template>\n" +
		"<script>\n" +
		"const decoy = { data() { return { wrong: true } } };\n" +
		"const prose = \"export default { data(){ fake(){} } }\";\n" +
		"export default {\n" +
		"  props: { config: { data() { return {} } } },\n" +
		"  data() { return { count: 1, nested: { state: true } } },\n" +
		"  computed: { total() { return this.count } },\n" +
		"  methods: { increment() { this.count++ } },\n" +
		"}\n</script>\n"
	const negative = "<script>\n" +
		"export default {\n" +
		"  props: { data() { return {} } },\n" +
		"  data: () => ({ count: 1 }),\n" +
		"  methods: { data() { return false } },\n" +
		"}\n</script>\n"
	for _, eol := range []string{"LF", "CRLF"} {
		t.Run("portable-"+eol, func(t *testing.T) {
			a, b := fixture, negative
			if eol == "CRLF" {
				a = strings.ReplaceAll(a, "\n", "\r\n")
				b = strings.ReplaceAll(b, "\n", "\r\n")
			}
			check(t, a, 7, "total", true)
			check(t, b, 0, "", false)
		})
	}

	root := os.Getenv("SCRIPTHOLD_REAL_VUE_SORTGROUPCITY_ROOT")
	if root == "" {
		return
	}
	for _, tc := range []struct {
		name, file, header string
		line               int
	}{
		{"vue2-elm", "vue2-elm-home.vue", "data(){", 48},
		{"vue-weather", "vue-weather-citylist.vue", "data () {", 61},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := filepath.Join(root, tc.file)
			info, err := os.Lstat(file)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("independent original source unavailable: %v %v", info, err)
			}
			content, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			source := string(content)
			lines := strings.Split(source, "\n")
			if tc.line < 1 || tc.line > len(lines) || strings.TrimSpace(lines[tc.line-1]) != tc.header ||
				!strings.Contains(source, "export default {") || !strings.Contains(source, "sortgroupcity") {
				t.Fatalf("original Vue data() source oracle changed: %s", tc.name)
			}
			for _, eol := range []string{"LF", "CRLF"} {
				t.Run(eol, func(t *testing.T) {
					s := source
					if eol == "CRLF" {
						s = strings.ReplaceAll(s, "\n", "\r\n")
					}
					check(t, s, tc.line, "sortgroupcity", true)
				})
			}
		})
	}
}
