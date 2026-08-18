package filetoolsserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestConnectorWorkflowAcrossHeterogeneousProject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := canonicalServerTestDir(t)
	sentinel := "unrelated-connector-source-sentinel"
	files := map[string]string{
		"base.ts":       "export class Base {}\n",
		"child.ts":      "import { Base } from \"./base\";\nexport class Child extends Base {}\n",
		"Service.scala": "package demo\nclass ScalaBox:\n  def run(value: Int): Int = value\n",
		"model.js.flow": "/* @flow */\nexport type ID = string;\nexport class FlowBox { run(value: ID): ID { return value; } }\n",
		"secret.rb":     "class Secret\n  def hidden\n    \"" + sentinel + "\"\n  end\nend\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	session := connectTestClient(t, ctx, NewServer([]string{root}, nil, nil), "r27-phase17-heterogeneous-workflow")

	outlineResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "outline", "paths": []string{root}, "encoding": "utf-8", "includeSignatures": true,
		"maxFiles": 16, "maxSymbols": 256,
	})
	outline := contractStructuredMap(t, outlineResult)
	if outline["coverageComplete"] != true || outline["filesParsed"] != float64(len(files)) {
		t.Fatalf("heterogeneous outline=%#v", outline)
	}
	fileEvidence := map[string]map[string]any{}
	for _, file := range contractObjectSlice(t, outline["files"]) {
		path, _ := file["path"].(string)
		fileEvidence[path] = file
	}
	for _, path := range []string{filepath.Join(root, "child.ts"), filepath.Join(root, "Service.scala"), filepath.Join(root, "model.js.flow")} {
		if fileEvidence[path] == nil {
			t.Fatalf("outline missing file evidence for %s: %#v", path, fileEvidence)
		}
	}

	findResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "find", "paths": []string{root}, "query": "ScalaBox", "match": "exact", "encoding": "utf-8",
		"maxFiles": 16, "maxSymbols": 16,
	})
	found := contractObjectSlice(t, contractStructuredMap(t, findResult)["symbols"])
	if len(found) != 1 || found[0]["name"] != "ScalaBox" || found[0]["language"] != "scala" {
		t.Fatalf("heterogeneous find=%#v", found)
	}
	scalaPath := filepath.Join(root, "Service.scala")
	scalaID, _ := found[0]["id"].(string)
	scalaFingerprint, _ := fileEvidence[scalaPath]["sourceFingerprint"].(string)
	if scalaID == "" || scalaFingerprint == "" {
		t.Fatalf("ScalaBox identity incomplete: symbol=%#v file=%#v", found[0], fileEvidence[scalaPath])
	}

	showResult := callSourceSymbolsContract(t, ctx, session, map[string]any{
		"operation": "show", "path": scalaPath, "symbolId": scalaID, "sourceFingerprint": scalaFingerprint,
		"language": "scala", "encoding": "utf-8", "maxBytes": 4096,
	})
	shown := contractSchemaMap(t, contractStructuredMap(t, showResult)["show"])
	showText, _ := shown["text"].(string)
	if !strings.Contains(showText, "class ScalaBox") || strings.Contains(showText, sentinel) {
		t.Fatalf("heterogeneous show text=%q", showText)
	}

	childPath := filepath.Join(root, "child.ts")
	childFingerprint, _ := fileEvidence[childPath]["sourceFingerprint"].(string)
	relationsResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: map[string]any{
		"operation": "relations", "paths": []string{root}, "relation": "dependencies",
		"subject":  map[string]any{"kind": "path", "path": childPath, "sourceFingerprint": childFingerprint},
		"maxFiles": 16, "maxResults": 16, "maxNodes": 32, "maxEdges": 64, "maxDepth": 4,
	}})
	if err != nil || relationsResult == nil || relationsResult.IsError {
		t.Fatalf("heterogeneous relations result=%#v err=%v", relationsResult, err)
	}
	relations := contractStructuredMap(t, relationsResult)
	relationPayload := contractSchemaMap(t, relations["relations"])
	edges := contractObjectSlice(t, relationPayload["relations"])
	if len(edges) != 1 {
		t.Fatalf("heterogeneous dependency relations=%#v", relationPayload)
	}
	encodedRelations, err := json.Marshal(relations)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedRelations), sentinel) {
		t.Fatalf("relations leaked unrelated source content: %s", encodedRelations)
	}

	contextResult, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "source_query", Arguments: map[string]any{
		"operation": "context", "paths": []string{root},
		"targets":     []any{map[string]any{"kind": "symbol", "path": scalaPath, "symbolId": scalaID, "sourceFingerprint": scalaFingerprint}},
		"budgetBytes": 4096, "bodyPolicy": "prefer", "encoding": "utf-8", "maxFiles": 16, "maxItems": 8, "maxDepth": 2,
	}})
	if err != nil || contextResult == nil || contextResult.IsError {
		t.Fatalf("heterogeneous context result=%#v err=%v", contextResult, err)
	}
	contextOutput := contractStructuredMap(t, contextResult)
	contextPayload := contractSchemaMap(t, contextOutput["context"])
	items := contractObjectSlice(t, contextPayload["items"])
	if len(items) == 0 || contextPayload["budgetBytes"] != float64(4096) {
		t.Fatalf("heterogeneous context=%#v", contextPayload)
	}
	encodedContext, err := json.Marshal(contextOutput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encodedContext), sentinel) {
		t.Fatalf("context leaked unrelated source content: %s", encodedContext)
	}
	index := contractSchemaMap(t, contextOutput["index"])
	if index["staleness"] != "current" || index["generation"] == nil || index["fingerprint"] == nil {
		t.Fatalf("heterogeneous context index=%#v", index)
	}
}
