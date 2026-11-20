package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original projects are read-only. Expectations come from independent source
// inspection, not from source_* results.
func TestSourceSymbolsOriginalTypeScriptTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original TypeScript corpus not configured")
	}
	type symbol struct {
		file, name, kind string
		line             int
		excerpt          string
	}
	cases := []struct {
		name, root, query, distractor string
		files                         []string
		expected                      []symbol
		evidence                      []string
	}{
		{
			name:  "zustand-state-update-and-subscription",
			root:  "project-typescript-pmndrs-zustand",
			files: []string{"src/vanilla.ts", "src/middleware/subscribeWithSelector.ts", "tests/vanilla/subscribe.test.tsx", "src/vanilla/shallow.ts"},
			expected: []symbol{
				{"src/vanilla.ts", "createStoreImpl", "function", 60, "Object.is(nextState, state)"},
				{"src/vanilla.ts", "createStore", "function", 99, "createStoreImpl"},
				{"src/middleware/subscribeWithSelector.ts", "subscribeWithSelectorImpl", "constant", 46, ""},
			},
			evidence: []string{"listeners.forEach((listener) => listener(state, previousState))", "const equalityFn = options?.equalityFn || Object.is", "should not be called if new state identity is the same", "should be called when state slice changes"},
			query:    "createStoreImpl", distractor: "src/vanilla/shallow.ts",
		},
		{
			name:  "openspec-bulk-validation-concurrency",
			root:  "project-typescript-fission-ai-openspec",
			files: []string{"src/commands/validate.ts", "test/commands/validate.test.ts", "src/commands/show.ts"},
			expected: []symbol{
				{"src/commands/validate.ts", "ValidateCommand", "class", 45, ""},
				{"src/commands/validate.ts", "runBulkValidation", "method", 288, "DEFAULT_CONCURRENCY = 6"},
				{"src/commands/validate.ts", "normalizeConcurrency", "function", 544, "parseInt(value, 10)"},
				{"src/commands/show.ts", "ShowCommand", "class", 29, ""},
			},
			evidence: []string{"normalizeConcurrency(process.env.OPENSPEC_CONCURRENCY)", "while (running < concurrency && index < queue.length)", "respects --concurrency"},
			query:    "normalizeConcurrency", distractor: "src/commands/show.ts",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.root)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			contents := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				stat, err := os.Lstat(path)
				if err != nil || !stat.Mode().IsRegular() {
					t.Fatalf("missing original %s: %v", rel, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
				contents = append(contents, string(raw))
			}
			for _, v := range tc.evidence {
				found := false
				for _, source := range contents {
					if strings.Contains(source, v) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("original source evidence changed: %q", v)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "typescript", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete || digest.FilesParsed != len(paths) {
				t.Fatalf("digest failed: %v %+v %+v", err, failed, digest)
			}
			encoded, err := json.Marshal(digest)
			if err != nil || len(encoded) > 8192 {
				t.Fatalf("large map %d: %v", len(encoded), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d reportedComplete=%v", digest.FilesParsed, digest.SymbolCount, len(encoded), digest.CoverageComplete)
			for _, want := range tc.expected {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "typescript", Encoding: "utf-8", Query: want.name, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
				})
				if err != nil || failed != nil || found.Truncated {
					t.Fatalf("find %s failed: %v %+v", want.name, err, failed)
				}
				id := ""
				for _, s := range found.Symbols {
					if s.Name == want.name && string(s.Kind) == want.kind && s.DeclarationRange.Start.Line == want.line {
						id = s.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing %s:%d %s kind=%s: %+v", want.file, want.line, want.name, want.kind, found.Symbols)
				}
				if want.excerpt == "" {
					continue
				}
				if len(found.Files) != 1 || found.Files[0].SourceFingerprint == "" {
					t.Fatalf("missing source fingerprint %s", want.name)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "typescript", Encoding: "utf-8", MaxBytes: 16384,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("missing excerpt %s: %v %+v %+v", want.name, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: "typescript", Encoding: "utf-8", Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("search failed %s: %v %+v", tc.query, err, failed)
			}
			failed, other, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.distractor))},
				Language: "typescript", Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || failed != nil || other.Truncated || len(other.Symbols) != 0 {
				t.Fatalf("false target in distractor %s: %v %+v", tc.distractor, err, other.Symbols)
			}
		})
	}
}
