package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestBackupStoreGCDryRunApplyAndReplay(t *testing.T) {
	fixture := newBackupGCHandlerFixture(t)
	result, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionGCDryRun})
	if err != nil || result.IsError || preview.GC == nil {
		t.Fatalf("gc dry run result=%+v output=%+v err=%v", result, preview, err)
	}
	if len(preview.GC.PreviewID) != 64 || preview.GC.State != BackupStoreGCStatePrepared || preview.GC.ManifestCount != 2 ||
		preview.GC.ObjectCount != 2 || len(preview.GC.Manifests) != 2 || len(preview.GC.Objects) != 2 || preview.GC.Applied {
		t.Fatalf("gc preview=%#v", preview.GC)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fixture.publicRoot) || strings.Contains(string(encoded), fixture.store.Root()) {
		t.Fatalf("GC preview exposed a target or store path: %s", encoded)
	}
	if fixture.store.Index().ManifestCount != 3 {
		t.Fatalf("dry run changed store: %#v", fixture.store.Index())
	}

	result, applied, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionGCApply,
		PreviewID: preview.GC.PreviewID,
	})
	if err != nil || result.IsError || applied.GC == nil {
		t.Fatalf("gc apply result=%+v output=%+v err=%v", result, applied, err)
	}
	if !applied.GC.Applied || applied.GC.State != BackupStoreGCStateApplied || applied.GC.ManifestsRemoved != 2 ||
		applied.GC.ObjectsRemoved != 2 || applied.GC.BytesReclaimed == 0 || applied.GC.TrashEntriesRemaining != 0 ||
		applied.GC.PreviousGeneration == applied.GC.Generation {
		t.Fatalf("gc apply=%#v", applied.GC)
	}
	if fixture.store.Index().ManifestCount != 1 || fixture.store.Index().ObjectCount != 1 {
		t.Fatalf("post-GC index=%#v", fixture.store.Index())
	}

	replay, _, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionGCApply,
		PreviewID: preview.GC.PreviewID,
	})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("gc replay result=%+v err=%v", replay, err)
	}
}

func TestBackupStoreGCApplyRejectsStaleGenerationAndConsumesPreview(t *testing.T) {
	fixture := newBackupGCHandlerFixture(t)
	_, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionGCDryRun})
	if err != nil || preview.GC == nil {
		t.Fatal(err)
	}
	other := filepath.Join(fixture.publicRoot, "other.txt")
	if err := os.WriteFile(other, []byte("new generation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.Capture(context.Background(), backupstore.CaptureRequest{
		TargetPath:      other,
		SourceOperation: backupstore.SourceOperationEdit,
		Pinned:          true,
	}); err != nil {
		t.Fatal(err)
	}
	result, output, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionGCApply,
		PreviewID: preview.GC.PreviewID,
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict || output.GC == nil || output.GC.Applied {
		t.Fatalf("stale apply result=%+v output=%+v err=%v", result, output, err)
	}
	if fixture.store.Index().ManifestCount != 4 {
		t.Fatalf("stale apply changed store: %#v", fixture.store.Index())
	}
	replay, _, _ := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionGCApply,
		PreviewID: preview.GC.PreviewID,
	})
	if !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale preview remained replayable: %+v", replay)
	}
}

func TestBackupStoreGCDryRunOutputLimitLeavesNoCapability(t *testing.T) {
	fixture := newBackupGCHandlerFixture(t)
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	cfg.Limits.MaxOutputBytes = 1
	limited := NewHandler([]string{fixture.publicRoot}, WithConfig(cfg), WithBackupStore(fixture.store))
	result, _, err := limited.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionGCDryRun})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("limited dry run result=%+v err=%v", result, err)
	}
	if limited.gcPreviews.len() != 0 || fixture.store.Index().ManifestCount != 3 {
		t.Fatalf("limited dry run retained authority or changed store")
	}
}

func TestBackupStoreGCApplyPreservesPartialProgressInStructuredError(t *testing.T) {
	fixture := newBackupGCHandlerFixture(t)
	wrapper := &gcApplyErrorStore{Store: fixture.store}
	h := NewHandler([]string{fixture.publicRoot}, WithBackupStore(wrapper))
	_, preview, err := h.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionGCDryRun})
	if err != nil || preview.GC == nil {
		t.Fatal(err)
	}
	result, output, err := h.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionGCApply,
		PreviewID: preview.GC.PreviewID,
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeIO || output.GC == nil ||
		!output.GC.Applied || output.GC.State != BackupStoreGCStatePartial || output.GC.ManifestsRemoved != 1 ||
		output.GC.ObjectsRemoved != 1 || output.GC.TrashEntriesRemaining != 1 {
		t.Fatalf("partial apply result=%+v output=%+v err=%v", result, output, err)
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), fixture.store.Root()) || strings.Contains(backupStoreResultText(result), fixture.store.Root()) {
		t.Fatalf("partial error exposed store path: output=%s text=%q", encoded, backupStoreResultText(result))
	}
}

type backupGCHandlerFixture struct {
	handler    *Handler
	store      *backupstore.Store
	publicRoot string
	target     string
}

func newBackupGCHandlerFixture(t *testing.T) backupGCHandlerFixture {
	t.Helper()
	base := canonicalHandlerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	storeRoot := filepath.Join(base, "store")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	limits := backupstore.Limits{MaxVersionsPerTarget: 8, RetentionDays: 365}
	store, err := backupstore.Open(backupstore.Options{
		Directory:                storeRoot,
		PublicAllowedDirectories: []string{publicRoot},
		Limits:                   limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(publicRoot, "target.txt")
	for _, content := range []string{"first", "second", "third"} {
		if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Capture(context.Background(), backupstore.CaptureRequest{
			TargetPath:      target,
			SourceOperation: backupstore.SourceOperationEdit,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	limits.MaxVersionsPerTarget = 1
	store, err = backupstore.Open(backupstore.Options{
		Directory:                storeRoot,
		PublicAllowedDirectories: []string{publicRoot},
		Limits:                   limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return backupGCHandlerFixture{
		handler:    NewHandler([]string{publicRoot}, WithBackupStore(store)),
		store:      store,
		publicRoot: publicRoot,
		target:     target,
	}
}

type gcApplyErrorStore struct {
	*backupstore.Store
}

func (store *gcApplyErrorStore) ApplyGC(_ context.Context, plan backupstore.GCPlan) (backupstore.GCResult, error) {
	return backupstore.GCResult{
		PreviousGeneration:    plan.Generation,
		Generation:            strings.Repeat("e", 64),
		ManifestsRemoved:      1,
		ObjectsRemoved:        1,
		TrashCleanupFailures:  1,
		TrashEntriesRemaining: 1,
	}, operation.New(operation.KindFilesystem, "injected partial GC failure")
}

func backupStoreResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	text, _ := result.Content[0].(*mcp.TextContent)
	if text == nil {
		return ""
	}
	return text.Text
}
