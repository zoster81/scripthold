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

// Inspect the pinned original projects through the public MCP SDK client.
// Third-party source is only read, never imported, compiled or executed.
func TestSourceResponseConciseOriginalPythonPublicMCP(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Python corpus not configured")
	}
	cases := []struct {
		project, source, testFile, distractor, neighbor                                  string
		task, owner, target, signature, testOracle, additionalTestOracle, neighborOracle string
		line                                                                             int
		functions, classes, methods                                                      []string
	}{
		{
			project:    "python-click",
			source:     "src/click/formatting.py",
			testFile:   "tests/test_formatting.py",
			distractor: "src/click/exceptions.py",
			neighbor:   "src/click/__init__.py",
			task:       "investigate wrapped CLI help text and paragraph preservation",
			owner:      "HelpFormatter", target: "wrap_text",
			signature: "def wrap_text(", testOracle: "def test_basic_functionality(runner):",
			additionalTestOracle: "def test_wrapping_long_options_strings(runner):",
			neighborOracle:       "from .formatting import wrap_text as wrap_text",
			line:                 31,
			functions:            []string{"measure_table", "iter_rows", "wrap_text"},
			classes:              []string{"HelpFormatter"},
			methods:              []string{"__init__", "write", "indent", "dedent"},
		},
		{
			project:    "python-httpx",
			source:     "httpx/_decoders.py",
			testFile:   "tests/test_decoders.py",
			distractor: "httpx/_multipart.py",
			neighbor:   "httpx/_models.py",
			task:       "investigate raw deflate fallback and HTTP response decompression",
			owner:      "DeflateDecoder", target: "DeflateDecoder",
			signature:            "class DeflateDecoder(ContentDecoder):",
			testOracle:           "def test_deflate():",
			additionalTestOracle: "def test_zlib():",
			neighborOracle:       "def _get_content_decoder(self) -> ContentDecoder:",
			line:                 56,
			classes:              []string{"ContentDecoder", "IdentityDecoder", "DeflateDecoder", "GZipDecoder"},
			methods:              []string{"__init__", "decode", "flush"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			relatives := []string{tc.source, tc.testFile, tc.distractor, tc.neighbor}
			paths := make([]string, 0, len(relatives))
			sourceBytes := make(map[string][]byte, len(relatives))
			for _, rel := range relatives {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("enabled original project source missing or nonregular: %s: %v", rel, err)
				}
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
				sourceBytes[path] = content
			}
			mainPath := paths[0]
			if !bytes.Contains(sourceBytes[mainPath], []byte(tc.signature)) ||
				!bytes.Contains(sourceBytes[paths[1]], []byte(tc.testOracle)) ||
				!bytes.Contains(sourceBytes[paths[1]], []byte(tc.additionalTestOracle)) ||
				!bytes.Contains(sourceBytes[paths[3]], []byte(tc.neighborOracle)) ||
				bytes.Contains(sourceBytes[paths[2]], []byte(tc.signature)) {
				t.Fatal("original production/test/neighbor/distractor oracle changed")
			}
			if tc.project == "python-httpx" {
				for _, evidence := range []string{
					"def decode(self, data: bytes) -> bytes:",
					"zlib.decompressobj(-zlib.MAX_WBITS)",
					"class GZipDecoder(ContentDecoder):",
				} {
					if !bytes.Contains(sourceBytes[mainPath], []byte(evidence)) {
						t.Fatalf("independent deflate behavior oracle missing %q", evidence)
					}
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "original-python-response-qualification")
			compactTotal, legacyTotal := 0, 0
			call := func(tool string, args map[string]any) (map[string]any, int) {
				t.Helper()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
				if err != nil || result == nil || result.IsError {
					t.Fatalf("MCP %s %s error: result=%#v err=%v", tool, args["operation"], result, err)
				}
				payload := contractStructuredMap(t, result)
				raw, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				parity := false
				for _, item := range result.Content {
					if txt, ok := item.(*mcp.TextContent); ok {
						var fallback map[string]any
						if json.Unmarshal([]byte(txt.Text), &fallback) == nil &&
							reflect.DeepEqual(fallback, payload) {
							parity = true
						}
					}
				}
				if !parity {
					t.Fatalf("MCP %s %s structured/text response differs", tool, args["operation"])
				}
				return payload, len(raw)
			}
			paired := func(tool string, concise, legacy map[string]any) map[string]any {
				t.Helper()
				current, bytesCompact := call(tool, concise)
				previous, bytesLegacy := call(tool, legacy)
				if concise["operation"] == "relations" {
					var edges, oldEdges []map[string]any
					if v := contractSchemaMap(t, current["relations"])["edges"]; v != nil {
						edges = contractObjectSlice(t, v)
					}
					if v := contractSchemaMap(t, previous["relations"])["relations"]; v != nil {
						oldEdges = contractObjectSlice(t, v)
					}
					if len(edges) != len(oldEdges) {
						t.Fatalf("indexed relation count changed: %d vs %d", len(edges), len(oldEdges))
					}
					for i, edge := range edges {
						for _, k := range []string{"source", "target", "evidence", "resolution"} {
							if !reflect.DeepEqual(edge[k], oldEdges[i][k]) {
								t.Fatalf("indexed relation %d lost %s", i, k)
							}
						}
						kind := edge["kind"]
						if kind == nil {
							kind = concise["relation"]
						}
						if kind != oldEdges[i]["kind"] {
							t.Fatalf("indexed relation %d kind changed", i)
						}
					}
				}
				compactTotal += bytesCompact
				legacyTotal += bytesLegacy
				t.Logf("%s concise=%d legacy=%d SDK structured JSON bytes",
					concise["operation"], bytesCompact, bytesLegacy)
				return current
			}
			base := map[string]any{"paths": paths, "language": "python", "encoding": "utf-8", "maxFiles": len(paths)}
			makeArgs := func(op string, fields map[string]any) map[string]any {
				args := make(map[string]any, len(base)+len(fields)+1)
				for k, v := range base {
					args[k] = v
				}
				args["operation"] = op
				for k, v := range fields {
					args[k] = v
				}
				return args
			}
			compact := func(args map[string]any) map[string]any {
				out := make(map[string]any, len(args)+1)
				for k, v := range args {
					out[k] = v
				}
				out["presentation"] = "concise"
				return out
			}
			digestArgs := makeArgs("digest", nil)
			digest := paired("source_symbols", compact(digestArgs), digestArgs)
			names := make(map[string]bool)
			owners := make(map[string]bool)
			for _, f := range contractObjectSlice(t, digest["files"]) {
				if f["sourceFingerprint"] != nil || f["fields"] != nil || f["symbolId"] != nil {
					t.Fatalf("overview leaked unwanted source navigation details: %#v", f)
				}
				if functions := f["functions"]; functions != nil {
					for _, name := range contractStringSlice(t, functions) {
						names[name] = true
					}
				}
				if classes := f["classes"]; classes != nil {
					for _, name := range contractStringSlice(t, classes) {
						owners[name] = true
					}
				}
			}
			for _, name := range tc.functions {
				if !names[name] {
					t.Fatalf("source-known top-level Python function omitted from digest: %s", name)
				}
			}
			for _, name := range tc.classes {
				if !owners[name] {
					t.Fatalf("source-known Python class omitted from digest: %s", name)
				}
			}
			outlineArgs := makeArgs("outline", map[string]any{
				"paths": []string{mainPath}, "owner": tc.owner, "maxFiles": 1, "maxSymbols": 256,
			})
			outline := paired("source_symbols", compact(outlineArgs),
				makeArgs("outline", map[string]any{"paths": []string{mainPath}, "maxFiles": 1, "maxSymbols": 256}))
			outlineFiles := contractObjectSlice(t, outline["files"])
			if len(outlineFiles) != 1 {
				t.Fatalf("selected original owner not found in outline: %#v", outlineFiles)
			}
			outlineOwners := contractObjectSlice(t, outlineFiles[0]["owners"])
			if len(outlineOwners) != 1 || outlineOwners[0]["name"] != tc.owner ||
				outlineOwners[0]["fields"] != nil {
				t.Fatalf("owner mismatch or default fields in concise Python outline: %#v", outlineOwners)
			}
			seenMethods := make(map[string]bool)
			for _, member := range contractObjectSlice(t, outlineOwners[0]["methods"]) {
				seenMethods[fmt.Sprint(member["name"])] = true
			}
			for _, want := range tc.methods {
				if !seenMethods[want] {
					t.Fatalf("source-known Python method %s omitted from %s", want, tc.owner)
				}
			}
			findArgs := makeArgs("find", map[string]any{
				"paths": []string{mainPath}, "query": tc.target, "match": "exact",
				"maxFiles": 1, "maxSymbols": 32,
			})
			found := paired("source_symbols", compact(findArgs), findArgs)
			foundFiles := contractObjectSlice(t, found["files"])
			if len(foundFiles) != 1 || foundFiles[0]["path"] != mainPath ||
				foundFiles[0]["language"] != "python" || foundFiles[0]["encoding"] != "utf-8" {
				t.Fatalf("find selected incorrect original Python file: %#v", foundFiles)
			}
			fingerprint, _ := foundFiles[0]["sourceFingerprint"].(string)
			symbolID := ""
			for _, match := range contractObjectSlice(t, foundFiles[0]["matches"]) {
				if match["name"] == tc.target && match["line"] == float64(tc.line) {
					symbolID, _ = match["symbolId"].(string)
				}
			}
			if len(symbolID) != 64 || len(fingerprint) != 64 {
				t.Fatalf("missing safe exact selection for %s:%d: %#v", tc.target, tc.line, foundFiles)
			}
			showArgs := map[string]any{"operation": "show", "path": mainPath,
				"symbolId": symbolID, "sourceFingerprint": fingerprint,
				"language": "python", "encoding": "utf-8", "maxBytes": 8192,
			}
			showCompact := compact(showArgs)
			showCompact["part"] = "declaration"
			shown := paired("source_symbols", showCompact, showArgs)
			code, _ := shown["code"].(string)
			if shown["file"] != mainPath || shown["representation"] != "declaration" ||
				!strings.Contains(code, tc.signature) ||
				!bytes.Contains(sourceBytes[mainPath], []byte(code)) {
				t.Fatalf("show differs from original source declaration: %#v", shown)
			}
			searchArgs := makeArgs("search", map[string]any{
				"query": tc.target, "mode": "structural", "match": "exact", "maxResults": 32,
			})
			search := paired("source_query", compact(searchArgs), searchArgs)
			safeSearch := false
			for _, file := range contractObjectSlice(t, contractSchemaMap(t, search["search"])["files"]) {
				for _, match := range contractObjectSlice(t, file["matches"]) {
					if file["path"] == mainPath && match["symbolId"] == symbolID &&
						strings.HasSuffix(fmt.Sprint(match["symbol"]), tc.target) {
						safeSearch = true
					}
					if file["path"] == paths[2] && match["symbolId"] != nil {
						t.Fatalf("unrelated source falsely declared selected target: %#v", match)
					}
				}
			}
			if !safeSearch {
				t.Fatalf("source_query search omitted source-proven selector: %#v", search["search"])
			}
			relationArgs := makeArgs("relations", map[string]any{
				"relation": "dependencies", "subject": map[string]any{
					"kind": "path", "path": mainPath, "sourceFingerprint": fingerprint,
				}, "maxResults": 32, "maxNodes": 32, "maxEdges": 64, "maxDepth": 3,
			})
			relations := paired("source_query", compact(relationArgs), relationArgs)
			rel := contractSchemaMap(t, relations["relations"])
			if rel["relation"] != "dependencies" || rel["evidenceScope"] != "indexed-relations-only" {
				t.Fatalf("relations lost declared evidence scope: %#v", rel)
			}
			for _, edge := range contractObjectSlice(t, rel["edges"]) {
				if edge["evidence"] == nil || edge["resolution"] == nil {
					t.Fatalf("indexed relation missing evidence/resolution: %#v", edge)
				}
			}
			contextArgs := makeArgs("context", map[string]any{
				"targets": []any{map[string]any{"kind": "symbol", "path": mainPath,
					"symbolId": symbolID, "sourceFingerprint": fingerprint}},
				"budgetBytes": 4096, "bodyPolicy": "signatures-only",
				"maxItems": 1, "maxDepth": 1,
			})
			selected := paired("source_query", compact(contextArgs), contextArgs)
			items := contractObjectSlice(t, contractSchemaMap(t, selected["context"])["items"])
			if len(items) != 1 {
				t.Fatalf("context did not select one original symbol: %#v", items)
			}
			contextCode, ok := items[0]["code"].(string)
			if !ok || contextCode == "" || items[0]["representation"] != "signature" ||
				!bytes.Contains(sourceBytes[mainPath], []byte(contextCode)) {
				t.Fatalf("context excerpt not exact original source: %#v", items)
			}

			// HTTPX has many distinct decode methods. Prove that an unqualified
			// method-name query retains owner/line identity before exact drilldown.
			if tc.project == "python-httpx" {
				methodArgs := makeArgs("find", map[string]any{
					"paths": []string{mainPath}, "query": "decode", "match": "exact",
					"maxFiles": 1, "maxSymbols": 64,
				})
				methodFind, _ := call("source_symbols", compact(methodArgs))
				matches := contractObjectSlice(t, contractObjectSlice(t, methodFind["files"])[0]["matches"])
				targetID, conflicting := "", false
				for _, item := range matches {
					if item["line"] == float64(67) &&
						strings.Contains(fmt.Sprint(item["owner"]), "DeflateDecoder") {
						targetID, _ = item["symbolId"].(string)
					}
					if item["line"] == float64(95) &&
						strings.Contains(fmt.Sprint(item["owner"]), "GZipDecoder") {
						conflicting = true
					}
				}
				if len(targetID) != 64 || !conflicting {
					t.Fatalf("ambiguous class-owned Python method selection collapsed: %#v", matches)
				}
				decodeArgs := map[string]any{"operation": "show", "presentation": "concise",
					"path": mainPath, "symbolId": targetID,
					"sourceFingerprint": fingerprint, "language": "python",
					"encoding": "utf-8", "part": "declaration", "maxBytes": 2048,
				}
				decoded, _ := call("source_symbols", decodeArgs)
				sourceCode := fmt.Sprint(decoded["code"])
				if decoded["representation"] != "declaration" ||
					!strings.Contains(sourceCode, "def decode(self, data: bytes) -> bytes:") ||
					!bytes.Contains(sourceBytes[mainPath], []byte(sourceCode)) {
					t.Fatalf("wrong original DeflateDecoder method selected: %#v", decoded)
				}
			}
			if compactTotal >= legacyTotal {
				t.Fatalf("aggregate of seven concise operations not smaller: %d vs %d SDK JSON bytes",
					compactTotal, legacyTotal)
			}
			for path, before := range sourceBytes {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(after, before) {
					t.Fatalf("read-only MCP qualification modified original source %s: %v", path, err)
				}
			}
			t.Logf("%s: seven public MCP operations concise=%d legacy=%d SDK structured JSON bytes",
				tc.task, compactTotal, legacyTotal)
		})
	}
}
