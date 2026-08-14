package backupstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFinalizeRecoveryDestinationPromotesReportsAndCompletesIdempotently(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("finalize first"), []byte("finalize second"))
	beforeSource := snapshotDiagnosticTree(t, root)
	diagnostic, _, plan, planPath, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)

	report, err := diagnostic.FinalizeRecoveryDestination(context.Background(), session, plan, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != RecoveryStatusRecovered || !report.FullAudit || report.AuditIssueCount != 0 ||
		report.ManifestCount != plan.DestinationManifestCount || report.ObjectCount != plan.DestinationObjectCount ||
		report.PinnedCount != plan.DestinationPinnedCount || report.TotalObjectBytes != plan.DestinationBytes ||
		report.OmittedRecordCount != 0 || report.PlanID != plan.PlanID {
		t.Fatalf("recovery report=%#v plan=%#v", report, plan)
	}
	if _, err := os.Lstat(destinationPath); err != nil {
		t.Fatalf("promoted destination missing: %v", err)
	}
	if _, err := os.Lstat(session.paths.staging); !os.IsNotExist(err) {
		t.Fatalf("staging remained after promotion: %v", err)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhasePromoted)
	assertOwnerOnlyPermissions(t, session.paths.state, false)
	assertOwnerOnlyPermissions(t, reportPath, false)
	reportData, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeRecoveryReport(reportData)
	if err != nil || decoded != report {
		t.Fatalf("persisted report=%#v err=%v want=%#v", decoded, err, report)
	}
	reportInfo, err := os.Lstat(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, beforeSource)

	diagnostic = openRecoveryDiagnosticStore(t, root)
	completed, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destinationPath, reportPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !completed.completed || !completed.promoted || completed.store != nil {
		t.Fatalf("completed recovery was not recognized exactly: %#v", completed)
	}
	again, err := diagnostic.FinalizeRecoveryDestination(context.Background(), completed, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if again != report {
		t.Fatalf("idempotent report=%#v want=%#v", again, report)
	}
	afterInfo, err := os.Lstat(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(reportInfo, afterInfo) {
		t.Fatal("idempotent completion replaced the existing report")
	}
}

func TestFinalizeRecoveryDestinationRejectsStaleSourceBeforePromotion(t *testing.T) {
	root, manifests := createRecoveryScanStore(t, []byte("stale source before promotion"))
	diagnostic, _, plan, _, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	defer session.Close()
	defer diagnostic.Close()
	sourceObject := objectPath(root, manifests[0].ObjectDigest)
	data, err := os.ReadFile(sourceObject)
	if err != nil {
		t.Fatal(err)
	}
	data[0] ^= 0xff
	if err := os.WriteFile(sourceObject, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(sourceObject, false); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.FinalizeRecoveryDestination(context.Background(), session, plan, false); err == nil {
		t.Fatal("stale source was promoted")
	}
	if _, err := os.Lstat(destinationPath); !os.IsNotExist(err) {
		t.Fatalf("stale source exposed requested destination: %v", err)
	}
	if _, err := os.Lstat(session.paths.staging); err != nil {
		t.Fatalf("audited staging was removed on stale source conflict: %v", err)
	}
	if _, err := os.Lstat(reportPath); !os.IsNotExist(err) {
		t.Fatalf("stale source produced report: %v", err)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhaseAudited)
}

func TestFinalizeRecoveryDestinationResumesCrashAfterPromotion(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("post promotion retry"))
	diagnostic, _, plan, planPath, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	_, err := diagnostic.finalizeRecoveryDestinationWithOps(context.Background(), session, plan, false, recoveryFinalizeOps{
		afterPromotion: func() error { return errors.New("injected post-promotion crash") },
	})
	if err == nil {
		t.Fatal("post-promotion failure was ignored")
	}
	if _, statErr := os.Lstat(destinationPath); statErr != nil {
		t.Fatalf("promotion did not happen before injected failure: %v", statErr)
	}
	if _, statErr := os.Lstat(session.paths.staging); !os.IsNotExist(statErr) {
		t.Fatalf("staging still exists after promotion: %v", statErr)
	}
	if _, statErr := os.Lstat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("report appeared before retry: %v", statErr)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhaseAudited)
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}

	diagnostic = openRecoveryDiagnosticStore(t, root)
	resumed, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destinationPath, reportPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.promoted || resumed.completed || resumed.store != nil || resumed.state.Phase != RecoveryPhaseAudited {
		t.Fatalf("post-promotion recovery state not recognized: %#v", resumed)
	}
	report, err := diagnostic.FinalizeRecoveryDestination(context.Background(), resumed, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.PlanID != plan.PlanID || !report.FullAudit {
		t.Fatalf("retry report=%#v", report)
	}
	assertRecoveryStatePhase(t, resumed, RecoveryPhasePromoted)
}

func TestFinalizeRecoveryDestinationResumesCrashAfterReport(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("post report retry"))
	diagnostic, _, plan, planPath, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	first, err := diagnostic.finalizeRecoveryDestinationWithOps(context.Background(), session, plan, true, recoveryFinalizeOps{
		afterReport: func() error { return errors.New("injected post-report crash") },
	})
	if err == nil {
		t.Fatal("post-report failure was ignored")
	}
	if _, statErr := os.Lstat(destinationPath); statErr != nil {
		t.Fatal(statErr)
	}
	dataBefore, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if decoded, decodeErr := DecodeRecoveryReport(dataBefore); decodeErr != nil || decoded != first {
		t.Fatalf("durable pre-crash report=%#v err=%v first=%#v", decoded, decodeErr, first)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhasePromoted)
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}

	diagnostic = openRecoveryDiagnosticStore(t, root)
	resumed, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destinationPath, reportPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !resumed.promoted || !resumed.completed || resumed.state.Phase != RecoveryPhasePromoted {
		t.Fatalf("post-report state not recognized: %#v", resumed)
	}
	second, err := diagnostic.FinalizeRecoveryDestination(context.Background(), resumed, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("retry report=%#v first=%#v", second, first)
	}
	dataAfter, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(dataBefore, dataAfter) {
		t.Fatal("retry replaced or reformatted durable report")
	}
	assertRecoveryStatePhase(t, resumed, RecoveryPhasePromoted)
}

