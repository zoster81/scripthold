package filetoolsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

const (
	sourceQueryMaxInputPaths   = 256
	sourceQueryMaxFiles        = 4_096
	sourceQueryMaxResults      = 100_000
	sourceQueryMaxGraphNodes   = 50_000
	sourceQueryMaxGraphEdges   = 200_000
	sourceQueryMaxGraphDepth   = 64
	sourceQueryMaxContextBytes = 8 * 1024 * 1024
	sourceQueryMaxContextItems = 4_096
)

var sourceQueryContractEvidenceValues = []string{"structural", "scope-resolved", "project-resolved"}

func TestSourceQueryPublicContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := canonicalServerTestDir(t)
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "r27-source-query-contract")
	tool := sourceQueryToolContract(t, ctx, session, "source_query")
	assertSourceQueryReadOnlyTool(t, tool)

	schema := sourceSymbolsSchemaContract(t, tool)
	assertSourceQueryStrictSchema(t, "source_query", schema,
		[]string{"operation", "paths"},
		[]string{
			"operation", "paths", "query", "mode", "match", "cursor", "presentation", "relation", "subject", "target", "targets", "budgetBytes", "bodyPolicy",
			"language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "evidence", "maxFiles", "maxResults",
			"maxNodes", "maxEdges", "maxDepth", "maxItems", "index",
		},
	)
	props := contractSchemaProperties(t, schema)
	assertSourceQueryEnum(t, "source_query.operation", props["operation"], []string{"search", "relations", "context"})
	assertSourceQueryEnum(t, "source_query.mode", props["mode"], []string{"structural"})
	assertSourceQueryEnum(t, "source_query.match", props["match"], []string{"exact", "prefix", "contains"})
	assertSourceQueryEnum(t, "source_query.relation", props["relation"], []string{
		"dependencies", "dependents", "references", "definitions", "inheritance", "implementations",
		"trace", "impact", "cycles",
	})
	assertSourceQueryEnum(t, "source_query.bodyPolicy", props["bodyPolicy"], []string{"prefer", "signatures-only"})
	assertSourceQueryEvidenceFilter(t, "source_query.evidence", props["evidence"])
	defs := contractSchemaMap(t, schema["$defs"])
	assertSourceQueryIndexBinding(t, defs["i"])
	assertSourceQuerySelector(t, defs["s"])
	for _, field := range []string{"subject", "target", "index"} {
		if ref := contractSchemaMap(t, props[field])["$ref"]; ref == nil {
			t.Fatalf("source_query.%s does not reuse a local schema definition", field)
		}
	}
	targets := contractSchemaMap(t, props["targets"])
	if ref := contractSchemaMap(t, targets["items"])["$ref"]; ref != "#/$defs/s" {
		t.Fatalf("source_query.targets.items.$ref = %#v", ref)
	}

	assertSourceQueryRejected(t, ctx, session, "source_query", map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural", "unknown": true,
	})
	assertSourceQueryRejected(t, ctx, session, "source_query", map[string]any{
		"operation": "relations", "paths": []string{root}, "relation": "dependencies",
		"subject": map[string]any{"kind": "path", "path": root, "sourceFingerprint": sourceQueryContractZeroDigest(), "unknown": true},
	})
	assertSourceQueryRejected(t, ctx, session, "source_query", map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural",
		"index": map[string]any{"generation": 1, "unknown": true},
	})
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural", "relation": "cycles",
	}, handler.ErrCodeInvalidInput)
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "relations", "paths": []string{root}, "relation": "cycles",
		"subject": sourceQueryPathSelector(root),
	}, handler.ErrCodeInvalidInput)
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "relations", "paths": []string{root}, "relation": "trace", "subject": sourceQueryPathSelector(root),
	}, handler.ErrCodeInvalidInput)
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "context", "paths": []string{root}, "targets": []any{sourceQueryPathSelector(root)}, "budgetBytes": 4096, "query": "illegal",
	}, handler.ErrCodeInvalidInput)
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural",
		"index": map[string]any{"generation": 0},
	}, handler.ErrCodeInvalidInput)

	basePath := filepath.Join(root, "Base.java")
	childPath := filepath.Join(root, "Child.java")
	if err := os.WriteFile(basePath, []byte("package demo; public class Base {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(childPath, []byte("package demo; public class Child extends Base {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	live, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Base", "mode": "structural", "match": "exact", "language": "java", "maxResults": 16,
	}})
	if err != nil || live == nil || live.IsError {
		t.Fatalf("live source_query result=%#v err=%v", live, err)
	}
	structured := contractStructuredMap(t, live)
	index := contractSchemaMap(t, structured["index"])
	if index["staleness"] != "current" || index["generation"] == nil || index["fingerprint"] == nil {
		t.Fatalf("live source_query index=%#v", index)
	}
	search := contractSchemaMap(t, structured["search"])
	matches, ok := search["matches"].([]any)
	if !ok || len(matches) < 2 {
		t.Fatalf("live source_query search=%#v", search)
	}
	seenDefinition, seenRelation := false, false
	for _, raw := range matches {
		match := contractSchemaMap(t, raw)
		if match["path"] == basePath && match["symbolId"] != "" && match["evidence"] == "structural" {
			seenDefinition = true
		}
		if match["path"] == childPath && match["symbolId"] == nil && match["evidence"] == "project-resolved" {
			seenRelation = true
		}
	}
	if !seenDefinition || !seenRelation {
		t.Fatalf("live source_query matches=%#v", matches)
	}

	childDocument, err := sourceintelligence.OpenSourceDocument(ctx, childPath, sourceintelligence.OpenDocumentOptions{
		RequestedEncoding: "utf-8", MaxFileBytes: 1024 * 1024, MaxDecodedCharacters: 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("open child source for context contract: %v", err)
	}
	liveContext, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: map[string]any{
		"operation": "context", "paths": []string{root}, "language": "java", "encoding": "utf-8",
		"targets":     []any{map[string]any{"kind": "path", "path": childPath, "sourceFingerprint": childDocument.SourceFingerprint}},
		"budgetBytes": 4096, "bodyPolicy": "signatures-only", "maxItems": 8, "maxDepth": 2,
	}})
	if err != nil || liveContext == nil || liveContext.IsError {
		t.Fatalf("live source_query context result=%#v err=%v", liveContext, err)
	}
	contextStructured := contractStructuredMap(t, liveContext)
	contextResult := contractSchemaMap(t, contextStructured["context"])
	contextItems, ok := contextResult["items"].([]any)
	if !ok || len(contextItems) == 0 || contextResult["budgetBytes"] != float64(4096) {
		t.Fatalf("live source_query context=%#v", contextResult)
	}
	contextIndex := contractSchemaMap(t, contextStructured["index"])
	if contextIndex["staleness"] != "current" || contextIndex["generation"] == nil || contextIndex["fingerprint"] == nil {
		t.Fatalf("live source_query context index=%#v", contextStructured["index"])
	}

	tooManyPaths := make([]string, sourceQueryMaxInputPaths+1)
	for index := range tooManyPaths {
		tooManyPaths[index] = root
	}
	tooManyTargets := make([]any, 33)
	for index := range tooManyTargets {
		tooManyTargets[index] = sourceQueryPathSelector(root)
	}
	for _, args := range []map[string]any{
		{"operation": "search", "paths": tooManyPaths, "query": "Box", "mode": "structural"},
		{"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural", "maxFiles": sourceQueryMaxFiles + 1},
		{"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural", "maxResults": sourceQueryMaxResults + 1},
		{"operation": "relations", "paths": []string{root}, "relation": "dependencies", "subject": sourceQueryPathSelector(root), "maxNodes": sourceQueryMaxGraphNodes + 1},
		{"operation": "relations", "paths": []string{root}, "relation": "dependencies", "subject": sourceQueryPathSelector(root), "maxEdges": sourceQueryMaxGraphEdges + 1},
		{"operation": "relations", "paths": []string{root}, "relation": "dependencies", "subject": sourceQueryPathSelector(root), "maxDepth": sourceQueryMaxGraphDepth + 1},
		{"operation": "context", "paths": []string{root}, "targets": tooManyTargets, "budgetBytes": 4096},
		{"operation": "context", "paths": []string{root}, "targets": []any{sourceQueryPathSelector(root)}, "budgetBytes": sourceQueryMaxContextBytes + 1},
		{"operation": "context", "paths": []string{root}, "targets": []any{sourceQueryPathSelector(root)}, "budgetBytes": 4096, "maxItems": sourceQueryMaxContextItems + 1},
	} {
		assertSourceQueryCallErrorCode(t, ctx, session, args, handler.ErrCodeLimit)
	}

	for _, args := range []map[string]any{
		{"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural"},
		{"operation": "relations", "paths": []string{root}, "relation": "cycles"},
	} {
		assertSourceQueryCallSuccess(t, ctx, session, args)
	}
	for _, args := range []map[string]any{
		{"operation": "search", "paths": []string{root}, "query": "Box", "mode": "textual"},
		{"operation": "search", "paths": []string{root}, "query": "Box", "mode": "lexical"},
		{"operation": "relations", "paths": []string{root}, "relation": "callers"},
		{"operation": "relations", "paths": []string{root}, "relation": "callees"},
		{"operation": "relations", "paths": []string{root}, "relation": "overrides"},
	} {
		assertSourceQueryRejected(t, ctx, session, "source_query", args)
	}
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural",
		"index": map[string]any{"fingerprint": sourceQueryContractZeroDigest()},
	}, handler.ErrCodeConflict)
}

