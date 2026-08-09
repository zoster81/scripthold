package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestPersistentBackupLifecycleCaptureRestoreGCAndReopen(t *testing.T) {
	ctx := context.Background()
	base := canonicalHandlerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	storeRoot := filepath.Join(base, "backup-store")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(publicRoot, "target.txt")
	if err := os.WriteFile(target, []byte("alpha\n"), 0o600); err != nil {
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
	t.Cleanup(func() { _ = store.Close() })
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	h := NewHandler([]string{publicRoot}, WithConfig(cfg), WithBackupStore(store))

	alphaBackupID := applyRequiredLifecycleEdit(t, ctx, h, target, "alpha", "beta")
	_ = applyRequiredLifecycleEdit(t, ctx, h, target, "beta", "gamma")
	assertEditBackupBytes(t, target, []byte("gamma\n"))
	if store.Index().ManifestCount != 2 {
		t.Fatalf("edit capture manifest count=%d, want 2", store.Index().ManifestCount)
	}

	previewResult, restorePreview, err := h.HandleBackupStore(ctx, nil, BackupStoreInput{
		Action:   BackupStoreActionRestorePreview,
		BackupID: alphaBackupID,
	})
	if err != nil || previewResult.IsError || restorePreview.Restore == nil {
		t.Fatalf("restore preview result=%+v output=%+v err=%v", previewResult, restorePreview, err)
	}
	applyResult, restored, err := h.HandleBackupStore(ctx, nil, BackupStoreInput{
		Action:    BackupStoreActionRestoreApply,
		PreviewID: restorePreview.Restore.PreviewID,
	})
	if err != nil || applyResult.IsError || restored.Restore == nil || !restored.Restore.Applied || len(restored.Restore.SafetyBackupID) != 64 {
		t.Fatalf("restore apply result=%+v output=%+v err=%v", applyResult, restored, err)
	}
	assertEditBackupBytes(t, target, []byte("alpha\n"))
	if store.Index().ManifestCount != 3 {
		t.Fatalf("restore manifest count=%d, want 3", store.Index().ManifestCount)
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
	h = NewHandler([]string{publicRoot}, WithConfig(cfg), WithBackupStore(store))

	dryRunResult, gcPreview, err := h.HandleBackupStore(ctx, nil, BackupStoreInput{Action: BackupStoreActionGCDryRun})
	if err != nil || dryRunResult.IsError || gcPreview.GC == nil || gcPreview.GC.ManifestCount != 2 {
		t.Fatalf("GC dry run result=%+v output=%+v err=%v", dryRunResult, gcPreview, err)
	}
	gcApplyResult, gcApplied, err := h.HandleBackupStore(ctx, nil, BackupStoreInput{
		Action:    BackupStoreActionGCApply,
		PreviewID: gcPreview.GC.PreviewID,
	})
	if err != nil || gcApplyResult.IsError || gcApplied.GC == nil || !gcApplied.GC.Applied || gcApplied.GC.ManifestsRemoved != 2 {
		t.Fatalf("GC apply result=%+v output=%+v err=%v", gcApplyResult, gcApplied, err)
	}
	assertEditBackupBytes(t, target, []byte("alpha\n"))
	if store.Index().ManifestCount != 1 || store.Index().ObjectCount != 1 {
		t.Fatalf("post-GC index=%#v", store.Index())
	}

	audit, err := store.Audit(ctx, backupstore.AuditOptions{Mode: backupstore.AuditFull})
	if err != nil || !audit.Healthy || len(audit.Issues) != 0 {
		t.Fatalf("post-GC audit=%#v err=%v", audit, err)
	}
	listed, err := store.List(ctx, backupstore.ListOptions{Limit: 10})
	if err != nil || len(listed.Items) != 1 {
		t.Fatalf("post-GC list=%#v err=%v", listed, err)
	}
	survivor, err := store.Inspect(ctx, listed.Items[0].BackupID, backupstore.InspectOptions{})
	if err != nil || survivor.Manifest.SourceOperation != backupstore.SourceOperationRestore ||
		survivor.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData([]byte("gamma\n")) {
		t.Fatalf("surviving safety backup=%#v err=%v", survivor, err)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = backupstore.Open(backupstore.Options{
		Directory:                storeRoot,
		PublicAllowedDirectories: []string{publicRoot},
		Limits:                   limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	finalAudit, err := store.Audit(ctx, backupstore.AuditOptions{Mode: backupstore.AuditFull})
	if err != nil || !finalAudit.Healthy || finalAudit.ManifestCount != 1 || finalAudit.ObjectCount != 1 || len(finalAudit.Issues) != 0 {
		t.Fatalf("reopened audit=%#v err=%v", finalAudit, err)
	}
	assertEditBackupBytes(t, target, []byte("alpha\n"))
}

func applyRequiredLifecycleEdit(t *testing.T, ctx context.Context, h *Handler, target, oldText, newText string) string {
	t.Helper()
	previewResult, preview, err := h.HandleEditFile(ctx, nil, EditFileInput{
		Action:       editActionPreview,
		Path:         target,
		Edits:        []EditOperation{{OldText: oldText, NewText: newText}},
		BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("edit preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	applyResult, applied, err := h.HandleEditFile(ctx, nil, EditFileInput{
		Action:    editActionApply,
		PreviewID: preview.PreviewID,
	})
	if err != nil || applyResult.IsError || !applied.Applied || len(applied.BackupID) != 64 {
		t.Fatalf("edit apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	return applied.BackupID
}
