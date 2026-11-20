package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original Sass sources are immutable test oracles. We navigate their authored
// selectors but never execute their imports, mixins or stylesheet compilation.
func TestSassOriginalIndentedSelectorSourceShowIncludesNestedBody(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Sass snapshots not configured")
	}
	cases := []struct {
		name, project, file, selector, expectedHeader, required, nested, forbidden string
		startLine, endLine                                                         int
	}{
		{
			name:    "Compass horizontal display and nested flex mixin",
			project: "sass-compass-sass-selected", file: "files/sass/box.sass",
			selector: ".hbox", expectedHeader: ".hbox",
			required: "+display-box", nested: "+box-flex",
			forbidden: ".vbox",
			startLine: 3, endLine: 9,
		},
		{
			name:    "Compass vertical display stays separate from spacer",
			project: "sass-compass-sass-selected", file: "files/sass/box.sass",
			selector: ".vbox", expectedHeader: ".vbox",
			required: "+box-orient(vertical)", nested: "+box-flex",
			forbidden: ".spacer",
			startLine: 11, endLine: 17,
		},
		{
			name:    "Wallet app responsive media and final shadow",
			project: "sass97-danilo-selected", file: "files/sass/style.sass",
			selector: "#app", expectedHeader: "#app",
			required: "width: 320px", nested: "@media (max-width: $break-small)",
			forbidden: ".secondary",
			startLine: 47, endLine: 62,
		},
		{
			name:    "Wallet balance flex positioning and media overrides",
			project: "sass97-danilo-selected", file: "files/sass/style.sass",
			selector: "#balance", expectedHeader: "#balance",
			required: "font-size: 30px", nested: "@media (max-height: 430px)",
			forbidden: ".clickable",
			startLine: 83, endLine: 95,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Sass file unavailable or unsafe: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.startLine < 1 || tc.endLine > len(lines) ||
				strings.TrimSpace(lines[tc.startLine-1]) != tc.expectedHeader ||
				!strings.Contains(strings.Join(lines[tc.startLine-1:tc.endLine], "\n"), tc.required) ||
				!strings.Contains(strings.Join(lines[tc.startLine-1:tc.endLine], "\n"), tc.nested) ||
				strings.Contains(strings.Join(lines[tc.startLine-1:tc.endLine], "\n"), tc.forbidden) {
				t.Fatalf("original Sass range oracle changed: %s:%d-%d", tc.file, tc.startLine, tc.endLine)
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "sass", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.Truncated || digest.FilesParsed != 1 {
				t.Fatalf("original Sass digest incomplete: err=%v %+v", err, digest)
			}
			_, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "sass",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || !outlined.CoverageComplete || outlined.Truncated {
				t.Fatalf("Sass outline incomplete: err=%v %+v", err, outlined)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "sass",
				Query: tc.selector, Match: "exact", FileDetails: "relevant",
				MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Sass find incomplete: err=%v %+v", err, found)
			}
			symbolID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.selector &&
					sym.NameRange.Start.Line == tc.startLine &&
					sym.DeclarationRange.Start.Line == tc.startLine &&
					sym.DeclarationRange.End.Line == tc.endLine &&
					string(sym.Kind) == "selector" {
					symbolID = sym.ID
				}
			}
			if symbolID == "" {
				t.Fatalf("original Sass selector range missing: %s:%d %+v", tc.file, tc.startLine, found.Symbols)
			}
			seen := false
			for _, sym := range outlined.Symbols {
				if sym.ID == symbolID {
					seen = true
					if strings.Contains(sym.Signature, tc.required) || strings.Contains(sym.Signature, tc.nested) {
						t.Fatalf("Sass outline signature should stay compact: %s:%d %+v", tc.file, tc.startLine, sym)
					}
				}
			}
			if !seen {
				t.Fatalf("Sass selector missing from outline: %s:%d", tc.file, tc.startLine)
			}
			f := found.Files[0]
			if f.Encoding == "" || f.SourceFingerprint == "" {
				t.Fatalf("unsafe Sass evidence: %+v", f)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "sass",
				Encoding: f.Encoding, SymbolID: symbolID, SourceFingerprint: f.SourceFingerprint,
				MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.expectedHeader) ||
				!strings.Contains(shown.Show.Text, tc.required) ||
				!strings.Contains(shown.Show.Text, tc.nested) ||
				strings.Contains(shown.Show.Text, tc.forbidden) {
				t.Fatalf("original Sass show crossed sibling or omitted nested source: %s:%d err=%v show=%+v",
					tc.file, tc.startLine, err, shown.Show)
			}
		})
	}
}