// TestSourceQueryPublicCursorPages verifies pagination through a real MCP client.
func TestSourceQueryPublicCursorPages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	for _, name := range []string{"ItemAlpha", "ItemBeta", "ItemGamma"} {
		if err := os.WriteFile(filepath.Join(root, name+".java"), []byte("package demo; public class "+name+" {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "source-query-public-cursor")
	args := map[string]any{"operation": "search", "paths": []string{root}, "query": "Item", "mode": "structural", "match": "prefix", "language": "java", "encoding": "utf-8", "maxResults": 1}
	var names []string
	for page := 0; page < 3; page++ {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: args})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("public cursor page %d: result=%#v err=%v", page, result, err)
		}
		payload := contractSchemaMap(t, contractStructuredMap(t, result)["search"])
		matches := contractObjectSlice(t, payload["matches"])
		if len(matches) != 1 {
			t.Fatalf("public page %d matches=%#v", page, matches)
		}
		name, _ := matches[0]["name"].(string)
		names = append(names, name)
		cursor, _ := payload["nextCursor"].(string)
		if page < 2 && cursor == "" {
			t.Fatalf("public page %d has no continuation", page)
		}
		if page == 2 && cursor != "" {
			t.Fatal("last page unexpectedly continued")
		}
		args["cursor"] = cursor
	}
	if !reflect.DeepEqual(names, []string{"ItemAlpha", "ItemBeta", "ItemGamma"}) {
		t.Fatalf("public cursor pages=%v", names)
	}
	args["cursor"] = "invalid!"
	assertSourceQueryCallErrorCode(t, ctx, session, args, handler.ErrCodeInvalidInput)
}

