package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Two exact Git-original Julia sources supply independent task-first oracles.
// Source navigation locates declarations, not the runtime dispatch or execution
// semantics of compiler internals and iteration methods.
func TestJuliaOriginalCallableNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, excerpt, sibling, unrelated string
		line, siblingLine                              int
	}{
		{
			name:   "Compiler disjoint set union and root merge",
			env:    "SCRIPTHOLD_REAL_JULIA_COMPILER",
			target: "union!", line: 94,
			excerpt: "function union!(s::IntDisjointSet",
			sibling: "root_union!", siblingLine: 108,
			unrelated: "Base.iterate",
		},
		{
			name:   "Exercism Fibonacci iteration and length",
			env:    "SCRIPTHOLD_REAL_JULIA_EXERCISM",
			target: "Base.iterate", line: 7,
			excerpt: "function Base.iterate(fib::Fiberator",
			sibling: "Base.length", siblingLine: 21,
			unrelated: "union!",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_JULIA_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original Julia source fixtures not configured")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "julia", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Julia original digest incomplete: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "julia",
				MaxFiles: 1, MaxSymbols: 128, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("Julia original outline incomplete: err=%v outline=%+v", err, outline)
			}
			inOutline := false
			for _, symbol := range outline.Symbols {
				if symbol.Name == tc.target && symbol.NameRange.Start.Line == tc.line {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("Julia actual maintenance function %q missing from outline", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "julia",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Julia original exact find incomplete: err=%v found=%+v", err, found)
			}
			selected := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.target && string(symbol.Kind) == "function" &&
					symbol.NameRange.Start.Line == tc.line {
					selected = symbol.ID
				}
			}
			if selected == "" {
				t.Fatalf("Julia target %q missing or wrong kind: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("Julia original source fingerprint/encoding mismatch")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "julia", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Julia exact declaration excerpt unavailable: err=%v shown=%+v", err, shown.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "julia", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 40,
			})
			if err != nil || searched.Search == nil || !searched.Coverage.CoverageComplete ||
				searched.Coverage.ResultsTruncated {
				t.Fatalf("Julia structural search incomplete: err=%v searched=%+v", err, searched)
			}
			matched := false
			for _, hit := range searched.Search.Matches {
				if hit.Name == tc.target && hit.SymbolID == selected &&
					hit.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("Julia exact structural search missing %q", tc.target)
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "julia",
				Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || !sibling.CoverageComplete || len(sibling.Symbols) != 1 ||
				sibling.Symbols[0].NameRange.Start.Line != tc.siblingLine {
				t.Fatalf("Julia related function %q absent: err=%v symbols=%+v", tc.sibling, err, sibling.Symbols)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "julia",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("Julia unrelated declaration fabricated: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
