package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/filesystempackage"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestConfirmationOwnerIndexReservation(t *testing.T) {
	index := newConfirmationOwnerIndex(2)
	first := strings.Repeat("a", 64)
	second := strings.Repeat("b", 64)
	third := strings.Repeat("c", 64)
	if err := index.reserve(first, "edit_file"); err != nil {
		t.Fatal(err)
	}
	if err := index.reserve(first, "markdown"); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("collision error: %v", err)
	}
	if owner, ok := index.resolve(first); !ok || owner != "edit_file" {
		t.Fatalf("owner %q, %v", owner, ok)
	}
	if err := index.reserve(second, "markdown"); err != nil {
		t.Fatal(err)
	}
	if err := index.reserve(third, "backup_gc"); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("capacity error: %v", err)
	}
	if index.forget(first, "markdown") {
		t.Fatal("wrong owner removed reservation")
	}
	if index.forget(strings.Repeat("f", 64), "") {
		t.Fatal("missing ID was reported as removed")
	}
	if !index.forget(first, "edit_file") {
		t.Fatal("correct owner did not remove reservation")
	}
	if _, ok := index.resolve(first); ok {
		t.Fatal("forgotten ID still resolves")
	}
	if err := index.reserve(third, "backup_gc"); err != nil {
		t.Fatal(err)
	}
}

func TestConfirmationResultPreservesLargeIntegers(t *testing.T) {
	type countResult struct {
		Count uint64 `json:"count"`
	}
	result, projected, err := confirmationResult("filesystem_package", &mcp.CallToolResult{}, countResult{Count: 9007199254740993}, nil)
	if err != nil || result == nil {
		t.Fatalf("projection failed: %v", err)
	}
	number, ok := projected["count"].(json.Number)
	if !ok || number.String() != "9007199254740993" {
		t.Fatalf("integer precision lost: %#v", projected["count"])
	}
}

func TestConfirmDispatcherConcurrentSingleWinner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "concurrent.txt")
	if err := os.WriteFile(path, []byte("start"), 0600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	prepared, preview, err := h.HandleEditFilePreview(context.Background(), nil, EditFilePreviewInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "start", NewText: "done"}},
	})
	if err != nil || prepared.IsError {
		t.Fatalf("preview %+v %v", prepared, err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make(chan string, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			result, output, callErr := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.PreviewID})
			if callErr != nil {
				codes <- "go-error"
				return
			}
			if !result.IsError && output["applied"] == true {
				codes <- "success"
				return
			}
			if result.IsError && result.Meta[ErrorCodeMetaKey] == ErrCodeConflict {
				codes <- "conflict"
				return
			}
			codes <- "unexpected"
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	counts := map[string]int{}
	for code := range codes {
		counts[code]++
	}
	if counts["success"] != 1 || counts["conflict"] != 1 {
		t.Fatalf("outcomes %+v", counts)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "done" {
		t.Fatalf("bytes=%q err=%v", data, err)
	}
}

func TestConfirmDispatcherRestartAndUnknownFailClosed(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	preview, err := h.markdownPreviews.putEdit(preparedMarkdownEdit{resultUTF8: []byte("test")})
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewHandler([]string{root})
	result, _, err := restarted.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.id})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("restart %+v %v", result, err)
	}
	if _, ok := h.confirmOwners.resolve(preview.id); !ok {
		t.Fatal("original handler unexpectedly consumed token")
	}
}

func TestConfirmDispatcherBackupRestore(t *testing.T) {
	fixture := newBackupRestoreFixture(t, backupstore.Limits{})
	backupID := fixture.captureOriginal(t, "original payload")
	if err := os.WriteFile(fixture.target, []byte("changed payload"), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionRestorePreview, BackupID: backupID})
	if err != nil || prepared.IsError || preview.Restore == nil {
		t.Fatalf("preview %+v %v", prepared, err)
	}
	result, output, err := fixture.handler.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.Restore.PreviewID})
	if err != nil || result.IsError || output["operationKind"] != "backup_restore" {
		t.Fatalf("confirm %+v %+v %v", result, output, err)
	}
	restore, ok := output["restore"].(map[string]any)
	if !ok || restore["applied"] != true || restore["state"] != BackupStoreRestoreStateRestored {
		t.Fatalf("restore output %+v", output)
	}
	if data, err := os.ReadFile(fixture.target); err != nil || string(data) != "original payload" {
		t.Fatalf("restored %q %v", data, err)
	}
	if fixture.store.Index().ManifestCount != 2 {
		t.Fatalf("missing safety backup")
	}
}

