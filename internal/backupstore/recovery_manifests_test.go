package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReconstructRecoveryManifestsPreservesLogicalIdentityAndResumesExact(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("first manifest object"), []byte("second manifest object"))
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
	planPath := filepath.Join(parent, "manifest-plan.json")
	destination := filepath.Join(parent, "manifest-destination")
	report := filepath.Join(parent, "manifest-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}

	result, err := diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.VerifiedManifestCount != len(plan.Actions) || result.CreatedManifestCount != len(plan.Actions) || result.ReusedManifestCount != 0 {
		t.Fatalf("manifest result=%#v", result)
	}
	for _, record := range evidence.TrustedRecords {
		path := manifestPath(session.store.root, record.Manifest.BackupID)
		info, statErr := os.Lstat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		recovered, readErr := readManifest(path, info, session.store.descriptor)
		if readErr != nil {
			t.Fatal(readErr)
		}
		expected := record.Manifest
		expected.StoreID = session.store.descriptor.StoreID
		expected.ManifestChecksum = ""
		expected, err = finalizeManifestChecksum(expected)
		if err != nil {
			t.Fatal(err)
		}
		if recovered != expected {
			t.Fatalf("recovered manifest=%#v want=%#v", recovered, expected)
		}
		if recovered.BackupID != record.Manifest.BackupID || recovered.StoreID == record.Manifest.StoreID || recovered.ManifestChecksum == record.Manifest.ManifestChecksum {
			t.Fatalf("logical identity/store rewrite mismatch: source=%#v recovered=%#v", record.Manifest, recovered)
		}
		assertOwnerOnlyPermissions(t, path, false)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}

	resumed, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	freshEvidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), plan.Bounds)
	if err != nil {
		t.Fatal(err)
	}
	result, err = diagnostic.ReconstructRecoveryManifests(context.Background(), resumed, plan, freshEvidence)
	if err != nil {
		t.Fatal(err)
	}
	if result.VerifiedManifestCount != len(plan.Actions) || result.CreatedManifestCount != 0 || result.ReusedManifestCount != len(plan.Actions) {
		t.Fatalf("resumed manifest result=%#v", result)
	}
	if err := resumed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestReconstructRecoveryManifestsRequiresVerifiedDestinationObject(t *testing.T) {
	diagnostic, evidence, plan, planPath, destination, report := recoveryObjectFixture(t, []byte("object must precede manifest"))
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	_, err = diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence)
	if err == nil {
		t.Fatal("manifest was reconstructed without its verified destination object")
	}
	if _, statErr := os.Lstat(manifestPath(session.store.root, plan.Actions[0].BackupID)); !os.IsNotExist(statErr) {
		t.Fatalf("manifest appeared without object: %v", statErr)
	}
}

func TestReconstructRecoveryManifestsRejectsExistingCollisionWithoutOverwrite(t *testing.T) {
	diagnostic, evidence, plan, planPath, destination, report := recoveryObjectFixture(t, []byte("manifest collision"))
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	if _, err := diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	path := manifestPath(session.store.root, plan.Actions[0].BackupID)
	collision := []byte("foreign manifest residue")
	if err := os.WriteFile(path, collision, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}
	_, err = diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence)
	if err == nil {
		t.Fatal("foreign destination manifest collision was adopted")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != string(collision) {
		t.Fatal("foreign destination manifest collision was overwritten")
	}
}

func TestReconstructRecoveryManifestsRejectsChangedSourceManifest(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("changed source manifest"))
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
	planPath := filepath.Join(parent, "changed-manifest-plan.json")
	destination := filepath.Join(parent, "changed-manifest-destination")
	report := filepath.Join(parent, "changed-manifest-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	path := manifestPath(root, plan.Actions[0].BackupID)
	if err := os.WriteFile(path, []byte("{not-json}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}

	diagnostic = openRecoveryDiagnosticStore(t, root)
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	if _, err := diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	_, err = diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence)
	if err == nil {
		t.Fatal("changed source manifest was reconstructed")
	}
	if _, statErr := os.Lstat(manifestPath(session.store.root, plan.Actions[0].BackupID)); !os.IsNotExist(statErr) {
		t.Fatalf("changed source produced destination manifest: %v", statErr)
	}
}
