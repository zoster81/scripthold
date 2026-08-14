package backupstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestReconstructRecoveryObjectsCopiesUniqueObjectsAndResumesByteExact(t *testing.T) {
	shared := []byte("shared recovery object")
	unique := []byte("unique recovery object")
	root, _ := createRecoveryScanStore(t, shared, shared, unique)
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "object-plan.json")
	destination := filepath.Join(parent, "object-destination")
	report := filepath.Join(parent, "object-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}

	first, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	result, err := diagnostic.ReconstructRecoveryObjects(context.Background(), first, plan, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.VerifiedObjectCount != 2 || result.CreatedObjectCount != 2 || result.ReusedObjectCount != 0 || result.VerifiedBytes != int64(len(shared)+len(unique)) {
		t.Fatalf("first object result=%#v", result)
	}
	for _, record := range evidence.TrustedRecords {
		path := objectPath(first.store.root, record.Manifest.ObjectDigest)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		sourceData, readErr := os.ReadFile(record.objectPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !bytes.Equal(data, sourceData) {
			t.Fatalf("destination object %s differs from source", record.Manifest.ObjectDigest)
		}
		assertOwnerOnlyPermissions(t, path, false)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	freshEvidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), plan.Bounds)
	if err != nil {
		t.Fatal(err)
	}
	result, err = diagnostic.ReconstructRecoveryObjects(context.Background(), second, plan, freshEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.VerifiedObjectCount != 2 || result.CreatedObjectCount != 0 || result.ReusedObjectCount != 2 || result.VerifiedBytes != int64(len(shared)+len(unique)) {
		t.Fatalf("resume object result=%#v", result)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestReconstructRecoveryObjectsRejectsChangedSourceWithoutMutation(t *testing.T) {
	content := []byte("source object must remain immutable")
	root, _ := createRecoveryScanStore(t, content)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "changed-source-plan.json")
	destination := filepath.Join(parent, "changed-source-destination")
	report := filepath.Join(parent, "changed-source-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}

	sourceObject := evidence.TrustedRecords[0].objectPath
	corrupt := bytes.Repeat([]byte{'X'}, len(content))
	if err := os.WriteFile(sourceObject, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(sourceObject, false); err != nil {
		t.Fatal(err)
	}
	before := snapshotDiagnosticTree(t, root)
	diagnostic = openRecoveryDiagnosticStore(t, root)
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	_, err = diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence)
	if err == nil {
		t.Fatal("changed source object was copied")
	}
	if _, statErr := os.Lstat(objectPath(session.store.root, evidence.TrustedRecords[0].Manifest.ObjectDigest)); !os.IsNotExist(statErr) {
		t.Fatalf("changed source produced destination object: %v", statErr)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestReconstructRecoveryObjectsRejectsCorruptExistingDestinationObject(t *testing.T) {
	content := []byte("destination object must be verified")
	diagnostic, evidence, plan, planPath, destination, report := recoveryObjectFixture(t, content)
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	digest := evidence.TrustedRecords[0].Manifest.ObjectDigest
	path := objectPath(session.store.root, digest)
	if err := ensureDirectory(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	wrong := bytes.Repeat([]byte{'Z'}, len(content))
	if err := os.WriteFile(path, wrong, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}

	_, err = diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence)
	if err == nil {
		t.Fatal("corrupt pre-existing destination object was adopted")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, wrong) {
		t.Fatal("corrupt pre-existing destination object was overwritten")
	}
}

func TestReconstructRecoveryObjectsHonorsCancellationBeforeMutation(t *testing.T) {
	diagnostic, evidence, plan, planPath, destination, report := recoveryObjectFixture(t, []byte("cancel recovery copy"))
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = diagnostic.ReconstructRecoveryObjects(ctx, session, plan, evidence)
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancelled recovery copy error=%v", err)
	}
	for _, action := range plan.Actions {
		if _, statErr := os.Lstat(objectPath(session.store.root, action.ObjectDigest)); !os.IsNotExist(statErr) {
			t.Fatalf("cancelled recovery created object %s: %v", action.ObjectDigest, statErr)
		}
	}
}

func recoveryObjectFixture(t *testing.T, contents ...[]byte) (*DiagnosticStore, RecoveryEvidence, RecoveryPlan, string, string, string) {
	t.Helper()
	root, _ := createRecoveryScanStore(t, contents...)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "recovery-object-plan.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	return diagnostic, evidence, plan, planPath, filepath.Join(parent, "recovery-object-destination"), filepath.Join(parent, "recovery-object-report.json")
}

func TestReconstructRecoveryObjectsCleansOwnedStagingOnWriteAndSyncFailures(t *testing.T) {
	cases := []struct {
		name string
		ops  recoveryObjectCopyOps
	}{
		{
			name: "disk full write",
			ops: recoveryObjectCopyOps{write: func(_ *os.File, _ []byte) (int, error) {
				return 0, errors.New("injected disk full")
			}},
		},
		{
			name: "short write",
			ops: recoveryObjectCopyOps{write: func(_ *os.File, data []byte) (int, error) {
				if len(data) == 0 {
					return 0, nil
				}
				return len(data) - 1, nil
			}},
		},
		{
			name: "sync failure",
			ops: recoveryObjectCopyOps{sync: func(_ *os.File) error {
				return errors.New("injected sync failure")
			}},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			diagnostic, evidence, plan, planPath, destination, report := recoveryObjectFixture(t, []byte("failure injection object"))
			session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			defer diagnostic.Close()
			sourcePath := evidence.TrustedRecords[0].objectPath
			beforeSource, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}

			result, err := diagnostic.reconstructRecoveryObjectsWithOps(context.Background(), session, plan, evidence, test.ops)
			if err == nil {
				t.Fatal("injected recovery object failure was ignored")
			}
			if result.VerifiedObjectCount != 0 || result.CreatedObjectCount != 0 || result.ReusedObjectCount != 0 || result.VerifiedBytes != 0 {
				t.Fatalf("failure reported verified progress: %#v", result)
			}
			if _, statErr := os.Lstat(objectPath(session.store.root, evidence.TrustedRecords[0].Manifest.ObjectDigest)); !os.IsNotExist(statErr) {
				t.Fatalf("failed copy exposed canonical object: %v", statErr)
			}
			assertRecoveryObjectStagingEmpty(t, session)
			afterSource, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(beforeSource, afterSource) {
				t.Fatal("destination-side failure mutated source object")
			}
		})
	}
}

func TestReconstructRecoveryObjectsCancelsDuringStreamingAndCleansStage(t *testing.T) {
	content := bytes.Repeat([]byte("0123456789abcdef"), 20_000)
	diagnostic, evidence, plan, planPath, destination, report := recoveryObjectFixture(t, content)
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	ctx, cancel := context.WithCancel(context.Background())
	writes := 0
	ops := recoveryObjectCopyOps{write: func(file *os.File, data []byte) (int, error) {
		written, writeErr := file.Write(data)
		writes++
		if writes == 1 {
			cancel()
		}
		return written, writeErr
	}}
	result, err := diagnostic.reconstructRecoveryObjectsWithOps(ctx, session, plan, evidence, ops)
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("mid-stream cancellation error=%v", err)
	}
	if result.VerifiedObjectCount != 0 || result.VerifiedBytes != 0 {
		t.Fatalf("cancelled copy reported progress: %#v", result)
	}
	if _, statErr := os.Lstat(objectPath(session.store.root, evidence.TrustedRecords[0].Manifest.ObjectDigest)); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled copy exposed canonical object: %v", statErr)
	}
	assertRecoveryObjectStagingEmpty(t, session)
}

func TestReconstructRecoveryObjectsRejectsConcurrentSourceTruncation(t *testing.T) {
	content := bytes.Repeat([]byte("abcdef0123456789"), 20_000)
	diagnostic, evidence, plan, planPath, destination, report := recoveryObjectFixture(t, content)
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	sourcePath := evidence.TrustedRecords[0].objectPath
	writes := 0
	ops := recoveryObjectCopyOps{write: func(file *os.File, data []byte) (int, error) {
		written, writeErr := file.Write(data)
		writes++
		if writes == 1 {
			if truncateErr := os.Truncate(sourcePath, int64(len(content)/4)); truncateErr != nil {
				return written, truncateErr
			}
		}
		return written, writeErr
	}}
	result, err := diagnostic.reconstructRecoveryObjectsWithOps(context.Background(), session, plan, evidence, ops)
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("concurrent truncation error=%v", err)
	}
	if result.VerifiedObjectCount != 0 || result.VerifiedBytes != 0 {
		t.Fatalf("truncated source reported progress: %#v", result)
	}
	if _, statErr := os.Lstat(objectPath(session.store.root, evidence.TrustedRecords[0].Manifest.ObjectDigest)); !os.IsNotExist(statErr) {
		t.Fatalf("truncated source exposed canonical object: %v", statErr)
	}
	assertRecoveryObjectStagingEmpty(t, session)
}

func assertRecoveryObjectStagingEmpty(t *testing.T, session *RecoveryDestination) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(session.store.root, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("recovery object staging residue=%v", entries)
	}
}
