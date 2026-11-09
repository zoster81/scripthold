package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Opt-in original Laravel Blade projects: symbols are source-backed DOM IDs,
// not inferred route handlers or runtime Livewire method calls.
func TestBladeOriginalComponentTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, sibling, excerpt, unrelated string
		line, siblingLine                              int
	}{
		{
			name:   "Bagisto admin account form template",
			env:    "SCRIPTHOLD_REAL_BLADE_BAGISTO",
			target: "v-two-factor-setup-template", line: 10,
			sibling: "email", siblingLine: 91,
			excerpt:   "type=\"text/x-template\"",
			unrelated: "setOutputToFoo",
		},
		{
			name:   "Livewire autocomplete email binding",
			env:    "SCRIPTHOLD_REAL_BLADE_LIVEWIRE",
			target: "email", line: 8,
			sibling: "password", siblingLine: 12,
			excerpt:   "wire:model.blur.live=\"email\"",
			unrelated: "v-two-factor-setup-template",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, root := os.Getenv(tc.env), os.Getenv("SCRIPTHOLD_REAL_BLADE_ROOT")
			if path == "" || root == "" {
				t.Skip("original Blade fixture not configured")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "blade", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Blade original digest incomplete: error=%v result=%+v", err, digest)
			}
			_, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "blade",
				MaxFiles: 1, MaxSymbols: 30, Presentation: "detailed",
			})
			if err != nil || !outlined.CoverageComplete || outlined.Truncated ||
				outlined.FilesParsed != 1 {
				t.Fatalf("Blade original outline incomplete: error=%v result=%+v", err, outlined)
			}
			seen := false
			for _, symbol := range outlined.Symbols {
				if symbol.Name == tc.target && symbol.NameRange.Start.Line == tc.line {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("Blade target %q missing from outline", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "blade",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				len(found.Files) != 1 {
				t.Fatalf("Blade original find incomplete: err=%v result=%+v", err, found)
			}
			selected := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.target && string(symbol.Kind) == "entity" &&
					symbol.NameRange.Start.Line == tc.line {
					selected = symbol.ID
				}
			}
			if selected == "" {
				t.Fatalf("Blade source target %q not found: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("Blade fingerprint / encoding mismatch")
			}
			_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "blade", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.excerpt) {
				t.Fatalf("Blade original source excerpt missing: err=%v show=%+v", err, show.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "blade", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 30,
			})
			if err != nil || searched.Search == nil || !searched.Coverage.CoverageComplete ||
				searched.Coverage.ResultsTruncated {
				t.Fatalf("Blade structural search incomplete: err=%v result=%+v", err, searched)
			}
			linked := false
			for _, match := range searched.Search.Matches {
				if match.Name == tc.target && match.SymbolID == selected &&
					match.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Blade structural search missing %q", tc.target)
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "blade",
				Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
			})
			if err != nil || len(sibling.Symbols) != 1 ||
				sibling.Symbols[0].NameRange.Start.Line != tc.siblingLine ||
				string(sibling.Symbols[0].Kind) != "entity" {
				t.Fatalf("Blade related field %q missing: err=%v found=%+v", tc.sibling, err, sibling)
			}
			_, absent, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "blade",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
			})
			if err != nil || !absent.CoverageComplete || len(absent.Symbols) != 0 {
				t.Fatalf("Blade unrelated symbol fabricated: err=%v found=%+v", err, absent.Symbols)
			}
		})
	}
}
