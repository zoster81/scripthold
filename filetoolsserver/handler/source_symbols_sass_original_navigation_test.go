package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type originalSassEvidence struct {
	file, text string
}

type originalSassTarget struct {
	file, name string
	line       int
}

type originalSassTask struct {
	project, query, unrelated string
	files                     []string
	evidence                  []originalSassEvidence
	targets                   []originalSassTarget
}

// Original projects are read-only inputs: the test never compiles or runs them.
func TestSourceSymbolsOriginalSassTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Sass corpus unavailable")
	}
	cases := []originalSassTask{
		{
			project:   "sass-pradyunsg-furo",
			query:     ".sidebar-search-container",
			unrelated: "src/furo/assets/styles/components/_footer.sass",
			files: []string{
				"src/furo/assets/styles/components/_sidebar.sass",
				"src/furo/assets/styles/components/_footer.sass",
				"src/furo/assets/styles/base/_theme.sass",
				"src/furo/assets/styles/furo.sass",
			},
			evidence: []originalSassEvidence{
				{"src/furo/assets/styles/components/_sidebar.sass", "background: var(--color-sidebar-search-background--focus)"},
				{"src/furo/assets/styles/components/_sidebar.sass", "&:focus-within"},
				{"src/furo/assets/styles/base/_theme.sass", "@include colors-dark"},
				{"src/furo/assets/styles/furo.sass", "@use \"components\""},
				{"src/furo/assets/styles/components/_index.sass", "@forward \"sidebar\""},
				{"src/furo/assets/styles/variables/_colors.scss", "--color-sidebar-search-background--focus: var(--color-background-primary);"},
			},
			targets: []originalSassTarget{
				{"src/furo/assets/styles/components/_sidebar.sass", ".sidebar-search-container", 32},
			},
		},
		{
			project:   "sass-readthedocs-sphinx_rtd_theme",
			query:     "nav-search-background-color",
			unrelated: "src/sass/_theme_badge.sass",
			files: []string{
				"src/sass/_theme_variables.sass",
				"src/sass/_theme_layout.sass",
				"src/sass/theme.sass",
				"src/sass/_theme_badge.sass",
			},
			evidence: []originalSassEvidence{
				{"src/sass/_theme_variables.sass", "$nav-search-background-color:         $blue"},
				{"src/sass/_theme_layout.sass", "background-color: $nav-search-background-color"},
				{"src/sass/_theme_layout.sass", ".wy-side-nav-search"},
				{"src/sass/theme.sass", "@import theme_variables"},
				{"src/sass/theme.sass", "@import theme_layout"},
				{"src/sass/_theme_badge.sass", ".rst-versions"},
			},
			targets: []originalSassTarget{
				{"src/sass/_theme_variables.sass", "nav-search-background-color", 51},
				{"src/sass/_theme_layout.sass", ".wy-side-nav-search", 190},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				requireOriginalSassTextFile(t, path)
				paths = append(paths, path)
			}
			for _, oracle := range tc.evidence {
				path := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireOriginalSassTextFile(t, path)
				bytes, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(bytes), oracle.text) {
					t.Fatalf("independent original text %s %q: %v", oracle.file, oracle.text, err)
				}
			}
			handler := NewHandler([]string{root})
			failed, digest, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "sass", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("compact digest size=%d err=%v", len(encoded), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
			for _, want := range tc.targets {
				p := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{p}, Query: want.name, Match: "exact", Language: "sass", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s err=%v failed=%+v result=%+v", want.name, err, failed, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == want.name && symbol.NameRange.Start.Line == want.line {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("original declaration %s:%d %s missing", want.file, want.line, want.name)
				}
				failed, shown, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "sass", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.name) {
					t.Fatalf("fingerprint show %s err=%v failed=%+v shown=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := handler.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "sass", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s err=%v failed=%+v result=%+v", tc.query, err, failed, search.Search)
			}
			failed, negative, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))}, Query: tc.query,
				Match: "exact", Language: "sass", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated source incorrectly defines %s err=%v failed=%+v result=%+v", tc.query, err, failed, negative.Symbols)
			}
		})
	}
}

func requireOriginalSassTextFile(t *testing.T, path string) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original file %s is unavailable or not regular: %v", path, err)
	}
}
