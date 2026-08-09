package handler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestEditPreviewRequiredBackupCapturesApprovedPreState(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("alpha\n")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}

	previewResult, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action:       editActionPreview,
		Path:         target,
		Edits:        []EditOperation{{OldText: "alpha", NewText: "omega"}},
		BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil || previewResult.IsError {
		t.Fatalf("preview result=%+v output=%+v err=%v", previewResult, preview, err)
	}
	if preview.BackupPolicy != editBackupPolicyRequired || preview.BackupID != "" {
		t.Fatalf("preview backup metadata=%+v", preview)
	}
	if store.Index().ManifestCount != 0 {
		t.Fatal("preview created a persistent backup")
	}
	assertEditBackupBytes(t, target, original)

	applyResult, applied, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action:    editActionApply,
		PreviewID: preview.PreviewID,
	})
	if err != nil || applyResult.IsError {
		t.Fatalf("apply result=%+v output=%+v err=%v", applyResult, applied, err)
	}
	if !applied.Applied || applied.BackupPolicy != editBackupPolicyRequired || len(applied.BackupID) != 64 {
		t.Fatalf("apply backup metadata=%+v", applied)
	}
	if store.Index().ManifestCount != 1 {
		t.Fatalf("manifest count=%d, want 1", store.Index().ManifestCount)
	}
	inspected, err := store.Inspect(context.Background(), applied.BackupID, backupstore.InspectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !inspected.ObjectVerified || inspected.Manifest.TargetPath != target ||
		inspected.Manifest.SourceOperation != backupstore.SourceOperationEdit ||
		inspected.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData(original) {
		t.Fatalf("captured manifest=%+v", inspected)
	}
	assertEditBackupBytes(t, target, []byte("omega\n"))
}

func TestEditPreviewRequiredBackupRejectsUnavailableStoreAndStrictUnion(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	for _, input := range []EditFileInput{
		{Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired},
		{Action: editActionDirect, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired},
		{Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: "optional"},
		{Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: "Required"},
		{Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: " required "},
		{Action: editActionApply, PreviewID: strings64("a"), BackupPolicy: editBackupPolicyRequired},
	} {
		result, _, err := h.HandleEditFile(context.Background(), nil, input)
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("input=%+v result=%+v err=%v", input, result, err)
		}
	}
	assertEditBackupBytes(t, target, []byte("alpha"))
}

func TestEditRequiredBackupNeedsCaptureAuthorityNotOnlyReadAccess(t *testing.T) {
	base := canonicalHandlerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{
		Directory:                filepath.Join(base, "backup-store"),
		PublicAllowedDirectories: []string{publicRoot},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	target := filepath.Join(publicRoot, "target.txt")
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	readerOnly := readOnlyEditBackupStore{BackupStoreReader: store}
	h := NewHandler([]string{publicRoot}, WithBackupStore(readerOnly))

	statusResult, status, err := h.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionStatus})
	if err != nil || statusResult.IsError || !status.Enabled {
		t.Fatalf("read-only status result=%+v output=%+v err=%v", statusResult, status, err)
	}
	previewResult, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil || !previewResult.IsError || previewResult.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("reader-only required preview result=%+v err=%v", previewResult, err)
	}
	assertEditBackupBytes(t, target, []byte("alpha"))
}

func TestEditPreviewOmittedAndNoOpRequiredPoliciesCreateNoBackup(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, ordinary, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ordinaryResult, ordinaryApplied, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: ordinary.PreviewID})
	if err != nil || ordinaryResult.IsError || ordinaryApplied.BackupPolicy != "" || ordinaryApplied.BackupID != "" {
		t.Fatalf("ordinary apply result=%+v output=%+v err=%v", ordinaryResult, ordinaryApplied, err)
	}
	if store.Index().ManifestCount != 0 {
		t.Fatal("omitted policy created a backup")
	}

	_, noOp, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action:       editActionPreview,
		Path:         target,
		Edits:        []EditOperation{{OldText: "omega", NewText: "omega"}},
		BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	noOpResult, noOpApplied, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: noOp.PreviewID})
	if err != nil || noOpResult.IsError || !noOpApplied.Applied || noOpApplied.Changed || noOpApplied.BackupID != "" {
		t.Fatalf("no-op apply result=%+v output=%+v err=%v", noOpResult, noOpApplied, err)
	}
	if noOpApplied.BackupPolicy != editBackupPolicyRequired || store.Index().ManifestCount != 0 {
		t.Fatalf("no-op policy/count output=%+v count=%d", noOpApplied, store.Index().ManifestCount)
	}
}