// TestSourceQueryConciseSearchPublicMCP qualifies paging through the real
// public schema, preserving one fingerprint-bound drilldown selector per hit.
func TestSourceQueryConciseSearchPublicMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	for _, name := range []string{"ItemAlpha", "ItemBeta", "ItemGamma"} {
		if err := os.WriteFile(filepath.Join(root, name+".java"), []byte("package demo; public class "+name+" {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "source-query-concise-search")
	args := map[string]any{
		"operation": "search", "presentation": "concise", "paths": []string{root},
		"query": "Item", "match": "prefix", "mode": "structural",
		"language": "java", "encoding": "utf-8", "maxResults": 1,
	}
	var names []string
	var cursor string
	for page := 0; page < 3; page++ {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: args})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("concise page %d: result=%#v err=%v", page, result, err)
		}
		payload := contractStructuredMap(t, result)
		if payload["index"] != nil || payload["operation"] != nil || payload["coordinateSystem"] != nil {
			t.Fatalf("concise page includes redundant index/inventory: %#v", payload)
		}
		coverage := contractSchemaMap(t, payload["coverage"])
		if coverage["analysisComplete"] != true || coverage["fileSelectionTruncated"] != false ||
			coverage["resultsTruncated"] != false || coverage["filesConsidered"] != nil {
			t.Fatalf("page is not incomplete analysis: %#v", coverage)
		}
		search := contractSchemaMap(t, payload["search"])
		files := contractObjectSlice(t, search["files"])
		if len(files) != 1 {
			t.Fatalf("concise page file groups=%#v", files)
		}
		file := files[0]
		assertSourceSymbolsHexDigest(t, "file.sourceFingerprint", file["sourceFingerprint"])
		if file["language"] != "java" || file["encoding"] != "utf-8" || file["path"] == nil {
			t.Fatalf("file identity missing: %#v", file)
		}
		matches := contractObjectSlice(t, file["matches"])
		if len(matches) != 1 || matches[0]["path"] != nil || matches[0]["sourceFingerprint"] != nil ||
			matches[0]["evidence"] != "structural" || matches[0]["kind"] != "class" {
			t.Fatalf("concise evidence or data duplication: %#v", matches)
		}
		assertSourceSymbolsHexDigest(t, "symbolId", matches[0]["symbolId"])
		if contractSchemaMap(t, matches[0]["range"])["start"] == nil {
			t.Fatalf("missing source coordinates: %#v", matches)
		}
		names = append(names, fmt.Sprint(matches[0]["symbol"]))
		cursor, _ = search["nextCursor"].(string)
		if page < 2 && cursor == "" {
			t.Fatalf("page %d missing continuation: %#v", page, search)
		}
		if page == 2 && cursor != "" {
			t.Fatalf("last page had unexpected cursor: %#v", search)
		}
		if page == 0 {
			var fallback map[string]any
			textEqual := false
			for _, part := range result.Content {
				if text, ok := part.(*mcp.TextContent); ok &&
					json.Unmarshal([]byte(text.Text), &fallback) == nil && reflect.DeepEqual(fallback, payload) {
					textEqual = true
				}
			}
			if !textEqual {
				t.Fatal("MCP content did not match structuredContent")
			}
			originalCursor := cursor
			delete(args, "presentation")
			args["cursor"] = originalCursor
			assertSourceQueryCallErrorCode(t, ctx, session, args, handler.ErrCodeInvalidInput)
			args["presentation"] = "concise"
		}
		args["cursor"] = cursor
	}
	if !reflect.DeepEqual(names, []string{"demo.ItemAlpha", "demo.ItemBeta", "demo.ItemGamma"}) {
		t.Fatalf("page order/identities changed: %v", names)
	}
	delete(args, "cursor")
	args["query"] = "Absent"
	empty, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: args})
	if err != nil || empty == nil || empty.IsError {
		t.Fatalf("empty result failed: %#v err=%v", empty, err)
	}
	emptySearch := contractSchemaMap(t, contractStructuredMap(t, empty)["search"])
	if len(contractObjectSlice(t, emptySearch["files"])) != 0 || emptySearch["nextCursor"] != nil {
		t.Fatalf("empty search incorrectly claimed matches or pages: %#v", emptySearch)
	}
	legacy, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Item", "mode": "structural",
		"match": "prefix", "language": "java", "encoding": "utf-8",
	}})
	if err != nil || legacy == nil || legacy.IsError {
		t.Fatalf("historical search changed: %#v err=%v", legacy, err)
	}
	historical := contractStructuredMap(t, legacy)
	if historical["index"] == nil || contractSchemaMap(t, historical["search"])["matches"] == nil {
		t.Fatalf("historical search contract was altered: %#v", historical)
	}
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "relations", "presentation": "concise", "paths": []string{root},
		"relation": "cycles",
	}, handler.ErrCodeInvalidInput)
}

