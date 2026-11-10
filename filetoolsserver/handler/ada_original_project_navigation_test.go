package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Independent original Ada source files qualify operation navigation without
// compiling either project or interpreting Ada runtime behavior.
func TestAdaOriginalProjectOperatorAndPackageNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, sibling, siblingOwner, excerpt, oracle, unrelated string
		line, siblingLine                                                           int
	}{
		{
			name:   "Geany generic stack Push and Empty declarations",
			env:    "SCRIPTHOLD_REAL_ADA_GEANY",
			target: "Push", owner: "GenStack", line: 25,
			sibling: "Empty", siblingOwner: "GenStack", siblingLine: 28,
			excerpt: "procedure Push(S: in out Stack;",
			oracle:  "package GenStack is", unrelated: "MakeTag",
		},
		{
			name:   "Pygments scanner multiplication operator and following function",
			env:    "SCRIPTHOLD_REAL_ADA_PYGMENTS",
			target: "*", owner: "Scanner", line: 55,
			sibling: "Function_Specification", siblingOwner: "Scanner", siblingLine: 62,
			excerpt: "function \"*\" (Left, Right : in Integer) return Integer is",
			oracle:  "end \"*\";", unrelated: "Left",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_ADA_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original Ada project not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original Ada project only partially configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original Ada source maintenance oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "ada", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Ada digest unusable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "ada",
				MaxFiles: 1, MaxSymbols: 256, Presentation: "detailed",
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated {
				t.Fatalf("original Ada outline unusable: err=%v outline=%+v", err, outline)
			}
			targetOK, siblingOK := false, false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.line && sym.ID != "" {
					targetOK = true
				}
				if sym.Name == tc.sibling && sym.ParentQualifiedName == tc.siblingOwner &&
					sym.NameRange.Start.Line == tc.siblingLine && sym.ID != "" {
					siblingOK = true
				}
				if tc.target == "*" && sym.Name == "Left" && sym.Kind == "function" &&
					sym.NameRange.Start.Line == 55 {
					t.Fatalf("Ada operator parameter fabricated as function: %+v", sym)
				}
			}
			if !targetOK || !siblingOK {
				t.Fatalf("original Ada target and following declaration not safely indexed: target=%v sibling=%v", targetOK, siblingOK)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ada",
				Query: tc.target, Match: "exact", MaxFiles: 1,
				MaxSymbols: 64, FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Ada find failed: err=%v found=%+v", err, found)
			}
			targetID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.line {
					targetID = sym.ID
				}
			}
			file := found.Files[0]
			if targetID == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("safe exact Ada selector missing: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ada",
				Encoding: file.Encoding, SymbolID: targetID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Ada original declaration excerpt incorrect: err=%v shown=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "ada", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("Ada source search failed: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == targetID &&
					hit.SourceFingerprint == file.SourceFingerprint &&
					hit.Range.Start.Line == tc.line {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Ada structural query does not locate exact function: %+v", search.Search)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ada",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("Ada unrelated/parameter name incorrectly indexed: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