func TestEditRequiredBackupFailureAndOutputLimitPreventMutation(t *testing.T) {
	t.Run("object limit", func(t *testing.T) {
		h, store, target := newEditBackupFixture(t, backupstore.Limits{MaxObjectBytes: 1})
		if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
			Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit || output.BackupID != "" {
			t.Fatalf("limited result=%+v output=%+v err=%v", result, output, err)
		}
		assertEditBackupBytes(t, target, []byte("alpha"))
		if store.Index().ManifestCount != 0 {
			t.Fatal("failed capture committed a manifest")
		}
		terminal, _, _ := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
		if !terminal.IsError || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
			t.Fatalf("failed apply did not consume preview: %+v", terminal)
		}
	})

	t.Run("output limit", func(t *testing.T) {
		h, store, target := newEditBackupFixture(t, backupstore.Limits{})
		if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
			Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
		})
		if err != nil {
			t.Fatal(err)
		}
		h.config.Limits.MaxOutputBytes = 1
		result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit || output.BackupID != "" {
			t.Fatalf("output-limited result=%+v output=%+v err=%v", result, output, err)
		}
		assertEditBackupBytes(t, target, []byte("alpha"))
		if store.Index().ManifestCount != 0 {
			t.Fatal("output-limited apply created a backup")
		}
	})
}

func TestEditApplyFailureAfterDurableBackupReturnsBackupID(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("alpha")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	h.replaceFile = func(string, []byte, filesystem.ReplaceOptions) error { return errors.New("injected write failure") }

	result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
	if err != nil || !result.IsError || output.Applied || len(output.BackupID) != 64 || output.BackupPolicy != editBackupPolicyRequired {
		t.Fatalf("failed apply result=%+v output=%+v err=%v", result, output, err)
	}
	assertEditBackupBytes(t, target, original)
	if store.Index().ManifestCount != 1 {
		t.Fatalf("manifest count=%d, want 1", store.Index().ManifestCount)
	}
	inspected, inspectErr := store.Inspect(context.Background(), output.BackupID, backupstore.InspectOptions{})
	if inspectErr != nil || !inspected.ObjectVerified || inspected.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData(original) {
		t.Fatalf("durable backup=%+v err=%v", inspected, inspectErr)
	}
}

func TestEditApplyContinuesAfterDerivedIndexFailureWithDurableManifest(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	wrapped := &editBackupStoreWrapper{Store: store}
	wrapped.capture = func(ctx context.Context, request backupstore.CaptureRequest) (backupstore.CaptureResult, error) {
		result, err := store.Capture(ctx, request)
		if err != nil {
			return result, err
		}
		return result, errors.New("injected derived index persistence failure")
	}
	h.backupStore = wrapped
	h.backupCapture = wrapped

	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
	if err != nil || result.IsError || !output.Applied || len(output.BackupID) != 64 {
		t.Fatalf("apply result=%+v output=%+v err=%v", result, output, err)
	}
	assertEditBackupBytes(t, target, []byte("omega"))
}

func TestEditRequiredBackupPreservesUTF16BytesAndReadOnlyMetadata(t *testing.T) {
	t.Run("utf16 bom and crlf", func(t *testing.T) {
		h, store, target := newEditBackupFixture(t, backupstore.Limits{})
		original := encodeUTF16LEWithBOM(t, "alpha\r\nbeta\r\n")
		if err := os.WriteFile(target, original, 0o600); err != nil {
			t.Fatal(err)
		}
		_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
			Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "beta", NewText: "gamma"}}, BackupPolicy: editBackupPolicyRequired,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
		if err != nil || result.IsError || len(output.BackupID) != 64 {
			t.Fatalf("apply result=%+v output=%+v err=%v", result, output, err)
		}
		inspected, err := store.Inspect(context.Background(), output.BackupID, backupstore.InspectOptions{})
		if err != nil || inspected.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData(original) {
			t.Fatalf("backup=%+v err=%v", inspected, err)
		}
		want := encodeUTF16LEWithBOM(t, "alpha\r\ngamma\r\n")
		assertEditBackupBytes(t, target, want)
	})

	t.Run("read only mode", func(t *testing.T) {
		h, store, target := newEditBackupFixture(t, backupstore.Limits{})
		if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(target, 0o400); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}
		force := true
		_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
			Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, ForceWritable: &force, BackupPolicy: editBackupPolicyRequired,
		})
		if err != nil {
			t.Fatal(err)
		}
		result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
		if err != nil || result.IsError || !output.ReadOnlyCleared || len(output.BackupID) != 64 {
			t.Fatalf("apply result=%+v output=%+v err=%v", result, output, err)
		}
		inspected, err := store.Inspect(context.Background(), output.BackupID, backupstore.InspectOptions{})
		if err != nil || inspected.Manifest.OriginalMode != uint32(info.Mode().Perm()) {
			t.Fatalf("backup mode=%#o want=%#o err=%v", inspected.Manifest.OriginalMode, info.Mode().Perm(), err)
		}
	})
}

