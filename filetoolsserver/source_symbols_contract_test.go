package filetoolsserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
)

const (
	sourceSymbolsContractMaxInputPaths = 256
	sourceSymbolsContractMaxFiles      = 4_096
	sourceSymbolsContractMaxSymbols    = 100_000
	sourceSymbolsContractMaxShowBytes  = 8 * 1024 * 1024
	sourceSymbolsMaxLanguageCandidates = 16
	sourceSymbolsMaxLanguageEvidence   = 32
)

func TestSourceSymbolsPublicContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	server := NewServer([]string{root}, nil, nil)
	session := connectTestClient(t, ctx, server, "r25-source-symbols-contract")
	tool := sourceSymbolsToolContract(t, ctx, session)

	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Fatalf("source_symbols is not advertised read-only: %#v", tool.Annotations)
	}
	if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
		t.Fatalf("source_symbols is advertised destructive: %#v", tool.Annotations)
	}

	schema := sourceSymbolsSchemaContract(t, tool)
	properties := contractSchemaProperties(t, schema)
	branches := sourceSymbolsSchemaBranches(t, tool)
	if got, want := len(branches), 4; got != want {
		t.Fatalf("source_symbols operation branches = %d, want %d", got, want)
	}

	assertSourceSymbolsOperationSchema(t, branches, "outline",
		[]string{"operation", "paths"},
		[]string{"operation", "paths", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "presentation", "owner", "maxSymbols", "maxFiles"},
	)
	digestBranch := assertSourceSymbolsOperationSchema(t, branches, "digest",
		[]string{"operation", "paths"},
		[]string{"operation", "paths", "language", "encoding", "includes", "excludes", "respectGitignore", "presentation", "maxFiles"},
	)
	assertSourceSymbolsOperationSchema(t, branches, "find",
		[]string{"operation", "paths", "query"},
		[]string{"operation", "paths", "query", "match", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "fileDetails", "presentation", "maxSymbols", "maxFiles"},
	)
	assertSourceSymbolsOperationSchema(t, branches, "show",
		[]string{"operation", "path", "symbolId", "sourceFingerprint", "language", "encoding"},
		[]string{"operation", "path", "symbolId", "sourceFingerprint", "language", "encoding", "maxBytes"},
	)

	for _, operation := range []string{"outline", "digest", "find"} {
		assertSourceSymbolsIntegerBound(t, operation+".paths.maxItems", properties["paths"], "maxItems", sourceSymbolsContractMaxInputPaths)
		assertSourceSymbolsIntegerBound(t, operation+".maxFiles.maximum", properties["maxFiles"], "maximum", sourceSymbolsContractMaxFiles)
	}
	for _, operation := range []string{"outline", "find"} {
		assertSourceSymbolsIntegerBound(t, operation+".maxSymbols.maximum", properties["maxSymbols"], "maximum", sourceSymbolsContractMaxSymbols)
		assertSourceSymbolsIntegerBound(t, operation+".kinds.maxItems", properties["kinds"], "maxItems", 32)
	}
	if got := contractStringSlice(t, contractSchemaMap(t, properties["match"])["enum"]); !reflect.DeepEqual(got, []string{"exact", "prefix", "qualified"}) {
		t.Fatalf("find.match enum = %v", got)
	}
	if got := contractStringSlice(t, contractSchemaMap(t, properties["presentation"])["enum"]); !reflect.DeepEqual(got, []string{"detailed", "compact", "tree", "concise"}) {
		t.Fatalf("outline.presentation enum = %v", got)
	}
	if got := contractStringSlice(t, contractSchemaMap(t, contractSchemaProperties(t, digestBranch)["presentation"])["enum"]); !reflect.DeepEqual(got, []string{"detailed", "compact", "concise"}) {
		t.Fatalf("digest.presentation enum = %v", got)
	}
	assertSourceSymbolsIntegerBound(t, "find.query.maxLength", properties["query"], "maxLength", 512)
	if got := contractStringSlice(t, contractSchemaMap(t, properties["fileDetails"])["enum"]); !reflect.DeepEqual(got, []string{"all", "relevant"}) {
		t.Fatalf("find.fileDetails enum = %v", got)
	}

	assertSourceSymbolsIntegerBound(t, "show.maxBytes.maximum", properties["maxBytes"], "maximum", sourceSymbolsContractMaxShowBytes)
	for _, field := range []string{"symbolId", "sourceFingerprint"} {
		schema := contractSchemaMap(t, properties[field])
		if schema["pattern"] != "^[0-9a-f]{64}$" {
			t.Fatalf("show.%s pattern = %#v, want lowercase SHA-256 form", field, schema["pattern"])
		}
	}

	rejected, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "source_symbols",
		Arguments: map[string]any{
			"operation": "outline",
			"paths":     []string{root},
			"query":     "illegal-for-outline",
		},
	})
	if err == nil && (rejected == nil || !rejected.IsError) {
		t.Fatalf("outline accepted operation-illegal field query: result=%#v err=%v", rejected, err)
	}
}

func TestSourceSymbolsConciseDigestPublicMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	javaRoot, goRoot := filepath.Join(root, "java"), filepath.Join(root, "go")
	for _, dir := range []string{javaRoot, goRoot} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	javaFile := filepath.Join(javaRoot, "Example.java")
	goFile := filepath.Join(goRoot, "example.go")
	for path, content := range map[string]string{
		javaFile: "package demo;\nclass Hidden { private int field; private void work() {} }\npublic class Visible {}\n",
		goFile:   "package demo\ntype Record struct { Field int }\nfunc Start() {}\n",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "source-concise-digest")
	conciseResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "digest", "paths": []string{javaRoot, goRoot}, "presentation": "concise", "maxFiles": 4,
	})
	concise := contractStructuredMap(t, conciseResult)
	for _, omitted := range []string{"digests", "overviewFiles", "symbolCount", "filesConsidered", "sourceFingerprint"} {
		if _, present := concise[omitted]; present {
			t.Fatalf("concise response leaks %s: %#v", omitted, concise)
		}
	}
	files := contractObjectSlice(t, concise["files"])
	if len(files) != 2 {
		t.Fatalf("concise files=%#v", files)
	}
	byRoot := make(map[string]map[string]any, len(files))
	for _, file := range files {
		rootName, _ := file["root"].(string)
		byRoot[rootName] = file
		if _, exists := file["sourceFingerprint"]; exists {
			t.Fatalf("per-file digest leaked hash: %#v", file)
		}
	}
	java := byRoot[javaRoot]
	goSource := byRoot[goRoot]
	if java == nil || java["path"] != "Example.java" ||
		!reflect.DeepEqual(contractStringSlice(t, java["classes"]), []string{"Hidden", "Visible"}) {
		t.Fatalf("missing non-public Java top-level class: %#v", java)
	}
	if goSource == nil || goSource["path"] != "example.go" ||
		!reflect.DeepEqual(contractStringSlice(t, goSource["types"]), []string{"Record"}) ||
		!reflect.DeepEqual(contractStringSlice(t, goSource["functions"]), []string{"Start"}) {
		t.Fatalf("classless Go file was not mapped: %#v", goSource)
	}
	legacy := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "digest", "paths": []string{goRoot}, "presentation": "compact",
	}))
	if legacy["overviewFiles"] == nil {
		t.Fatal("existing compact digest projection changed")
	}
}

