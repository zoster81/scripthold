package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

func TestYAMLSourceDerivedRepeatedSequenceKeySelectors(t *testing.T) {
	const originalShape = "jobs:\n" +
		"  ci:\n" +
		"    steps:\n" +
		"      - uses: actions/checkout@v7\n" +
		"      - name: Setup cache\n" +
		"        uses: Swatinem/rust-cache@v2\n" +
		"      - name: Install just\n" +
		"        uses: taiki-e/install-action@v2\n" +
		"      - run: |\n" +
		"          echo 'uses: not-a-step'\n" +
		"  test:\n" +
		"    strategy:\n" +
		"      matrix:\n" +
		"        include:\n" +
		"        - build: pinned\n" +
		"        - build: stable\n"
	for _, tc := range []struct{ label, source string }{
		{"LF", originalShape},
		{"CRLF", strings.ReplaceAll(originalShape, "\n", "\r\n")},
	} {
		t.Run(tc.label, func(t *testing.T) {
			result, err := (YAMLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("ci.yml", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("item identity remains intentionally incomplete: %+v", result.Analysis)
			}
			expect := map[string][]int{
				"jobs.ci.steps.uses":                      {4, 6, 8},
				"jobs.test.strategy.matrix.include.build": {15, 16},
			}
			actual := make(map[string][]int)
			ids := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name == "fabricated" {
					t.Fatalf("invented key from YAML block content: %+v", symbol)
				}
				if _, wanted := expect[symbol.QualifiedName]; !wanted {
					continue
				}
				if symbol.ID == "" || ids[symbol.ID] {
					t.Fatalf("ambiguous or duplicated YAML source selector: %+v", symbol)
				}
				ids[symbol.ID] = true
				actual[symbol.QualifiedName] = append(actual[symbol.QualifiedName], symbol.NameRange.Start.Line)
			}
			for name, expected := range expect {
				sort.Ints(actual[name])
				if !slices.Equal(actual[name], expected) {
					t.Errorf("%s source selections = %v, want %v", name, actual[name], expected)
				}
			}
		})
	}
}

// Independent whole upstream workflows retain every authored sequence mapping
// key as a distinct source selector while list-item ownership stays partial.
func TestYAMLPinnedWholeWorkflowRepeatedKeysHaveDistinctSourceSelectors(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent pinned workflow corpus not configured")
	}
	cases := []struct {
		name, project, member, qualified string
		bytes                            int
		lines                            []int
	}{
		{
			name: "zoxide repeated action uses", project: "yaml99-zoxide-selected",
			member: "uses", qualified: "jobs.ci.steps.uses", bytes: 1570,
			lines: []int{24, 27, 33, 39, 43, 49, 53},
		},
		{
			name: "ripgrep independent build entries", project: "yaml99-ripgrep-selected",
			member: "build", qualified: "jobs.test.strategy.matrix.include.build", bytes: 9222,
			lines: []int{54, 57, 60, 63, 66, 70, 74, 78, 82, 86, 90, 94, 98, 102, 106, 109, 112, 115},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(corpus, tc.project, "files", ".github", "workflows", "ci.yml")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != int64(tc.bytes) {
				t.Fatalf("pinned source identity mismatch: info=%v err=%v", info, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (YAMLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("ci.yml", string(bytes)), testAnalyzeOptions(true, 1024))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("source-specified YAML list item ownership still partial: %+v", result.Analysis)
			}
			actual := make([]int, 0, len(tc.lines))
			seenIDs := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				if symbol.QualifiedName != tc.qualified || symbol.Name != tc.member {
					continue
				}
				if symbol.ID == "" || seenIDs[symbol.ID] {
					t.Fatalf("missing or repeated distinct source selector: %+v", symbol)
				}
				seenIDs[symbol.ID] = true
				if symbol.Kind != SymbolKindKey || symbol.ParentQualifiedName == "" {
					t.Fatalf("incorrect lexical owner or kind for repeated key: %+v", symbol)
				}
				actual = append(actual, symbol.NameRange.Start.Line)
			}
			sort.Ints(actual)
			if len(actual) != len(tc.lines) {
				t.Fatalf("whole pinned original has %d distinct %s entries; expected %d: %v", len(actual), tc.member, len(tc.lines), actual)
			}
			for i := range actual {
				if actual[i] != tc.lines[i] {
					t.Errorf("wrong original %s position at index %d: got %d, authored %d", tc.member, i, actual[i], tc.lines[i])
				}
			}
		})
	}
}
