package handler

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/config"
)

func TestPatchPackageApplyRejectsStaleAndSameContentReplacement(t *testing.T) {
	t.Run("stale content", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "target.txt")
		if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
			t.Fatal(err)
		}
		h := NewHandler([]string{root})
		manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
		_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
		if err := os.WriteFile(path, []byte("changed"), 0644); err != nil {
			t.Fatal(err)
		}
		result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
			t.Fatalf("stale result=%+v err=%v", result, err)
		}
		terminal, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
		if !terminal.IsError || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
			t.Fatalf("stale capability was not terminal: %+v", terminal)
		}
	})

	t.Run("same content path replacement", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "target.txt")
		displaced := filepath.Join(root, "displaced.txt")
		if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
			t.Fatal(err)
		}
		h := NewHandler([]string{root})
		manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
		_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
		if err := os.Rename(path, displaced); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
			t.Fatal(err)
		}
		result, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
		if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
			t.Fatalf("replacement result=%+v err=%v", result, err)
		}
		assertFileBytes(t, path, []byte("alpha"))
	})
}

func TestPatchPackageCancelledApplyConsumesCapability(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	_, dryRun, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, _, err := h.HandlePatchPackage(ctx, nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancelled result=%+v err=%v", result, err)
	}
	terminal, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if !terminal.IsError || terminal.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("cancelled capability was not terminal: %+v", terminal)
	}
	assertFileBytes(t, path, []byte("alpha"))
}

func TestPatchPackagePreviewExpiryEvictionRestartAndCleanup(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "first.txt"), filepath.Join(root, "second.txt")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := patchPackagePreviewTestConfig()
	cfg.Limits.MaxPatchPackagePreviews = 1
	now := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	h := NewHandler([]string{root}, WithConfig(cfg))
	h.patchPackagePreviews.now = func() time.Time { return now }

	manifest1 := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: paths[0], oldText: "alpha", newText: "first"}})
	_, first, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest1})
	h.patchPackagePreviews.mu.Lock()
	firstEntry := h.patchPackagePreviews.entries[first.PreviewID]
	h.patchPackagePreviews.mu.Unlock()

	manifest2 := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: paths[1], oldText: "alpha", newText: "second"}})
	_, second, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest2})
	if firstEntry == nil || firstEntry.prepared.plan.targets[0].prepared.identityFile != nil {
		t.Fatalf("eviction did not close retained identity: %+v", firstEntry)
	}
	evicted, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: first.PreviewID})
	if !evicted.IsError || evicted.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("evicted result=%+v", evicted)
	}

	now = now.Add(2 * time.Minute)
	expired, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: second.PreviewID})
	if !expired.IsError || expired.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("expired result=%+v", expired)
	}

	h2 := NewHandler([]string{root}, WithConfig(cfg))
	restarted, _, _ := h2.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: second.PreviewID})
	if !restarted.IsError || restarted.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("restart result=%+v", restarted)
	}
}

func TestPatchPackagePreviewAggregateByteLimitEvictsOldest(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "first.txt"), filepath.Join(root, "second.txt")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root}, WithConfig(patchPackagePreviewTestConfig()))
	manifest1 := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: paths[0], oldText: "alpha", newText: "omega"}})
	_, first, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest1})
	h.patchPackagePreviews.mu.Lock()
	firstSize := h.patchPackagePreviews.entries[first.PreviewID].retainedBytes
	h.patchPackagePreviews.maxBytes = firstSize*2 - 1
	h.patchPackagePreviews.mu.Unlock()

	manifest2 := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: paths[1], oldText: "alpha", newText: "omega"}})
	_, second, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest2})
	evicted, _, _ := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: first.PreviewID})
	if !evicted.IsError || evicted.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("byte-evicted result=%+v", evicted)
	}
	if len(second.PreviewID) != 64 {
		t.Fatalf("second preview=%+v", second)
	}
}

func TestPatchPackagePreviewByteAndOutputLimitsReleaseState(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: strings.Repeat("x", 256)}})

	cfg := patchPackagePreviewTestConfig()
	cfg.Limits.MaxPatchPackagePreviewBytes = 1
	h := NewHandler([]string{root}, WithConfig(cfg))
	limited, _, err := h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || !limited.IsError || limited.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("preview byte limit result=%+v err=%v", limited, err)
	}

	cfg = patchPackagePreviewTestConfig()
	cfg.Limits.MaxOutputBytes = 1
	h = NewHandler([]string{root}, WithConfig(cfg))
	limited, _, err = h.HandlePatchPackage(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || !limited.IsError || limited.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("output limit result=%+v err=%v", limited, err)
	}
	h.patchPackagePreviews.mu.Lock()
	entries, retained := len(h.patchPackagePreviews.entries), h.patchPackagePreviews.totalBytes
	h.patchPackagePreviews.mu.Unlock()
	if entries != 0 || retained != 0 {
		t.Fatalf("preview state leaked after output limit: entries=%d retained=%d", entries, retained)
	}
}

func TestPatchPackagePreviewTokenIsNotLogged(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	if err := os.WriteFile(path, []byte("alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	h := NewHandler([]string{root})
	wrapped := Wrap(logger, "patch_package", h.HandlePatchPackage)
	manifest := patchPackageManifestForApplyTest(t, []patchPackageApplyFixture{{path: path, oldText: "alpha", newText: "omega"}})
	result, dryRun, err := wrapped(context.Background(), nil, PatchPackageInput{Action: patchPackageActionDryRun, Manifest: manifest})
	if err != nil || result.IsError {
		t.Fatalf("dryRun result=%+v output=%+v err=%v", result, dryRun, err)
	}
	if strings.Contains(logs.String(), dryRun.PreviewID) {
		t.Fatalf("preview token entered logs: %s", logs.String())
	}
	result, _, err = wrapped(context.Background(), nil, PatchPackageInput{Action: patchPackageActionApply, PreviewID: dryRun.PreviewID})
	if err != nil || result.IsError {
		t.Fatalf("apply result=%+v err=%v", result, err)
	}
	if strings.Contains(logs.String(), dryRun.PreviewID) {
		t.Fatalf("preview token entered logs after apply: %s", logs.String())
	}
}

func patchPackagePreviewTestConfig() *config.Config {
	return &config.Config{DefaultEncoding: "utf-8", Limits: config.Limits{
		MaxFileBytes:                  config.DefaultMaxFileBytes,
		MaxBatchFiles:                 config.DefaultMaxBatchFiles,
		MaxOutputBytes:                config.DefaultMaxOutputBytes,
		MaxPatchPackageBytes:          config.DefaultMaxPatchPackageBytes,
		MaxPatchPackagePreparedBytes:  config.DefaultMaxPatchPackagePreparedBytes,
		MaxPatchPackagePreviews:       4,
		MaxPatchPackagePreviewBytes:   1 << 20,
		PatchPackagePreviewTTLSeconds: 60,
	}}
}
