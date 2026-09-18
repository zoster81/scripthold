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

func TestMarkdownEditPreviewStoreIsOneShotBoundedAndExpiring(t *testing.T) {
	store := newMarkdownEditPreviewStore(2, 1024, time.Minute)
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
	preview, err := store.put(prepared)
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

	expiring, err := store.put(prepared)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, err := store.claim(expiring.id); err == nil || operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("expired error=%v, want conflict", err)
	}
}

func TestMarkdownEditPreviewStoreRejectsOversizedPreparedState(t *testing.T) {
	store := newMarkdownEditPreviewStore(1, 32, time.Minute)
	_, err := store.put(preparedMarkdownEdit{resultUTF8: []byte(strings.Repeat("x", 64))})
	if err == nil || operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("error=%v, want limit", err)
	}
}

func TestMarkdownEditPreviewStoreRejectsInvalidID(t *testing.T) {
	store := newMarkdownEditPreviewStore(1, 1024, time.Minute)
	_, err := store.claim("not-a-preview")
	if err == nil || operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("error=%v, want invalid input", err)
	}
}

func TestMarkdownEditPreviewStoreTransfersIdentityOwnershipOnClaim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	identity, err := filesystem.OpenFileIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = identity.Close() })

	store := newMarkdownEditPreviewStore(1, 1024, time.Minute)
	preview, err := store.put(preparedMarkdownEdit{
		resultUTF8:   []byte("# Changed\n"),
		identityFile: identity,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.prepared.identityFile != nil {
		t.Fatal("put result must not expose the store-owned file identity")
	}

	claimed, err := store.claim(preview.id)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.prepared.identityFile != identity {
		t.Fatal("claim did not transfer the stored file identity")
	}
	store.discard(preview.id)
	if matches, err := identity.Matches(path); err != nil || !matches {
		t.Fatalf("claimed identity was closed by later discard: matches=%v err=%v", matches, err)
	}
}

func TestMarkdownEditPreviewStoreClosesIdentityOnEvictionAndExpiry(t *testing.T) {
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
	store := newMarkdownEditPreviewStore(1, 4096, time.Minute)
	store.now = func() time.Time { return now }

	evictedIdentity, evictedPath := openIdentity("evicted.md")
	if _, err := store.put(preparedMarkdownEdit{resultUTF8: []byte("one"), identityFile: evictedIdentity}); err != nil {
		t.Fatal(err)
	}
	expiringIdentity, expiringPath := openIdentity("expiring.md")
	expiring, err := store.put(preparedMarkdownEdit{resultUTF8: []byte("two"), identityFile: expiringIdentity})
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