func TestSourceSymbolsConciseOutlinePublicMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	path := filepath.Join(root, "Example.java")
	code := "package demo;\nclass Example {\n" +
		" private int field;\n" +
		" Example() {}\n" +
		" private void hidden() {}\n" +
		" void Text(int value) {}\n" +
		" void Text(String value) {}\n" +
		"}\n"
	if err := os.WriteFile(path, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "source-concise-outline")
	result := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "outline", "paths": []string{path}, "presentation": "concise",
		"owner": "Example", "language": "java", "encoding": "utf-8", "maxSymbols": 16,
	})
	payload := contractStructuredMap(t, result)
	for _, forbidden := range []string{"outlineFiles", "symbols", "filesConsidered", "symbolCount"} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("concise outline contains %s", forbidden)
		}
	}
	files := contractObjectSlice(t, payload["files"])
	if len(files) != 1 || files[0]["path"] != "Example.java" {
		t.Fatalf("concise outline files=%#v", files)
	}
	owners := contractObjectSlice(t, files[0]["owners"])
	if len(owners) != 1 || owners[0]["name"] != "Example" {
		t.Fatalf("requested owner not isolated: %#v", owners)
	}
	methods := contractObjectSlice(t, owners[0]["methods"])
	if len(methods) != 4 {
		t.Fatalf("expected constructor, private and both overloads, got %#v", methods)
	}
	counts := map[string]int{}
	signatures := map[string]bool{}
	for _, m := range methods {
		name, _ := m["name"].(string)
		counts[name]++
		sig, _ := m["signature"].(string)
		if sig == "" || signatures[sig] {
			t.Fatalf("missing or duplicate overload signature: %#v", methods)
		}
		signatures[sig] = true
		if m["line"] == nil || m["symbolId"] != nil || m["sourceFingerprint"] != nil ||
			m["parentId"] != nil || m["field"] != nil {
			t.Fatalf("unexpected method metadata: %#v", m)
		}
	}
	if !reflect.DeepEqual(counts, map[string]int{"Example": 1, "hidden": 1, "Text": 2}) {
		t.Fatalf("constructor or overload lost: %#v", counts)
	}
	legacy := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "outline", "paths": []string{path}, "language": "java",
	}))
	if legacy["outlineFiles"] == nil {
		t.Fatal("default tree projection changed")
	}
}

func TestSourceSymbolsConciseOutlineCoverageAndAmbiguity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	for _, dir := range []string{"one", "two"} {
		sub := filepath.Join(root, dir)
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatal(err)
		}
		source := "package demo;\nclass Widget { int field; Widget() {} void Work() {} }\n"
		if err := os.WriteFile(filepath.Join(sub, "Widget.java"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "source-concise-coverage")
	request := map[string]any{
		"operation": "outline", "paths": []string{filepath.Join(root, "one"), filepath.Join(root, "two")},
		"presentation": "concise", "owner": "Widget", "language": "java", "encoding": "utf-8",
	}
	multiple := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, request))
	if multiple["ambiguous"] != true || len(contractObjectSlice(t, multiple["files"])) != 2 {
		t.Fatalf("same-name owners were silently conflated: %#v", multiple)
	}
	for _, file := range contractObjectSlice(t, multiple["files"]) {
		if file["root"] == nil || file["path"] != "Widget.java" {
			t.Fatalf("multi-root owner lacks unambiguous location: %#v", file)
		}
	}
	limited := make(map[string]any, len(request)+1)
	for k, v := range request {
		limited[k] = v
	}
	limited["maxSymbols"] = 2
	result := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, limited))
	cov := contractSchemaMap(t, result["coverage"])
	if cov["analysisComplete"] != true || cov["resultsTruncated"] != true || cov["fileSelectionTruncated"] != false {
		t.Fatalf("result limit confused with analysis or file selection: %#v", cov)
	}
	limited["paths"] = []string{filepath.Join(root, "one")}
	limited["kinds"] = []string{"field"}
	limited["maxSymbols"] = 8
	fieldsOnly := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, limited))
	files := contractObjectSlice(t, fieldsOnly["files"])
	owners := contractObjectSlice(t, files[0]["owners"])
	if len(contractObjectSlice(t, owners[0]["fields"])) != 1 || owners[0]["methods"] != nil {
		t.Fatalf("explicit field selection did not isolate fields: %#v", owners[0])
	}
	invalid := make(map[string]any, len(limited))
	for k, v := range limited {
		invalid[k] = v
	}
	invalid["presentation"] = "tree"
	rejected, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_symbols", Arguments: invalid})
	if err == nil && (rejected == nil || !rejected.IsError) {
		t.Fatalf("owner filter leaked into legacy tree: %#v", rejected)
	}
}

