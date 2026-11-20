package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original inputs are opt-in; a configured but missing source is a test failure.
func TestPLSQLOriginalQualifiedMCPPackageNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PLSQL_QUALIFIED_ROOT")
	if root == "" {
		t.Skip("verified original PL/SQL corpus not configured")
	}
	for _, tc := range []struct {
		file, owner, name string
		ownerLine         int
		targetLines       []int
	}{
		{file: "ut_expectation_processor.pks", owner: "ut_expectation_processor",
			name: "nulls_are_equal", ownerLine: 1, targetLines: []int{23, 25}},
		{file: "xdbConfiguration.sql", owner: "COE_CONFIGURATION",
			name: "setHTTPport", ownerLine: 56, targetLines: []int{59, 177}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			raw, err := os.ReadFile(path)
			if err != nil || len(raw) == 0 {
				t.Fatalf("missing original complete source %s: %v", tc.file, err)
			}
			ctx, handler := context.Background(), NewHandler([]string{root})
			failed, outline, err := handler.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "plsql", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("original detailed outline failed: err=%v failed=%v result=%+v", err, failed, outline)
			}
			foundPackage, targets := 0, map[int]bool{}
			for _, sym := range outline.Symbols {
				if sym.Kind == "package" && sym.Name == tc.owner {
					foundPackage++
					if sym.NameRange.Start.Line != tc.ownerLine {
						t.Errorf("package spec location: got %d want %d", sym.NameRange.Start.Line, tc.ownerLine)
					}
				}
				if sym.Name != tc.name {
					continue
				}
				if sym.ParentQualifiedName != tc.owner || sym.QualifiedName != tc.owner+"."+tc.name {
					t.Errorf("MCP routine owner invalid: %+v", sym)
				}
				targets[sym.NameRange.Start.Line] = true
			}
			if foundPackage != 1 || len(targets) != len(tc.targetLines) {
				t.Fatalf("wrong package or overload count: packages=%d targets=%v", foundPackage, targets)
			}
			for _, line := range tc.targetLines {
				if !targets[line] {
					t.Errorf("missing original authored routine at line %d", line)
				}
			}
			failed, digest, err := handler.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "plsql", Encoding: "utf-8", MaxFiles: 1,
			})
			if err != nil || failed != nil || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original digest missing: err=%v failed=%v", err, failed)
			}
			failed, found, err := handler.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "plsql", Encoding: "utf-8",
				Query: tc.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64, FileDetails: "relevant",
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != len(tc.targetLines) ||
				len(found.Files) != 1 || found.Files[0].SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("original find/fingerprint mismatch: err=%v found=%+v", err, found)
			}
			for _, sym := range found.Symbols {
				if sym.ParentQualifiedName != tc.owner || sym.ID == "" {
					t.Errorf("unsafe original selector: %+v", sym)
					continue
				}
				failed, shown, err := handler.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "plsql", Encoding: "utf-8",
					SymbolID: sym.ID, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(strings.ToLower(shown.Show.Text), strings.ToLower(tc.name)) {
					t.Errorf("fingerprint-bound source excerpt failed at line %d: %v", sym.NameRange.Start.Line, err)
				}
			}
			failed, search, err := handler.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "plsql", Query: tc.name, Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || search.Search == nil ||
				!search.Coverage.CoverageComplete || len(search.Search.Matches) != len(tc.targetLines) {
				t.Errorf("structural query mismatched original identities: err=%v result=%+v", err, search)
			}
		})
	}
}
