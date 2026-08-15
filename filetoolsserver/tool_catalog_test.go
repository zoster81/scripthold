package filetoolsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/toolcatalog"
)

func expectedToolCount() int {
	return len(toolcatalog.All())
}

func TestRuntimeToolsMatchAuthoritativeCatalog(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := NewServer([]string{t.TempDir()}, nil, nil).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "catalog-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })

	result, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	definitions := toolcatalog.All()
	if got, want := len(result.Tools), len(definitions); got != want {
		t.Fatalf("runtime tool count = %d, want %d", got, want)
	}

	byName := make(map[string]*mcp.Tool, len(result.Tools))
	for _, tool := range result.Tools {
		if _, exists := byName[tool.Name]; exists {
			t.Fatalf("runtime returned duplicate tool %q", tool.Name)
		}
		byName[tool.Name] = tool
	}
	serializedCatalog, err := json.Marshal(result.Tools)
	if err != nil {
		t.Fatalf("marshal connector catalog: %v", err)
	}
	// The connector rejects oversized function catalogs. This byte ceiling remains
	// conservative while accounting for the strict R25 source_symbols and compact
	// R27 source_query contracts in the exact runtime tools/list payload.
	const maxConnectorCatalogBytes = 24_000
	t.Logf("connector catalog = %d bytes (budget %d)", len(serializedCatalog), maxConnectorCatalogBytes)
	if got := len(serializedCatalog); got > maxConnectorCatalogBytes {
		t.Fatalf("connector catalog = %d bytes, exceeds budget %d", got, maxConnectorCatalogBytes)
	}

	if byName["write_file"] != nil {
		t.Fatal("runtime must not expose the ambiguous write_file compatibility alias")
	}
	if byName["write_whole_file"] == nil {
		t.Fatal("runtime must expose write_whole_file")
	}

	for _, definition := range definitions {
		tool, ok := byName[definition.Name]
		if !ok {
			t.Fatalf("runtime is missing catalog tool %q", definition.Name)
		}
		if tool.Description != definition.Description {
			t.Errorf("tool %q description diverged from catalog", definition.Name)
		}
		wantAnnotations := &mcp.ToolAnnotations{
			Title:           definition.Title,
			ReadOnlyHint:    definition.Annotations.ReadOnlyHint,
			IdempotentHint:  definition.Annotations.IdempotentHint,
			DestructiveHint: definition.Annotations.DestructiveHint,
			OpenWorldHint:   definition.Annotations.OpenWorldHint,
		}
		if !reflect.DeepEqual(tool.Annotations, wantAnnotations) {
			t.Errorf("tool %q annotations = %#v, want %#v", definition.Name, tool.Annotations, wantAnnotations)
		}
		inputSchema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s input schema: %v", definition.Name, err)
		}
		var inputObject map[string]any
		if err := json.Unmarshal(inputSchema, &inputObject); err != nil {
			t.Fatalf("decode %s input schema: %v", definition.Name, err)
		}
		if _, ok := inputObject["properties"].(map[string]any); !ok {
			t.Errorf("tool %q input schema must contain an object properties map: %s", definition.Name, inputSchema)
		}
		outputSchema, err := json.Marshal(tool.OutputSchema)
		if err != nil {
			t.Fatalf("marshal %s output schema: %v", definition.Name, err)
		}
		if string(outputSchema) != `{"type":"object"}` {
			t.Errorf("tool %q output schema must remain connector-compatible: %s", definition.Name, outputSchema)
		}
	}

	editTool := byName["edit_file"]
	inputSchema, err := json.Marshal(editTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal edit_file input schema: %v", err)
	}
	if !bytes.Contains(inputSchema, []byte(`"backupPolicy"`)) {
		t.Fatalf("edit_file input schema does not expose backupPolicy: %s", inputSchema)
	}

	packageTool := byName["patch_package"]
	packageInputSchema, err := json.Marshal(packageTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal patch_package input schema: %v", err)
	}
	if !bytes.Contains(packageInputSchema, []byte(`"backupPolicy"`)) {
		t.Fatalf("patch_package input schema does not expose backupPolicy: %s", packageInputSchema)
	}

	backupTool := byName["backup_store"]
	backupInputSchema, err := json.Marshal(backupTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal backup_store input schema: %v", err)
	}
	for _, field := range [][]byte{[]byte(`"backupId"`), []byte(`"otherBackupId"`)} {
		if !bytes.Contains(backupInputSchema, field) {
			t.Fatalf("backup_store input schema does not expose %s: %s", field, backupInputSchema)
		}
	}
	if bytes.Contains(backupInputSchema, []byte(`"previewId"`)) {
		t.Fatalf("read-only backup_store schema unexpectedly exposes previewId: %s", backupInputSchema)
	}
	if backupTool.Annotations == nil || !backupTool.Annotations.ReadOnlyHint ||
		(backupTool.Annotations.DestructiveHint != nil && *backupTool.Annotations.DestructiveHint) {
		t.Fatalf("backup_store annotations are not read-only: %#v", backupTool.Annotations)
	}

	for _, name := range []string{"task_run", "task_list", "task_get", "task_logs", "task_cancel"} {
		if byName[name] == nil {
			t.Fatalf("task catalog is missing %q", name)
		}
	}
	taskRunSchema, err := json.Marshal(byName["task_run"].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range [][]byte{[]byte(`"idempotencyKey"`), []byte(`"lockKeys"`), []byte(`"maxRuntimeSeconds"`)} {
		if !bytes.Contains(taskRunSchema, field) {
			t.Fatalf("task_run schema is missing %s: %s", field, taskRunSchema)
		}
	}
}