func TestSourceQueryConciseSearchOverloadsAndReferenceSites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	path := filepath.Join(root, "Service.java")
	source := "package demo;\nclass Service {\n" +
		" void Work(int value) {}\n" +
		" private void Work(String value) {}\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(root, "Base.java")
	child := filepath.Join(root, "Child.java")
	if err := os.WriteFile(base, []byte("package demo; public class Base {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(child, []byte("package demo; public class Child extends Base {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "source-query-concise-evidence")
	call := func(query string) map[string]any {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: map[string]any{
			"operation": "search", "presentation": "concise",
			"paths": []string{root}, "query": query, "mode": "structural",
			"match": "exact", "language": "java", "encoding": "utf-8", "maxResults": 12,
		}})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("query %s: result=%#v err=%v", query, result, err)
		}
		return contractSchemaMap(t, contractStructuredMap(t, result)["search"])
	}
	workFiles := contractObjectSlice(t, call("Work")["files"])
	if len(workFiles) != 1 || workFiles[0]["path"] != path {
		t.Fatalf("overload grouping not file-scoped: %#v", workFiles)
	}
	matches := contractObjectSlice(t, workFiles[0]["matches"])
	if len(matches) != 2 {
		t.Fatalf("lost distinct overloads: %#v", matches)
	}
	lines, ids := map[float64]bool{}, map[string]bool{}
	for _, match := range matches {
		id, _ := match["symbolId"].(string)
		assertSourceSymbolsHexDigest(t, "overload.id", id)
		if ids[id] || match["symbol"] != "demo.Service.Work" {
			t.Fatalf("distinct overload identity lost: %#v", matches)
		}
		ids[id] = true
		site := contractSchemaMap(t, contractSchemaMap(t, match["range"])["start"])
		line, ok := site["line"].(float64)
		if !ok || lines[line] {
			t.Fatalf("overload source site collapsed: %#v", matches)
		}
		lines[line] = true
	}
	baseFiles := contractObjectSlice(t, call("Base")["files"])
	seenDecl, seenRef := false, false
	for _, file := range baseFiles {
		for _, match := range contractObjectSlice(t, file["matches"]) {
			if file["path"] == base && match["evidence"] == "structural" && match["symbolId"] != nil {
				seenDecl = true
			}
			if file["path"] == child && match["evidence"] == "project-resolved" && match["symbolId"] == nil {
				seenRef = true
				if contractSchemaMap(t, contractSchemaMap(t, match["range"])["start"])["line"] == nil {
					t.Fatal("source reference site was lost")
				}
			}
		}
	}
	if !seenDecl || !seenRef {
		t.Fatalf("declaration and reference site were conflated: %#v", baseFiles)
	}
}

