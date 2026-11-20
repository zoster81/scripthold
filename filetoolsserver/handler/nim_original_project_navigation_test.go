package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Exact Git-original Nim files provide separate compiler and application tasks.
// Their related source, tests, and unrelated alternatives are checked directly;
// navigation does not claim runtime behavior or full function bodies.
func TestNimOriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, excerpt, sibling, unrelated string
		line, siblingLine                              int
	}{
		{
			name:   "Nim compiler bitset insertion and exclusion",
			env:    "SCRIPTHOLD_REAL_NIM_COMPILER",
			target: "bitSetIncl", line: 32,
			excerpt: "proc bitSetIncl*(x: var TBitSet, elem: BiggestInt)",
			sibling: "bitSetExcl", siblingLine: 37,
			unrelated: "binarySearch",
		},
		{
			name:   "Exercism binary search bounds and empty array",
			env:    "SCRIPTHOLD_REAL_NIM_EXERCISM",
			target: "binarySearch", line: 1,
			excerpt: "func binarySearch*[T](a: openArray[T], val: T)",
			sibling: "left", siblingLine: 3,
			unrelated: "bitSetIncl",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_NIM_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original Nim source fixture is not configured")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nim", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Nim original digest incomplete: err=%v result=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "nim",
				MaxFiles: 1, MaxSymbols: 64, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("Nim original outline incomplete: err=%v result=%+v", err, outline)
			}
			foundTarget := false
			blockLocals := 0
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line &&
					string(sym.Kind) == "function" {
					foundTarget = true
				}
				if tc.target == "binarySearch" && sym.Name == "mid" &&
					sym.NameRange.Start.Line == 7 &&
					sym.ParentQualifiedName != "binarySearch" {
					t.Fatalf("Nim original loop local mid assigned to wrong scope: %+v", sym)
				}
				if tc.target == "bitSetIncl" && ((sym.Name == "arr" && sym.NameRange.Start.Line == 70) ||
					(sym.Name == "countSetBits" && sym.NameRange.Start.Line == 72)) {
					if sym.ParentQualifiedName != "populationCount" {
						t.Fatalf("Nim original constant initializer local incorrectly global: %+v", sym)
					}
					blockLocals++
				}
			}
			if tc.target == "bitSetIncl" && blockLocals != 2 {
				t.Fatalf("Nim original constant initializer declarations not both indexed: %d", blockLocals)
			}
			if !foundTarget {
				t.Fatalf("Nim original maintenance function %q absent from outline", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nim",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				len(found.Files) != 1 {
				t.Fatalf("Nim original find incomplete: err=%v found=%+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line &&
					string(sym.Kind) == "function" {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("Nim original target %q missing: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("Nim safe exact-source selector unavailable")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "nim",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Nim verified declaration unavailable: err=%v show=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "nim", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 24,
			})
			if err != nil || query.Search == nil ||
				!query.Coverage.CoverageComplete || query.Coverage.ResultsTruncated {
				t.Fatalf("Nim original structural search incomplete: err=%v query=%+v", err, query)
			}
			linked := false
			for _, match := range query.Search.Matches {
				if match.Name == tc.target && match.SymbolID == id &&
					match.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Nim original target %q missing from structural search", tc.target)
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nim",
				Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			related := false
			if err == nil && sibling.CoverageComplete {
				for _, sym := range sibling.Symbols {
					if sym.Name == tc.sibling &&
						sym.NameRange.Start.Line == tc.siblingLine &&
						(tc.target != "binarySearch" ||
							sym.ParentQualifiedName == "binarySearch") {
						related = true
					}
				}
			}
			if !related {
				t.Fatalf("Nim related declaration %q missing or misowned: err=%v symbols=%+v",
					tc.sibling, err, sibling.Symbols)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nim",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("Nim unrelated declaration fabricated: err=%v symbols=%+v",
					err, negative.Symbols)
			}
		})
	}
}
