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
	sourceSymbolsMaxInputPaths         = 256
	sourceSymbolsMaxFiles              = 4_096
	sourceSymbolsMaxSymbols            = 100_000
	sourceSymbolsMaxShowBytes          = 8 * 1024 * 1024
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
		[]string{"operation", "paths", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "maxSymbols", "maxFiles"},
	)
	assertSourceSymbolsOperationSchema(t, branches, "digest",
		[]string{"operation", "paths"},
		[]string{"operation", "paths", "language", "encoding", "includes", "excludes", "respectGitignore", "maxFiles"},
	)
	assertSourceSymbolsOperationSchema(t, branches, "find",
		[]string{"operation", "paths", "query"},
		[]string{"operation", "paths", "query", "match", "language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "includeSignatures", "maxSymbols", "maxFiles"},
	)
	assertSourceSymbolsOperationSchema(t, branches, "show",
		[]string{"operation", "path", "symbolId", "sourceFingerprint", "language", "encoding"},
		[]string{"operation", "path", "symbolId", "sourceFingerprint", "language", "encoding", "maxBytes"},
	)

	for _, operation := range []string{"outline", "digest", "find"} {
		assertSourceSymbolsIntegerBound(t, operation+".paths.maxItems", properties["paths"], "maxItems", sourceSymbolsMaxInputPaths)
		assertSourceSymbolsIntegerBound(t, operation+".maxFiles.maximum", properties["maxFiles"], "maximum", sourceSymbolsMaxFiles)
	}
	for _, operation := range []string{"outline", "find"} {
		assertSourceSymbolsIntegerBound(t, operation+".maxSymbols.maximum", properties["maxSymbols"], "maximum", sourceSymbolsMaxSymbols)
		assertSourceSymbolsIntegerBound(t, operation+".kinds.maxItems", properties["kinds"], "maxItems", 32)
	}
	if got := contractStringSlice(t, contractSchemaMap(t, properties["match"])["enum"]); !reflect.DeepEqual(got, []string{"exact", "prefix", "qualified"}) {
		t.Fatalf("find.match enum = %v", got)
	}
	assertSourceSymbolsIntegerBound(t, "find.query.maxLength", properties["query"], "maxLength", 512)

	assertSourceSymbolsIntegerBound(t, "show.maxBytes.maximum", properties["maxBytes"], "maximum", sourceSymbolsMaxShowBytes)
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

	outlineResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation":         "outline",
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
