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
	r25MaxInputPaths         = 256
	r25MaxFiles              = 4_096
	r25MaxSymbols            = 100_000
	r25MaxShowBytes          = 8 * 1024 * 1024
	r25MaxLanguageCandidates = 16
	r25MaxLanguageEvidence   = 32
)

func TestR25SourceSymbolsPublicContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	server := NewServer([]string{root}, nil, nil)
	session := connectTestClient(t, ctx, server, "r25-source-symbols-contract")
	tool := r25SourceSymbolsTool(t, ctx, session)

	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Fatalf("source_symbols is not advertised read-only: %#v", tool.Annotations)
	}
	if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
		t.Fatalf("source_symbols is advertised destructive: %#v", tool.Annotations)
	}

	schema := r25SourceSymbolsSchema(t, tool)
	properties := r25SchemaProperties(t, schema)
	branches := r25SourceSymbolsBranches(t, tool)
	if got, want := len(branches), 4; got != want {
		t.Fatalf("source_symbols operation branches = %d, want %d", got, want)
	}

	r25AssertOperationSchema(t, branches, "outline",
		[]string{"operation", "paths"},
		[]string{"operation", "paths", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "maxSymbols", "maxFiles"},
	)
	r25AssertOperationSchema(t, branches, "digest",
		[]string{"operation", "paths"},
		[]string{"operation", "paths", "language", "encoding", "includes", "excludes", "respectGitignore", "maxFiles"},
	)
	r25AssertOperationSchema(t, branches, "find",
		[]string{"operation", "paths", "query"},
		[]string{"operation", "paths", "query", "match", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "maxSymbols", "maxFiles"},
	)
	r25AssertOperationSchema(t, branches, "show",
		[]string{"operation", "path", "symbolId", "sourceFingerprint", "language", "encoding"},
		[]string{"operation", "path", "symbolId", "sourceFingerprint", "language", "encoding", "maxBytes"},
	)

	for _, operation := range []string{"outline", "digest", "find"} {
		r25AssertIntegerBound(t, operation+".paths.maxItems", properties["paths"], "maxItems", r25MaxInputPaths)
		r25AssertIntegerBound(t, operation+".maxFiles.maximum", properties["maxFiles"], "maximum", r25MaxFiles)
	}
	for _, operation := range []string{"outline", "find"} {
		r25AssertIntegerBound(t, operation+".maxSymbols.maximum", properties["maxSymbols"], "maximum", r25MaxSymbols)
		r25AssertIntegerBound(t, operation+".kinds.maxItems", properties["kinds"], "maxItems", 32)
	}
	if got := r25StringSlice(t, r25SchemaMap(t, properties["match"])["enum"]); !reflect.DeepEqual(got, []string{"exact", "prefix", "qualified"}) {
		t.Fatalf("find.match enum = %v", got)
	}
	r25AssertIntegerBound(t, "find.query.maxLength", properties["query"], "maxLength", 512)

	r25AssertIntegerBound(t, "show.maxBytes.maximum", properties["maxBytes"], "maximum", r25MaxShowBytes)
	for _, field := range []string{"symbolId", "sourceFingerprint"} {
		schema := r25SchemaMap(t, properties[field])
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

func TestR25SourceSymbolsNavigationOutputContract(t *testing.T) {
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
	_ = r25SourceSymbolsTool(t, ctx, session)

	outlineResult := r25CallSourceSymbols(t, ctx, session, map[string]any{
		"operation":         "outline",
		"paths":             []string{path},
		"language":          "go",
		"includeSignatures": true,
		"maxSymbols":        20,
		"maxFiles":          4,
	})
	outline := r25StructuredMap(t, outlineResult)
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

	files := r25ObjectSlice(t, outline["files"])
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
	r25AssertHexDigest(t, "file.sourceFingerprint", file["sourceFingerprint"])
	if analyzer, _ := file["analyzer"].(string); analyzer == "" {
		t.Fatalf("file.analyzer = %#v", file["analyzer"])
	}
	detection := r25SchemaMap(t, file["detection"])
	if detection["state"] != "exact" || detection["language"] != "go" {
		t.Fatalf("file detection = %#v", detection)
	}
	candidates := r25ObjectSlice(t, detection["candidates"])
	evidence := r25ObjectSlice(t, detection["evidence"])
	if len(candidates) == 0 || len(candidates) > r25MaxLanguageCandidates {
		t.Fatalf("language candidates = %d, want 1..%d", len(candidates), r25MaxLanguageCandidates)
	}
	if len(evidence) == 0 || len(evidence) > r25MaxLanguageEvidence {
		t.Fatalf("language evidence = %d, want 1..%d", len(evidence), r25MaxLanguageEvidence)
	}
	if evidence[0]["kind"] != "explicit" {
		t.Fatalf("first language evidence = %#v, want explicit", evidence[0])
	}

	symbols := r25ObjectSlice(t, outline["symbols"])
	var method map[string]any
	for _, symbol := range symbols {
		r25AssertNormalizedSymbol(t, symbol)
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

	digestResult := r25CallSourceSymbols(t, ctx, session, map[string]any{
		"operation": "digest",
		"paths":     []string{path},
		"language":  "go",
		"maxFiles":  4,
	})
	digest := r25StructuredMap(t, digestResult)
	digests := r25ObjectSlice(t, digest["digests"])
	if len(digests) != 1 || digests[0]["path"] != path || digests[0]["language"] != "go" {
		t.Fatalf("digest output = %#v", digests)
	}
	if _, ok := digests[0]["sourceBytes"].(float64); !ok {
		t.Fatalf("digest sourceBytes = %#v", digests[0]["sourceBytes"])
	}
	if counts := r25ObjectSlice(t, digests[0]["declarationCounts"]); len(counts) == 0 {
		t.Fatalf("digest declarationCounts = %#v", digests[0]["declarationCounts"])
	}
	serializedDigest, err := json.Marshal(digest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serializedDigest), "return b.Value") {
		t.Fatalf("digest embedded source body: %s", serializedDigest)
	}

	findResult := r25CallSourceSymbols(t, ctx, session, map[string]any{
		"operation":  "find",
		"paths":      []string{path},
		"query":      "Get",
		"match":      "exact",
		"language":   "go",
		"maxSymbols": 20,
		"maxFiles":   4,
	})
	found := r25ObjectSlice(t, r25StructuredMap(t, findResult)["symbols"])
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
	showResult := r25CallSourceSymbols(t, ctx, session, showArguments)
	show := r25StructuredMap(t, showResult)
	shown := r25SchemaMap(t, show["show"])
	if shown["symbolId"] != methodID || shown["sourceFingerprint"] != fingerprint || shown["path"] != path {
		t.Fatalf("show identity = %#v", shown)
	}
	text, _ := shown["text"].(string)
	if !strings.Contains(text, "func (b *Box) Get() int { return b.Value }") {
		t.Fatalf("show text = %q", text)
	}
	r25AssertRange(t, "show.range", shown["range"])

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

func r25SourceSymbolsTool(t *testing.T, ctx context.Context, session *mcp.ClientSession) *mcp.Tool {
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

func r25SourceSymbolsSchema(t *testing.T, tool *mcp.Tool) map[string]any {
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

func r25SourceSymbolsBranches(t *testing.T, tool *mcp.Tool) []map[string]any {
	t.Helper()
	schema := r25SourceSymbolsSchema(t, tool)
	raw, ok := schema["oneOf"].([]any)
	if !ok {
		t.Fatalf("source_symbols schema oneOf = %#v", schema["oneOf"])
	}
	branches := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		branches = append(branches, r25SchemaMap(t, item))
	}
	return branches
}

func r25AssertOperationSchema(t *testing.T, branches []map[string]any, operation string, required, properties []string) map[string]any {
	t.Helper()
	for _, branch := range branches {
		branchProperties := r25SchemaProperties(t, branch)
		operationSchema := r25SchemaMap(t, branchProperties["operation"])
		if operationSchema["const"] != operation {
			continue
		}
		if branch["additionalProperties"] != false {
			t.Fatalf("%s additionalProperties = %#v, want false", operation, branch["additionalProperties"])
		}
		gotRequired := r25StringSlice(t, branch["required"])
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

func r25SchemaProperties(t *testing.T, schema map[string]any) map[string]any {
	t.Helper()
	return r25SchemaMap(t, schema["properties"])
}

func r25SchemaMap(t *testing.T, value any) map[string]any {
	t.Helper()
	result, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("schema/object value = %#v, want object", value)
	}
	return result
}

func r25StringSlice(t *testing.T, value any) []string {
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

func r25AssertIntegerBound(t *testing.T, label string, value any, field string, want int) {
	t.Helper()
	schema := r25SchemaMap(t, value)
	got, ok := schema[field].(float64)
	if !ok || int(got) != want {
		t.Fatalf("%s = %#v, want %d", label, schema[field], want)
	}
}

func r25CallSourceSymbols(t *testing.T, ctx context.Context, session *mcp.ClientSession, arguments map[string]any) *mcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_symbols", Arguments: arguments})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("source_symbols(%v) result=%#v err=%v", arguments["operation"], result, err)
	}
	return result
}

func r25StructuredMap(t *testing.T, result *mcp.CallToolResult) map[string]any {
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

func r25ObjectSlice(t *testing.T, value any) []map[string]any {
	t.Helper()
	raw, ok := value.([]any)
	if !ok {
		t.Fatalf("value = %#v, want object array", value)
	}
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		result = append(result, r25SchemaMap(t, item))
	}
	return result
}

func r25AssertNormalizedSymbol(t *testing.T, symbol map[string]any) {
	t.Helper()
	for _, field := range []string{"id", "path", "language", "kind", "nativeKind", "name", "declarationRange", "nameRange", "evidence", "analyzer"} {
		if _, ok := symbol[field]; !ok {
			t.Fatalf("symbol is missing %s: %#v", field, symbol)
		}
	}
	r25AssertHexDigest(t, "symbol.id", symbol["id"])
	if symbol["evidence"] != "structural" {
		t.Fatalf("symbol evidence = %#v, want structural", symbol["evidence"])
	}
	r25AssertRange(t, "symbol.declarationRange", symbol["declarationRange"])
	r25AssertRange(t, "symbol.nameRange", symbol["nameRange"])
}

func r25AssertHexDigest(t *testing.T, label string, value any) {
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

func r25AssertRange(t *testing.T, label string, value any) {
	t.Helper()
	rangeObject := r25SchemaMap(t, value)
	start := r25SchemaMap(t, rangeObject["start"])
	end := r25SchemaMap(t, rangeObject["end"])
	for pointName, point := range map[string]map[string]any{"start": start, "end": end} {
		for _, field := range []string{"line", "column"} {
			number, ok := point[field].(float64)
			if !ok || number < 1 || number != float64(int(number)) {
				t.Fatalf("%s.%s.%s = %#v, want positive 1-based integer", label, pointName, field, point[field])
			}
		}
	}
}
