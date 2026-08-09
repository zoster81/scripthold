package filetoolsserver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
)

func TestBuildServerTreatsTypedNilBackupStoreAsDisabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var store *backupstore.Store
	server := BuildServer(ServerOptions{
		Version:            "disabled-backup-architecture-test",
		AllowedDirectories: []string{t.TempDir()},
		BackupStore:        store,
		Config:             config.Load(),
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})
	session := connectTestClient(t, ctx, server, "disabled-backup-architecture")

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "backup_store",
		Arguments: map[string]any{"action": "status"},
	})
	if err != nil || result.IsError {
		t.Fatalf("disabled backup status result=%#v err=%v", result, err)
	}
	var output struct {
		Action  string `json:"action"`
		Enabled bool   `json:"enabled"`
		State   string `json:"state"`
	}
	decodeStructuredOutput(t, result.StructuredContent, &output)
	if output.Action != "status" || output.Enabled || output.State != "disabled" {
		t.Fatalf("disabled backup status output=%+v", output)
	}
}

func TestBuildServerPreservesJSONTextInStringArguments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	path := filepath.Join(root, "state.json")
	server := BuildServer(ServerOptions{
		Version:            "json-string-architecture-test",
		AllowedDirectories: []string{root},
		Config:             config.Load(),
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})
	session := connectTestClient(t, ctx, server, "json-string-architecture")

	content := "{\"ok\":true}\n"
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "write_file",
		Arguments: map[string]any{
			"path": path, "content": content, "encoding": "utf-8",
		},
	})
	if err != nil || result.IsError {
		t.Fatalf("write JSON text result=%#v err=%v", result, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("written bytes=%q, want %q", data, content)
	}
}

func TestBuildServerWiresAndProtectsConfiguredBackupStore(t *testing.T) {
	// This integration test performs several complete MCP preview/apply and
	// backup/restore cycles. Keep it bounded, but allow for package-level CPU
	// and filesystem contention in shared CI runners.
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	base := canonicalServerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	storeRoot := filepath.Join(base, "backup-store")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{Directory: storeRoot})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := filepath.Join(publicRoot, "target.txt")
	if err := os.WriteFile(target, []byte("backup source bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	capture, err := store.Capture(ctx, backupstore.CaptureRequest{
		TargetPath:      target,
		SourceOperation: backupstore.SourceOperationEdit,
	})
	if err != nil {
		t.Fatal(err)
	}

	server := BuildServer(ServerOptions{
		Version:            "backup-architecture-test",
		AllowedDirectories: []string{publicRoot},
		BackupStore:        store,
		Config:             config.Load(),
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})
	session := connectTestClient(t, ctx, server, "backup-architecture")

	for _, call := range []struct {
		name      string
		arguments map[string]any
	}{
		{name: "status", arguments: map[string]any{"action": "status"}},
		{name: "list", arguments: map[string]any{"action": "list", "limit": 10}},
		{name: "inspect", arguments: map[string]any{"action": "inspect", "backupId": capture.Manifest.BackupID}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "backup_store", Arguments: call.arguments})
		if err != nil || result.IsError {
			t.Fatalf("backup_store %s result=%#v err=%v", call.name, result, err)
		}
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(encoded, []byte(storeRoot)) || bytes.Contains(encoded, []byte("backup source bytes")) {
			t.Fatalf("backup_store %s exposed internal data: %s", call.name, encoded)
		}
	}

	editPreview, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "edit_file",
		Arguments: map[string]any{
			"action":       "preview",
			"path":         target,
			"edits":        []map[string]any{{"oldText": "backup", "newText": "edited"}},
			"backupPolicy": "required",
		},
	})
	if err != nil || editPreview.IsError {
		t.Fatalf("edit preview result=%#v err=%v", editPreview, err)
	}
	var previewOutput struct {
		PreviewID    string `json:"previewId"`
		BackupPolicy string `json:"backupPolicy"`
	}
	decodeStructuredOutput(t, editPreview.StructuredContent, &previewOutput)
	if len(previewOutput.PreviewID) != 64 || previewOutput.BackupPolicy != "required" {
		t.Fatalf("edit preview output=%+v", previewOutput)
	}
	editApply, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "edit_file",
		Arguments: map[string]any{"action": "apply", "previewId": previewOutput.PreviewID},
	})
	if err != nil || editApply.IsError {
		t.Fatalf("edit apply result=%#v err=%v", editApply, err)
	}
	var applyOutput struct {
		Applied      bool   `json:"applied"`
		BackupPolicy string `json:"backupPolicy"`
		BackupID     string `json:"backupId"`
	}
	decodeStructuredOutput(t, editApply.StructuredContent, &applyOutput)
	if !applyOutput.Applied || applyOutput.BackupPolicy != "required" || len(applyOutput.BackupID) != 64 || applyOutput.BackupID == capture.Manifest.BackupID {
		t.Fatalf("edit apply output=%+v", applyOutput)
	}
	backupInspect, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "backup_store",
		Arguments: map[string]any{"action": "inspect", "backupId": applyOutput.BackupID},
	})
	if err != nil || backupInspect.IsError {
		t.Fatalf("backup inspect result=%#v err=%v", backupInspect, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "edited source bytes" {
		t.Fatalf("edited target=%q err=%v", data, err)
	}

	restorePreview, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "backup_store",
		Arguments: map[string]any{"action": "restorePreview", "backupId": capture.Manifest.BackupID},
	})
	if err != nil || restorePreview.IsError {
		t.Fatalf("restore preview result=%#v err=%v", restorePreview, err)
	}
	var restorePrepared struct {
		Restore struct {
			PreviewID          string `json:"previewId"`
			CurrentFingerprint string `json:"currentFingerprint"`
			ResultFingerprint  string `json:"resultFingerprint"`
		} `json:"restore"`
	}
	decodeStructuredOutput(t, restorePreview.StructuredContent, &restorePrepared)
	if len(restorePrepared.Restore.PreviewID) != 64 || len(restorePrepared.Restore.CurrentFingerprint) != 64 || len(restorePrepared.Restore.ResultFingerprint) != 64 {
		t.Fatalf("restore preview output=%+v", restorePrepared)
	}
	restoreApply, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "backup_store",
		Arguments: map[string]any{"action": "restoreApply", "previewId": restorePrepared.Restore.PreviewID},
	})
	if err != nil || restoreApply.IsError {
		t.Fatalf("restore apply result=%#v err=%v", restoreApply, err)
	}
	var restored struct {
		Restore struct {
			Applied        bool   `json:"applied"`
			State          string `json:"state"`
			SafetyBackupID string `json:"safetyBackupId"`
		} `json:"restore"`
	}
	decodeStructuredOutput(t, restoreApply.StructuredContent, &restored)
	if !restored.Restore.Applied || restored.Restore.State != "restored" || len(restored.Restore.SafetyBackupID) != 64 {
		t.Fatalf("restore apply output=%+v", restored)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "backup source bytes" {
		t.Fatalf("restored target=%q err=%v", data, err)
	}
	safetyInspect, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "backup_store",
		Arguments: map[string]any{"action": "inspect", "backupId": restored.Restore.SafetyBackupID},
	})
	if err != nil || safetyInspect.IsError {
		t.Fatalf("safety backup inspect result=%#v err=%v", safetyInspect, err)
	}

	denied, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "read_text_file",
		Arguments: map[string]any{"path": filepath.Join(storeRoot, "store.json")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !denied.IsError {
		t.Fatalf("ordinary tool accessed backup store: %#v", denied)
	}
}

