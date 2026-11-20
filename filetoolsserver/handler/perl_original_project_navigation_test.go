package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Two independent original Perl projects validate maintenance navigation and
// restoration of a package declared inside a lexical block. No Perl runs.
func TestPerlOriginalProjectPackageNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, sibling, siblingOwner, excerpt, oracle, unrelated string
		line, siblingLine                                                           int
	}{
		{
			name:   "Vim Perl tag formatter and package name scanner",
			env:    "SCRIPTHOLD_REAL_PERL_VIM",
			target: "MakeTag", owner: "", line: 57,
			sibling: "PackageName", siblingOwner: "", siblingLine: 92,
			excerpt: "sub MakeTag($$$$$)",
			oracle:  "sub VarNames($)", unrelated: "get_basic_credentials",
		},
		{
			name:   "Source Highlight nested request agent and outer constructor",
			env:    "SCRIPTHOLD_REAL_PERL_HIGHLIGHT",
			target: "get_basic_credentials", owner: "RequestAgent", line: 55,
			sibling: "new", siblingOwner: "MIME::Lite::HTML", siblingLine: 85,
			excerpt: "sub get_basic_credentials",
			oracle:  "package MIME::Lite::HTML;", unrelated: "MakeTag",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_PERL_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original Perl projects not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original Perl project fixture partially configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original Perl source maintenance oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "perl", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Perl source digest unavailable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "perl",
				MaxFiles: 1, MaxSymbols: 128, Presentation: "detailed",
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original Perl outline incomplete: err=%v outline=%+v", err, outline)
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
				if tc.env == "SCRIPTHOLD_REAL_PERL_HIGHLIGHT" &&
					sym.Name == "new" && sym.NameRange.Start.Line == 85 &&
					sym.ParentQualifiedName == "RequestAgent" {
					t.Fatalf("block-local RequestAgent package leaked to outer subroutine: %+v", sym)
				}
			}
			if !targetOK || !siblingOK {
				t.Fatalf("original Perl maintenance target/sibling not indexed: target=%v sibling=%v", targetOK, siblingOK)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "perl",
				Query: tc.target, Match: "exact", MaxFiles: 1,
				MaxSymbols: 64, FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Perl source find incomplete: err=%v result=%+v", err, found)
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
				t.Fatalf("original Perl exact source selector absent: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "perl", Encoding: file.Encoding,
				SymbolID: targetID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("original Perl fingerprint-verified excerpt incorrect: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "perl", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("Perl project structural search unavailable: err=%v search=%+v", err, search)
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
				t.Fatalf("Perl structural query cannot navigate to source selector: %+v", search.Search)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "perl",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("Perl cross-project function fabricated: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