func TestConfirmDispatcherBackupGCAndPartialError(t *testing.T) {
	t.Run("complete", func(t *testing.T) {
		fixture := newBackupGCHandlerFixture(t)
		prepared, preview, err := fixture.handler.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionGCDryRun})
		if err != nil || prepared.IsError || preview.GC == nil {
			t.Fatalf("preview %+v %v", prepared, err)
		}
		result, output, err := fixture.handler.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.GC.PreviewID})
		if err != nil || result.IsError || output["operationKind"] != "backup_gc" {
			t.Fatalf("confirm %+v %+v %v", result, output, err)
		}
		gc, ok := output["gc"].(map[string]any)
		if !ok || gc["applied"] != true || gc["state"] != BackupStoreGCStateApplied {
			t.Fatalf("gc output %+v", output)
		}
		if fixture.store.Index().ManifestCount != 1 {
			t.Fatalf("gc did not remove obsolete manifests")
		}
	})
	t.Run("partial", func(t *testing.T) {
		fixture := newBackupGCHandlerFixture(t)
		wrapper := &gcApplyErrorStore{Store: fixture.store}
		h := NewHandler([]string{fixture.publicRoot}, WithBackupStore(wrapper))
		prepared, preview, err := h.HandleBackupStore(context.Background(), nil, BackupStoreInput{Action: BackupStoreActionGCDryRun})
		if err != nil || prepared.IsError || preview.GC == nil {
			t.Fatalf("preview %+v %v", prepared, err)
		}
		result, output, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.GC.PreviewID})
		if err != nil || !result.IsError || output["operationKind"] != "backup_gc" || result.Meta[ErrorCodeMetaKey] != ErrCodeIO {
			t.Fatalf("partial %+v %+v %v", result, output, err)
		}
		gc, ok := output["gc"].(map[string]any)
		if !ok || gc["state"] != BackupStoreGCStatePartial || gc["applied"] != true {
			t.Fatalf("partial evidence lost: %+v", output)
		}
	})
}

func TestConfirmDispatcherPatchPackage(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "patched.txt")
	if err := os.WriteFile(target, []byte("before\n"), 0600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: target, oldText: "before", newText: "after"}})
	result, preview, err := h.HandlePatchPackageRead(context.Background(), nil, PatchPackageReadInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || result.IsError {
		t.Fatalf("preview %+v err=%v", result, err)
	}
	applied, output, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.PreviewID, Note: "approved patch"})
	if err != nil || applied.IsError || output["operationKind"] != "patch_package" || output["applied"] != true {
		t.Fatalf("apply %+v output=%+v err=%v", applied, output, err)
	}
	if data, err := os.ReadFile(target); err != nil || string(data) != "after\n" {
		t.Fatalf("content %q err=%v", data, err)
	}
	replay, _, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.PreviewID})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay %+v err=%v", replay, err)
	}
}

func TestConfirmDispatcherMarkdownCreate(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	path := filepath.Join(root, "created.md")
	preparedResult, preview, err := h.HandleMarkdownCreate(context.Background(), nil, MarkdownCreateInput{
		Path: path, Encoding: "utf-8", BOM: "never",
		Blocks: []MarkdownCreateBlock{{Type: "heading", Level: 1, Content: []MarkdownCreateInline{{Type: "text", Text: "New"}}}},
	})
	if err != nil || preparedResult.IsError {
		t.Fatalf("preview: %+v %v", preparedResult, err)
	}
	result, output, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.PreviewID})
	if err != nil || result.IsError {
		t.Fatalf("confirm: %+v %v", result, err)
	}
	if output["operationKind"] != "markdown" || output["applied"] != true || output["state"] != editApplyStateCommitted {
		t.Fatalf("output: %+v", output)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "# New\n" {
		t.Fatalf("content %q err=%v", got, err)
	}
	replay, _, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.PreviewID})
	if err != nil || !replay.IsError || replay.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("replay %+v %v", replay, err)
	}
}