func TestSourceSymbolsConciseFindSafeShow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	path := filepath.Join(root, "Service.java")
	source := "package demo;\nclass Service {\n" +
		" private int field;\n" +
		" void Work(int value) {}\n" +
		" private void Work(String value) {}\n" +
		"}\nclass Other { void Work() {} }\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "source-concise-find")
	args := map[string]any{
		"operation": "find", "paths": []string{path}, "presentation": "concise",
		"language": "java", "encoding": "utf-8", "query": "Work",
		"match": "exact", "maxSymbols": 8,
	}
	payload := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, args))
	if payload["ambiguous"] != true || payload["symbols"] != nil || payload["symbolCount"] != nil {
		t.Fatalf("concise find missing ambiguity or exposing historical symbol inventory: %#v", payload)
	}
	files := contractObjectSlice(t, payload["files"])
	if len(files) != 1 || files[0]["path"] != path {
		t.Fatalf("usable source path not returned: %#v", files)
	}
	file := files[0]
	assertSourceSymbolsHexDigest(t, "file.sourceFingerprint", file["sourceFingerprint"])
	matches := contractObjectSlice(t, file["matches"])
	if len(matches) != 3 {
		t.Fatalf("overloads or unrelated owner lost: %#v", matches)
	}
	owners, sigs := map[string]int{}, map[string]bool{}
	for _, match := range matches {
		if match["name"] != "Work" || match["kind"] != "method" {
			t.Fatalf("unexpected match: %#v", match)
		}
		owner, _ := match["owner"].(string)
		owners[owner]++
		sig, _ := match["signature"].(string)
		if sig == "" || sigs[sig] && owner == "demo.Service" {
			t.Fatalf("missing/distinct overload signatures: %#v", matches)
		}
		sigs[sig] = true
		assertSourceSymbolsHexDigest(t, "match.symbolId", match["symbolId"])
		if match["line"] == nil || match["path"] != nil || match["sourceFingerprint"] != nil {
			t.Fatalf("redundant or missing navigation data: %#v", match)
		}
	}
	if owners["demo.Service"] != 2 || owners["demo.Other"] != 1 {
		t.Fatalf("same-name owner identity lost: %#v", owners)
	}
	shown := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "show", "path": path, "language": file["language"],
		"encoding": file["encoding"], "sourceFingerprint": file["sourceFingerprint"],
		"symbolId": matches[0]["symbolId"], "maxBytes": 4096,
	}))
	body := contractSchemaMap(t, shown["show"])
	if snippet, _ := body["text"].(string); !strings.Contains(snippet, "Work(") {
		t.Fatalf("concise find cannot navigate to exact source: %#v", body)
	}
	legacy := contractStructuredMap(t, callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "find", "paths": []string{path}, "query": "Work",
		"match": "exact", "language": "java", "encoding": "utf-8",
	}))
	if legacy["symbols"] == nil || legacy["files"] == nil {
		t.Fatal("original find contract changed")
	}
}

func TestSourceSymbolsNavigationOutputContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := canonicalServerTestDir(t)
	path := filepath.Join(root, "sample.go")
	source := "package sample\n\n" +
		"type Box struct {\n\tValue int\n}\n\n" +
		"func (b *Box) Get() int { return b.Value }\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	server := NewServer([]string{root}, nil, nil)
	session := connectTestClient(t, ctx, server, "r25-source-symbols-output")
	_ = sourceSymbolsToolContract(t, ctx, session)

	defaultResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "outline", "paths": []string{path}, "language": "go",
	})
	defaultOutline := contractStructuredMap(t, defaultResult)
	if defaultOutline["presentation"] != "tree" || defaultOutline["symbols"] != nil || defaultOutline["files"] != nil {
		t.Fatalf("omitted presentation must return tree output: %#v", defaultOutline)
	}
	defaultFiles := contractObjectSlice(t, defaultOutline["outlineFiles"])
	if len(defaultFiles) != 1 || len(contractObjectSlice(t, defaultFiles[0]["symbols"])) == 0 {
		t.Fatalf("default tree has no declarations: %#v", defaultFiles)
	}
	outlineResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation":         "outline",
		"presentation":      "detailed",
		"paths":             []string{path},
		"language":          "go",
		"includeSignatures": true,
		"maxSymbols":        20,
		"maxFiles":          4,
	})
	outline := contractStructuredMap(t, outlineResult)
	if outline["operation"] != "outline" {
		t.Fatalf("outline operation = %#v", outline["operation"])
	}
	if outline["coordinateSystem"] != "unicode-scalar-1-based-half-open" {
		t.Fatalf("coordinateSystem = %#v", outline["coordinateSystem"])
	}
	if complete, ok := outline["coverageComplete"].(bool); !ok || !complete {
		t.Fatalf("outline coverageComplete = %#v", outline["coverageComplete"])
	}
	for _, field := range []string{"filesConsidered", "filesParsed", "filesSkipped", "symbolCount"} {
		if _, ok := outline[field].(float64); !ok {
			t.Fatalf("outline %s = %#v, want number", field, outline[field])
		}
	}

	files := contractObjectSlice(t, outline["files"])
	if len(files) != 1 {
		t.Fatalf("outline files = %d, want 1", len(files))
	}
	file := files[0]
	for field, want := range map[string]any{
		"path":             path,
		"status":           "parsed",
		"encoding":         "utf-8",
		"language":         "go",
		"coverageComplete": true,
	} {
		if file[field] != want {
			t.Fatalf("file.%s = %#v, want %#v", field, file[field], want)
		}
	}
	assertSourceSymbolsHexDigest(t, "file.sourceFingerprint", file["sourceFingerprint"])
	if analyzer, _ := file["analyzer"].(string); analyzer == "" {
		t.Fatalf("file.analyzer = %#v", file["analyzer"])
	}
	detection := contractSchemaMap(t, file["detection"])
	if detection["state"] != "exact" || detection["language"] != "go" {
		t.Fatalf("file detection = %#v", detection)
	}
	candidates := contractObjectSlice(t, detection["candidates"])
	evidence := contractObjectSlice(t, detection["evidence"])
	if len(candidates) == 0 || len(candidates) > sourceSymbolsMaxLanguageCandidates {
		t.Fatalf("language candidates = %d, want 1..%d", len(candidates), sourceSymbolsMaxLanguageCandidates)
	}
	if len(evidence) == 0 || len(evidence) > sourceSymbolsMaxLanguageEvidence {
		t.Fatalf("language evidence = %d, want 1..%d", len(evidence), sourceSymbolsMaxLanguageEvidence)
	}
	if evidence[0]["kind"] != "explicit" {
		t.Fatalf("first language evidence = %#v, want explicit", evidence[0])
	}

	symbols := contractObjectSlice(t, outline["symbols"])
	var method map[string]any
	for _, symbol := range symbols {
		assertSourceSymbolsNormalizedSymbol(t, symbol)
		if symbol["name"] == "Get" && symbol["kind"] == "method" {
			method = symbol
		}
	}
	if method == nil {
		t.Fatalf("outline did not return Get method: %#v", symbols)
	}
	methodID, _ := method["id"].(string)
	fingerprint, _ := file["sourceFingerprint"].(string)

	serializedOutline, err := json.Marshal(outline)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serializedOutline), "return b.Value") {
		t.Fatalf("outline embedded source body: %s", serializedOutline)
	}

	digestResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "digest",
		"paths":     []string{path},
		"language":  "go",
		"maxFiles":  4,
	})
	digest := contractStructuredMap(t, digestResult)
	digests := contractObjectSlice(t, digest["digests"])
	if len(digests) != 1 || digests[0]["path"] != path || digests[0]["language"] != "go" {
		t.Fatalf("digest output = %#v", digests)
	}
	if _, ok := digests[0]["sourceBytes"].(float64); !ok {
		t.Fatalf("digest sourceBytes = %#v", digests[0]["sourceBytes"])
	}
	if counts := contractObjectSlice(t, digests[0]["declarationCounts"]); len(counts) == 0 {
		t.Fatalf("digest declarationCounts = %#v", digests[0]["declarationCounts"])
	}
	serializedDigest, err := json.Marshal(digest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serializedDigest), "return b.Value") {
		t.Fatalf("digest embedded source body: %s", serializedDigest)
	}

	findResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation":  "find",
		"paths":      []string{path},
		"query":      "Get",
		"match":      "exact",
		"language":   "go",
		"maxSymbols": 20,
		"maxFiles":   4,
	})
	found := contractObjectSlice(t, contractStructuredMap(t, findResult)["symbols"])
	if len(found) != 1 || found[0]["id"] != methodID {
		t.Fatalf("find(Get) = %#v, want method id %s", found, methodID)
	}

	showArguments := map[string]any{
		"operation":         "show",
		"path":              path,
		"symbolId":          methodID,
		"sourceFingerprint": fingerprint,
		"language":          "go",
		"encoding":          "utf-8",
		"maxBytes":          4096,
	}
	showResult := callSourceSymbolsContract(t, ctx, session, showArguments)
	show := contractStructuredMap(t, showResult)
	shown := contractSchemaMap(t, show["show"])
	if shown["symbolId"] != methodID || shown["sourceFingerprint"] != fingerprint || shown["path"] != path {
		t.Fatalf("show identity = %#v", shown)
	}
	text, _ := shown["text"].(string)
	if !strings.Contains(text, "func (b *Box) Get() int { return b.Value }") {
		t.Fatalf("show text = %q", text)
	}
	assertSourceSymbolsRange(t, "show.range", shown["range"])

	if err := os.WriteFile(path, []byte(source+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stale, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_symbols", Arguments: showArguments})
	if err != nil {
		t.Fatalf("stale show transport error: %v", err)
	}
	if stale == nil || !stale.IsError || stale.Meta[handler.ErrorCodeMetaKey] != handler.ErrCodeConflict {
		t.Fatalf("stale show result = %#v, want %s", stale, handler.ErrCodeConflict)
	}
}

func sourceSymbolsToolContract(t *testing.T, ctx context.Context, session *mcp.ClientSession) *mcp.Tool {
	t.Helper()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == "source_symbols" {
			return tool
		}
	}
	t.Fatal("R25 source_symbols implementation is absent")
	return nil
}

