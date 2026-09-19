package handler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/markdownintelligence"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestMarkdownPreviewStoreIsOneShotBoundedAndExpiring(t *testing.T) {
	store := newMarkdownPreviewStore(2, 1024, time.Minute)
	now := time.Date(2026, 9, 17, 17, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	prepared := preparedMarkdownEdit{
		requestedPath:     `D:\docs\a.md`,
		resolvedPath:      `D:\docs\a.md`,
		resultUTF8:        []byte("# New\n"),
		targetFingerprint: strings.Repeat("a", 64),
		resultFingerprint: strings.Repeat("b", 64),
		encoding:          "utf-8",
		lineEndingStyle:   LineEndingLF,
		semantic:          markdownintelligence.PreparedChange{},
	}
	preview, err := store.putEdit(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.id) != 64 || preview.expiresAt.Sub(preview.createdAt) != time.Minute {
		t.Fatalf("preview=%+v", preview)
	}

	claimed, err := store.claim(preview.id)
	if err != nil || claimed.id != preview.id {
		t.Fatalf("claim=%+v err=%v", claimed, err)
	}
	if _, err := store.claim(preview.id); err == nil || operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("replay error=%v, want conflict", err)
	}

	expiring, err := store.putEdit(prepared)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.claim(expiring.id); err == nil || operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("expired error=%v, want conflict", err)
	}
}

func TestMarkdownPreviewStoreRejectsOversizedPreparedState(t *testing.T) {
	store := newMarkdownPreviewStore(1, 32, time.Minute)
	_, err := store.putEdit(preparedMarkdownEdit{resultUTF8: []byte(strings.Repeat("x", 64))})
	if err == nil || operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("error=%v, want limit", err)
	}
}

func TestMarkdownPreviewStoreRejectsInvalidID(t *testing.T) {
	store := newMarkdownPreviewStore(1, 1024, time.Minute)
	_, err := store.claim("not-a-preview")
	if err == nil || operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("error=%v, want invalid input", err)
	}
}

func TestMarkdownPreviewStoreTransfersIdentityOwnershipOnClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := filesystem.OpenFileIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identity.Close() })

	store := newMarkdownPreviewStore(1, 1024, time.Minute)
	preview, err := store.putEdit(preparedMarkdownEdit{
		resultUTF8:   []byte("# Changed\n"),
		identityFile: identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.edit.identityFile != nil {
		t.Fatal("put result must not expose the store-owned file identity")
	}

	claimed, err := store.claim(preview.id)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.edit.identityFile != identity {
		t.Fatal("claim did not transfer the stored file identity")
	}
	store.discard(preview.id)
	if matches, err := identity.Matches(path); err != nil || !matches {
		t.Fatalf("claimed identity was closed by later discard: matches=%v err=%v", matches, err)
	}
}

func TestMarkdownPreviewStoreSeparatesCreateAndEditPayloads(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "new.md")
	parentIdentity, err := filesystem.CaptureObjectIdentity(dir)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := filesystem.CaptureSnapshot(target)
	if err != nil {
		t.Fatal(err)
	}
	if missing.Exists {
		t.Fatal("expected missing target snapshot")
	}

	store := newMarkdownPreviewStore(2, 4096, time.Minute)
	source := []byte("# New\\n")
	createPreview, err := store.putCreate(preparedMarkdownCreate{
		requestedPath:     target,
		resolvedPath:      target,
		parentPath:        dir,
		resultData:        source,
		resultFingerprint: strings.Repeat("c", 64),
		encoding:          "utf-8",
		lineEndingStyle:   LineEndingLF,
		expectedTarget:    missing,
		parentIdentity:    parentIdentity,
	})
	if err != nil {
		t.Fatal(err)
	}
	source[0] = 'X'
	if createPreview.kind != markdownPreviewCreate || createPreview.create == nil || createPreview.edit != nil {
		t.Fatalf("create preview payload = %+v", createPreview)
	}
	if string(createPreview.create.resultData) != "# New\\n" {
		t.Fatalf("create preview aliases caller bytes: %q", createPreview.create.resultData)
	}

	claimed, err := store.claim(createPreview.id)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.kind != markdownPreviewCreate || claimed.create == nil || claimed.edit != nil {
		t.Fatalf("claimed create payload = %+v", claimed)
	}
	if matches, err := claimed.create.parentIdentity.Matches(dir); err != nil || !matches {
		t.Fatalf("parent identity mismatch: matches=%v err=%v", matches, err)
	}
	if err := claimed.create.expectedTarget.Verify(target); err != nil {
		t.Fatalf("missing target snapshot changed: %v", err)
	}

	editPreview, err := store.putEdit(preparedMarkdownEdit{resultUTF8: []byte("# Edit\\n")})
	if err != nil {
		t.Fatal(err)
	}
	if editPreview.kind != markdownPreviewEdit || editPreview.edit == nil || editPreview.create != nil {
		t.Fatalf("edit preview payload = %+v", editPreview)
	}
}

func TestMarkdownPreviewStoreClosesIdentityOnEvictionAndExpiry(t *testing.T) {
	dir := t.TempDir()
	openIdentity := func(name string) (*filesystem.FileIdentity, string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("# Title\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		identity, err := filesystem.OpenFileIdentity(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = identity.Close() })
		return identity, path
	}

	now := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	store := newMarkdownPreviewStore(1, 4096, time.Minute)
	store.now = func() time.Time { return now }

	evictedIdentity, evictedPath := openIdentity("evicted.md")
	if _, err := store.putEdit(preparedMarkdownEdit{resultUTF8: []byte("one"), identityFile: evictedIdentity}); err != nil {
		t.Fatal(err)
	}
	expiringIdentity, expiringPath := openIdentity("expiring.md")
	expiring, err := store.putEdit(preparedMarkdownEdit{resultUTF8: []byte("two"), identityFile: expiringIdentity})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := evictedIdentity.Matches(evictedPath); err == nil {
		t.Fatal("evicted preview retained its file identity")
	}

	now = now.Add(2 * time.Minute)
	if _, err := store.claim(expiring.id); err == nil || operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("expired claim error=%v, want conflict", err)
	}
	if _, err := expiringIdentity.Matches(expiringPath); err == nil {
		t.Fatal("expired preview retained its file identity")
	}
}
