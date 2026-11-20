package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source-derived tasks: navigate Phobos strict JSON parsing and vibe.d MongoDB
// connect-timeout configuration without executing either original project.
func TestSourceSymbolsOriginalDMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original D projects are not configured")
	}
	type evidence struct{ file, text string }
	type target struct {
		file, name, excerpt string
		line                int
	}
	cases := []struct {
		project, query, negative string
		files                    []string
		evidence                 []evidence
		targets                  []target
	}{
		{
			project: "d-dlang-phobos", query: "parseJSON",
			negative: "std/algorithm/searching.d",
			files:    []string{"std/json.d", "std/algorithm/searching.d"},
			evidence: []evidence{
				{"std/json.d", "strictParsing = 0x8"},
				{"std/json.d", "JSONValue parseJSON(T)(T json, int maxDepth = -1, JSONOptions options = JSONOptions.none)"},
				{"std/json.d", "immutable bool strict = (options & JSONOptions.strictParsing) != 0;"},
				{"std/json.d", "assertThrown!JSONException(parseJSON(\"True\", JSONOptions.strictParsing));"},
				{"std/json.d", "if (maxDepth != -1 && depth > maxDepth) error(\"Nesting too deep.\");"},
				{"std/algorithm/searching.d", "generic searching algorithms"},
			},
			targets: []target{
				{"std/json.d", "JSONOptions", "strictParsing = 0x8", 81},
				{"std/json.d", "parseJSON", "", 1225},
				{"std/json.d", "parseJSON", "", 1765},
				{"std/json.d", "toJSON", "", 1780},
			},
		},
		{
			project: "d-vibe-d-vibe.d", query: "connectTimeoutMS",
			negative: "mongodb/vibe/db/mongo/collection.d",
			files: []string{
				"mongodb/vibe/db/mongo/settings.d",
				"mongodb/vibe/db/mongo/client.d",
				"tests/mongodb/_connection/source/app.d",
				"mongodb/vibe/db/mongo/collection.d",
			},
			evidence: []evidence{
				{"mongodb/vibe/db/mongo/settings.d", "Duration connectTimeout = 10.seconds;"},
				{"mongodb/vibe/db/mongo/settings.d", "long connectTimeoutMS() const @property"},
				{"mongodb/vibe/db/mongo/settings.d", "connectTimeout = ms.msecs;"},
				{"mongodb/vibe/db/mongo/settings.d", "case \"connecttimeoutms\": setMsecs(cfg.connectTimeout); break;"},
				{"mongodb/vibe/db/mongo/settings.d", "assert(cfg.connectTimeoutMS == 10_000);"},
				{"mongodb/vibe/db/mongo/settings.d", "assert(parseMongoDBUrl(cfg, \"mongodb://localhost/?connectTimeoutMS=5000\"));"},
				{"mongodb/vibe/db/mongo/client.d", "auto goodUrl = parseMongoDBUrl(settings, url);"},
				{"tests/mongodb/_connection/source/app.d", "settings.connectTimeoutMS = 5_000;"},
			},
			targets: []target{
				{"mongodb/vibe/db/mongo/settings.d", "parseMongoDBUrl", "", 36},
				{"mongodb/vibe/db/mongo/settings.d", "MongoClientSettings", "", 1015},
				{"mongodb/vibe/db/mongo/settings.d", "connectTimeout", "10.seconds", 1150},
				{"mongodb/vibe/db/mongo/settings.d", "connectTimeoutMS", "connectTimeout.total", 1153},
				{"mongodb/vibe/db/mongo/settings.d", "connectTimeoutMS", "connectTimeout = ms.msecs", 1159},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original path invalid %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, e := range tc.evidence {
				path := filepath.Join(root, filepath.FromSlash(e.file))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original evidence path invalid %s: %v", e.file, err)
				}
				data, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(data), e.text) {
					t.Fatalf("original source oracle %s %q: %v", e.file, e.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "d", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("D bounded digest: err=%v failed=%+v result=%+v", err, failed, digest)
			}
			b, err := json.Marshal(digest)
			if err != nil || len(b) > 8192 {
				t.Fatalf("D initial response bytes=%d err=%v", len(b), err)
			}
			t.Logf("originalProject=1 files=%d symbols=%d compactBytes=%d coverageComplete=%v",
				digest.FilesParsed, digest.SymbolCount, len(b), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact", Language: "d", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 80,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("D exact find %s: err=%v failed=%+v found=%+v", want.name, err, failed, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && sym.NameRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("D original declaration missing %s:%d %s results=%+v", want.file, want.line, want.name, found.Symbols)
				}
				if want.excerpt != "" {
					failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
						Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "d", Encoding: "utf-8", MaxBytes: 8192,
					})
					if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
						t.Fatalf("D fingerprint show %s: err=%v failed=%+v shown=%+v", want.name, err, failed, shown.Show)
					}
				}
			}
			failed, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural",
				Language: "d", Encoding: "utf-8", MaxResults: 32,
			})
			if err != nil || failed != nil || result.Search == nil || len(result.Search.Matches) == 0 {
				t.Fatalf("D structural search %s: err=%v failed=%+v search=%+v", tc.query, err, failed, result.Search)
			}
			failed, unrelated, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.negative))},
				Query: tc.query, Match: "exact", Language: "d", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || failed != nil || unrelated.Truncated || len(unrelated.Symbols) != 0 {
				t.Fatalf("unrelated D module falsely defines %s: err=%v failed=%+v symbols=%+v", tc.query, err, failed, unrelated.Symbols)
			}
		})
	}
}
