package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
)

func TestHandleBackupStoreDisabledStatusAndActionRejection(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})

	result, output, err := h.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionStatus})
	if err != nil || result.IsError {
		t.Fatalf("disabled status result=%+v err=%v", result, err)
	}
	if output.Action != BackupStoreActionStatus || output.Enabled || output.State != BackupStoreStateDisabled {
		t.Fatalf("disabled status output = %#v", output)
	}

	for _, input := range []BackupStoreInput{
		{Action: BackupStoreActionList},
		{Action: BackupStoreActionRestorePreview, BackupID: strings.Repeat("a", 64)},
		{Action: BackupStoreActionRestoreApply, PreviewID: strings.Repeat("a", 64)},
		{Action: BackupStoreActionGCDryRun},
		{Action: BackupStoreActionGCApply, PreviewID: strings.Repeat("a", 64)},
	} {
		result, _, err = h.HandleBackupStore(context.Background(), nil, input)
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("disabled action %q result=%+v err=%v", input.Action, result, err)
		}
	}
}

func TestHandleBackupStoreReadOnlyActions(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	publicTarget := filepath.Join(fixture.publicRoot, "public.txt")
	capture := fixture.capture(t, publicTarget, "public backup bytes", true)

	result, status, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionStatus})
	if err != nil || result.IsError {
		t.Fatalf("status result=%+v err=%v", result, err)
	}
	if !status.Enabled || status.State != BackupStoreStateReady || status.Status == nil || status.Status.ManifestCount != 1 {
		t.Fatalf("status output = %#v", status)
	}

	result, listed, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action: BackupStoreActionList,
		Limit:  10,
	})
	if err != nil || result.IsError {
		t.Fatalf("list result=%+v err=%v", result, err)
	}
	if len(listed.Items) != 1 || listed.Items[0].BackupID != capture.Manifest.BackupID || listed.Items[0].TargetPath != publicTarget {
		t.Fatalf("list output = %#v", listed)
	}

	result, inspected, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionInspect,
		BackupID: capture.Manifest.BackupID,
	})
	if err != nil || result.IsError {
		t.Fatalf("inspect result=%+v err=%v", result, err)
	}
	if inspected.Manifest == nil || !inspected.Manifest.ObjectVerified || inspected.Manifest.BackupID != capture.Manifest.BackupID {
		t.Fatalf("inspect output = %#v", inspected)
	}
	encoded, err := json.Marshal(inspected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "public backup bytes") || strings.Contains(string(encoded), fixture.store.Root()) {
		t.Fatalf("inspect exposed bytes or internal path: %s", encoded)
	}

	result, audited, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:    BackupStoreActionAudit,
		AuditMode: string(backupstore.AuditQuick),
	})
	if err != nil || result.IsError {
		t.Fatalf("audit result=%+v err=%v", result, err)
	}
	if audited.Audit == nil || !audited.Audit.Healthy || audited.Audit.ManifestCount != 1 {
		t.Fatalf("audit output = %#v", audited)
	}
}

func TestHandleBackupStoreFiltersTargetsByCurrentAuthorization(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	publicCapture := fixture.capture(t, filepath.Join(fixture.publicRoot, "public.txt"), "public", false)
	outsideRoot := filepath.Join(filepath.Dir(fixture.publicRoot), "outside")
	if err := os.Mkdir(outsideRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outsideCapture := fixture.capture(t, filepath.Join(outsideRoot, "outside.txt"), "outside secret", false)

	result, listed, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action: BackupStoreActionList,
		Limit:  10,
	})
	if err != nil || result.IsError {
		t.Fatalf("list result=%+v err=%v", result, err)
	}
	if len(listed.Items) != 1 || listed.Items[0].BackupID != publicCapture.Manifest.BackupID {
		t.Fatalf("authorization-filtered list = %#v", listed)
	}

	result, _, err = fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:   BackupStoreActionInspect,
		BackupID: outsideCapture.Manifest.BackupID,
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("outside inspect result=%+v err=%v", result, err)
	}

	result, _, err = fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action:     BackupStoreActionList,
		TargetPath: outsideCapture.Manifest.TargetPath,
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("outside target filter result=%+v err=%v", result, err)
	}
}