func TestEditRequiredBackupCancellationOccursBeforeCapture(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, output, err := h.HandleEditFile(ctx, nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeCancelled || output.BackupID != "" {
		t.Fatalf("cancel result=%+v output=%+v err=%v", result, output, err)
	}
	if store.Index().ManifestCount != 0 {
		t.Fatal("cancelled apply created a backup")
	}
	assertEditBackupBytes(t, target, []byte("alpha"))
}

func TestEditRequiredBackupDetectsTargetChangeAfterDurableCapture(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte("alpha")
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	wrapped := &editBackupStoreWrapper{Store: store}
	wrapped.capture = func(ctx context.Context, request backupstore.CaptureRequest) (backupstore.CaptureResult, error) {
		result, err := store.Capture(ctx, request)
		if err != nil {
			return result, err
		}
		if writeErr := os.WriteFile(target, []byte("external"), 0o600); writeErr != nil {
			return result, writeErr
		}
		return result, nil
	}
	h.backupStore = wrapped
	h.backupCapture = wrapped

	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
	if err != nil || !result.IsError || len(output.BackupID) != 64 || output.Applied {
		t.Fatalf("changed target result=%+v output=%+v err=%v", result, output, err)
	}
	assertEditBackupBytes(t, target, []byte("external"))
	inspected, inspectErr := store.Inspect(context.Background(), output.BackupID, backupstore.InspectOptions{})
	if inspectErr != nil || inspected.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData(original) {
		t.Fatalf("captured pre-state=%+v err=%v", inspected, inspectErr)
	}
}

func TestEditRequiredBackupCapturesExactCP1251Bytes(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	original := []byte{
		0xCD, 0xE5, 0xE2, 0xE0, 0xEB, 0xE8, 0xE4, 0xE5, 0xED,
		0x20, 0xF2, 0xE8, 0xEF, 0x2E,
	}
	if err := os.WriteFile(target, original, 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action:       editActionPreview,
		Path:         target,
		Encoding:     "cp1251",
		Edits:        []EditOperation{{OldText: "Невалиден тип.", NewText: "Типът е невалиден."}},
		BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}
	result, output, err := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
	if err != nil || result.IsError || len(output.BackupID) != 64 {
		t.Fatalf("apply result=%+v output=%+v err=%v", result, output, err)
	}
	inspected, err := store.Inspect(context.Background(), output.BackupID, backupstore.InspectOptions{})
	if err != nil || inspected.Manifest.ContentFingerprint != filesystem.FingerprintRegularFileData(original) || inspected.Manifest.ObjectBytes != int64(len(original)) {
		t.Fatalf("captured CP1251 manifest=%+v err=%v", inspected, err)
	}
	want := []byte{
		0xD2, 0xE8, 0xEF, 0xFA, 0xF2,
		0x20, 0xE5, 0x20,
		0xED, 0xE5, 0xE2, 0xE0, 0xEB, 0xE8, 0xE4, 0xE5, 0xED,
		0x2E,
	}
	assertEditBackupBytes(t, target, want)
}

func TestEditRequiredBackupConcurrentApplyHasOneManifest(t *testing.T) {
	h, store, target := newEditBackupFixture(t, backupstore.Limits{})
	if err := os.WriteFile(target, []byte("alpha"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, preview, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Action: editActionPreview, Path: target, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}}, BackupPolicy: editBackupPolicyRequired,
	})
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	outcomes := make(chan bool, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, _, callErr := h.HandleEditFile(context.Background(), nil, EditFileInput{Action: editActionApply, PreviewID: preview.PreviewID})
			outcomes <- callErr == nil && !result.IsError
		}()
	}
	close(start)
	wg.Wait()
	close(outcomes)
	successes := 0
	for success := range outcomes {
		if success {
			successes++
		}
	}
	if successes != 1 || store.Index().ManifestCount != 1 {
		t.Fatalf("successes=%d manifests=%d", successes, store.Index().ManifestCount)
	}
}

type readOnlyEditBackupStore struct {
	BackupStoreReader
}

type editBackupStoreWrapper struct {
	*backupstore.Store
	capture func(context.Context, backupstore.CaptureRequest) (backupstore.CaptureResult, error)
}

func (store *editBackupStoreWrapper) Capture(ctx context.Context, request backupstore.CaptureRequest) (backupstore.CaptureResult, error) {
	return store.capture(ctx, request)
}

func newEditBackupFixture(t *testing.T, limits backupstore.Limits) (*Handler, *backupstore.Store, string) {
	t.Helper()
	base := canonicalHandlerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	storeRoot := filepath.Join(base, "backup-store")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{
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
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	return NewHandler([]string{publicRoot}, WithConfig(cfg), WithBackupStore(store)), store, filepath.Join(publicRoot, "target.txt")
}

func assertEditBackupBytes(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s bytes=%q, want %q", filepath.Base(path), got, want)
	}
}
