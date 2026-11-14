package handler

import (
	"context"
	"os"
	"path/filepath"
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

func TestYAMLOriginalWorkflowMultilineCommandShow(t *testing.T) {
	cases := []struct {
		name, env, relative, owner, marker, nextStep string
		start, end                                   int
	}{
		{
			name:     "zoxide release packaging script",
			env:      "SCRIPTHOLD_REAL_YAML_BLOCK_ZOXIDE_ROOT",
			relative: filepath.Join(".github", "workflows", "release.yml"),
			owner:    "jobs.release.steps", start: 95, end: 100,
			marker: "gzip --best >", nextStep: "Upload artifact",
		},
		{
			name:     "ripgrep cross compilation setup",
			env:      "SCRIPTHOLD_REAL_YAML_BLOCK_RIPGREP_ROOT",
			relative: filepath.Join(".github", "workflows", "ci.yml"),
			owner:    "jobs.test.steps", start: 134, end: 146,
			marker: "echo \"TARGET_DIR=./target/", nextStep: "Show command used for Cargo",
		},
	}
	configured := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("original YAML block-scalar projects not configured")
	}
	if configured != len(cases) {
		t.Fatal("both independent YAML block-scalar roots must be configured")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, tc.relative)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original workflow source unavailable: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if len(lines) <= tc.end || strings.TrimSpace(lines[tc.start-1]) != "run: |" ||
				!strings.Contains(strings.Join(lines[tc.start-1:tc.end], "\n"), tc.marker) ||
				!strings.Contains(strings.Join(lines[tc.end:], "\n"), tc.nextStep) {
				t.Fatal("original workflow maintenance oracle differs from independently reviewed source")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "yaml", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" || digest.CoverageComplete {
				t.Fatalf("original workflow digest or incomplete YAML coverage: %v %+v", err, digest)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "yaml", Query: "run",
				Match: "exact", MaxFiles: 1, MaxSymbols: 256, FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("YAML original find failed: %v %+v", err, found)
			}
			id := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == "run" && symbol.ParentQualifiedName == tc.owner &&
					symbol.NameRange.Start.Line == tc.start &&
					symbol.DeclarationRange.End.Line == tc.end {
					id = symbol.ID
				}
			}
			file := found.Files[0]
			if id == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("YAML full block selector unavailable: %+v", found.Symbols)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "yaml", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.marker) ||
				!strings.HasPrefix(shown.Show.Text, "run: |") ||
				strings.Contains(shown.Show.Text, tc.nextStep) {
				t.Fatalf("YAML command source show did not isolate full scalar: %v %+v", err, shown.Show)
			}
		})
	}
}