func sourceSymbolsSchemaContract(t *testing.T, tool *mcp.Tool) map[string]any {
	t.Helper()
	data, err := json.Marshal(tool.InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	return schema
}

func sourceSymbolsSchemaBranches(t *testing.T, tool *mcp.Tool) []map[string]any {
	t.Helper()
	schema := sourceSymbolsSchemaContract(t, tool)
	raw, ok := schema["oneOf"].([]any)
	if !ok {
		t.Fatalf("source_symbols schema oneOf = %#v", schema["oneOf"])
	}
	branches := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		branches = append(branches, contractSchemaMap(t, item))
	}
	return branches
}

func assertSourceSymbolsOperationSchema(t *testing.T, branches []map[string]any, operation string, required, properties []string) map[string]any {
	t.Helper()
	for _, branch := range branches {
		branchProperties := contractSchemaProperties(t, branch)
		operationSchema := contractSchemaMap(t, branchProperties["operation"])
		if operationSchema["const"] != operation {
			continue
		}
		if branch["additionalProperties"] != false {
			t.Fatalf("%s additionalProperties = %#v, want false", operation, branch["additionalProperties"])
		}
		gotRequired := contractStringSlice(t, branch["required"])
		sort.Strings(gotRequired)
		wantRequired := append([]string(nil), required...)
		sort.Strings(wantRequired)
		if !reflect.DeepEqual(gotRequired, wantRequired) {
			t.Fatalf("%s required = %v, want %v", operation, gotRequired, wantRequired)
		}
		gotProperties := make([]string, 0, len(branchProperties))
		for name := range branchProperties {
			gotProperties = append(gotProperties, name)
		}
		sort.Strings(gotProperties)
		wantProperties := append([]string(nil), properties...)
		sort.Strings(wantProperties)
		if !reflect.DeepEqual(gotProperties, wantProperties) {
			t.Fatalf("%s properties = %v, want %v", operation, gotProperties, wantProperties)
		}
		return branch
	}
	t.Fatalf("source_symbols schema is missing %s branch", operation)
	return nil
}

func contractSchemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	return contractSchemaMap(t, schema["properties"])
}

func contractSchemaMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("schema/object value = %#v, want object", value)
	}
	return result
}

func contractStringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want string array", value)
	}
	result := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("array item = %#v, want string", item)
		}
		result = append(result, text)
	}
	return result
}

func assertSourceSymbolsIntegerBound(t *testing.T, label string, value any, field string, want int) {
	t.Helper()
	schema := contractSchemaMap(t, value)
	got, ok := schema[field].(float64)
	if !ok || int(got) != want {
		t.Fatalf("%s = %#v, want %d", label, schema[field], want)
	}
}

func callSourceSymbolsContract(t *testing.T, ctx context.Context, session *mcp.ClientSession, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_symbols", Arguments: arguments})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("source_symbols(%v) result=%#v err=%v", arguments["operation"], result, err)
	}
	return result
}

func contractStructuredMap(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("decode structured content %s: %v", data, err)
	}
	return object
}

func contractObjectSlice(t *testing.T, value any) []map[string]any {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want object array", value)
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		result = append(result, contractSchemaMap(t, item))
	}
	return result
}

func assertSourceSymbolsNormalizedSymbol(t *testing.T, symbol map[string]any) {
	t.Helper()
	for _, field := range []string{"id", "path", "language", "kind", "nativeKind", "name", "declarationRange", "nameRange", "evidence", "analyzer"} {
		if _, ok := symbol[field]; !ok {
			t.Fatalf("symbol is missing %s: %#v", field, symbol)
		}
	}
	assertSourceSymbolsHexDigest(t, "symbol.id", symbol["id"])
	if symbol["evidence"] != "structural" {
		t.Fatalf("symbol evidence = %#v, want structural", symbol["evidence"])
	}
	assertSourceSymbolsRange(t, "symbol.declarationRange", symbol["declarationRange"])
	assertSourceSymbolsRange(t, "symbol.nameRange", symbol["nameRange"])
}

func assertSourceSymbolsHexDigest(t *testing.T, label string, value any) {
	t.Helper()
	text, ok := value.(string)
	if !ok || len(text) != 64 {
		t.Fatalf("%s = %#v, want 64 lowercase hex characters", label, value)
	}
	for _, current := range text {
		if !strings.ContainsRune("0123456789abcdef", current) {
			t.Fatalf("%s = %q, want lowercase hex", label, text)
		}
	}
}

func assertSourceSymbolsRange(t *testing.T, label string, value any) {
	t.Helper()
	rangeObject := contractSchemaMap(t, value)
	start := contractSchemaMap(t, rangeObject["start"])
	end := contractSchemaMap(t, rangeObject["end"])
	for pointName, point := range map[string]map[string]any{"start": start, "end": end} {
		for _, field := range []string{"line", "column"} {
			number, ok := point[field].(float64)
			if !ok || number < 1 || number != float64(int(number)) {
				t.Fatalf("%s.%s.%s = %#v, want positive 1-based integer", label, pointName, field, point[field])
			}
		}
	}
}
