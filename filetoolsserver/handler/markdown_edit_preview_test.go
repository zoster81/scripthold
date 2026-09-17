package handler

import (
	"strings"
	"testing"
	"time"

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
