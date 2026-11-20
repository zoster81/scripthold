package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Original AutoHotkey programs are inspected, not executed. Each project has a
// separate maintenance question and byte-verified Git source oracle; timer
// references are call sites, not declarations or resolved call relationships.
func TestAutoHotkeyOriginalTimerNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, excerpt, sibling, unrelated string
		line, endLine, siblingLine                     int
	}{
		{
			name:   "Window Spy refresh interval when minimized",
			env:    "SCRIPTHOLD_REAL_AHK_WINDOWSPY",
			target: "WinSpySize", line: 60, endLine: 72,
			excerpt: "SetTimer Update, (MinMax=0)?250:0",
			sibling: "Update", siblingLine: 78,
			unrelated: "DestroyCalendar",
		},
		{
			name:   "Popup calendar cleanup of previous timer",
			env:    "SCRIPTHOLD_REAL_AHK_CALENDAR",
			target: "DestroyCalendar", line: 27, endLine: 37,
			excerpt: "SetTimer(UpdateTitle, 0)",
			sibling: "CreateCalendar", siblingLine: 39,
			unrelated: "WinSpySize",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_AHK_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original AutoHotkey file fixtures not configured")
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "autohotkey",
				MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original AutoHotkey digest incomplete: err=%v result=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "autohotkey",
				MaxFiles: 1, MaxSymbols: 64, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original AutoHotkey outline incomplete: err=%v result=%+v", err, outline)
			}
			inOutline := false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line &&
					sym.DeclarationRange.End.Line == tc.endLine && sym.BodyRange != nil {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("timer maintenance function %q missing complete range", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "autohotkey",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("AutoHotkey find incomplete: err=%v result=%+v", err, found)
			}
			symbolID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == "function" &&
					sym.NameRange.Start.Line == tc.line && sym.BodyRange != nil {
					symbolID = sym.ID
				}
			}
			if symbolID == "" {
				t.Fatalf("AutoHotkey function %q not indexed: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				file.Encoding == "" {
				t.Fatal("AutoHotkey file identity/encoding mismatch")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "autohotkey",
				Encoding: file.Encoding, SymbolID: symbolID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("AutoHotkey source excerpt missing %q: err=%v show=%+v", tc.excerpt, err, shown.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "autohotkey", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 30,
			})
			if err != nil || searched.Search == nil || !searched.Coverage.CoverageComplete ||
				searched.Coverage.ResultsTruncated {
				t.Fatalf("AutoHotkey structural search incomplete: err=%v result=%+v", err, searched)
			}
			matched := false
			for _, hit := range searched.Search.Matches {
				if hit.Name == tc.target && hit.SymbolID == symbolID &&
					hit.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("AutoHotkey structural search omitted %q", tc.target)
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "autohotkey",
				Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || !sibling.CoverageComplete || len(sibling.Symbols) != 1 ||
				sibling.Symbols[0].NameRange.Start.Line != tc.siblingLine {
				t.Fatalf("related timer function %q missing: err=%v result=%+v", tc.sibling, err, sibling.Symbols)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "autohotkey",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated AutoHotkey function invented: err=%v result=%+v", err, negative.Symbols)
			}
		})
	}
}
