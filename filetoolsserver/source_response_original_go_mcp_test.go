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

// Original projects supply independent source oracles; no third-party Go code
// is compiled, executed, modified, or treated as a trusted test expectation.
func TestSourceResponseConciseOriginalGoPublicMCP(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Go source corpus not configured")
	}
	cases := []struct {
		project, primary, testFile, distractor, neighbor    string
		task, target, signature, testOracle, neighborOracle string
		line                                                int
		functions                                           []string
	}{
		{
			project: "go-cobra", primary: "flag_groups.go", testFile: "flag_groups_test.go",
			distractor: "args.go", neighbor: "command.go",
			task:           "investigate mutual exclusion flag-group validation",
			target:         "validateExclusiveFlagGroups",
			signature:      "func validateExclusiveFlagGroups(data map[string]map[string]bool) error {",
			testOracle:     "func TestValidateFlagGroups(t *testing.T) {",
			neighborOracle: "if err := c.ValidateFlagGroups(); err != nil {",
			line:           188,
			functions:      []string{"hasAllFlags", "processFlagForGroupAnnotation", "validateRequiredFlagGroups", "validateExclusiveFlagGroups"},
		},
		{
			project: "go-chi", primary: "middleware/recoverer.go", testFile: "middleware/recoverer_test.go",
			distractor: "middleware/request_id.go",
			task:       "investigate panic recovery and HTTP 500 handling",
			target:     "Recoverer", signature: "func Recoverer(next http.Handler) http.Handler {",
			testOracle: "func TestRecoverer(t *testing.T) {",
			line:       22,
			functions:  []string{"Recoverer", "PrintPrettyStack", "printPrettyStack"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			relatives := []string{tc.primary, tc.testFile, tc.distractor}
			if tc.neighbor != "" {
				relatives = append(relatives, tc.neighbor)
			}
			paths := make([]string, 0, len(relatives))
			original := make(map[string][]byte, len(relatives))
			for _, relative := range relatives {
				path := filepath.Join(root, filepath.FromSlash(relative))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("enabled corpus missing regular original file %s: %v", relative, err)
				}
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				paths, original[path] = append(paths, path), content
			}
			mainPath := paths[0]
			if !bytes.Contains(original[mainPath], []byte(tc.signature)) ||
				!bytes.Contains(original[paths[1]], []byte(tc.testOracle)) ||
				bytes.Contains(original[paths[2]], []byte(tc.signature)) {
				t.Fatal("source/test/distractor oracle changed before MCP inspection")
			}
			if tc.neighbor != "" && !bytes.Contains(original[paths[3]], []byte(tc.neighborOracle)) {
				t.Fatal("independently observed direct caller no longer exists")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "original-go-response-qualification")
			conciseTotal, legacyTotal := 0, 0
			call := func(tool string, args map[string]any) (map[string]any, int) {
				t.Helper()
				result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
				if err != nil || result == nil || result.IsError {
					t.Fatalf("%s %s failed: err=%v result=%#v", tool, args["operation"], err, result)
				}
				got := contractStructuredMap(t, result)
				data, err := json.Marshal(got)
				if err != nil {
					t.Fatal(err)
				}
				same := false
				for _, content := range result.Content {
					if text, ok := content.(*mcp.TextContent); ok {
						var decoded map[string]any
						if json.Unmarshal([]byte(text.Text), &decoded) == nil && reflect.DeepEqual(decoded, got) {
							same = true
						}
					}
				}
				if !same {
					t.Fatalf("%s %s changed MCP text fallback versus structured content", tool, args["operation"])
				}
				return got, len(data)
			}
			pair := func(tool string, optin, originalArgs map[string]any) map[string]any {
				t.Helper()
				compact, shortBytes := call(tool, optin)
				legacy, longBytes := call(tool, originalArgs)
				if optin["operation"] == "relations" {
					var conciseEdges, legacyEdges []map[string]any
					if raw := contractSchemaMap(t, compact["relations"])["edges"]; raw != nil {
						conciseEdges = contractObjectSlice(t, raw)
					}
					if raw := contractSchemaMap(t, legacy["relations"])["relations"]; raw != nil {
						legacyEdges = contractObjectSlice(t, raw)
					}
					if len(conciseEdges) != len(legacyEdges) {
						t.Fatalf("relation edge count changed: %d vs %d", len(conciseEdges), len(legacyEdges))
					}
					for i, edge := range conciseEdges {
						for _, field := range []string{"source", "target", "evidence", "resolution"} {
							if !reflect.DeepEqual(edge[field], legacyEdges[i][field]) {
								t.Fatalf("relation %d lost %s", i, field)
							}
						}
						kind := edge["kind"]
						if kind == nil {
							kind = optin["relation"]
						}
						if kind != legacyEdges[i]["kind"] {
							t.Fatalf("relation %d kind drift", i)
						}
					}
				}
				conciseTotal, legacyTotal = conciseTotal+shortBytes, legacyTotal+longBytes
				t.Logf("%s: concise=%d legacy=%d SDK structured JSON bytes", optin["operation"], shortBytes, longBytes)
				return compact
			}
			base := map[string]any{"paths": paths, "language": "go", "encoding": "utf-8", "maxFiles": len(paths)}
			request := func(op string, additions map[string]any) map[string]any {
				args := make(map[string]any, len(base)+len(additions)+1)
				for k, v := range base {
					args[k] = v
				}
				args["operation"] = op
				for k, v := range additions {
					args[k] = v
				}
				return args
			}
			concise := func(args map[string]any) map[string]any {
				out := make(map[string]any, len(args)+1)
				for k, v := range args {
					out[k] = v
				}
				out["presentation"] = "concise"
				return out
			}
			digestArgs := request("digest", nil)
			digest := pair("source_symbols", concise(digestArgs), digestArgs)
			observed := make(map[string]bool)
			for _, file := range contractObjectSlice(t, digest["files"]) {
				for _, name := range contractStringSlice(t, file["functions"]) {
					observed[name] = true
				}
				if file["sourceFingerprint"] != nil || file["symbolId"] != nil {
					t.Fatalf("classless digest leaked navigation hashes: %#v", file)
				}
			}
			for _, want := range tc.functions {
				if !observed[want] {
					t.Fatalf("missing original top-level Go function %s in digest", want)
				}
			}

			outlineArgs := request("outline", map[string]any{"paths": []string{mainPath}, "maxFiles": 1, "maxSymbols": 256})
			outline := pair("source_symbols", concise(outlineArgs), outlineArgs)
			outlineFunctions := make(map[string]bool)
			for _, file := range contractObjectSlice(t, outline["files"]) {
				for _, name := range contractStringSlice(t, file["functions"]) {
					outlineFunctions[name] = true
				}
			}
			for _, want := range tc.functions {
				if !outlineFunctions[want] {
					t.Fatalf("classless outline omitted original function %s", want)
				}
			}

			findArgs := request("find", map[string]any{"paths": []string{mainPath}, "maxFiles": 1,
				"query": tc.target, "match": "exact", "maxSymbols": 16})
			found := pair("source_symbols", concise(findArgs), findArgs)
			files := contractObjectSlice(t, found["files"])
			if len(files) != 1 || files[0]["path"] != mainPath ||
				files[0]["language"] != "go" || files[0]["encoding"] != "utf-8" {
				t.Fatalf("find returned unsafe original file selection: %#v", files)
			}
			fingerprint, _ := files[0]["sourceFingerprint"].(string)
			symbolID := ""
			for _, match := range contractObjectSlice(t, files[0]["matches"]) {
				if match["name"] == tc.target && match["line"] == float64(tc.line) {
					symbolID, _ = match["symbolId"].(string)
				}
			}
			if len(fingerprint) != 64 || len(symbolID) != 64 {
				t.Fatalf("safe source identity missing for %s:%d: %#v", tc.target, tc.line, files[0])
			}

			showArgs := map[string]any{"operation": "show", "path": mainPath,
				"symbolId": symbolID, "sourceFingerprint": fingerprint,
				"language": "go", "encoding": "utf-8", "maxBytes": 8192}
			shown := pair("source_symbols", concise(func() map[string]any {
				out := make(map[string]any, len(showArgs)+1)
				for k, v := range showArgs {
					out[k] = v
				}
				out["part"] = "declaration"
				return out
			}()), showArgs)
			code, _ := shown["code"].(string)
			if shown["representation"] != "declaration" || shown["file"] != mainPath ||
				!strings.Contains(code, tc.signature) || !bytes.Contains(original[mainPath], []byte(code)) {
				t.Fatalf("original exact declaration not retained: %#v", shown)
			}

			searchArgs := request("search", map[string]any{"query": tc.target,
				"mode": "structural", "match": "exact", "maxResults": 24})
			search := pair("source_query", concise(searchArgs), searchArgs)
			verified := false
			for _, file := range contractObjectSlice(t, contractSchemaMap(t, search["search"])["files"]) {
				for _, match := range contractObjectSlice(t, file["matches"]) {
					if file["path"] == mainPath && match["symbolId"] == symbolID &&
						strings.HasSuffix(fmt.Sprint(match["symbol"]), "."+tc.target) {
						verified = true
					}
					if file["path"] == paths[2] && match["symbolId"] != nil {
						t.Fatalf("distractor falsely declares %s: %#v", tc.target, match)
					}
				}
			}
			if !verified {
				t.Fatalf("search omitted original declaration selector for %s", tc.target)
			}

			relationsArgs := request("relations", map[string]any{
				"relation": "dependencies", "subject": map[string]any{
					"kind": "path", "path": mainPath, "sourceFingerprint": fingerprint},
				"maxResults": 24, "maxNodes": 32, "maxEdges": 64, "maxDepth": 3})
			relations := pair("source_query", concise(relationsArgs), relationsArgs)
			relationData := contractSchemaMap(t, relations["relations"])
			if relationData["relation"] != "dependencies" || relationData["evidenceScope"] != "indexed-relations-only" {
				t.Fatalf("relations omitted evidence limitations: %#v", relationData)
			}

			contextArgs := request("context", map[string]any{
				"targets": []any{map[string]any{"kind": "symbol", "path": mainPath,
					"symbolId": symbolID, "sourceFingerprint": fingerprint}},
				"budgetBytes": 4096, "bodyPolicy": "signatures-only",
				"maxItems": 1, "maxDepth": 1})
			context := pair("source_query", concise(contextArgs), contextArgs)
			items := contractObjectSlice(t, contractSchemaMap(t, context["context"])["items"])
			if len(items) != 1 || items[0]["representation"] != "signature" ||
				!bytes.Contains(original[mainPath], []byte(fmt.Sprint(items[0]["code"]))) {
				t.Fatalf("context not an exact original-source signature: %#v", items)
			}

			if conciseTotal >= legacyTotal {
				t.Fatalf("seven concise public MCP responses not smaller: concise=%d legacy=%d", conciseTotal, legacyTotal)
			}
			for path, before := range original {
				after, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(after, before) {
					t.Fatalf("read-only qualification changed original source %s: %v", path, err)
				}
			}
			t.Logf("%s: seven public MCP operations concise=%d legacy=%d SDK structured JSON bytes",
				tc.task, conciseTotal, legacyTotal)
		})
	}
}

