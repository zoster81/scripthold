package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Navigate two entire independently pinned F# sources through the local MCP
// handler. No connected server or program execution is used as an oracle.
func TestFSharpPinnedWholeOriginalLetNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_FSHARP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete independent FSharp original projects not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, name, qualified, excerpt string
		count, line, start, end        int
	}{
		{"elmish-program.fs", "withSubscription", "Elmish.Program.withSubscription",
			"let withSubscription (subscribe :", 24, 55, 9, 25},
		{"fsharp-data-csv-extensions.fs", "?", "FSharp.Data.CsvExtensions.?",
			"let (?) (csvRow: CsvRow)", 13, 116, 10, 11},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 1<<20 {
				t.Fatalf("unsafe complete FSharp upstream source: %v %v", info, err)
			}
			failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "fsharp", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || outlined.Truncated || !outlined.CoverageComplete ||
				outlined.FilesParsed != 1 || len(outlined.Files) != 1 || len(outlined.Symbols) != tc.count {
				t.Fatalf("original detailed FSharp outline: err=%v failed=%+v symbols=%+v", err, failed, outlined.Symbols)
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("original FSharp source fingerprint missing")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "fsharp", Encoding: "utf-8",
				Query: tc.name, Match: "exact", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || len(found.Symbols) == 0 {
				t.Fatalf("original FSharp exact find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
			}
			var symID string
			for _, s := range found.Symbols {
				if s.QualifiedName == tc.qualified {
					if symID != "" {
						t.Fatal("original FSharp name unexpectedly ambiguous")
					}
					if s.Kind != "function" || s.NativeKind != "let" ||
						s.NameRange.Start.Line != tc.line || s.NameRange.Start.Column != tc.start ||
						s.NameRange.End.Column != tc.end || !strings.Contains(s.Signature, tc.excerpt) {
						t.Fatalf("original FSharp source-authored function: %+v", s)
					}
					symID = s.ID
				}
			}
			if symID == "" {
				t.Fatalf("original named function %s missing", tc.qualified)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "fsharp", Encoding: "utf-8",
				SymbolID: symID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("original fingerprint-bound FSharp show: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "fsharp", Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxResults: 256,
			})
			if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) < 1 {
				t.Fatalf("original FSharp structural search: err=%v failed=%+v", err, failed)
			}
		})
	}
}
