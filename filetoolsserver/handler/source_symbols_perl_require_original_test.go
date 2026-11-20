package handler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The MCP digest must not promote strings nested in computed Perl require
// targets into definite imports. Authored use imports remain navigable.
func TestPerlOriginalComputedRequireDigestEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker   string
		forbidden, required []string
	}{
		{"opennms", "SCRIPTHOLD_REAL_PERL_OPENNMS",
			"require(File::Spec->catfile($PREFIX, 'bin', 'functions.pl'))",
			[]string{"bin", "functions.pl"}, []string{"File::Spec", "File::Basename"}},
		{"regripper", "SCRIPTHOLD_REAL_PERL_REGRIPPER",
			"require File::Spec->catfile($RealBin,\"time.pl\")",
			[]string{"time.pl", "rr_helper.pl"}, []string{"File::Spec", "JSON::PP"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Perl source not configured")
			}
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() < 100 || stat.Size() > 60000 {
				t.Fatalf("original Perl source missing/unbounded: %v %v", stat, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original Perl source oracle changed")
			}
			ctx, h := context.Background(), NewHandler([]string{filepath.Dir(path)})
			failure, output, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "perl",
				MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || output.Truncated || len(output.Digests) != 1 {
				t.Fatalf("Perl MCP digest unavailable: %v %+v %+v", err, failure, output)
			}
			got := make([]string, 0, len(output.Digests[0].Dependencies))
			for _, d := range output.Digests[0].Dependencies {
				got = append(got, d.Value)
			}
			if output.Digests[0].SourceFingerprint == "" {
				t.Fatal("missing source fingerprint for authored Perl original")
			}
			for _, forbidden := range tc.forbidden {
				if slices.Contains(got, forbidden) {
					t.Fatalf("invented Perl dependency %q: %q", forbidden, got)
				}
			}
			for _, required := range tc.required {
				if !slices.Contains(got, required) {
					t.Fatalf("lost authored Perl use import %q: %q", required, got)
				}
			}
		})
	}
}
