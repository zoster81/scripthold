package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Opt-in original Blazor projects are verified independently against their
// tracked source, parent components, and unrelated files; no application is run.
func TestBlazorOriginalComponentTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, query, kind, excerpt, secondary, secondaryKind, notInFile string
		line, secondaryLine                                                  int
	}{
		{
			name:          "Blazorise todo completion event",
			env:           "SCRIPTHOLD_REAL_BLAZOR_BLAZORISE",
			query:         "OnCheckedChanged",
			kind:          "method",
			excerpt:       "await StatusChanged.InvokeAsync( isChecked );",
			secondary:     "OnRemoveClicked",
			secondaryKind: "method",
			notInFile:     "OnTodoStatusChanged",
			line:          23, secondaryLine: 30,
		},
		{
			name:          "MudBlazor inline code styling",
			env:           "SCRIPTHOLD_REAL_BLAZOR_MUDBLAZOR",
			query:         "SecondaryColor",
			kind:          "property",
			excerpt:       "public bool SecondaryColor { get; set; }",
			secondary:     "Tag",
			secondaryKind: "property",
			notInFile:     "ApiMemberTable",
			line:          12, secondaryLine: 16,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			root := os.Getenv("SCRIPTHOLD_REAL_BLAZOR_ROOT")
			if path == "" || root == "" {
				t.Skip("original Blazor source fixture not configured")
			}
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "blazor", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("source overview unavailable: error=%v digest=%+v", err, digest)
			}
			_, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "blazor",
				MaxFiles: 1, MaxSymbols: 30, Presentation: "detailed",
			})
			if err != nil || !outlined.CoverageComplete ||
				outlined.Truncated || len(outlined.Files) != 1 {
				t.Fatalf("source outline unavailable: error=%v outline=%+v", err, outlined)
			}
			foundOutline := false
			for _, symbol := range outlined.Symbols {
				if symbol.Name == tc.query && symbol.NameRange.Start.Line == tc.line {
					foundOutline = true
				}
			}
			if !foundOutline {
				t.Fatalf("outline omitted task-relevant %q", tc.query)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "blazor",
				Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 20, FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				found.FilesParsed != 1 || len(found.Files) != 1 {
				t.Fatalf("source find unavailable: error=%v find=%+v", err, found)
			}
			selected := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.query && string(symbol.Kind) == tc.kind &&
					symbol.NameRange.Start.Line == tc.line && symbol.BodyRange != nil {
					selected = symbol.ID
				}
			}
			if selected == "" {
				t.Fatalf("source find omitted target %q: %+v", tc.query, found.Symbols)
			}
			file := found.Files[0]
			if file.SourceFingerprint == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("missing or mismatched file source fingerprint / encoding")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "blazor", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("fingerprint-bound source excerpt missing %s: error=%v show=%+v", tc.query, err, shown)
			}
			_, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "blazor",
				Query: tc.query, Match: "exact", MaxFiles: 1, MaxResults: 20,
			})
			if err != nil || queried.Search == nil ||
				!queried.Coverage.CoverageComplete || queried.Coverage.ResultsTruncated {
				t.Fatalf("structural source search incomplete: error=%v query=%+v", err, queried)
			}
			matched := false
			for _, match := range queried.Search.Matches {
				if match.SymbolID == selected && match.Name == tc.query &&
					match.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("structural search omitted %s", tc.query)
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "blazor",
				Query: tc.secondary, Match: "exact", MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || len(sibling.Symbols) != 1 ||
				sibling.Symbols[0].NameRange.Start.Line != tc.secondaryLine ||
				string(sibling.Symbols[0].Kind) != tc.secondaryKind {
				t.Fatalf("related member %q not navigable: error=%v find=%+v", tc.secondary, err, sibling)
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "blazor",
				Query: tc.notInFile, Match: "exact", MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || !unrelated.CoverageComplete ||
				len(unrelated.Symbols) != 0 {
				t.Fatalf("other-file symbol %q fabricated in target file: error=%v find=%+v",
					tc.notInFile, err, unrelated)
			}
		})
	}
}
