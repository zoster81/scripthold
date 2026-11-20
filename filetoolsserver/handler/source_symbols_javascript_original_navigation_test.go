package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Maintenance questions, relevant tests and unrelated files were identified
// from independent original-source reads. Original projects are never executed.
func TestSourceSymbolsOriginalJavaScriptTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original JavaScript corpus not configured")
	}
	type symbol struct {
		file, name, kind string
		line             int
		excerpt          string
	}
	cases := []struct {
		name, project, query, distractor string
		files, evidence                  []string
		expected                         []symbol
	}{
		{
			name:    "lozad-auto-reload-attribute-filter-and-tests",
			project: "real-projects/javascript-apoorvsaxena-lozad.js",
			files:   []string{"src/lozad.js", "test/index.js", "rollup.config.js"},
			evidence: []string{
				"validAttribute.indexOf(entry.attributeName) > -1",
				"data-background-image-set",
				"enableAutoReload: true",
				"input: 'src/lozad.js'",
			},
			expected: []symbol{
				{"src/lozad.js", "validAttribute", "constant", 15, ""},
				{"src/lozad.js", "defaultConfig", "constant", 17, ""},
				{"src/lozad.js", "markAsLoaded", "function", 96, "setAttribute('data-loaded', true)"},
				{"src/lozad.js", "preLoad", "function", 100, "data-placeholder-background"},
				{"src/lozad.js", "onMutation", "function", 122, ""},
			},
			query: "onMutation", distractor: "rollup.config.js",
		},
		{
			name:    "anyproxy-remote-rule-loading-and-regression",
			project: "real-projects/javascript-alibaba-anyproxy",
			files:   []string{"lib/ruleLoader.js", "test/lib/ruleLoader.spec.js", "lib/rule_default.js"},
			evidence: []string{
				"response.statusCode !== 200",
				"failed to load with a status code",
				"ruleLoader.requireModule('https://cdn.bootcss.com",
				"*beforeSendRequest(requestDetail)",
			},
			expected: []symbol{
				{"lib/ruleLoader.js", "cacheRemoteFile", "function", 16, "remote_rule_"},
				{"lib/ruleLoader.js", "loadLocalPath", "function", 42, "fs.existsSync(ruleFilePath)"},
				{"lib/ruleLoader.js", "requireModule", "function", 60, "cacheRemoteFile(urlOrPath)"},
			},
			query: "requireModule", distractor: "lib/rule_default.js",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, filepath.FromSlash(tc.project))
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			originals := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				fi, err := os.Lstat(path)
				if err != nil || !fi.Mode().IsRegular() {
					t.Fatalf("original source unavailable %s: %v", rel, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
				originals = append(originals, string(raw))
			}
			for _, fragment := range tc.evidence {
				seen := false
				for _, s := range originals {
					if strings.Contains(s, fragment) {
						seen = true
						break
					}
				}
				if !seen {
					t.Fatalf("original source/test oracle changed: %q", fragment)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "javascript", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete || digest.FilesParsed != len(paths) {
				t.Fatalf("digest err=%v tool=%+v digest=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("large first map bytes=%d err=%v", len(data), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d reportedComplete=%v", digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.expected {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "javascript", Encoding: "utf-8", Query: want.name, Match: "exact", MaxFiles: 1, MaxSymbols: 20,
				})
				if err != nil || failed != nil || found.Truncated {
					t.Fatalf("find %s err=%v tool=%+v", want.name, err, failed)
				}
				id := ""
				for _, s := range found.Symbols {
					if s.Name == want.name && string(s.Kind) == want.kind && s.DeclarationRange.Start.Line == want.line {
						id = s.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing %s kind=%s at %s:%d: %+v", want.name, want.kind, want.file, want.line, found.Symbols)
				}
				if want.excerpt == "" {
					continue
				}
				if len(found.Files) != 1 || found.Files[0].SourceFingerprint == "" {
					t.Fatalf("missing fingerprint %s", want.name)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
					Language: "javascript", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("fingerprint-bound body for %s missing %q err=%v tool=%+v show=%+v", want.name, want.excerpt, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: "javascript", Encoding: "utf-8", Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s err=%v tool=%+v", tc.query, err, failed)
			}
			failed, other, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.distractor))},
				Language: "javascript", Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || failed != nil || other.Truncated || len(other.Symbols) != 0 {
				t.Fatalf("fabricated target in unrelated %s err=%v symbols=%+v", tc.distractor, err, other.Symbols)
			}
		})
	}
}
