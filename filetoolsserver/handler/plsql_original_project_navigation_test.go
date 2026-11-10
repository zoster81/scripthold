package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Two independent byte-verified original PL/SQL projects exercise overloaded
// package methods and multiple package identities without running SQL.
func TestPLSQLOriginalProjectPackageNavigation(t *testing.T) {
	tests := []struct {
		name, env, target, owner, sibling, siblingOwner, oracle, excerpt, unrelated string
		lines                                                                       []int
		siblingLine, packageCount                                                   int
	}{
		{
			name:   "Alexandria SHA-256 raw and blob overloads",
			env:    "SCRIPTHOLD_REAL_PLSQL_ALEXANDRIA",
			target: "sha256", owner: "hash_util_pkg", lines: []int{31, 40},
			sibling: "sha512", siblingOwner: "hash_util_pkg", siblingLine: 33,
			oracle:  "create or replace package hash_util_pkg as",
			excerpt: "function sha256(", unrelated: "run_automated_tests",
			packageCount: 1,
		},
		{
			name:   "Oxar test runner must belong to second package, not print package",
			env:    "SCRIPTHOLD_REAL_PLSQL_OXAR",
			target: "run_automated_tests", owner: "aop_test3_pkg", lines: []int{629, 641},
			sibling: "get_file", siblingOwner: "aop_sample3_pkg", siblingLine: 260,
			oracle:  "create or replace package body aop_test3_pkg as",
			excerpt: "procedure run_automated_tests(", unrelated: "sha256",
			packageCount: 2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_PLSQL_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("byte-verified original PL/SQL project snapshots not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original PL/SQL project fixture is partially configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("cannot inspect original PL/SQL source: %v", err)
			}
			if !strings.Contains(string(raw), tc.oracle) || !strings.Contains(string(raw), tc.excerpt) {
				t.Fatalf("original source oracle did not match source bytes")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "plsql", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original PL/SQL source digest unusable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "plsql",
				MaxFiles: 1, MaxSymbols: 128, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("PL/SQL original outline incomplete: err=%v outline=%+v", err, outline)
			}
			packages, siblingOK := map[string]bool{}, false
			targetLines := map[int]bool{}
			for _, sym := range outline.Symbols {
				if string(sym.Kind) == "package" {
					packages[sym.QualifiedName] = true
				}
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner {
					targetLines[sym.NameRange.Start.Line] = true
				}
				if sym.Name == tc.sibling && sym.ParentQualifiedName == tc.siblingOwner &&
					sym.NameRange.Start.Line == tc.siblingLine {
					siblingOK = true
				}
				if tc.owner == "aop_test3_pkg" && sym.Name == tc.target &&
					sym.ParentQualifiedName != tc.owner {
					t.Fatalf("original PL/SQL runner attributed to false package: %+v", sym)
				}
			}
			if len(packages) != tc.packageCount || !packages[tc.owner] || !siblingOK ||
				len(targetLines) != len(tc.lines) {
				t.Fatalf("PL/SQL package or related routine mapping is wrong: packages=%v targetLines=%v sibling=%v", packages, targetLines, siblingOK)
			}
			for _, line := range tc.lines {
				if !targetLines[line] {
					t.Fatalf("PL/SQL original declaration line %d absent: %v", line, targetLines)
				}
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "plsql",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 ||
				len(found.Symbols) != len(tc.lines) {
				t.Fatalf("PL/SQL overloaded method lookup unusable: err=%v found=%+v", err, found)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("PL/SQL exact source identity unavailable: %+v", file)
			}
			selectors := make(map[int]string, len(tc.lines))
			for _, sym := range found.Symbols {
				if sym.ParentQualifiedName != tc.owner || sym.Name != tc.target ||
					sym.ID == "" || sym.QualifiedName != tc.owner+"."+tc.target {
					t.Fatalf("PL/SQL false edit target: %+v", sym)
				}
				selectors[sym.NameRange.Start.Line] = sym.ID
			}
			for _, line := range tc.lines {
				id := selectors[line]
				if id == "" {
					t.Fatalf("missing PL/SQL exact source selector at line %d: %+v", line, found.Symbols)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "plsql", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
					t.Fatalf("PL/SQL exact source excerpt line %d incorrect: err=%v shown=%+v", line, err, shown.Show)
				}
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "plsql", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || search.Search == nil || !search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated {
				t.Fatalf("PL/SQL structural search incomplete: err=%v result=%+v", err, search)
			}
			matches := 0
			for _, hit := range search.Search.Matches {
				if hit.Name == tc.target && hit.SourceFingerprint == file.SourceFingerprint &&
					hit.SymbolID == selectors[hit.Range.Start.Line] {
					matches++
				}
			}
			if matches != len(tc.lines) {
				t.Fatalf("PL/SQL structural selectors do not match exact declarations: %+v", search.Search)
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "plsql",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || !unrelated.CoverageComplete || len(unrelated.Symbols) != 0 {
				t.Fatalf("PL/SQL unrelated declaration was fabricated: err=%v result=%+v", err, unrelated.Symbols)
			}
		})
	}
}
