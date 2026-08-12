package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemPackagePreflightIsSideEffectFreeAndCapturePersistsProvenance(t *testing.T) {
	base := canonicalTempDir(t)
	store := openPhase2TestStore(t, filepath.Join(base, "store"), phase2TestLimits())
	targets := []string{filepath.Join(base, "first.txt"), filepath.Join(base, "second.txt")}
	for index, target := range targets {
		if err := os.WriteFile(target, []byte{byte('a' + index)}, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	requests := make([]CaptureRequest, 0, len(targets))
	for _, target := range targets {
		requests = append(requests, CaptureRequest{
			TargetPath: target, SourceOperation: SourceOperationFilesystemPackage,
		})
	}

	if err := store.PreflightCaptureBatch(context.Background(), requests); err != nil {
		t.Fatalf("PreflightCaptureBatch() error = %v", err)
	}
	if index := store.Index(); index.ManifestCount != 0 || index.ObjectCount != 0 || index.TotalObjectBytes != 0 {
		t.Fatalf("filesystem package preflight changed store: %#v", index)
	}

	results, err := store.CaptureBatch(context.Background(), requests)
	if err != nil {
		t.Fatalf("CaptureBatch() error = %v", err)
	}
	if len(results) != len(requests) {
		t.Fatalf("capture result count = %d, want %d", len(results), len(requests))
	}
	for index, result := range results {
		if result.Manifest.SourceOperation != SourceOperationFilesystemPackage || result.Manifest.TargetPath != requests[index].TargetPath {
			t.Fatalf("result %d provenance = %#v", index, result.Manifest)
		}
		if result.Manifest.BackupID == "" || result.Manifest.ContentFingerprint == "" {
			t.Fatalf("result %d missing durable evidence: %#v", index, result.Manifest)
		}
	}
}