func TestConfirmDispatcherBOMAndEncoding(t *testing.T) {
	t.Run("bom", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "bom.txt")
		if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
			t.Fatal(err)
		}
		h := NewHandler([]string{root})
		preparedResult, preview, err := h.HandleManageBOMRead(context.Background(), nil, ManageBOMReadInput{
			Path: path, Action: manageBOMActionAddPreview, Encoding: "utf-8",
		})
		if err != nil || preparedResult.IsError {
			t.Fatalf("preview: %+v %v", preparedResult, err)
		}
		result, output, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.PreviewID})
		if err != nil || result.IsError || output["operationKind"] != "manage_bom" {
			t.Fatalf("confirm: %+v %+v %v", result, output, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || !bytes.HasPrefix(got, []byte{0xef, 0xbb, 0xbf}) {
			t.Fatalf("BOM not added: %x %v", got, err)
		}
	})
	t.Run("encoding", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "encode.txt")
		if err := os.WriteFile(path, []byte("hello"), 0600); err != nil {
			t.Fatal(err)
		}
		h := NewHandler([]string{root})
		preparedResult, preview, err := h.HandleConvertEncodingPreview(context.Background(), nil, ConvertEncodingPreviewInput{
			Path: path, To: "utf-16-le", DryRun: true,
		})
		if err != nil || preparedResult.IsError {
			t.Fatalf("preview: %+v %v", preparedResult, err)
		}
		result, output, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.PreviewID})
		if err != nil || result.IsError || output["operationKind"] != "convert_encoding" {
			t.Fatalf("confirm: %+v %+v %v", result, output, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || bytes.Equal(got, []byte("hello")) {
			t.Fatalf("conversion unchanged: %x %v", got, err)
		}
	})
}

func TestSharedIndexRejectsCrossStoreTokenCollision(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	repeated := strings.Repeat("a", 32)
	h.markdownPreviews.random = strings.NewReader(repeated)
	h.editPreviews.random = strings.NewReader(repeated)
	preview, err := h.markdownPreviews.putEdit(preparedMarkdownEdit{resultUTF8: []byte("# test")})
	if err != nil {
		t.Fatal(err)
	}
	plan := preparedEditPlan{targets: []preparedEditPlanTarget{{prepared: preparedEdit{data: []byte("changed")}}}}
	if _, err := h.editPreviews.put(plan); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("cross-store collision error = %v", err)
	}
	if owner, ok := h.confirmOwners.resolve(preview.id); !ok || owner != "markdown" {
		t.Fatalf("original owner changed %q/%v", owner, ok)
	}
	if len(h.editPreviews.entries) != 0 {
		t.Fatal("colliding edit preview retained")
	}
}

func TestInternalConfirmExpiredCapabilityFailsClosed(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	h.editPreviews.now = func() time.Time { return now }
	preview, err := h.editPreviews.put(preparedEditPlan{targets: []preparedEditPlanTarget{{prepared: preparedEdit{data: []byte("ok")}}}})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Minute)
	result, _, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: preview.id})
	if err != nil || !result.IsError {
		t.Fatalf("expired result=%v err=%v", result, err)
	}
	if _, ok := h.confirmOwners.resolve(preview.id); ok {
		t.Fatal("expired preview not removed from routing")
	}
}

func TestInternalConfirmFilesystemPackage(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	if h.filesystemPackageInitErr != nil {
		t.Fatal(h.filesystemPackageInitErr)
	}
	target := filepath.Join(root, "created")
	result, prepared, err := h.HandleFilesystemPackage(context.Background(), nil, filesystempackage.Manifest{
		FormatVersion: filesystempackage.FormatV1,
		Operations:    []filesystempackage.Operation{{Type: filesystempackage.OperationMkdir, Path: target}},
	})
	if err != nil || result.IsError {
		t.Fatalf("prepare: %v, %v", result, err)
	}
	confirm, output, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: prepared.PreviewID})
	if err != nil || confirm.IsError {
		t.Fatalf("confirm: %v %v", confirm, err)
	}
	if output["operationKind"] != "filesystem_package" || output["applied"] != true {
		t.Fatalf("output=%v", output)
	}
	if stat, err := os.Stat(target); err != nil || !stat.IsDir() {
		t.Fatalf("target state=%v, %v", stat, err)
	}
	replay, _, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: prepared.PreviewID})
	if err != nil || !replay.IsError {
		t.Fatalf("replay=%v, %v", replay, err)
	}
}

func TestInternalConfirmEditDispatchesOriginalEngine(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "edit.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	preparedResult, prepared, err := h.HandleEditFilePreview(context.Background(), nil, EditFilePreviewInput{
		Action: "preview", Path: path, Edits: []EditOperation{{OldText: "alpha", NewText: "omega"}},
	})
	if err != nil || preparedResult.IsError {
		t.Fatalf("prepare result=%v err=%v", preparedResult, err)
	}
	result, output, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: prepared.PreviewID})
	if err != nil || result.IsError {
		t.Fatalf("confirm result=%v err=%v", result, err)
	}
	if output["operationKind"] != "edit_file" || output["applied"] != true {
		t.Fatalf("confirm output=%v", output)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "omega" {
		t.Fatalf("content %q err=%v", data, err)
	}
	replay, _, err := h.HandleConfirmVerifiedOperation(context.Background(), nil, ConfirmVerifiedOperationInput{PreviewID: prepared.PreviewID})
	if err != nil || !replay.IsError {
		t.Fatalf("replay result=%v err=%v", replay, err)
	}
}

