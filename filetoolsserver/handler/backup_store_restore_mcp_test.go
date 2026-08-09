package handler

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestBackupStoreRestorePreservesSafetyEvidenceThroughMCPError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	base := canonicalHandlerTestDir(t)
	root := filepath.Join(base, "public")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{
		Directory:                filepath.Join(base, "store"),
		PublicAllowedDirectories: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	captured, err := store.Capture(ctx, backupstore.CaptureRequest{TargetPath: target, SourceOperation: backupstore.SourceOperationEdit})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("current"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root}, WithBackupStore(store))
	originalCommit := h.restoreCommitReplacement
	h.restoreCommitReplacement = func(staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		changed, commitErr := originalCommit(staged, options)
		if commitErr != nil {
			return changed, commitErr
		}
		return changed, errors.New("injected post-commit failure")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "restore-error-test", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "backup_store"}, Wrap(nil, "backup_store", h.HandleBackupStore))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "restore-error-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	previewResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "backup_store",
		Arguments: map[string]any{"action": BackupStoreActionRestorePreview, "backupId": captured.Manifest.BackupID},
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("preview result=%+v err=%v", previewResult, err)
	}
	var preview BackupStoreOutput
	decodeRestoreMCPOutput(t, previewResult.StructuredContent, &preview)
	if preview.Restore == nil || len(preview.Restore.PreviewID) != 64 {
		t.Fatalf("preview output=%+v", preview)
	}

	applyResult, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "backup_store",
		Arguments: map[string]any{"action": BackupStoreActionRestoreApply, "previewId": preview.Restore.PreviewID},
	})
	if err != nil || !applyResult.IsError {
		t.Fatalf("apply result=%+v err=%v", applyResult, err)
	}
	var output BackupStoreOutput
	decodeRestoreMCPOutput(t, applyResult.StructuredContent, &output)
	if output.Restore == nil || output.Restore.State != BackupStoreRestoreStateRestored || !output.Restore.Applied ||
		len(output.Restore.SafetyBackupID) != 64 || output.Restore.ActualFingerprint != captured.Manifest.ContentFingerprint {
		t.Fatalf("apply output=%+v", output)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "original" {
		t.Fatalf("restored target=%q err=%v", data, err)
	}
	encoded, err := json.Marshal(applyResult.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || containsRestoreInternalPath(string(encoded), store.Root()) {
		t.Fatalf("restore output exposed internal store state: %s", encoded)
	}
}

func decodeRestoreMCPOutput(t *testing.T, content any, output *BackupStoreOutput) {
	t.Helper()
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, output); err != nil {
		t.Fatal(err)
	}
}

func containsRestoreInternalPath(encoded, storeRoot string) bool {
	return storeRoot != "" && len(encoded) >= len(storeRoot) && stringContains(encoded, storeRoot)
}

func stringContains(value, part string) bool {
	for index := 0; index+len(part) <= len(value); index++ {
		if value[index:index+len(part)] == part {
			return true
		}
	}
	return false
}
