package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildRecoveryPlanIsDeterministicAndCanonical(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("first plan object"), []byte("second plan object"))
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}

	first, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plans differ:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if !first.Applicable || !first.CoverageComplete || first.SourceStoreID != evidence.SourceDescriptor.StoreID || first.EvidenceDigest != evidence.EvidenceDigest {
		t.Fatalf("plan authority=%#v", first)
	}
	if first.TrustedRecordCount != len(evidence.TrustedRecords) || first.DestinationManifestCount != len(evidence.TrustedRecords) || first.DestinationObjectCount != evidence.TrustedObjectCount || first.DestinationBytes != evidence.TrustedBytes {
		t.Fatalf("plan counts=%#v evidence=%#v", first, evidence)
	}
	for index, action := range first.Actions {
		if index > 0 && first.Actions[index-1].BackupID >= action.BackupID {
			t.Fatalf("actions not canonical: %#v", first.Actions)
		}
		if action.BackupID != evidence.TrustedRecords[index].Manifest.BackupID || action.ManifestChecksum != evidence.TrustedRecords[index].Manifest.ManifestChecksum || action.ObjectDigest != evidence.TrustedRecords[index].Manifest.ObjectDigest || action.ObjectBytes != evidence.TrustedRecords[index].Manifest.ObjectBytes {
			t.Fatalf("action[%d]=%#v evidence=%#v", index, action, evidence.TrustedRecords[index])
		}
	}
}

func TestBuildRecoveryPlanMarksLimitedEvidenceNonApplicable(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("first"), []byte("second"))
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{MaxManifests: 1, MaxObjects: 2, MaxBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if plan.CoverageComplete || plan.Applicable {
		t.Fatalf("limited plan became applicable: %#v", plan)
	}
	if plan.PlanID == "" || plan.EvidenceDigest != evidence.EvidenceDigest {
		t.Fatalf("limited review evidence lost identity: %#v", plan)
	}
}

func TestWriteRecoveryPlanNoReplaceOwnerOnlyAndSourceImmutable(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("persisted recovery plan"))
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	plan, err := diagnostic.CreateRecoveryPlan(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(filepath.Dir(root), "reviewed-plan.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), output, plan, true); err != nil {
		t.Fatal(err)
	}
	assertOwnerOnlyPermissions(t, output, false)
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecoveryPlan(data)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.PlanID != plan.PlanID || !reflect.DeepEqual(decoded, plan) {
		t.Fatalf("persisted plan=%#v want=%#v", decoded, plan)
	}
	if err := diagnostic.WriteRecoveryPlan(context.Background(), output, plan, false); err == nil {
		t.Fatal("recovery plan output was replaced")
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}

func TestWriteRecoveryPlanRejectsSourceOverlapAndRelativeOutput(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("overlap rejected"))
	diagnostic := openRecoveryDiagnosticStore(t, root)
	plan, err := diagnostic.CreateRecoveryPlan(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{
		"relative-plan.json",
		filepath.Join(root, "plan.json"),
		filepath.Join(root, "manifests", "plan.json"),
	} {
		if err := diagnostic.WriteRecoveryPlan(context.Background(), output, plan, false); err == nil {
			t.Fatalf("unsafe plan output accepted: %q", output)
		}
		if filepath.IsAbs(output) {
			if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
				t.Fatalf("unsafe output was created: %q err=%v", output, statErr)
			}
		}
	}
}
