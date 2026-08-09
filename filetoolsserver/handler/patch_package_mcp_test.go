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

func TestPatchPackagePartialCommitPreservesStructuredContentThroughMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("beta"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	server := mcp.NewServer(&mcp.Implementation{Name: "partial-test", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "patch_package"}, Wrap(nil, "patch_package", h.HandlePatchPackage))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "partial-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{
		{path: first, oldText: "alpha", newText: "omega"},
		{path: second, oldText: "beta", newText: "gamma"},
	})
	dryRun, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "patch_package",
		Arguments: map[string]any{
			"action":   patchPackageActionDryRun,
			"manifest": manifest,
		},
	})
	if err != nil || dryRun.IsError {
		t.Fatalf("dryRun=%+v err=%v", dryRun, err)
	}
	var prepared PatchPackageOutput
	decodeStructuredPatchPackageOutput(t, dryRun.StructuredContent, &prepared)
	if len(prepared.PreviewID) != 64 {
		t.Fatalf("previewId=%q", prepared.PreviewID)
	}

	originalCommit := h.patchPackageCommitReplacement
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		if index == 1 {
			return false, errors.New("injected commit failure")
		}
		return originalCommit(index, staged, options)
	}
	applied, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name: "patch_package",
		Arguments: map[string]any{
			"action":    patchPackageActionApply,
			"previewId": prepared.PreviewID,
		},
	})
	if err != nil || !applied.IsError || applied.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
	var partial PatchPackageOutput
	decodeStructuredPatchPackageOutput(t, applied.StructuredContent, &partial)
	if !partial.PartialCommit || partial.CommittedCount != 1 || partial.UnchangedCount != 1 || partial.UnknownCount != 0 {
		t.Fatalf("partial structured output=%+v", partial)
	}
	if partial.Results[0].State != patchPackageStateCommitted || partial.Results[1].State != patchPackageStateUnchanged {
		t.Fatalf("partial states=%+v", partial.Results)
	}
}

func TestPatchPackageRequiredBackupsPreserveStructuredContentThroughMCP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	base := canonicalHandlerTestDir(t)
	root := filepath.Join(base, "public")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{
		Directory:                filepath.Join(base, "backup-store"),
		PublicAllowedDirectories: []string{root},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := filepath.Join(root, "first.txt")
	second := filepath.Join(root, "second.txt")
	if err := os.WriteFile(first, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("beta"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root}, WithBackupStore(store))
	server := mcp.NewServer(&mcp.Implementation{Name: "backup-package-test", Version: "test"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "patch_package"}, Wrap(nil, "patch_package", h.HandlePatchPackage))
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "backup-package-client", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{
		{path: first, oldText: "alpha", newText: "omega"},
		{path: second, oldText: "beta", newText: "gamma"},
	})
	manifest.BackupPolicy = editBackupPolicyRequired
	dryRun, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "patch_package",
		Arguments: map[string]any{"action": patchPackageActionDryRun, "manifest": manifest},
	})
	if err != nil || dryRun.IsError {
		t.Fatalf("dryRun=%+v err=%v", dryRun, err)
	}
	var prepared PatchPackageOutput
	decodeStructuredPatchPackageOutput(t, dryRun.StructuredContent, &prepared)
	if prepared.BackupPolicy != editBackupPolicyRequired || prepared.BackupCount != 0 || len(prepared.PreviewID) != 64 {
		t.Fatalf("prepared output=%+v", prepared)
	}

	originalCommit := h.patchPackageCommitReplacement
	h.patchPackageCommitReplacement = func(index int, staged *filesystem.StagedReplacement, options filesystem.ReplaceOptions) (bool, error) {
		if index == 1 {
			return false, errors.New("injected commit failure")
		}
		return originalCommit(index, staged, options)
	}
	applied, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
		Name:      "patch_package",
		Arguments: map[string]any{"action": patchPackageActionApply, "previewId": prepared.PreviewID},
	})
	if err != nil || !applied.IsError || applied.Meta[ErrorCodeMetaKey] != ErrCodePartialCommit {
		t.Fatalf("apply=%+v err=%v", applied, err)
	}
	var partial PatchPackageOutput
	decodeStructuredPatchPackageOutput(t, applied.StructuredContent, &partial)
	if partial.BackupPolicy != editBackupPolicyRequired || partial.BackupCount != 2 || !partial.PartialCommit {
		t.Fatalf("partial backup output=%+v", partial)
	}
	for index := range partial.Results {
		if len(partial.Results[index].BackupID) != 64 {
			t.Fatalf("result %d backup ID=%q", index, partial.Results[index].BackupID)
		}
	}
	if store.Index().ManifestCount != 2 {
		t.Fatalf("manifest count=%d, want 2", store.Index().ManifestCount)
	}
}

func decodeStructuredPatchPackageOutput(t *testing.T, content any, output *PatchPackageOutput) {
	t.Helper()
	encoded, err := json.Marshal(content)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, output); err != nil {
		t.Fatal(err)
	}
}
