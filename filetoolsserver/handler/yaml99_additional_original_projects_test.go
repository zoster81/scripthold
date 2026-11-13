package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type yaml99Target struct {
	name, owner, excerpt string
	line                 int
}

// Original Git workflow sources were read and independently pinned before
// any local Source Intelligence handler was queried. YAML sequences are
// deliberately partial: a step key is not a unique executable step identity.
func TestYAML99AdditionalOriginalProjectNavigation(t *testing.T) {
	zoxide := os.Getenv("SCRIPTHOLD_REAL_YAML99_ZOXIDE_ROOT")
	ripgrep := os.Getenv("SCRIPTHOLD_REAL_YAML99_RIPGREP_ROOT")
	if zoxide == "" && ripgrep == "" {
		t.Skip("independent original YAML project roots not configured")
	}
	if zoxide == "" || ripgrep == "" {
		t.Fatal("both independent original YAML project roots must be configured")
	}
	for _, tc := range []struct {
		name, root, neighbor, neighborEvidence, releaseEvidence, negative string
		ciEvidence                                                        []string
		targets                                                           []yaml99Target
	}{
		{
			name:             "zoxide CI lint/test commands versus artifact packaging",
			root:             zoxide,
			neighbor:         "justfile",
			neighborEvidence: "test *args:",
			releaseEvidence:  "tag_name:",
			negative:         "tag_name",
			ciEvidence:       []string{"run: just lint test", "uses: taiki-e/install-action@v2.85.13", "fetch-depth: 0"},
			targets: []yaml99Target{
				{name: "fetch-depth", owner: "jobs.ci.steps.with", line: 26, excerpt: "fetch-depth: 0"},
				{name: "run", owner: "jobs.ci.steps", line: 57, excerpt: "run: just lint test"},
			},
		},
		{
			name:             "ripgrep cross target and PCRE2 CI versus release workflow",
			root:             ripgrep,
			neighbor:         filepath.Join("ci", "ubuntu-install-packages"),
			neighborEvidence: "sudo apt-get install -y --no-install-recommends",
			releaseEvidence:  "attestations: write",
			negative:         "attestations",
			ciEvidence:       []string{"CROSS_VERSION: v0.2.5", "ci/ubuntu-install-packages", "Build ripgrep with PCRE2"},
			targets: []yaml99Target{
				{name: "CROSS_VERSION", owner: "jobs.test.env", line: 46, excerpt: "CROSS_VERSION: v0.2.5"},
				{name: "run", owner: "jobs.test.steps", line: 158, excerpt: "--features pcre2"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ci := filepath.Join(tc.root, ".github", "workflows", "ci.yml")
			release := filepath.Join(tc.root, ".github", "workflows", "release.yml")
			source := make(map[string]string)
			for _, name := range []string{ci, release, filepath.Join(tc.root, tc.neighbor)} {
				info, err := os.Lstat(name)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("missing or oversized original source %s: info=%v err=%v", name, info, err)
				}
				raw, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				source[name] = string(raw)
			}
			for _, evidence := range tc.ciEvidence {
				if !strings.Contains(source[ci], evidence) {
					t.Fatalf("original CI source oracle missing %q", evidence)
				}
			}
			if !strings.Contains(source[release], tc.releaseEvidence) ||
				!strings.Contains(source[filepath.Join(tc.root, tc.neighbor)], tc.neighborEvidence) ||
				strings.Contains(source[ci], tc.negative+":") {
				t.Fatal("original CI/release/neighbor source distinction changed")
			}
			lines := strings.Split(source[ci], "\n")
			for _, target := range tc.targets {
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], target.name+":") ||
					!strings.Contains(lines[target.line-1], target.excerpt) {
					t.Fatalf("original line oracle missing %s:%d %s", ci, target.line, target.name)
				}
			}

			ctx, h := context.Background(), NewHandler([]string{tc.root})
			for _, path := range []string{ci, release} {
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Presentation: "compact", Paths: []string{path},
					Language: "yaml", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
					len(digest.OverviewFiles) != 1 {
					t.Fatalf("original YAML digest missing %s: err=%v digest=%+v", path, err, digest)
				}
				if path == ci && digest.CoverageComplete {
					t.Fatalf("YAML sequence-index limitations concealed by complete coverage: %s", path)
				}
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{ci},
				Language: "yaml", MaxFiles: 1, MaxSymbols: 1024,
			})
			if err != nil || outline.Truncated || outline.CoverageComplete || len(outline.Files) != 1 {
				t.Fatalf("original YAML outline failed or hid partial coverage: err=%v outline=%+v", err, outline)
			}
			for _, target := range tc.targets {
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{ci}, Language: "yaml",
					Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 512,
					FileDetails: "relevant",
				})
				if err != nil || found.Truncated || found.CoverageComplete || len(found.Files) != 1 {
					t.Fatalf("original YAML find failed %s: err=%v found=%+v", target.name, err, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Path == ci && symbol.Name == target.name &&
						symbol.ParentQualifiedName == target.owner &&
						symbol.NameRange.Start.Line == target.line {
						id = symbol.ID
					}
				}
				outlined := false
				for _, symbol := range outline.Symbols {
					if symbol.ID == id && symbol.Name == target.name &&
						symbol.NameRange.Start.Line == target.line {
						outlined = true
					}
				}
				if id == "" || !outlined {
					t.Fatalf("original YAML declaration not found/outlined %s:%d %s matches=%+v", ci, target.line, target.name, found.Symbols)
				}
				f := found.Files[0]
				if f.SourceFingerprint == "" || f.Encoding == "" {
					t.Fatalf("original YAML source identity unavailable: %+v", f)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: ci, Language: "yaml", Encoding: f.Encoding,
					SymbolID: id, SourceFingerprint: f.SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("fingerprint-bound original YAML excerpt missing %s:%d: err=%v shown=%+v", target.name, target.line, err, shown.Show)
				}
				_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{ci},
					Language: "yaml", Query: target.name, Match: "exact",
					MaxFiles: 1, MaxResults: 128,
				})
				linked := false
				if searched.Search != nil {
					for _, hit := range searched.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == f.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || searched.Search == nil || searched.Coverage.ResultsTruncated ||
					searched.Coverage.CoverageComplete || !linked {
					t.Fatalf("source_query structural YAML linkage inaccurate %s:%d: err=%v search=%+v", target.name, target.line, err, searched)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{ci}, Language: "yaml",
				Query: tc.negative, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("release-only YAML key fabricated inside CI %s: err=%v symbols=%+v", tc.negative, err, negative.Symbols)
			}
			t.Logf("original YAML project source navigation PASS: CI, release and neighbor; targets=%d", len(tc.targets))
		})
	}
}
