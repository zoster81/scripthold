package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The original projects are independently pinned in the private qualification
// corpus. The test checks bounded maintenance navigation, not IL execution.
func TestCILAdditionalIndependentOriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, oracle, target, ownerSuffix, neighbor, negative, excerpt string
		targetLine, neighborLine                                            int
		overload                                                            string
	}{
		{
			name:   "ILSpy signed-size complement must distinguish a call from its method",
			env:    "SCRIPTHOLD_REAL_CIL_ADDITIONAL_ILSPY",
			oracle: ".class private auto ansi abstract sealed beforefieldinit Program",
			target: "BitwiseComplementWithSmallEnum", targetLine: 66,
			ownerSuffix: ".Program", neighbor: "BitwiseComplementWithSmallInteger", neighborLine: 59,
			negative: "conv.u8", excerpt: "BitwiseComplementWithSmallEnum(",
			overload: "PrintHex",
		},
		{
			name:   "Mono delegate asynchronous entry points and unrelated interface",
			env:    "SCRIPTHOLD_REAL_CIL_ADDITIONAL_MONO",
			oracle: ".class public auto ansi sealed D`1<(I) T>",
			target: "EndInvoke", targetLine: 39,
			ownerSuffix: ".D`1", neighbor: "BeginInvoke", neighborLine: 33,
			negative: "IAsyncResult", excerpt: "EndInvoke(",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original selected CIL source not configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.oracle) || !strings.Contains(string(raw), tc.excerpt) {
				t.Fatal("independently selected original-source oracle changed")
			}
			ctx, h := context.Background(), NewHandler([]string{filepath.Dir(path)})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: []string{path},
				Language: "cil", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.OverviewFiles) != 1 ||
				digest.OverviewFiles[0].SourceFingerprint == "" {
				t.Fatalf("source map unavailable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "cil", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated || len(outline.Symbols) == 0 {
				t.Fatalf("bounded original outline unusable: err=%v outline=%+v", err, outline)
			}
			related, overloadLines := false, map[int]bool{}
			for _, sym := range outline.Symbols {
				if sym.Name == tc.neighbor && sym.NameRange.Start.Line == tc.neighborLine &&
					strings.HasSuffix(sym.ParentQualifiedName, tc.ownerSuffix) {
					related = true
				}
				if sym.Name == tc.overload {
					overloadLines[sym.NameRange.Start.Line] = true
				}
			}
			if !related {
				t.Fatalf("source-related neighboring method %q absent: %+v", tc.neighbor, outline.Symbols)
			}
			if tc.overload != "" && (!overloadLines[73] || !overloadLines[82] || len(overloadLines) != 2) {
				t.Fatalf("original distinct PrintHex overloads lost: %v", overloadLines)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cil",
				Query: tc.target, Match: "exact", FileDetails: "relevant",
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || found.Truncated || len(found.Files) != 1 || len(found.Symbols) != 1 {
				t.Fatalf("missing or duplicated maintenance edit target: err=%v result=%+v", err, found)
			}
			sym, file := found.Symbols[0], found.Files[0]
			if sym.Kind != "method" || sym.Name != tc.target ||
				sym.NameRange.Start.Line != tc.targetLine ||
				!strings.HasSuffix(sym.ParentQualifiedName, tc.ownerSuffix) ||
				sym.ID == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.OverviewFiles[0].SourceFingerprint {
				t.Fatalf("wrong source owner, line or identity: %+v file=%+v", sym, file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "cil", Encoding: file.Encoding,
				SymbolID: sym.ID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("exact source drill-down failed: err=%v shown=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "cil", Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated ||
				len(query.Search.Matches) != 1 ||
				query.Search.Matches[0].SymbolID != sym.ID ||
				query.Search.Matches[0].SourceFingerprint != file.SourceFingerprint {
				t.Fatalf("structural lookup disagrees with original source: err=%v query=%+v", err, query)
			}
			_, neg, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cil",
				Query: tc.negative, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || len(neg.Symbols) != 0 {
				t.Fatalf("IL instruction or type reference fabricated as method: err=%v symbols=%+v", err, neg.Symbols)
			}
			t.Logf("original-project navigation passed, source bytes=%d symbols=%d complete=%t",
				len(raw), len(outline.Symbols), outline.CoverageComplete)
		})
	}
}
