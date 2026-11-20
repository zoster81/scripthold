package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Complete pinned independent Microsoft AL originals: ensure the semicolon
// following an OnRun trigger header does not hide the authored var/body source.
func TestALPinnedWholeOriginalTriggerSourceNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_AL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned AL originals not configured")
	}
	for _, tc := range []struct {
		file, qualified, excerpt            string
		bytes, count, start, end, bodyStart int
	}{
		{
			file: "microsoft-hello.al", bytes: 583, count: 2,
			qualified: "HelloWorld.OnRun", start: 10, end: 15, bodyStart: 13,
			excerpt: "HelloText.GetRandomGreeting()",
		},
		{
			file: "alappextensions-localexport.al", bytes: 3021, count: 3,
			qualified: "Microsoft.Service.Reports.Local Export Serv. Decl..OnRun",
			start:     14, end: 64, bodyStart: 25, excerpt: "DataExch.ExportFromDataExch(DataExchMapping)",
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != int64(tc.bytes) {
				t.Fatalf("pinned AL original changed: %+v %v", info, err)
			}
			h := NewHandler([]string{root})
			ctx := context.Background()
			rej, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "al", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || rej != nil || outline.FilesParsed != 1 || outline.Truncated ||
				!outline.CoverageComplete || len(outline.Symbols) != tc.count {
				t.Fatalf("whole original AL outline mismatch: %v %+v", err, outline)
			}
			rej, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "al",
				Query: "OnRun", Match: "exact", MaxFiles: 1, MaxSymbols: 16, FileDetails: "relevant",
			})
			if err != nil || rej != nil || len(found.Symbols) != 1 || len(found.Files) != 1 {
				t.Fatalf("OnRun source find failed: %v %+v", err, found)
			}
			sym := found.Symbols[0]
			if sym.QualifiedName != tc.qualified ||
				sym.DeclarationRange.Start.Line != tc.start ||
				sym.DeclarationRange.End.Line != tc.end || sym.BodyRange == nil ||
				sym.BodyRange.Start.Line != tc.bodyStart || sym.BodyRange.End.Line != tc.end {
				t.Errorf("whole original OnRun is header-only/wrong owner: %+v", sym)
			}
			file := found.Files[0]
			rej, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "al", Encoding: file.Encoding,
				SymbolID: sym.ID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || rej != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) ||
				!strings.Contains(shown.Show.Text, "end;") {
				t.Fatalf("fingerprint-verified AL source is truncated: %v %+v", err, shown.Show)
			}
			qr, queried, qe := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Query: "OnRun", Match: "exact", Language: "al", MaxFiles: 1, MaxResults: 16,
			})
			if qe != nil || qr != nil || queried.Search == nil || len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != sym.ID ||
				queried.Search.Matches[0].SourceFingerprint != file.SourceFingerprint {
				t.Errorf("whole original structural query source identity failed: %v %+v", qe, queried)
			}
		})
	}
}
