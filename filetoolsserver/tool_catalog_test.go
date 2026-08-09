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
	}

	editTool := byName["edit_file"]
	inputSchema, err := json.Marshal(editTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal edit_file input schema: %v", err)
	}
	outputSchema, err := json.Marshal(editTool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal edit_file output schema: %v", err)
	}
	if !bytes.Contains(inputSchema, []byte(`"backupPolicy"`)) {
		t.Fatalf("edit_file input schema does not expose backupPolicy: %s", inputSchema)
	}
	if !bytes.Contains(outputSchema, []byte(`"backupPolicy"`)) || !bytes.Contains(outputSchema, []byte(`"backupId"`)) {
		t.Fatalf("edit_file output schema does not expose backup metadata: %s", outputSchema)
	}

	packageTool := byName["patch_package"]
	packageInputSchema, err := json.Marshal(packageTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal patch_package input schema: %v", err)
	}
	packageOutputSchema, err := json.Marshal(packageTool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal patch_package output schema: %v", err)
	}
	if !bytes.Contains(packageInputSchema, []byte(`"backupPolicy"`)) {
		t.Fatalf("patch_package input schema does not expose backupPolicy: %s", packageInputSchema)
	}
	for _, field := range [][]byte{[]byte(`"backupPolicy"`), []byte(`"backupCount"`), []byte(`"backupId"`)} {
		if !bytes.Contains(packageOutputSchema, field) {
			t.Fatalf("patch_package output schema does not expose %s: %s", field, packageOutputSchema)
		}
	}

	backupTool := byName["backup_store"]
	backupInputSchema, err := json.Marshal(backupTool.InputSchema)
	if err != nil {
		t.Fatalf("marshal backup_store input schema: %v", err)
	}
	backupOutputSchema, err := json.Marshal(backupTool.OutputSchema)
	if err != nil {
		t.Fatalf("marshal backup_store output schema: %v", err)
	}
	for _, field := range [][]byte{[]byte(`"backupId"`), []byte(`"previewId"`)} {
		if !bytes.Contains(backupInputSchema, field) {
			t.Fatalf("backup_store input schema does not expose %s: %s", field, backupInputSchema)
		}
	}
	for _, field := range [][]byte{
		[]byte(`"restore"`), []byte(`"safetyBackupId"`), []byte(`"actualFingerprint"`),
		[]byte(`"gc"`), []byte(`"reclaimableBytes"`), []byte(`"trashEntriesRemaining"`),
	} {
		if !bytes.Contains(backupOutputSchema, field) {
			t.Fatalf("backup_store output schema does not expose %s: %s", field, backupOutputSchema)
		}
	}
	if backupTool.Annotations == nil || backupTool.Annotations.ReadOnlyHint ||
		backupTool.Annotations.DestructiveHint == nil || !*backupTool.Annotations.DestructiveHint {
		t.Fatalf("backup_store annotations do not reflect restore/GC mutation: %#v", backupTool.Annotations)
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
