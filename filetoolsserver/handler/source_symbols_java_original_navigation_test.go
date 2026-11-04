package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original source and test fixtures are read-only; no third-party Java is run.
// Expectations follow independently inspected project sources, not tool output.
func TestSourceSymbolsOriginalJavaTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Java corpus not configured")
	}
	type expected struct {
		path, name string
		line       int
	}
	cases := []struct {
		project, query, distractor string
		files, oracle              []string
		symbols                    []expected
	}{
		{
			project: "project-java-greenrobot-eventbus",
			files: []string{
				"EventBus/src/org/greenrobot/eventbus/SubscriberMethodFinder.java",
				"EventBus/src/org/greenrobot/eventbus/EventBus.java",
				"EventBusTestJava/src/main/java/org/greenrobot/eventbus/EventBusSubscriberLegalTest.java",
				"EventBus/src/org/greenrobot/eventbus/EventBusBuilder.java",
			},
			oracle: []string{
				"List<SubscriberMethod> findSubscriberMethods(Class<?> subscriberClass)",
				"and its super classes have no public methods with the @Subscribe annotation",
				"subscriberMethodFinder.findSubscriberMethods(subscriberClass)",
				"public void testSubscriberLegal()",
				"boolean strictMethodVerification;",
			},
			symbols: []expected{
				{"EventBus/src/org/greenrobot/eventbus/SubscriberMethodFinder.java", "findSubscriberMethods", 55},
				{"EventBus/src/org/greenrobot/eventbus/EventBus.java", "register", 142},
				{"EventBusTestJava/src/main/java/org/greenrobot/eventbus/EventBusSubscriberLegalTest.java", "testSubscriberLegal", 28},
			},
			query:      "findSubscriberMethods",
			distractor: "EventBus/src/org/greenrobot/eventbus/EventBusBuilder.java",
		},
		{
			project: "project-java-thealgorithms-java",
			files: []string{
				"src/main/java/com/thealgorithms/ciphers/Caesar.java",
				"src/test/java/com/thealgorithms/ciphers/CaesarTest.java",
				"src/main/java/com/thealgorithms/ciphers/AtbashCipher.java",
			},
			oracle: []string{
				"private static int normalizeShift(final int shift)",
				"return ((shift % 26) + 26) % 26;",
				"public String encode(String message, int shift)",
				"public String decode(String encryptedMessage, int shift)",
				"void caesarEncryptWithNegativeShiftTest()",
				"new int[] {-1, -5, -25, -26, -27, -52}",
				"public String convert()",
			},
			symbols: []expected{
				{"src/main/java/com/thealgorithms/ciphers/Caesar.java", "normalizeShift", 12},
				{"src/main/java/com/thealgorithms/ciphers/Caesar.java", "encode", 22},
				{"src/main/java/com/thealgorithms/ciphers/Caesar.java", "decode", 50},
			},
			query:      "normalizeShift",
			distractor: "src/main/java/com/thealgorithms/ciphers/AtbashCipher.java",
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			var original strings.Builder
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original file %s unavailable: %v", rel, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				original.Write(raw)
				paths = append(paths, path)
			}
			for _, phrase := range tc.oracle {
				if !strings.Contains(original.String(), phrase) {
					t.Fatalf("original source/test oracle changed: %q", phrase)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "java", Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete ||
				digest.FilesParsed != len(paths) {
				t.Fatalf("digest error=%v failed=%+v result=%+v", err, failed, digest)
			}
			compact, err := json.Marshal(digest)
			if err != nil || len(compact) > 8192 {
				t.Fatalf("compact map size %d err=%v", len(compact), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d", digest.FilesParsed, digest.SymbolCount, len(compact))
			for _, want := range tc.symbols {
				target := filepath.Join(root, filepath.FromSlash(want.path))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{target}, Query: want.name, Match: "exact",
					Language: "java", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 10,
				})
				if err != nil || failed != nil || found.Truncated || !found.CoverageComplete || len(found.Files) != 1 {
					t.Fatalf("find %s: %v failed=%+v", want.name, err, failed)
				}
				id := ""
				for _, sym := range found.Symbols {
					if sym.Name == want.name && sym.NameRange.Start.Line == want.line &&
						sym.DeclarationRange.Start.Line <= want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing source-derived %s at %s:%d: %+v", want.name, want.path, want.line, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: target, SymbolID: id,
					SourceFingerprint: found.Files[0].SourceFingerprint,
					Language:          "java", Encoding: "utf-8", MaxBytes: 16384,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.name) {
					t.Fatalf("fingerprint-bound excerpt for %s failed: %v failed=%+v", want.name, err, failed)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Language: "java", Encoding: "utf-8",
				Query: tc.query, Match: "exact", Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s: %v failed=%+v", tc.query, err, failed)
			}
			failed, other, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find",
				Paths:     []string{filepath.Join(root, filepath.FromSlash(tc.distractor))},
				Query:     tc.query, Match: "exact", Language: "java", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 5,
			})
			if err != nil || failed != nil || other.Truncated || len(other.Symbols) != 0 {
				t.Fatalf("unrelated %s declares %s: %v failed=%+v symbols=%+v", tc.distractor, tc.query, err, failed, other.Symbols)
			}
		})
	}
}