func TestConfirmInputIsStrictAndNoteIsBounded(t *testing.T) {
	var input ConfirmVerifiedOperationInput
	for _, raw := range []string{
		`{"previewId":"bad"}`,
		`{"previewId":"` + strings.Repeat("a", 64) + `","path":"forbidden"}`,
		`{"previewId":"` + strings.Repeat("a", 64) + `","note":"` + strings.Repeat("x", 257) + `"}`,
	} {
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			t.Fatalf("invalid input accepted: %s", raw)
		}
	}
	if err := json.Unmarshal([]byte(`{"previewId":"`+strings.Repeat("a", 64)+`","note":"prepared change"}`), &input); err != nil {
		t.Fatal(err)
	}
}

func TestHandlerOwnsOneSharedConfirmationIndex(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	if h.confirmOwners == nil {
		t.Fatal("shared confirmation index not initialized")
	}
	if h.editPreviews.ownerIndex != h.confirmOwners ||
		h.markdownPreviews.ownerIndex != h.confirmOwners ||
		h.patchPackagePreviews.ownerIndex != h.confirmOwners ||
		h.restorePreviews.ownerIndex != h.confirmOwners ||
		h.gcPreviews.ownerIndex != h.confirmOwners ||
		h.byteMutationPreviews.ownerIndex != h.confirmOwners {
		t.Fatal("not all preview stores share the same index")
	}
}

func TestRemainingPreviewStoreOwnership(t *testing.T) {
	index := newConfirmationOwnerIndex(8)
	patch := newPatchPackagePreviewStore(1, 1024, time.Minute)
	patch.ownerIndex = index
	p, err := patch.put(preparedPatchPackage{})
	if err != nil {
		t.Fatal(err)
	}
	assertOwner := func(id, owner string) {
		t.Helper()
		actual, ok := index.resolve(id)
		if !ok || actual != owner {
			t.Fatalf("owner = %q/%v, want %q", actual, ok, owner)
		}
	}
	assertGone := func(id string) {
		t.Helper()
		if _, ok := index.resolve(id); ok {
			t.Fatalf("preview %s still indexed", id)
		}
	}
	assertOwner(p.id, "patch_package")
	if _, err := patch.claim(p.id); err != nil {
		t.Fatal(err)
	}
	assertGone(p.id)

	restore := newRestorePreviewStore(1, 1024, time.Minute)
	restore.ownerIndex = index
	r, err := restore.put(preparedRestore{})
	if err != nil {
		t.Fatal(err)
	}
	assertOwner(r.id, "backup_restore")
	restore.discard(r.id)
	assertGone(r.id)

	gc := newGCPreviewStore(1, 1024, time.Minute)
	gc.ownerIndex = index
	g, err := gc.put(backupstore.GCPlan{})
	if err != nil {
		t.Fatal(err)
	}
	assertOwner(g.id, "backup_gc")
	if _, err := gc.claim(g.id); err != nil {
		t.Fatal(err)
	}
	assertGone(g.id)

	byteStore := newByteMutationPreviewStore(2, 1024, time.Minute)
	byteStore.ownerIndex = index
	bom, err := byteStore.put(preparedByteMutation{kind: byteMutationKindBOM})
	if err != nil {
		t.Fatal(err)
	}
	assertOwner(bom.id, "manage_bom")
	if _, err := byteStore.claim(bom.id, byteMutationKindEncoding); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("wrong kind: %v", err)
	}
	assertOwner(bom.id, "manage_bom")
	if _, err := byteStore.claim(bom.id, byteMutationKindBOM); err != nil {
		t.Fatal(err)
	}
	assertGone(bom.id)
	enc, err := byteStore.put(preparedByteMutation{kind: byteMutationKindEncoding})
	if err != nil {
		t.Fatal(err)
	}
	assertOwner(enc.id, "convert_encoding")
	byteStore.mu.Lock()
	byteStore.removeLocked(enc.id)
	byteStore.mu.Unlock()
	assertGone(enc.id)
}

