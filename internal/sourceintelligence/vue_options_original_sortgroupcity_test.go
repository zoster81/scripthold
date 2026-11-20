package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Qualify the original city-ordering computed getter, rather than treating
// identical template references as proof of source declaration navigation.
func TestVueOriginalSortgroupcityComputedNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_SORTGROUPCITY_ROOT")
	if root == "" {
		t.Skip("original Vue 2 city-list sources not configured")
	}
	cases := []struct {
		name, filename, neighboringMethod, sourceOracle string
		computedLine                                    int
	}{
		{"vue2-elm", "vue2-elm-home.vue", "reload", "    sortgroupcity(){", 81},
		{"vue-weather", "vue-weather-citylist.vue", "showCity", "        sortgroupcity() {", 81},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.filename)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded independent source is missing: info=%v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			lines := strings.Split(source, "\n")
			if tc.computedLine > len(lines) || strings.TrimSpace(lines[tc.computedLine-1]) != strings.TrimSpace(tc.sourceOracle) ||
				!strings.Contains(source, "in sortgroupcity") || !strings.Contains(source, "computed:") ||
				!strings.Contains(source, "methods:") {
				t.Fatalf("original Vue 2 source oracle changed: %s", tc.name)
			}
			for _, eol := range []string{"LF", "CRLF"} {
				t.Run(eol, func(t *testing.T) {
					current := source
					if eol == "CRLF" {
						current = strings.ReplaceAll(current, "\n", "\r\n")
					}
					result, err := (VueAnalyzer{}).Analyze(context.Background(),
						testSourceDocument(tc.filename, current), testAnalyzeOptions(true, 512))
					if err != nil {
						t.Fatal(err)
					}
					if result.Analysis.Truncated {
						t.Fatalf("original Vue 2 scan truncated: %+v", result.Analysis)
					}
					computedCount, siblingFound := 0, false
					for _, sym := range result.Analysis.Symbols {
						if sym.Name == "sortgroupcity" {
							if sym.NativeKind != "vue-options-computed" || sym.Kind != SymbolKindProperty ||
								sym.NameRange.Start.Line != tc.computedLine || sym.BodyRange == nil ||
								sym.ID == "" || sym.RegionID == "" {
								t.Errorf("computed source identity incorrect: %+v", sym)
							}
							computedCount++
						}
						if sym.Name == tc.neighboringMethod && sym.NativeKind == "vue-options-method" {
							siblingFound = true
						}
					}
					if computedCount != 1 || !siblingFound {
						t.Fatalf("original Vue 2 computed getter count=%d adjacent method=%t; names=%v",
							computedCount, siblingFound, sortedSymbolQualifiedNames(result.Analysis.Symbols))
					}
				})
			}
		})
	}
}