// Two distinct original file selections must retain their selected root
// identities; a relative display name is never a safe MCP show selector.
func TestSourceResponseConciseOriginalGoMultiRootMCP(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Go corpus not configured")
	}
	roots := []string{
		filepath.Join(corpus, "go-cobra"),
		filepath.Join(corpus, "go-chi"),
	}
	relative := []string{"flag_groups.go", "middleware/recoverer.go"}
	paths := make([]string, len(roots))
	source := make([][]byte, len(roots))
	for i := range roots {
		paths[i] = filepath.Join(roots[i], filepath.FromSlash(relative[i]))
		info, err := os.Lstat(paths[i])
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("configured multi-root fixture missing: %s: %v", relative[i], err)
		}
		source[i], err = os.ReadFile(paths[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Contains(source[0], []byte("func validateExclusiveFlagGroups(")) ||
		!bytes.Contains(source[1], []byte("func Recoverer(")) {
		t.Fatal("original multi-root source oracle changed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	session := connectTestClient(t, ctx, NewServer(roots, nil, nil), "original-go-multiroot-response")
	digest := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "digest", "presentation": "concise",
		"paths": paths, "language": "go", "encoding": "utf-8", "maxFiles": 2,
	}))
	files := contractObjectSlice(t, digest["files"])
	if len(files) != 2 {
		t.Fatalf("multi-root concise digest files=%#v", files)
	}
	seenRoots := make(map[string]bool)
	for _, f := range files {
		root, _ := f["root"].(string)
		path, _ := f["path"].(string)
		switch root {
		case paths[0]:
			if path != filepath.Base(paths[0]) {
				t.Fatalf("Cobra file lost direct-selection location: %#v", f)
			}
		case paths[1]:
			if path != filepath.Base(paths[1]) {
				t.Fatalf("Chi file lost direct-selection location: %#v", f)
			}
		default:
			t.Fatalf("unexpected root identity in concise map: %#v", f)
		}
		seenRoots[root] = true
	}
	if len(seenRoots) != 2 {
		t.Fatalf("two original project roots were not independently identified: %#v", files)
	}

	find := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "find", "presentation": "concise", "paths": paths,
		"language": "go", "encoding": "utf-8", "query": "Recoverer",
		"match": "exact", "maxFiles": 2, "maxSymbols": 16,
	}))
	matched := contractObjectSlice(t, find["files"])
	if len(matched) != 1 || matched[0]["path"] != paths[1] {
		t.Fatalf("cross-root find selected ambiguous or unrelated source: %#v", matched)
	}
	symbols := contractObjectSlice(t, matched[0]["matches"])
	if len(symbols) != 1 || symbols[0]["name"] != "Recoverer" {
		t.Fatalf("cross-root lookup lost verified declaration: %#v", symbols)
	}
	id, _ := symbols[0]["symbolId"].(string)
	fingerprint, _ := matched[0]["sourceFingerprint"].(string)
	if len(id) != 64 || len(fingerprint) != 64 {
		t.Fatal("multi-root find did not supply safe show selector")
	}
	shown := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "show", "presentation": "concise", "part": "signature",
		"path": paths[1], "symbolId": id, "sourceFingerprint": fingerprint,
		"language": "go", "encoding": "utf-8", "maxBytes": 1024,
	}))
	code, _ := shown["code"].(string)
	if shown["representation"] != "signature" || !strings.Contains(code, "func Recoverer(") ||
		!bytes.Contains(source[1], []byte(code)) || bytes.Contains(source[0], []byte(code)) {
		t.Fatalf("multi-root show resolved wrong original source: %#v", shown)
	}
	for i, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, source[i]) {
			t.Fatalf("original fixture changed during read-only multi-root test: %s", path)
		}
	}
}