func TestFinalizeRecoveryDestinationReportsRecoveredWithOmissions(t *testing.T) {
	root, manifests := createRecoveryScanStore(t, []byte("recover me"), []byte("omit me"))
	corruptPath := objectPath(root, manifests[1].ObjectDigest)
	wrong := bytes.Repeat([]byte{'Q'}, int(manifests[1].ObjectBytes))
	if err := os.WriteFile(corruptPath, wrong, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(corruptPath, false); err != nil {
		t.Fatal(err)
	}
	diagnostic, evidence, plan, _, _, _, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	if !plan.HasOmissions || plan.RejectedRecordCount == 0 || len(evidence.TrustedRecords) != 1 {
		t.Fatalf("fixture did not produce omissions: plan=%#v evidence=%#v", plan, evidence)
	}
	report, err := diagnostic.FinalizeRecoveryDestination(context.Background(), session, plan, false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != RecoveryStatusRecoveredWithOmissions || report.OmittedRecordCount != plan.RejectedRecordCount {
		t.Fatalf("omission report=%#v plan=%#v", report, plan)
	}
}

func prepareAuditedRecoveryFixtureFromRoot(t *testing.T, root string) (*DiagnosticStore, RecoveryEvidence, RecoveryPlan, string, string, string, *RecoveryDestination) {
	t.Helper()
	diagnostic := openRecoveryDiagnosticStore(t, root)
	evidence, err := diagnostic.ScanRecoveryEvidence(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := BuildRecoveryPlan(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable {
		t.Fatalf("fixture recovery plan is not applicable: %#v", plan)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "finalize-plan.json")
	destinationPath := filepath.Join(parent, "finalize-destination")
	reportPath := filepath.Join(parent, "finalize-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destinationPath, reportPath, plan)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.ReconstructRecoveryObjects(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.ReconstructRecoveryManifests(context.Background(), session, plan, evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.RebuildAndAuditRecoveryDestination(context.Background(), session, plan); err != nil {
		t.Fatal(err)
	}
	return diagnostic, evidence, plan, planPath, destinationPath, reportPath, session
}
