package filetoolsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Original third-party projects are inspected as read-only source fixtures.
// This test intentionally compares the public MCP responses, not just the
// handler structs. Missing configured fixtures are setup failures, not skips.
func TestSourceResponseConciseOriginalJavaPublicMCP(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Java corpus not configured")
	}
	cases := []struct {
		project, source, test, distractor    string
		owner, target, signature, testOracle string
		line                                 int
		methods                              []string
	}{
		{
			project:    "project-java-greenrobot-eventbus",
			source:     "EventBus/src/org/greenrobot/eventbus/SubscriberMethodFinder.java",
			test:       "EventBusTestJava/src/main/java/org/greenrobot/eventbus/EventBusSubscriberLegalTest.java",
			distractor: "EventBus/src/org/greenrobot/eventbus/EventBusBuilder.java",
			owner:      "SubscriberMethodFinder", target: "findSubscriberMethods",
			signature:  "List<SubscriberMethod> findSubscriberMethods(Class<?> subscriberClass)",
			testOracle: "public void testSubscriberLegal()", line: 55,
			methods: []string{"SubscriberMethodFinder", "findSubscriberMethods", "findUsingInfo", "clearCaches"},
		},
		{
			project:    "project-java-thealgorithms-java",
			source:     "src/main/java/com/thealgorithms/ciphers/Caesar.java",
			test:       "src/test/java/com/thealgorithms/ciphers/CaesarTest.java",
			distractor: "src/main/java/com/thealgorithms/ciphers/AtbashCipher.java",
			owner:      "Caesar", target: "normalizeShift",
			signature:  "private static int normalizeShift(final int shift)",
			testOracle: "void caesarEncryptWithNegativeShiftTest()", line: 12,
			methods: []string{"normalizeShift", "encode", "decode", "bruteforce"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, 3)
			original := make(map[string][]byte, 3)
			for _, relative := range []string{tc.source, tc.test, tc.distractor} {
				path := filepath.Join(root, filepath.FromSlash(relative))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("configured original source missing or not regular %s: %v", relative, err)
				}
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
				original[path] = content
			}
			mainPath := paths[0]
			if !bytes.Contains(original[mainPath], []byte(tc.signature)) ||
				!bytes.Contains(original[paths[1]], []byte(tc.testOracle)) {
				t.Fatal("independent original production/test oracle changed")
			}
			if bytes.Contains(original[paths[2]], []byte(tc.signature)) {
				t.Fatal("the selected distractor now contains the production declaration")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "original-java-response-qualification")
			compactTotal, legacyTotal := 0, 0
			call := func(name string, args map[string]any) (map[string]any, int) {
				t.Helper()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
				if err != nil || result == nil || result.IsError {
					t.Fatalf("MCP %s %s failed: result=%#v err=%v", name, args["operation"], result, err)
				}
				payload := contractStructuredMap(t, result)
				data, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				parity := false
				for _, content := range result.Content {
					if item, ok := content.(*mcp.TextContent); ok {
						var fallback map[string]any
						if json.Unmarshal([]byte(item.Text), &fallback) == nil && reflect.DeepEqual(fallback, payload) {
							parity = true
						}
					}
				}
				if !parity {
					t.Fatalf("MCP %s %s text/structured projections differ", name, args["operation"])
				}
				return payload, len(data)
			}
			pair := func(name string, concise, legacy map[string]any) map[string]any {
				t.Helper()
				current, n := call(name, concise)
				old, oldN := call(name, legacy)
				if concise["operation"] == "relations" {
					var fresh, previous []map[string]any
					if value := contractSchemaMap(t, current["relations"])["edges"]; value != nil {
						fresh = contractObjectSlice(t, value)
					}
					if value := contractSchemaMap(t, old["relations"])["relations"]; value != nil {
						previous = contractObjectSlice(t, value)
					}
					if len(fresh) != len(previous) {
						t.Fatalf("relation count changed in concise response: %d vs %d", len(fresh), len(previous))
					}
					for i, edge := range fresh {
						for _, field := range []string{"source", "target", "evidence", "resolution"} {
							if !reflect.DeepEqual(edge[field], previous[i][field]) {
								t.Fatalf("relation %d lost %s: concise=%#v legacy=%#v", i, field, edge, previous[i])
							}
						}
						kind := edge["kind"]
						if kind == nil {
							kind = concise["relation"]
						}
						if kind != previous[i]["kind"] {
							t.Fatalf("relation %d changed kind: concise=%#v legacy=%#v", i, kind, previous[i]["kind"])
						}
					}
					if tc.project == "project-java-greenrobot-eventbus" && len(fresh) == 0 {
						t.Fatal("original project cross-file dependency evidence unexpectedly absent")
					}
				}
				compactTotal += n
				legacyTotal += oldN
				t.Logf("%s: concise=%d legacy=%d SDK structured JSON bytes", concise["operation"], n, oldN)
				return current
			}
			digest := pair("source_symbols",
				map[string]any{"operation": "digest", "presentation": "concise", "paths": paths, "language": "java", "encoding": "utf-8", "maxFiles": 3},
				map[string]any{"operation": "digest", "paths": paths, "language": "java", "encoding": "utf-8", "maxFiles": 3})
			foundClass := false
			for _, f := range contractObjectSlice(t, digest["files"]) {
				for _, name := range contractStringSlice(t, f["classes"]) {
					foundClass = foundClass || name == tc.owner
				}
				if f["sourceFingerprint"] != nil || f["fields"] != nil {
					t.Fatalf("overview leaked internal detail: %#v", f)
				}
			}
			if !foundClass {
				t.Fatalf("source-backed owner %s absent from digest", tc.owner)
			}

			outline := pair("source_symbols",
				map[string]any{"operation": "outline", "presentation": "concise", "paths": []string{mainPath}, "owner": tc.owner, "language": "java", "encoding": "utf-8", "maxFiles": 1, "maxSymbols": 256},
				map[string]any{"operation": "outline", "paths": []string{mainPath}, "language": "java", "encoding": "utf-8", "maxFiles": 1, "maxSymbols": 256})
			owners := contractObjectSlice(t, contractObjectSlice(t, outline["files"])[0]["owners"])
			if len(owners) != 1 || owners[0]["name"] != tc.owner || owners[0]["fields"] != nil {
				t.Fatalf("concise owner mismatch or default fields present: %#v", owners)
			}
			methodNames := make(map[string]bool)
			for _, member := range contractObjectSlice(t, owners[0]["methods"]) {
				methodNames[fmt.Sprint(member["name"])] = true
			}
			for _, want := range tc.methods {
				if !methodNames[want] {
					t.Fatalf("method %s omitted from original-source owner %s", want, tc.owner)
				}
			}

			findArgs := map[string]any{"operation": "find", "paths": []string{mainPath}, "query": tc.target,
				"match": "exact", "language": "java", "encoding": "utf-8", "maxFiles": 1, "maxSymbols": 16}
			findCompact := make(map[string]any, len(findArgs)+1)
			for k, v := range findArgs {
				findCompact[k] = v
			}
			findCompact["presentation"] = "concise"
			find := pair("source_symbols", findCompact, findArgs)
			f := contractObjectSlice(t, find["files"])
			if len(f) != 1 || f[0]["path"] != mainPath || f[0]["language"] != "java" || f[0]["encoding"] != "utf-8" {
				t.Fatalf("unsafe selected original source file: %#v", f)
			}
			fingerprint, _ := f[0]["sourceFingerprint"].(string)
			var symbolID string
			for _, match := range contractObjectSlice(t, f[0]["matches"]) {
				if match["name"] == tc.target && match["line"] == float64(tc.line) {
					symbolID, _ = match["symbolId"].(string)
				}
			}
			if len(symbolID) != 64 || len(fingerprint) != 64 {
				t.Fatalf("safe method selection missing at source line %d: %#v", tc.line, f[0])
			}

			showArgs := map[string]any{"operation": "show", "path": mainPath, "symbolId": symbolID,
				"sourceFingerprint": fingerprint, "language": "java", "encoding": "utf-8", "maxBytes": 8192}
			showCompact := make(map[string]any, len(showArgs)+2)
			for k, v := range showArgs {
				showCompact[k] = v
			}
			showCompact["presentation"] = "concise"
			showCompact["part"] = "declaration"
			shown := pair("source_symbols", showCompact, showArgs)
			code, _ := shown["code"].(string)
			if shown["file"] != mainPath || shown["representation"] != "declaration" ||
				!strings.Contains(code, tc.signature) || !bytes.Contains(original[mainPath], []byte(code)) {
				t.Fatalf("declaration differs from original source: %#v", shown)
			}

			searchArgs := map[string]any{"operation": "search", "paths": paths, "query": tc.target,
				"mode": "structural", "match": "exact", "language": "java", "encoding": "utf-8",
				"maxFiles": 3, "maxResults": 16}
			searchCompact := make(map[string]any, len(searchArgs)+1)
			for k, v := range searchArgs {
				searchCompact[k] = v
			}
			searchCompact["presentation"] = "concise"
			search := pair("source_query", searchCompact, searchArgs)
			hit := false
			for _, file := range contractObjectSlice(t, contractSchemaMap(t, search["search"])["files"]) {
				for _, match := range contractObjectSlice(t, file["matches"]) {
					if file["path"] == mainPath && match["symbolId"] == symbolID &&
						strings.HasSuffix(fmt.Sprint(match["symbol"]), "."+tc.target) {
						hit = true
					}
					if file["path"] == paths[2] && match["symbolId"] != nil {
						t.Fatalf("unrelated original file falsely declares %s: %#v", tc.target, match)
					}
				}
			}
			if !hit {
				t.Fatalf("structural search did not preserve a source-verifiable target: %#v", search["search"])
			}

			relationsArgs := map[string]any{"operation": "relations", "paths": paths, "relation": "dependencies",
				"subject":  map[string]any{"kind": "path", "path": mainPath, "sourceFingerprint": fingerprint},
				"language": "java", "encoding": "utf-8", "maxFiles": 3, "maxResults": 16,
				"maxNodes": 32, "maxEdges": 64, "maxDepth": 3}
			relationsCompact := make(map[string]any, len(relationsArgs)+1)
			for k, v := range relationsArgs {
				relationsCompact[k] = v
			}
			relationsCompact["presentation"] = "concise"
			relations := pair("source_query", relationsCompact, relationsArgs)
			relationPayload := contractSchemaMap(t, relations["relations"])
			if relationPayload["relation"] != "dependencies" || relationPayload["evidenceScope"] != "indexed-relations-only" {
				t.Fatalf("concise relations lost indexed-evidence boundaries: %#v", relationPayload)
			}
			for _, edge := range contractObjectSlice(t, relationPayload["edges"]) {
				if edge["evidence"] == nil || edge["resolution"] == nil {
					t.Fatalf("unqualified link claimed as a relation: %#v", edge)
				}
			}

			contextArgs := map[string]any{"operation": "context", "paths": paths, "language": "java",
				"encoding": "utf-8", "targets": []any{map[string]any{
					"kind": "symbol", "path": mainPath, "symbolId": symbolID, "sourceFingerprint": fingerprint,
				}}, "budgetBytes": 4096, "bodyPolicy": "signatures-only", "maxItems": 1, "maxDepth": 1, "maxFiles": 3}
			contextCompact := make(map[string]any, len(contextArgs)+1)
			for k, v := range contextArgs {
				contextCompact[k] = v
			}
			contextCompact["presentation"] = "concise"
			context := pair("source_query", contextCompact, contextArgs)
			items := contractObjectSlice(t, contractSchemaMap(t, context["context"])["items"])
			if len(items) != 1 || items[0]["representation"] != "signature" ||
				!bytes.Contains(original[mainPath], []byte(fmt.Sprint(items[0]["code"]))) {
				t.Fatalf("selected context is not exact original source: %#v", items)
			}

			if compactTotal >= legacyTotal {
				t.Fatalf("seven concise operations did not reduce aggregate SDK JSON: concise=%d legacy=%d",
					compactTotal, legacyTotal)
			}
			for path, before := range original {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("original third-party fixture changed during read-only test %s: %v", path, err)
				}
			}
			t.Logf("seven public MCP operations: concise=%d legacy=%d SDK structured JSON bytes", compactTotal, legacyTotal)
		})
	}
}
