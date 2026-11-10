package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Independent original GitHub Actions workflows check source-anchored keys
// inside YAML sequences without pretending list-item identities are modeled.
func TestYAMLOriginalWorkflowSequenceNavigation(t *testing.T) {
	cases := []struct {
		name, env, owner, target, sibling, siblingOwner, excerpt, oracle, unrelated string
		line, siblingLine                                                           int
	}{
		{
			name:   "Polly build action version and checkout options",
			env:    "SCRIPTHOLD_REAL_YAML_POLLY",
			target: "uses", owner: "jobs.build.steps", line: 75,
			sibling: "fetch-depth", siblingOwner: "jobs.build.steps.with", siblingLine: 77,
			excerpt: "uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1",
			oracle:  "timeout-minutes: 20", unrelated: "cancel-in-progress",
		},
		{
			name:   "pytest package action version and checkout options",
			env:    "SCRIPTHOLD_REAL_YAML_PYTEST",
			target: "uses", owner: "jobs.package.steps", line: 40,
			sibling: "fetch-depth", siblingOwner: "jobs.package.steps.with", siblingLine: 42,
			excerpt: "uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0",
			oracle:  "cancel-in-progress: true", unrelated: "DOTNET_SYSTEM_CONSOLE_ALLOW_ANSI_COLOR_REDIRECTION",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_YAML_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original YAML workflow not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original YAML workflow configuration is incomplete")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original YAML workflow source oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "yaml", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || digest.CoverageComplete ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("YAML workflow digest failed to expose partial coverage: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "yaml",
				MaxFiles: 1, MaxSymbols: 1024, Presentation: "detailed",
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated || outline.CoverageComplete {
				t.Fatalf("YAML source outline incomplete or misleading: err=%v outline=%+v", err, outline)
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
			}
			if !targetOK || !siblingOK {
				t.Fatalf("YAML workflow maintenance key or neighboring option missing: target=%v sibling=%v", targetOK, siblingOK)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "yaml", Query: tc.target,
				Match: "exact", MaxFiles: 1, MaxSymbols: 256, FileDetails: "relevant",
			})
			if err != nil || len(found.Files) != 1 || found.Truncated || found.CoverageComplete {
				t.Fatalf("YAML workflow targeted find failed: err=%v found=%+v", err, found)
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
				t.Fatalf("YAML safe source selector missing: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "yaml", Encoding: file.Encoding,
				SymbolID: targetID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("YAML selected action source incorrect: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "yaml", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 96,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated ||
				search.Coverage.CoverageComplete {
				t.Fatalf("YAML structural search hid partial coverage: err=%v search=%+v", err, search)
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
				t.Fatalf("YAML search-to-show identity unavailable: %+v", search.Search)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "yaml",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 100,
			})
			if err != nil || len(negative.Symbols) != 0 {
				t.Fatalf("YAML unrelated cross-project key fabricated: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