func sourceQueryToolContract(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string) *mcp.Tool {
	t.Helper()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	for _, tool := range listed.Tools {
		if tool.Name == name {
			return tool
		}
	}
	t.Fatalf("R27 tool %s is absent", name)
	return nil
}

func assertSourceQueryReadOnlyTool(t *testing.T, tool *mcp.Tool) {
	t.Helper()
	if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint {
		t.Fatalf("%s is not read-only: %#v", tool.Name, tool.Annotations)
	}
	if tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
		t.Fatalf("%s is destructive: %#v", tool.Name, tool.Annotations)
	}
	if tool.Annotations.OpenWorldHint != nil && *tool.Annotations.OpenWorldHint {
		t.Fatalf("%s is open-world: %#v", tool.Name, tool.Annotations)
	}
}

func assertSourceQueryStrictSchema(t *testing.T, label string, schema map[string]any, required, properties []string) {
	t.Helper()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("%s schema = %#v, want strict object", label, schema)
	}
	assertSourceQueryStringSet(t, label+".required", schema["required"], required)
	got := make([]string, 0, len(contractSchemaProperties(t, schema)))
	for name := range contractSchemaProperties(t, schema) {
		got = append(got, name)
	}
	sort.Strings(got)
	want := append([]string(nil), properties...)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s properties = %v, want %v", label, got, want)
	}
}