func TestBuildServerUsesExplicitVersionAndSharedProcessRoots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	roots := []string{t.TempDir(), t.TempDir()}
	server := BuildServer(ServerOptions{
		Version:            "architecture-test",
		AllowedDirectories: roots,
		Config:             config.Load(),
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})

	first := connectTestClient(t, ctx, server, "first")
	second := connectTestClient(t, ctx, server, "second")

	for name, session := range map[string]*mcp.ClientSession{"first": first, "second": second} {
		init := session.InitializeResult()
		if init == nil || init.ServerInfo == nil || init.ServerInfo.Version != "architecture-test" {
			t.Fatalf("%s session server version = %#v", name, init)
		}
	}

	firstTools := toolNames(t, ctx, first)
	secondTools := toolNames(t, ctx, second)
	if !reflect.DeepEqual(firstTools, secondTools) {
		t.Fatalf("tool catalogs differ: first=%v second=%v", firstTools, secondTools)
	}

	firstRoots := allowedDirectories(t, ctx, first)
	secondRoots := allowedDirectories(t, ctx, second)
	if !reflect.DeepEqual(firstRoots, secondRoots) {
		t.Fatalf("sessions observed different process roots: first=%v second=%v", firstRoots, secondRoots)
	}
	if len(firstRoots) != len(roots) {
		t.Fatalf("allowed root count = %d, want %d", len(firstRoots), len(roots))
	}
}

func TestBuildServerUsesProvidedConfiguration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	root := t.TempDir()
	cfg := config.Load()
	cfg.DefaultEncoding = "cp1251"
	server := BuildServer(ServerOptions{
		Version:            "configuration-test",
		AllowedDirectories: []string{root},
		Config:             cfg,
		EnableClientRoots:  false,
		LifecycleContext:   ctx,
	})
	session := connectTestClient(t, ctx, server, "configuration")

	path := filepath.Join(root, "configured-default.txt")
	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "write_file",
		Arguments: map[string]any{
			"path":    path,
			"content": "Привет",
		},
	})
	if err != nil {
		t.Fatalf("call write_file: %v", err)
	}
	if result.IsError {
		t.Fatalf("write_file returned an error: %#v", result.Content)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read configured output: %v", err)
	}
	want := []byte{0xcf, 0xf0, 0xe8, 0xe2, 0xe5, 0xf2}
	if !bytes.Equal(data, want) {
		t.Fatalf("configured output bytes = %x, want cp1251 %x", data, want)
	}
}

func decodeStructuredOutput(t *testing.T, value any, output any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, output); err != nil {
		t.Fatalf("decode structured output %s: %v", data, err)
	}
}

func connectTestClient(t *testing.T, ctx context.Context, server *mcp.Server, name string) *mcp.ClientSession {
	t.Helper()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server %s: %v", name, err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client %s: %v", name, err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func toolNames(t *testing.T, ctx context.Context, session *mcp.ClientSession) []string {
	t.Helper()
	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make([]string, 0, len(result.Tools))
	for _, tool := range result.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	return names
}

func allowedDirectories(t *testing.T, ctx context.Context, session *mcp.ClientSession) []string {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_allowed_directories"})
	if err != nil {
		t.Fatalf("call list_allowed_directories: %v", err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var output struct {
		Directories []string `json:"directories"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		t.Fatalf("decode structured content %s: %v", data, err)
	}
	return output.Directories
}
