package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Optional original-project checks use byte-verified Nix and Home Manager source.
// Structural navigation is deliberately partial for unindexed root attributes.
func TestNixOriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, excerpt, related, oracle, unrelated string
		line, relatedLine                                      int
	}{
		{
			name:   "Nix package matrix and system defaults",
			env:    "SCRIPTHOLD_REAL_NIX_MAIN",
			target: "nixpkgsFor", line: 97,
			excerpt: "nixpkgsFor = forAllSystems (",
			related: "forAllSystems", relatedLine: 78,
			oracle:    "packageSetsFor =",
			unrelated: "cfg",
		},
		{
			name:   "Home Manager module configuration",
			env:    "SCRIPTHOLD_REAL_NIX_HOME",
			target: "cfg", line: 8,
			excerpt:   "cfg = config.programs.home-manager;",
			oracle:    "default = null;",
			unrelated: "nixpkgsFor",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_NIX_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("original Nix source fixture not configured")
			}
			if root == "" || path == "" {
				t.Fatal("partially configured original Nix source fixture")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("original source unavailable: %v", err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original Nix source oracle differs from expected selected project")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "nix", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.CoverageComplete || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Nix root structure was not reported as partially covered: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "nix",
				MaxFiles: 1, MaxSymbols: 128, Presentation: "detailed",
			})
			if err != nil || outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("Nix original outline not safely partial: err=%v outline=%+v", err, outline)
			}
			seen := false
			for _, symbol := range outline.Symbols {
				if symbol.Name == tc.target && symbol.NameRange.Start.Line == tc.line {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("Nix source target %q missing at line %d", tc.target, tc.line)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nix",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || len(found.Files) != 1 || found.Truncated {
				t.Fatalf("Nix selected binding find failed: err=%v result=%+v", err, found)
			}
			id := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.target && symbol.NameRange.Start.Line == tc.line {
					id = symbol.ID
				}
			}
			if id == "" || found.Files[0].Encoding == "" ||
				found.Files[0].SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Nix target lacks exact safe selector: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "nix",
				Encoding: found.Files[0].Encoding, SymbolID: id,
				SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 2048,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Nix verified original excerpt unavailable: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "nix", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || search.Search == nil || search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated {
				t.Fatalf("Nix query did not expose honest partial coverage: err=%v query=%+v", err, search)
			}
			linked := false
			for _, match := range search.Search.Matches {
				if match.Name == tc.target && match.SymbolID == id &&
					match.SourceFingerprint == found.Files[0].SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Nix structural search did not find %q", tc.target)
			}
			if tc.related != "" {
				_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "nix",
					Query: tc.related, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				})
				related := false
				if err == nil {
					for _, symbol := range sibling.Symbols {
						if symbol.Name == tc.related && symbol.NameRange.Start.Line == tc.relatedLine {
							related = true
						}
					}
				}
				if !related {
					t.Fatalf("related original binding %q missing: err=%v", tc.related, err)
				}
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "nix",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || len(unrelated.Symbols) != 0 {
				t.Fatalf("unrelated Nix declaration fabricated: err=%v result=%+v", err, unrelated.Symbols)
			}
		})
	}
}