func assertSourceQueryStringSet(t *testing.T, label string, value any, want []string) {
	t.Helper()
	got := contractStringSlice(t, value)
	sort.Strings(got)
	expected := append([]string(nil), want...)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("%s = %v, want %v", label, got, expected)
	}
}

func assertSourceQueryEnum(t *testing.T, label string, value any, want []string) {
	t.Helper()
	got := contractStringSlice(t, contractSchemaMap(t, value)["enum"])
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s enum = %v, want %v", label, got, want)
	}
}

func assertSourceQueryEvidenceFilter(t *testing.T, label string, value any) {
	t.Helper()
	schema := contractSchemaMap(t, value)
	assertSourceQueryEnum(t, label+".items", schema["items"], sourceQueryContractEvidenceValues)
}

func assertSourceQueryIndexBinding(t *testing.T, value any) {
	t.Helper()
	schema := contractSchemaMap(t, value)
	if schema["additionalProperties"] != false {
		t.Fatalf("index = %#v, want strict known fields", schema)
	}
	props := contractSchemaProperties(t, schema)
	assertSourceQueryEnum(t, "index.stalePolicy", props["stalePolicy"], []string{"reject", "allow"})
}

func assertSourceQuerySelector(t *testing.T, value any) {
	t.Helper()
	schema := contractSchemaMap(t, value)
	if schema["additionalProperties"] != false {
		t.Fatalf("selector = %#v, want strict known fields", schema)
	}
	props := contractSchemaProperties(t, schema)
	assertSourceQueryEnum(t, "selector.kind", props["kind"], []string{"symbol", "position", "path"})
	position := contractSchemaMap(t, props["position"])
	if position["additionalProperties"] != false {
		t.Fatalf("selector.position = %#v, want strict known fields", position)
	}
	keys := mapKeysAny(contractSchemaProperties(t, position))
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"column", "line"}) {
		t.Fatalf("selector.position properties = %v", keys)
	}
}

func assertSourceQueryRejected(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return
	}
	if result == nil || !result.IsError {
		t.Fatalf("%s accepted invalid arguments: result=%#v", name, result)
	}
	if result.Meta[handler.ErrorCodeMetaKey] == handler.ErrCodeUnsupported {
		t.Fatalf("%s invalid arguments reached the unsupported engine stub instead of being rejected: %#v", name, args)
	}
}

func assertSourceQueryCallSuccess(t *testing.T, ctx context.Context, session *mcp.ClientSession, args map[string]any) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: args})
	if err != nil {
		t.Fatalf("source_query transport error: %v", err)
	}
	if result == nil || result.IsError {
		t.Fatalf("source_query(%v) = %#v, want success", args["operation"], result)
	}
}

func assertSourceQueryCallErrorCode(t *testing.T, ctx context.Context, session *mcp.ClientSession, args map[string]any, want string) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: args})
	if err != nil {
		t.Fatalf("source_query transport error: %v", err)
	}
	if result == nil || !result.IsError || result.Meta[handler.ErrorCodeMetaKey] != want {
		t.Fatalf("source_query(%v) = %#v, want error code %s", args["operation"], result, want)
	}
}

func sourceQueryPathSelector(path string) map[string]any {
	return map[string]any{"kind": "path", "path": path, "sourceFingerprint": sourceQueryContractZeroDigest()}
}

func sourceQueryContractZeroDigest() string {
	return "0000000000000000000000000000000000000000000000000000000000000000"
}

func mapKeysAny(values map[string]any) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}
