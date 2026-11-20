package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// These scenarios use unchanged source bytes from two independent original
// Git repositories. They verify local read-only MCP navigation, not execution.
func TestCOBOLProvider99OriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, kind, excerpt, oracle, neighbor, neighborKind string
		targetLine, neighborLine                                                int
	}{
		{
			name: "Geany fixed-format input file routing", env: "SCRIPTHOLD_REAL_COBOL99_GEANY",
			target: "INPUT-OUPUT", owner: "Program-Name", kind: "section", targetLine: 6,
			excerpt: "INPUT-OUPUT SECTION.", oracle: "SELECT File-Name ASSIGN TO \"SAMPLE.DAT\"",
			neighbor: "FILE", neighborKind: "section", neighborLine: 12,
		},
		{
			name: "Sample Programs free-form parity validation", env: "SCRIPTHOLD_REAL_COBOL99_SAMPLE",
			target: "main", owner: "even-odd", kind: "function", targetLine: 12,
			excerpt: "main.", oracle: "if function test-numval(cmdargs) not = 0",
			neighbor: "working-storage", neighborKind: "section", neighborLine: 5,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_COBOL99_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original COBOL source not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original COBOL file and allowed root must both be configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("selected COBOL source must be bounded and regular: info=%v err=%v", info, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(original), tc.excerpt) || !strings.Contains(string(original), tc.oracle) {
				t.Fatal("source-first original COBOL maintenance evidence mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "cobol", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original COBOL digest failed: err=%v result=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "cobol",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated || !outline.CoverageComplete {
				t.Fatalf("original COBOL outline incomplete: err=%v result=%+v", err, outline)
			}
			selected, neighbor := "", ""
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					string(sym.Kind) == tc.kind && sym.NameRange.Start.Line == tc.targetLine {
					selected = sym.ID
				}
				if sym.Name == tc.neighbor && sym.ParentQualifiedName == tc.owner &&
					string(sym.Kind) == tc.neighborKind && sym.NameRange.Start.Line == tc.neighborLine {
					neighbor = sym.ID
				}
				if sym.Name == "SAMPLE.DAT" || sym.Name == "test-numval" || sym.Name == "STOP" {
					t.Errorf("literal, call or instruction fabricated as COBOL declaration: %+v", sym)
				}
			}
			if selected == "" || neighbor == "" || selected == neighbor {
				t.Fatalf("COBOL edit target or distinct section missing: target=%q neighbor=%q symbols=%+v", selected, neighbor, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cobol",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("COBOL exact find failed: err=%v result=%+v", err, found)
			}
			matched := false
			for _, sym := range found.Symbols {
				if sym.ID == selected && sym.NameRange.Start.Line == tc.targetLine {
					matched = true
				}
			}
			file := found.Files[0]
			if !matched || file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("COBOL exact source selector missing: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "cobol", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("COBOL fingerprint-bound original excerpt unavailable: err=%v shown=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "cobol", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 128,
			})
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated {
				t.Fatalf("COBOL structural search unavailable: err=%v result=%+v", err, query)
			}
			linked := false
			for _, hit := range query.Search.Matches {
				if hit.SymbolID == selected && hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("COBOL search did not link original declaration: %+v", query.Search)
			}
		})
	}
}
