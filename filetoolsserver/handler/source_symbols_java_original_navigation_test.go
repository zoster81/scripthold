package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

// Original source and test fixtures are read-only; no third-party Java is run.
// Expectations follow independently inspected project sources, not tool output.
// Two independent original projects check that the opt-in overview/outline is
// useful against source-derived declarations, not just synthetic fixtures.
// Original sources are read, never built, executed or modified.
func TestSourceSymbolsConciseOriginalJavaProjects(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("private original project corpus not configured")
	}
	cases := []struct {
		project, relative, owner   string
		sourceOracles, wantMethods []string
	}{
		{
			project:  "project-java-greenrobot-eventbus",
			relative: "EventBus/src/org/greenrobot/eventbus/SubscriberMethodFinder.java",
			owner:    "SubscriberMethodFinder",
			sourceOracles: []string{
				"class SubscriberMethodFinder {",
				"SubscriberMethodFinder(List<SubscriberInfoIndex> subscriberInfoIndexes",
				"List<SubscriberMethod> findSubscriberMethods(Class<?> subscriberClass)",
				"private List<SubscriberMethod> findUsingInfo(Class<?> subscriberClass)",
			},
			wantMethods: []string{"SubscriberMethodFinder", "findSubscriberMethods", "findUsingInfo", "clearCaches"},
		},
		{
			project:  "project-java-thealgorithms-java",
			relative: "src/main/java/com/thealgorithms/ciphers/Caesar.java",
			owner:    "Caesar",
			sourceOracles: []string{
				"public class Caesar {",
				"private static int normalizeShift(final int shift)",
				"public String encode(String message, int shift)",
				"public String decode(String encryptedMessage, int shift)",
			},
			wantMethods: []string{"normalizeShift", "encode", "decode", "bruteforce"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.relative))
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, phrase := range tc.sourceOracles {
				if !strings.Contains(string(before), phrase) {
					t.Fatalf("original source oracle missing %q", phrase)
				}
			}
			h := NewHandler([]string{root})
			call := func(operation, view, owner string) SourceSymbolsOutput {
				t.Helper()
				failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: operation, Presentation: view, Paths: []string{path},
					Owner: owner, Language: "java", Encoding: "utf-8",
					MaxFiles: 1, MaxSymbols: 256,
				})
				if err != nil || failed != nil || !out.CoverageComplete || out.Truncated {
					t.Fatalf("%s %s: error=%v failed=%#v complete=%v truncated=%v", operation, view, err, failed, out.CoverageComplete, out.Truncated)
				}
				return out
			}
			digest := call("digest", "concise", "")
			if len(digest.ConciseFiles) != 1 || len(digest.ConciseFiles[0].Classes) == 0 {
				t.Fatalf("concise file map omitted real class: %#v", digest.ConciseFiles)
			}
			hasClass := false
			for _, name := range digest.ConciseFiles[0].Classes {
				if name == tc.owner {
					hasClass = true
				}
			}
			if !hasClass {
				t.Fatalf("real class %s missing: %#v", tc.owner, digest.ConciseFiles)
			}
			out := call("outline", "concise", tc.owner)
			if len(out.ConciseFiles) != 1 || len(out.ConciseFiles[0].Owners) != 1 {
				t.Fatalf("selected owner missing or duplicated: %#v", out.ConciseFiles)
			}
			class := out.ConciseFiles[0].Owners[0]
			if class.Name != tc.owner || len(class.Fields) != 0 {
				t.Fatalf("class mismatch or default fields leaked: %#v", class)
			}
			seen := make(map[string]bool)
			for _, member := range class.Methods {
				if member.Line <= 0 {
					t.Fatalf("member has no source line: %#v", member)
				}
				seen[member.Name] = true
			}
			for _, want := range tc.wantMethods {
				if !seen[want] {
					t.Fatalf("missing original method %s from %s: %#v", want, tc.owner, class.Methods)
				}
			}
			// Safe drilldown is checked against the original method in the same file,
			// without relying on a synthetic ID or repeated per-match fingerprints.
			target := tc.wantMethods[1]
			failed, located, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Presentation: "concise", Paths: []string{path},
				Query: target, Match: "exact", Language: "java", Encoding: "utf-8",
				MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || failed != nil || len(located.ConciseFiles) != 1 ||
				len(located.ConciseFiles[0].Matches) == 0 {
				t.Fatalf("concise find %s failed: err=%v result=%+v", target, err, located)
			}
			selected := located.ConciseFiles[0]
			symbolID := ""
			for _, match := range selected.Matches {
				if match.Name == target && match.Owner != "" {
					symbolID = match.SymbolID
					break
				}
			}
			if symbolID == "" || selected.SourceFingerprint == "" ||
				selected.Language != "java" || selected.Encoding != "utf-8" || selected.Path != path {
				t.Fatalf("unsafe concise find selection: %+v", selected)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: selected.Path,
				SymbolID: symbolID, SourceFingerprint: selected.SourceFingerprint,
				Language: selected.Language, Encoding: selected.Encoding, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, target) {
				t.Fatalf("fingerprint-bound original show failed for %s: err=%v", target, err)
			}
			// Qualify the query projection independently against the original
			// declaration, then drill down using its unchanged selector.
			queryArgs := SourceQueryInput{
				Operation: "search", Presentation: "concise", Mode: "structural",
				Paths: []string{path}, Query: target, Match: "exact",
				Language: "java", Encoding: "utf-8", MaxFiles: 1, MaxResults: 20,
			}
			failed, compactSearch, err := h.SourceQuery(context.Background(), nil, queryArgs)
			if err != nil || failed != nil || compactSearch.Search == nil || len(compactSearch.Search.Matches) == 0 {
				t.Fatalf("concise search %s failed: err=%v failed=%+v output=%+v", target, err, failed, compactSearch)
			}
			sourceHit := false
			for _, hit := range compactSearch.Search.Matches {
				if hit.Name == target && hit.Path == path &&
					hit.SourceFingerprint == selected.SourceFingerprint && hit.SymbolID == symbolID &&
					hit.Range.Start.Line > 0 && hit.Evidence == sourceintelligence.SymbolEvidenceStructural {
					sourceHit = true
				}
			}
			if !sourceHit {
				t.Fatalf("original target %s missing from index search: %+v", target, compactSearch.Search.Matches)
			}
			compactBytes, err := json.Marshal(compactSearch)
			if err != nil {
				t.Fatal(err)
			}
			var compactShape struct {
				Search struct {
					Files []struct {
						Path    string `json:"path"`
						Matches []struct {
							SymbolID string `json:"symbolId"`
						} `json:"matches"`
					} `json:"files"`
				} `json:"search"`
			}
			if err := json.Unmarshal(compactBytes, &compactShape); err != nil ||
				len(compactShape.Search.Files) != 1 || compactShape.Search.Files[0].Path != path {
				t.Fatalf("original query compact projection missing scoped file: %s err=%v", compactBytes, err)
			}
			queryArgs.Presentation = ""
			_, originalSearch, err := h.SourceQuery(context.Background(), nil, queryArgs)
			if err != nil || originalSearch.Search == nil {
				t.Fatalf("historical query failed: %v", err)
			}
			originalBytes, err := json.Marshal(originalSearch)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("source_query search bytes: concise=%d legacy=%d", len(compactBytes), len(originalBytes))

			failed, selectedBody, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Presentation: "concise", Part: "declaration",
				Path: selected.Path, SymbolID: symbolID, SourceFingerprint: selected.SourceFingerprint,
				Language: selected.Language, Encoding: selected.Encoding, MaxBytes: 8192,
			})
			if err != nil || failed != nil || selectedBody.ConciseShow == nil ||
				selectedBody.ConciseShow.Representation != "declaration" ||
				!selectedBody.ConciseShow.Complete ||
				!strings.Contains(selectedBody.ConciseShow.Code, target) {
				t.Fatalf("concise show did not retrieve original declaration: err=%v failed=%+v", err, failed)
			}
			// One explicit original method is an independently source-verified
			// context selection; no unrelated enclosing class is required.
			contextArgs := SourceQueryInput{
				Operation: "context", Presentation: "concise",
				Paths: []string{path}, Language: "java", Encoding: "utf-8",
				Targets: []SourceSelectorInput{{
					Kind: "symbol", Path: path, SymbolID: symbolID,
					SourceFingerprint: selected.SourceFingerprint,
				}},
				BudgetBytes: 4096, BodyPolicy: "signatures-only",
				MaxItems: 1, MaxDepth: 1, MaxFiles: 1,
			}
			failed, conciseContext, err := h.SourceQuery(context.Background(), nil, contextArgs)
			if err != nil || failed != nil || conciseContext.Context == nil ||
				len(conciseContext.Context.Items) != 1 {
				t.Fatalf("source-verified compact context: err=%v fail=%+v context=%+v",
					err, failed, conciseContext)
			}
			excerpt := conciseContext.Context.Items[0]
			if excerpt.Entity.Path != path || excerpt.Entity.SymbolID != symbolID ||
				excerpt.Entity.SourceFingerprint != selected.SourceFingerprint ||
				excerpt.Reason != sourceintelligence.ContextTarget ||
				excerpt.Representation != sourceintelligence.ContextSignature ||
				!strings.Contains(string(before), excerpt.Text) ||
				!strings.Contains(excerpt.Text, target) ||
				conciseContext.Context.UsedBytes != len(excerpt.Text) ||
				conciseContext.Context.UsedBytes > 4096 {
				t.Fatalf("original context not exact or correctly budgeted: %+v", conciseContext.Context)
			}
			conciseContextBytes, err := json.Marshal(conciseContext)
			if err != nil {
				t.Fatal(err)
			}
			var projected struct {
				Context struct {
					Items []struct {
						Entity         sourceintelligence.RelationEntity        `json:"entity"`
						Code           string                                   `json:"code"`
						Reason         sourceintelligence.ContextReason         `json:"reason"`
						Representation sourceintelligence.ContextRepresentation `json:"representation"`
					} `json:"items"`
					UsedBytes int `json:"usedBytes"`
				} `json:"context"`
			}
			if err := json.Unmarshal(conciseContextBytes, &projected); err != nil ||
				len(projected.Context.Items) != 1 ||
				projected.Context.Items[0].Code != excerpt.Text ||
				projected.Context.Items[0].Entity.SymbolID != symbolID ||
				projected.Context.UsedBytes != len(excerpt.Text) {
				t.Fatalf("original context projection lost exact source: %s err=%v", conciseContextBytes, err)
			}
			contextArgs.Presentation = ""
			failed, legacyContext, err := h.SourceQuery(context.Background(), nil, contextArgs)
			if err != nil || failed != nil || legacyContext.Context == nil ||
				len(legacyContext.Context.Items) != 1 ||
				legacyContext.Context.Items[0].Text != excerpt.Text {
				t.Fatalf("original context legacy divergence: err=%v fail=%+v", err, failed)
			}
			legacyContextBytes, err := json.Marshal(legacyContext)
			if err != nil {
				t.Fatal(err)
			}
			if len(conciseContextBytes) >= len(legacyContextBytes) {
				t.Fatalf("no context byte reduction concise=%d legacy=%d",
					len(conciseContextBytes), len(legacyContextBytes))
			}
			t.Logf("original context JSON bytes: concise=%d legacy=%d",
				len(conciseContextBytes), len(legacyContextBytes))
			concisely, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range []string{`"symbolId"`, `"sourceFingerprint"`, `"filesConsidered"`} {
				if strings.Contains(string(concisely), forbidden) {
					t.Fatalf("concise payload still includes %s", forbidden)
				}
			}
			legacy := call("outline", "tree", "")
			legacyBytes, err := json.Marshal(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if len(concisely) >= len(legacyBytes) {
				t.Fatalf("no response reduction: concise=%d tree=%d", len(concisely), len(legacyBytes))
			}
			t.Logf("selected overview and owner verified, concise=%d legacyTree=%d bytes", len(concisely), len(legacyBytes))
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("original source changed during read-only qualification")
			}
		})
	}
}

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
