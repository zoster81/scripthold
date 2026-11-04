package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original, independently read projects are never executed.
func TestSourceSymbolsOriginalFlowTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Flow corpus not configured")
	}
	cases := []struct {
		name, root, decl, query, excerpt, distractor string
		files, evidence                              []string
		line                                         int
	}{
		{
			name:     "select-typed-disabled-versus-runtime",
			root:     "flow-jsflow-atmulyana-rn-select-option",
			files:    []string{"index.js.flow", "index.js", "picker.js", "styles.js"},
			evidence: []string{"diabled?: boolean", "aria-disabled={props?.disabled}", "return props?.disabled", "touchPicker: {"},
			decl:     "picker.js", query: "Picker", line: 11, excerpt: "props?.disabled", distractor: "styles.js",
		},
		{
			name:     "secure-random-native-module-link-and-tests",
			root:     "flow-jsflow-robhogan-react-native-securerandom",
			files:    []string{"index.js.flow", "index.js", "__tests__/notLinked.js", "__tests__/happy.js"},
			evidence: []string{"declare export function generateSecureRandom(length: number): Promise<Uint8Array>", "not properly linked", "expect(generateSecureRandom(10)).rejects", "expect(result).toEqual(Uint8Array.from"},
			decl:     "index.js", query: "generateSecureRandom", line: 6, excerpt: "not properly linked", distractor: "__tests__/happy.js",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.root)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			originals := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				fi, err := os.Lstat(p)
				if err != nil || !fi.Mode().IsRegular() {
					t.Fatalf("original file %s unavailable: %v", rel, err)
				}
				data, err := os.ReadFile(p)
				if err != nil {
					t.Fatal(err)
				}
				paths = append(paths, p)
				originals = append(originals, string(data))
			}
			for _, e := range tc.evidence {
				matched := false
				for _, source := range originals {
					if strings.Contains(source, e) {
						matched = true
						break
					}
				}
				if !matched {
					t.Fatalf("source oracle missing %q", e)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "flow", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete || digest.FilesParsed != len(paths) {
				t.Fatalf("digest failed %v %+v", err, failed)
			}
			raw, err := json.Marshal(digest)
			if err != nil || len(raw) > 8192 {
				t.Fatalf("map bytes=%d err=%v", len(raw), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d", digest.FilesParsed, digest.SymbolCount, len(raw))
			target := filepath.Join(root, filepath.FromSlash(tc.decl))
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{target}, Language: "flow", Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("find %s failed: %v %+v", tc.query, err, failed)
			}
			id := ""
			for _, s := range found.Symbols {
				if s.Name == tc.query && string(s.Kind) == "function" && s.DeclarationRange.Start.Line == tc.line {
					id = s.ID
					break
				}
			}
			if id == "" {
				t.Fatalf("original function %s:%d missing: %+v", tc.decl, tc.line, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: target, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "flow", Encoding: "utf-8", MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("fingerprint-bound excerpt missing %s: %v %+v", tc.query, err, failed)
			}
			failed, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: "flow", Encoding: "utf-8", Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) == 0 {
				t.Fatalf("structural search missing %s: %v %+v", tc.query, err, failed)
			}
			failed, other, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.distractor))}, Language: "flow", Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || failed != nil || len(other.Symbols) != 0 {
				t.Fatalf("fabricated unrelated symbol %s: %v %+v", tc.query, err, other.Symbols)
			}
		})
	}
}
