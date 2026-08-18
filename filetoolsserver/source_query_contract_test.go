package filetoolsserver

import (
	"context"
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

var sourceQueryEvidenceValues = []string{"textual", "lexical", "structural", "scope-resolved", "project-resolved", "semantic"}

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
			"operation", "paths", "query", "mode", "match", "relation", "subject", "target", "targets", "budgetBytes", "bodyPolicy",
			"language", "encoding", "kinds", "includes", "excludes", "respectGitignore", "evidence", "maxFiles", "maxResults",
			"maxNodes", "maxEdges", "maxDepth", "maxItems", "index",
		},
	)
	props := contractSchemaProperties(t, schema)
	assertSourceQueryEnum(t, "source_query.operation", props["operation"], []string{"search", "relations", "context"})
	assertSourceQueryEnum(t, "source_query.mode", props["mode"], []string{"textual", "lexical", "structural"})
	assertSourceQueryEnum(t, "source_query.match", props["match"], []string{"exact", "prefix", "contains"})
	assertSourceQueryEnum(t, "source_query.relation", props["relation"], []string{
		"dependencies", "dependents", "references", "definitions", "inheritance", "implementations",
		"overrides", "callers", "callees", "trace", "impact", "cycles",
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
	} {
		assertSourceQueryCallErrorCode(t, ctx, session, args, handler.ErrCodeUnsupported)
	}
	assertSourceQueryCallErrorCode(t, ctx, session, map[string]any{
		"operation": "search", "paths": []string{root}, "query": "Box", "mode": "structural",
		"index": map[string]any{"fingerprint": sourceQueryContractZeroDigest()},
	}, handler.ErrCodeConflict)
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
	assertSourceQueryEnum(t, label+".items", schema["items"], sourceQueryEvidenceValues)
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
