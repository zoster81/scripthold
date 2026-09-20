package backupstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestScanRecoveryObjectsRootContracts(t *testing.T) {
	t.Run("missing namespace is complete and empty", func(t *testing.T) {
		root := canonicalTempDir(t)
		result, err := scanRecoveryObjects(context.Background(), root, nil, DefaultRecoveryBounds())
		if err != nil {
			t.Fatal(err)
		}
		if !result.complete || result.limited || result.structuralIssue || len(result.states) != 0 ||
			len(result.metadataObjects) != 0 || len(result.reasonCounts) != 0 ||
			len(result.events) != 1 || result.events[0] != "objects|missing" {
			t.Fatalf("missing object namespace result=%#v", result)
		}
	})

	t.Run("non-directory namespace is incomplete structural damage", func(t *testing.T) {
		root := canonicalTempDir(t)
		objectsParent := filepath.Join(root, "objects")
		if err := os.MkdirAll(objectsParent, 0o700); err != nil {
			t.Fatal(err)
		}
		objectRoot := filepath.Join(objectsParent, ObjectAlgorithm)
		if err := os.WriteFile(objectRoot, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := scanRecoveryObjects(context.Background(), root, nil, DefaultRecoveryBounds())
		if err != nil {
			t.Fatal(err)
		}
		if result.complete || result.limited || !result.structuralIssue ||
			len(result.events) != 1 || result.events[0] != "objects|unscannable" {
			t.Fatalf("unscannable object namespace result=%#v", result)
		}
	})
}

func TestScanRecoveryObjectsTrustAndReasonAccounting(t *testing.T) {
	content := []byte("recovery object contract")
	root, manifests := createRecoveryScanStore(t, content)
	digest := manifests[0].ObjectDigest
	references := map[string]int{digest: 1}

	result, err := scanRecoveryObjects(context.Background(), root, references, DefaultRecoveryBounds())
	if err != nil {
		t.Fatal(err)
	}
	state, ok := result.states[digest]
	if !ok || !state.trusted || state.reason != "" || state.bytes != int64(len(content)) {
		t.Fatalf("trusted state=%#v exists=%v", state, ok)
	}
	metadata, ok := result.metadataObjects[digest]
	if !ok || metadata.Digest != digest || metadata.Bytes != int64(len(content)) {
		t.Fatalf("trusted metadata=%#v exists=%v", metadata, ok)
	}
	wantTrustedEvent := "object|trusted|" + digest + "|" + strconv.Itoa(len(content))
	if !result.complete || result.limited || result.structuralIssue || len(result.reasonCounts) != 0 ||
		len(result.events) != 1 || result.events[0] != wantTrustedEvent {
		t.Fatalf("trusted object result=%#v", result)
	}

	corrupt := bytes.Repeat([]byte{'X'}, len(content))
	if err := os.WriteFile(objectPath(root, digest), corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(objectPath(root, digest), false); err != nil {
		t.Fatal(err)
	}

	referenced, err := scanRecoveryObjects(context.Background(), root, references, DefaultRecoveryBounds())
	if err != nil {
		t.Fatal(err)
	}
	referencedState := referenced.states[digest]
	if referencedState.trusted || referencedState.reason != RecoveryRejectObjectDigestMismatch ||
		referenced.reasonCounts[RecoveryRejectObjectDigestMismatch] != 0 {
		t.Fatalf("referenced mismatch result=%#v", referenced)
	}
	wantMismatchEvent := "object|digest-mismatch|" + digest + "|" + strconv.Itoa(len(content))
	if len(referenced.events) != 1 || referenced.events[0] != wantMismatchEvent {
		t.Fatalf("referenced mismatch events=%v want=%q", referenced.events, wantMismatchEvent)
	}

	orphan, err := scanRecoveryObjects(context.Background(), root, nil, DefaultRecoveryBounds())
	if err != nil {
		t.Fatal(err)
	}
	if orphan.states[digest].trusted || orphan.states[digest].reason != RecoveryRejectObjectDigestMismatch ||
		orphan.reasonCounts[RecoveryRejectObjectDigestMismatch] != 1 {
		t.Fatalf("orphan mismatch result=%#v", orphan)
	}
}

func TestScanRecoveryObjectsLimitsPreserveMetadataAndStopDeterministically(t *testing.T) {
	content := []byte("12345")
	root, manifests := createRecoveryScanStore(t, content)
	digest := manifests[0].ObjectDigest
	references := map[string]int{digest: 1}
	bounds := DefaultRecoveryBounds()
	bounds.MaxBytes = int64(len(content) - 1)

	result, err := scanRecoveryObjects(context.Background(), root, references, bounds)
	if err != nil {
		t.Fatal(err)
	}
	state := result.states[digest]
	metadata, metadataOK := result.metadataObjects[digest]
	wantEvent := "objects|byte-limit|" + digest + "|" + strconv.Itoa(len(content))
	if result.complete || !result.limited || state.trusted || state.reason != RecoveryRejectObjectInvalid ||
		!metadataOK || metadata.Digest != digest || metadata.Bytes != int64(len(content)) ||
		len(result.events) != 1 || result.events[0] != wantEvent {
		t.Fatalf("byte-limited result=%#v metadata=%#v", result, metadata)
	}
}

func TestScanRecoveryObjectsObjectLimitAndCancellationContracts(t *testing.T) {
	root := canonicalTempDir(t)
	shardPath := filepath.Join(root, "objects", ObjectAlgorithm, "aa")
	if err := os.MkdirAll(shardPath, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "objects"),
		filepath.Join(root, "objects", ObjectAlgorithm),
		shardPath,
	} {
		if err := restrictPathPermissions(path, true); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"invalid-a", "invalid-b"} {
		path := filepath.Join(shardPath, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := restrictPathPermissions(path, false); err != nil {
			t.Fatal(err)
		}
	}

	bounds := DefaultRecoveryBounds()
	bounds.MaxObjects = 1
	limited, err := scanRecoveryObjects(context.Background(), root, nil, bounds)
	if err != nil {
		t.Fatal(err)
	}
	if limited.complete || !limited.limited || len(limited.events) != 2 || limited.events[0] != "objects|object-limit" ||
		limited.reasonCounts[RecoveryRejectObjectInvalid] != 1 {
		t.Fatalf("object-limited result=%#v", limited)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = scanRecoveryObjects(ctx, root, nil, DefaultRecoveryBounds())
	var typed *operation.Error
	if !errors.As(err, &typed) || typed.Kind != operation.KindCancelled || typed.Operation != "scan_backup_recovery_objects" {
		t.Fatalf("cancel error=%#v", err)
	}
}
