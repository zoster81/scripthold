package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type json100Target struct {
	file, name, owner, ownerPrefix, excerpt string
	line                                    int
}

// These original Git manifests were read before testing the local handlers.
// Package export declarations are not executable module-resolution evidence.
func TestJSON100AdditionalOriginalProjectNavigation(t *testing.T) {
	vue := os.Getenv("SCRIPTHOLD_REAL_JSON100_VUE_ROOT")
	svelte := os.Getenv("SCRIPTHOLD_REAL_JSON100_SVELTE_ROOT")
	if vue == "" && svelte == "" {
		t.Skip("independent original JSON project roots not configured")
	}
	if vue == "" || svelte == "" {
		t.Fatal("both independent original JSON roots must be configured together")
	}
	cases := []struct {
		name, root, packageFile, distractorEvidence, rootEvidence, packageEvidence, absent string
		targets                                                                            []json100Target
	}{
		{
			name: "Vue build/test script versus package entrypoints",
			root: vue, packageFile: filepath.Join("packages", "vue", "package.json"),
			distractorEvidence: "// Use IntelliSense to learn about possible attributes.",
			rootEvidence:       "\"test-e2e\": \"node scripts/build.js vue -f global -d && vitest --project e2e --project e2e-browser\"",
			packageEvidence:    "\"module\": \"dist/vue.runtime.esm-bundler.js\"",
			absent:             "prepublishOnly",
			targets: []json100Target{
				{file: "package.json", name: "test-e2e", owner: "scripts", line: 21, excerpt: "\"test-e2e\": \"node scripts/build.js vue -f global -d"},
				{file: "package.json", name: "build", owner: "scripts", line: 8, excerpt: "\"build\": \"node scripts/build.js\""},
				{file: filepath.Join("packages", "vue", "package.json"), name: "module", line: 6, excerpt: "\"module\": \"dist/vue.runtime.esm-bundler.js\""},
				{file: filepath.Join("packages", "vue", "package.json"), name: "browser", ownerPrefix: "exports.", line: 49, excerpt: "\"browser\": \"./compiler-sfc/index.browser.mjs\""},
			},
		},
		{
			name: "Svelte build script versus browser and worker exports",
			root: svelte, packageFile: filepath.Join("packages", "svelte", "package.json"),
			distractorEvidence: "\"name\": \"Run sandbox\"",
			rootEvidence:       "\"build\": \"pnpm -r --filter=./packages/* build\"",
			packageEvidence:    "\"prepublishOnly\": \"pnpm build\"",
			absent:             "test-e2e",
			targets: []json100Target{
				{file: "package.json", name: "check", owner: "scripts", line: 18, excerpt: "\"check\": \"cd packages/svelte && pnpm build"},
				{file: filepath.Join("packages", "svelte", "package.json"), name: "worker", ownerPrefix: "exports.", line: 25, excerpt: "\"worker\": \"./src/index-server.js\""},
				{file: filepath.Join("packages", "svelte", "package.json"), name: "browser", ownerPrefix: "exports.", line: 26, excerpt: "\"browser\": \"./src/index-client.js\""},
				{file: filepath.Join("packages", "svelte", "package.json"), name: "browser", ownerPrefix: "exports.", line: 77, excerpt: "\"browser\": \"./src/legacy/legacy-client.js\""},
				{file: filepath.Join("packages", "svelte", "package.json"), name: "prepublishOnly", owner: "scripts", line: 150, excerpt: "\"prepublishOnly\": \"pnpm build\""},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := map[string]string{}
			for _, relative := range []string{"package.json", tc.packageFile, filepath.Join(".vscode", "launch.json")} {
				path := filepath.Join(tc.root, relative)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("unsafe or oversized original %s: info=%v err=%v", relative, info, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.HasSuffix(relative, "launch.json") && !json.Valid(raw) {
					t.Fatalf("original manifest is not valid JSON: %s", relative)
				}
				original[relative] = string(raw)
			}
			if !strings.Contains(original["package.json"], tc.rootEvidence) ||
				!strings.Contains(original[tc.packageFile], tc.packageEvidence) ||
				!strings.Contains(original[filepath.Join(".vscode", "launch.json")], tc.distractorEvidence) ||
				strings.Contains(original["package.json"], "\""+tc.absent+"\":") {
				t.Fatal("original manifest and unrelated editor configuration oracle mismatch")
			}
			for _, target := range tc.targets {
				lines := strings.Split(original[target.file], "\n")
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], "\""+target.name+"\":") ||
					!strings.Contains(lines[target.line-1], target.excerpt) {
					t.Fatalf("original JSON key/line oracle mismatch %s:%d %s", target.file, target.line, target.name)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			outlined := map[string]map[string]bool{}
			for _, relative := range []string{"package.json", tc.packageFile} {
				path := filepath.Join(tc.root, relative)
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Presentation: "compact", Paths: []string{path}, Language: "json", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
					len(digest.OverviewFiles) != 1 || !digest.CoverageComplete {
					t.Fatalf("valid JSON digest failed %s: err=%v result=%+v", relative, err, digest)
				}
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "json",
					MaxFiles: 1, MaxSymbols: 1024,
				})
				if err != nil || outline.Truncated || outline.FilesParsed != 1 ||
					len(outline.Files) != 1 || !outline.CoverageComplete {
					t.Fatalf("valid JSON outline failed %s: err=%v result=%+v", relative, err, outline)
				}
				ids := map[string]bool{}
				for _, sym := range outline.Symbols {
					ids[sym.ID] = true
				}
				outlined[relative] = ids
			}
			for _, target := range tc.targets {
				path := filepath.Join(tc.root, target.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "json",
					Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 512, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 || !found.CoverageComplete {
					t.Fatalf("original JSON find failed %s:%d: err=%v result=%+v", target.file, target.line, err, found)
				}
				id := ""
				for _, sym := range found.Symbols {
					if sym.Path == path && sym.Name == target.name && sym.NameRange.Start.Line == target.line &&
						(target.owner == "" || sym.ParentQualifiedName == target.owner) &&
						(target.ownerPrefix == "" || strings.HasPrefix(sym.ParentQualifiedName, target.ownerPrefix)) {
						id = sym.ID
					}
				}
				if id == "" || !outlined[target.file][id] {
					t.Fatalf("original JSON key absent or ownership wrong %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
				}
				file := found.Files[0]
				if file.SourceFingerprint == "" || file.Encoding == "" {
					t.Fatalf("missing source fingerprint/encoding %s: %+v", target.file, file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "json", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("fingerprint-bound original JSON excerpt failed %s:%d: err=%v shown=%+v", target.file, target.line, err, shown.Show)
				}
				_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "json", Query: target.name, Match: "exact", MaxFiles: 1, MaxResults: 256,
				})
				linked := false
				if searched.Search != nil {
					for _, hit := range searched.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || searched.Search == nil || searched.Coverage.ResultsTruncated ||
					!searched.Coverage.CoverageComplete || !linked {
					t.Fatalf("original JSON query-to-show identity missing %s:%d: err=%v query=%+v", target.file, target.line, err, searched)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(tc.root, "package.json")},
				Language: "json", Query: tc.absent, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated JSON key falsely indexed %s: err=%v symbols=%+v", tc.absent, err, negative.Symbols)
			}
			t.Logf("original JSON source navigation: 3 files, %d targets", len(tc.targets))
		})
	}
}