func TestMarkdownPreviewOwnerLifecycle(t *testing.T) {
	index := newConfirmationOwnerIndex(2)
	store := newMarkdownPreviewStore(1, 1024, time.Minute)
	store.ownerIndex = index
	first, err := store.putEdit(preparedMarkdownEdit{resultUTF8: []byte("# first")})
	if err != nil {
		t.Fatal(err)
	}
	if owner, ok := index.resolve(first.id); !ok || owner != "markdown" {
		t.Fatalf("owner = %q/%v", owner, ok)
	}
	second, err := store.putCreate(preparedMarkdownCreate{resultData: []byte("# second")})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := index.resolve(first.id); ok {
		t.Fatal("evicted markdown preview still indexed")
	}
	if _, ok := index.resolve(second.id); !ok {
		t.Fatal("create preview not indexed")
	}
	if _, err := store.claim(second.id); err != nil {
		t.Fatal(err)
	}
	if _, ok := index.resolve(second.id); ok {
		t.Fatal("consumed markdown preview still indexed")
	}
	third, err := store.putEdit(preparedMarkdownEdit{resultUTF8: []byte("# third")})
	if err != nil {
		t.Fatal(err)
	}
	store.discard(third.id)
	if _, ok := index.resolve(third.id); ok {
		t.Fatal("discarded markdown preview still indexed")
	}
}

func TestEditPreviewOwnerLifecycle(t *testing.T) {
	index := newConfirmationOwnerIndex(2)
	store := newEditPreviewStore(1, 1<<20, time.Minute)
	store.ownerIndex = index
	plan := preparedEditPlan{targets: []preparedEditPlanTarget{{prepared: preparedEdit{data: []byte("ok")}}}}
	first, err := store.put(plan)
	if err != nil {
		t.Fatal(err)
	}
	if owner, ok := index.resolve(first.id); !ok || owner != "edit_file" {
		t.Fatalf("first owner = %q/%v", owner, ok)
	}
	second, err := store.put(plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := index.resolve(first.id); ok {
		t.Fatal("evicted preview still registered")
	}
	if _, ok := index.resolve(second.id); !ok {
		t.Fatal("second preview not registered")
	}
	if _, err = store.claim(second.id); err != nil {
		t.Fatal(err)
	}
	if _, ok := index.resolve(second.id); ok {
		t.Fatal("claimed preview still registered")
	}
	third, err := store.put(plan)
	if err != nil {
		t.Fatal(err)
	}
	store.discard(third.id)
	if _, ok := index.resolve(third.id); ok {
		t.Fatal("discarded preview still registered")
	}
}

func TestEditPreviewOwnerCollisionDoesNotReplaceOtherOwner(t *testing.T) {
	index := newConfirmationOwnerIndex(2)
	collidingID := strings.Repeat("61", 32)
	if err := index.reserve(collidingID, "markdown"); err != nil {
		t.Fatal(err)
	}
	store := newEditPreviewStore(2, 1<<20, time.Minute)
	store.ownerIndex = index
	store.random = strings.NewReader(strings.Repeat("a", 32))
	plan := preparedEditPlan{targets: []preparedEditPlanTarget{{prepared: preparedEdit{data: []byte("ok")}}}}
	if _, err := store.put(plan); err == nil {
		t.Fatal("collision unexpectedly accepted")
	}
	if len(store.entries) != 0 {
		t.Fatal("orphaned preview after collision")
	}
	if owner, ok := index.resolve(collidingID); !ok || owner != "markdown" {
		t.Fatalf("existing owner replaced: %q/%v", owner, ok)
	}
}

func TestConfirmationOwnerIndexConcurrentCollision(t *testing.T) {
	index := newConfirmationOwnerIndex(2)
	id := strings.Repeat("d", 64)
	var wg sync.WaitGroup
	successes := make(chan struct{}, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := index.reserve(id, "edit_file"); err == nil {
				successes <- struct{}{}
			} else if operation.KindOf(err) != operation.KindConflict {
				t.Errorf("unexpected reservation error: %v", err)
			}
		}()
	}
	wg.Wait()
	if len(successes) != 1 {
		t.Fatalf("successful reservations = %d, want 1", len(successes))
	}
}

func TestConfirmationOwnerIndexRejectsMalformedIdentifier(t *testing.T) {
	index := newConfirmationOwnerIndex(1)
	for _, id := range []string{"", "abc", strings.Repeat("G", 64), strings.Repeat("A", 64)} {
		if err := index.reserve(id, "edit_file"); operation.KindOf(err) != operation.KindInvalidInput {
			t.Fatalf("identifier %q: error = %v", id, err)
		}
	}
}
