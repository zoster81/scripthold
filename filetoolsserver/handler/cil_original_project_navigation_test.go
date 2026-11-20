package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The selected sources come from two independent original projects. The
// handler must provide enough bounded evidence to navigate their IL methods.
func TestCILOriginalProjectMethodNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, sibling, excerpt, oracle string
		targetLine, siblingLine                            int
	}{
		{
			name:   "Linguist generic method and adjacent accessor",
			env:    "SCRIPTHOLD_REAL_CIL_LINGUIST",
			target: "GenericMethod", owner: "TestILAssembly.GenericWithModifiers", targetLine: 39,
			sibling: "get_GenericField", siblingLine: 48,
			excerpt: "GenericMethod<T>(!!T", oracle: ".class public auto ansi beforefieldinit GenericWithModifiers",
		},
		{
			name:   "KDE global Cdecl pointer method and Main overloads",
			env:    "SCRIPTHOLD_REAL_CIL_KDE",
			target: "test_pointer_operations", targetLine: 56,
			sibling: "Main", siblingLine: 10,
			excerpt: "test_pointer_operations(int32 param)", oracle: ".method private hidebysig static void Main(string[] args)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_CIL_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original CIL project not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original CIL project only partially configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original CIL source maintenance oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "cil", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original CIL digest unusable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "cil",
				MaxFiles: 1, MaxSymbols: 128, Presentation: "detailed",
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated {
				t.Fatalf("original CIL outline unusable: err=%v outline=%+v", err, outline)
			}
			targetOK, siblingOK := false, false
			mainLines := map[int]bool{}
			for _, sym := range outline.Symbols {
				if sym.Kind != "method" {
					continue
				}
				if sym.Name == "modopt" || sym.Name == "T" || sym.Name == "CallConvCdecl" {
					t.Fatalf("CIL method name fabricated from signature: %+v", sym)
				}
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.targetLine && sym.ID != "" {
					targetOK = true
				}
				if sym.Name == tc.sibling && sym.NameRange.Start.Line == tc.siblingLine &&
					sym.ID != "" {
					siblingOK = true
				}
				if sym.Name == "Main" {
					mainLines[sym.NameRange.Start.Line] = true
				}
			}
			if !targetOK || !siblingOK {
				t.Fatalf("original CIL edit target or adjacent method absent: target=%v sibling=%v", targetOK, siblingOK)
			}
			if tc.sibling == "Main" && (!mainLines[10] || !mainLines[19] || len(mainLines) != 2) {
				t.Fatalf("both original global Main overloads must be separate: %v", mainLines)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cil",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("CIL exact find failed: err=%v result=%+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.targetLine {
					id = sym.ID
				}
			}
			file := found.Files[0]
			if id == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("safe CIL selector missing or inconsistent: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "cil",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("CIL source-exact declaration unavailable: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "cil", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("CIL structural search failed: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("CIL structural search did not link to exact method: %+v", search.Search)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cil",
				Query: "modopt", Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("CIL type modifier fabricated as declaration: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