func TestHandleBackupStoreCursorBindsAuthorizationSnapshot(t *testing.T) {
	fixture := newBackupStoreHandlerFixture(t)
	fixture.capture(t, filepath.Join(fixture.publicRoot, "first.txt"), "first", false)
	fixture.capture(t, filepath.Join(fixture.publicRoot, "second.txt"), "second", false)

	_, firstPage, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action: BackupStoreActionList,
		Limit:  1,
	})
	if err != nil || firstPage.NextCursor == "" {
		t.Fatalf("first page = %#v err=%v", firstPage, err)
	}
	newRoot := filepath.Join(filepath.Dir(fixture.publicRoot), "new-root")
	if err := os.Mkdir(newRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	fixture.handler.UpdateAllowedDirectories([]string{newRoot})

	result, _, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{
		Action: BackupStoreActionList,
		Limit:  1,
		Cursor: firstPage.NextCursor,
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("authorization-swapped cursor result=%+v err=%v", result, err)
	}
}

func TestBackupStoreInputStrictUnionAndOutputLimit(t *testing.T) {
	for _, raw := range []string{
		`{"action":"status","unknown":true}`,
		`{"action":"status"}{}`,
	} {
		var input BackupStoreInput
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			t.Fatalf("strict input accepted %s", raw)
		}
	}

	fixture := newBackupStoreHandlerFixture(t)
	invalid := []BackupStoreInput{
		{},
		{Action: "unknown"},
		{Action: BackupStoreActionStatus, BackupID: strings.Repeat("a", 64)},
		{Action: BackupStoreActionList, BackupID: strings.Repeat("a", 64)},
		{Action: BackupStoreActionInspect},
		{Action: BackupStoreActionInspect, BackupID: strings.Repeat("a", 64), Limit: 1},
		{Action: BackupStoreActionAudit, Cursor: "cursor"},
		{Action: BackupStoreActionGCDryRun, BackupID: strings.Repeat("a", 64)},
		{Action: BackupStoreActionGCApply},
		{Action: BackupStoreActionGCApply, PreviewID: strings.Repeat("a", 64), Limit: 1},
	}
	for _, input := range invalid {
		result, _, err := fixture.handler.HandleBackupStore(context.Background(), nil, input)
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid input %#v result=%+v err=%v", input, result, err)
		}
	}

	limitedConfig := config.LoadFromEnvironment(func(string) string { return "" })
	limitedConfig.Limits.MaxOutputBytes = 1
	limited := NewHandler(
		[]string{fixture.publicRoot},
		WithConfig(limitedConfig),
		WithProtectedDirectories([]string{fixture.store.Root()}),
		WithBackupStore(fixture.store),
	)
	result, _, err := limited.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionStatus})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output-limited status result=%+v err=%v", result, err)
	}
}

type backupStoreHandlerFixture struct {
	handler    *Handler
	store      *backupstore.Store
	publicRoot string
}

func newBackupStoreHandlerFixture(t *testing.T) backupStoreHandlerFixture {
	t.Helper()
	base := canonicalHandlerTestDir(t)
	publicRoot := filepath.Join(base, "public")
	storeRoot := filepath.Join(base, "store")
	if err := os.Mkdir(publicRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := backupstore.Open(backupstore.Options{Directory: storeRoot})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return backupStoreHandlerFixture{
		handler: NewHandler(
			[]string{publicRoot},
			WithProtectedDirectories([]string{store.Root()}),
			WithBackupStore(store),
		),
		store:      store,
		publicRoot: publicRoot,
	}
}

func (fixture backupStoreHandlerFixture) capture(t *testing.T, path, content string, pinned bool) backupstore.CaptureResult {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := fixture.store.Capture(context.Background(), backupstore.CaptureRequest{
		TargetPath:      path,
		SourceOperation: backupstore.SourceOperationEdit,
		Pinned:          pinned,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
