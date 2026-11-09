package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt-in original-project tests verify the local MCP handlers rather than the
// potentially older connected server. External projects are read-only inputs.
func TestFSharpOriginalProjectNavigationHandlers(t *testing.T) {
	cases := []struct {
		name, env, query, qualified, excerpt string
		line                                 int
		paths                                []string
	}{
		{
			name:      "elmish-dispatch",
			env:       "SCRIPTHOLD_REAL_FS_ELMISH_ROOT",
			query:     "runWithDispatch",
			qualified: "Elmish.Program.runWithDispatch",
			excerpt:   "let runWithDispatch",
			line:      160,
			paths:     []string{"src/program.fs", "src/ring.fs", "tests/ProgramTest.fs", "tests/RingTest.fs"},
		},
		{
			name:      "saturn-controller-not-found",
			env:       "SCRIPTHOLD_REAL_FS_SATURN_ROOT",
			query:     "NotFoundHandler",
			qualified: "Saturn.Controller.ControllerBuilder.NotFoundHandler",
			excerpt:   "member __.NotFoundHandler",
			line:      158,
			paths:     []string{"src/Saturn/Controller.fs", "src/Saturn/Router.fs", "tests/Saturn.UnitTests/ControllerTests.fs", "tests/Saturn.UnitTests/RouterTests.fs"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			if root == "" {
				t.Skip("original project fixture not configured")
			}
			paths := make([]string, 0, len(tc.paths))
			for _, rel := range tc.paths {
				paths = append(paths, filepath.Join(root, filepath.FromSlash(rel)))
			}
			h := NewHandler([]string{root})
			toolErr, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: paths, Language: "fsharp", Query: tc.query,
				Match: "exact", MaxFiles: len(paths), MaxSymbols: 32, FileDetails: "relevant",
			})
			if err != nil || toolErr != nil || found.FilesParsed != len(paths) || found.Truncated {
				t.Fatalf("find failed/incomplete: result=%v err=%v files=%d truncated=%v", toolErr, err, found.FilesParsed, found.Truncated)
			}
			var symbolID, file, fingerprint, encoding string
			for _, sym := range found.Symbols {
				if sym.QualifiedName == tc.qualified && sym.NameRange.Start.Line == tc.line {
					symbolID, file = sym.ID, sym.Path
					break
				}
			}
			if symbolID == "" {
				t.Fatalf("missing exact owned declaration %s among %d symbols", tc.qualified, len(found.Symbols))
			}
			for _, source := range found.Files {
				if source.Path == file {
					fingerprint, encoding = source.SourceFingerprint, source.Encoding
				}
			}
			if fingerprint == "" || encoding == "" {
				t.Fatal("selected declaration lacks safe source selector")
			}
			showResult, show, showErr := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: file, Language: "fsharp", Encoding: encoding,
				SymbolID: symbolID, SourceFingerprint: fingerprint, MaxBytes: 32768,
			})
			if showErr != nil || showResult != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.excerpt) {
				t.Fatalf("fingerprint-bound show failed: result=%v err=%v", showResult, showErr)
			}
			queryResult, searched, queryErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: paths, Query: tc.query,
				Match: "exact", Language: "fsharp", MaxFiles: len(paths), MaxResults: 32,
			})
			if queryErr != nil || (queryResult != nil && queryResult.IsError) || searched.Search == nil ||
				searched.Coverage.FilesParsed != len(paths) || searched.Coverage.ResultsTruncated {
				t.Fatalf("structural search failed/incomplete: result=%v err=%v coverage=%+v", queryResult, queryErr, searched.Coverage)
			}
			var selected bool
			for _, match := range searched.Search.Matches {
				if match.QualifiedName == tc.qualified && match.Range.Start.Line == tc.line &&
					match.SymbolID != "" && match.SourceFingerprint == fingerprint {
					selected = true
				}
			}
			if !selected {
				t.Fatalf("search lost exact owned declaration %s among %d matches", tc.qualified, len(searched.Search.Matches))
			}
			t.Logf("original-project source navigation: selected=%s files=%d find=%d search=%d exact-show=true", tc.qualified, len(paths), len(found.Symbols), len(searched.Search.Matches))
		})
	}
}
