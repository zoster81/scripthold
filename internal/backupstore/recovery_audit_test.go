package backupstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestRebuildAndAuditRecoveryDestinationRebuildsIndexAndMarksAudited(t *testing.T) {
	root := createRecoveryPinnedStore(t)
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
	if plan.DestinationPinnedCount != 1 {
		t.Fatalf("destination pinned count=%d want=1", plan.DestinationPinnedCount)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "audit-plan.json")
	destination := filepath.Join(parent, "audit-destination")
	reportPath := filepath.Join(parent, "audit-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, reportPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}

	audit, err := diagnostic.RebuildAndAuditRecoveryDestination(context.Background(), session, plan)
	if err != nil {
		t.Fatal(err)
	}
	if audit.Mode != AuditFull || !audit.Healthy || !audit.IndexConsistent || len(audit.Issues) != 0 {
		t.Fatalf("full audit=%#v", audit)
	}
	if audit.ManifestCount != plan.DestinationManifestCount || audit.ObjectCount != plan.DestinationObjectCount ||
		audit.ReferencedBytes != plan.DestinationBytes || audit.OrphanObjectCount != 0 || audit.StagingEntryCount != 0 || audit.TrashEntryCount != 0 {
		t.Fatalf("audit counts=%#v plan=%#v", audit, plan)
	}
	index := session.store.Index()
	if index.Generation != audit.Generation || index.ManifestCount != plan.DestinationManifestCount ||
		index.ObjectCount != plan.DestinationObjectCount || index.PinnedCount != plan.DestinationPinnedCount || index.TotalObjectBytes != plan.DestinationBytes {
		t.Fatalf("rebuilt index=%#v audit=%#v plan=%#v", index, audit, plan)
	}
	stateInfo, err := os.Lstat(session.paths.state)
	if err != nil {
		t.Fatal(err)
	}
	state, err := readRecoveryStateFile(session.paths.state, stateInfo)
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != RecoveryPhaseAudited || session.state != state {
		t.Fatalf("audited state=%#v session=%#v", state, session.state)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestRebuildAndAuditRecoveryDestinationRejectsCorruptObjectAndKeepsBuildingState(t *testing.T) {
	diagnostic, evidence, plan, planPath, destination, reportPath := recoveryObjectFixture(t, []byte("audit corruption"))
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, reportPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	if _, err := diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	path := objectPath(session.store.root, plan.Actions[0].ObjectDigest)
	if err := os.WriteFile(path, []byte("audit corruption"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.RebuildAndAuditRecoveryDestination(context.Background(), session, plan); err == nil {
		t.Fatal("corrupt recovered object passed full staged audit")
	}
	assertRecoveryStatePhase(t, session, RecoveryPhaseBuilding)
}

func TestRebuildAndAuditRecoveryDestinationRejectsOrphanObjectAndKeepsBuildingState(t *testing.T) {
	diagnostic, evidence, plan, planPath, destination, reportPath := recoveryObjectFixture(t, []byte("audited record"))
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, reportPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	defer diagnostic.Close()
	if _, err := diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	orphan := []byte("unexpected orphan")
	digestBytes := sha256.Sum256(orphan)
	digest := hex.EncodeToString(digestBytes[:])
	path := objectPath(session.store.root, digest)
	if err := ensureDirectory(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, orphan, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(path, false); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.RebuildAndAuditRecoveryDestination(context.Background(), session, plan); err == nil {
		t.Fatal("unexpected orphan object passed staged recovery audit")
	}
	assertRecoveryStatePhase(t, session, RecoveryPhaseBuilding)
}

func createRecoveryPinnedStore(t *testing.T) string {
	t.Helper()
	base := canonicalTempDir(t)
	root := filepath.Join(base, "store")
	store, err := Open(Options{Directory: root, Limits: backupStoreTestLimits()})
	if err != nil {
		t.Fatal(err)
	}
	for index, item := range []struct {
		content []byte
		pinned  bool
	}{
		{content: []byte("pinned recovery object"), pinned: true},
		{content: []byte("unpinned recovery object"), pinned: false},
	} {
		target := filepath.Join(base, "pinned-target-"+string(rune('a'+index))+".txt")
		if err := os.WriteFile(target, item.content, 0o600); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
		if _, err := store.Capture(context.Background(), CaptureRequest{
			TargetPath: target, SourceOperation: SourceOperationEdit, Pinned: item.pinned,
		}); err != nil {
			_ = store.Close()
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

func assertRecoveryStatePhase(t *testing.T, session *RecoveryDestination, want RecoveryPhase) {
	t.Helper()
	info, err := os.Lstat(session.paths.state)
	if err != nil {
		t.Fatal(err)
	}
	state, err := readRecoveryStateFile(session.paths.state, info)
	if err != nil {
		t.Fatal(err)
	}
	if state.Phase != want || session.state.Phase != want {
		t.Fatalf("recovery state phase=%q session=%q want=%q", state.Phase, session.state.Phase, want)
	}
}
