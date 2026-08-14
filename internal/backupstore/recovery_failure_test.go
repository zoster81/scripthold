package backupstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestFinalizeRecoveryDestinationRejectsSourceChangeAtFinalRescan(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("late source mutation"))
	diagnostic, _, plan, _, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	defer session.Close()
	defer diagnostic.Close()
	sourceObject := objectPath(root, plan.Actions[0].ObjectDigest)
	_, err := diagnostic.finalizeRecoveryDestinationWithOps(context.Background(), session, plan, false, recoveryFinalizeOps{
		beforeFinalSourceCheck: func() error {
			data, readErr := os.ReadFile(sourceObject)
			if readErr != nil {
				return readErr
			}
			data[0] ^= 0xff
			if writeErr := os.WriteFile(sourceObject, data, 0o600); writeErr != nil {
				return writeErr
			}
			return restrictPathPermissions(sourceObject, false)
		},
	})
	if operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("late source mutation error=%v", err)
	}
	if _, statErr := os.Lstat(destinationPath); !os.IsNotExist(statErr) {
		t.Fatalf("late source mutation exposed destination: %v", statErr)
	}
	if _, statErr := os.Lstat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("late source mutation produced report: %v", statErr)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhaseAudited)
}

func TestFinalizeRecoveryDestinationRefusesDestinationRaceWithoutOverwrite(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("destination race"))
	diagnostic, _, plan, _, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	defer diagnostic.Close()
	foreign := filepath.Join(destinationPath, "foreign.txt")
	_, err := diagnostic.finalizeRecoveryDestinationWithOps(context.Background(), session, plan, false, recoveryFinalizeOps{
		beforePromotionRename: func() error {
			if mkdirErr := os.Mkdir(destinationPath, 0o700); mkdirErr != nil {
				return mkdirErr
			}
			return os.WriteFile(foreign, []byte("foreign destination"), 0o600)
		},
	})
	if err == nil {
		t.Fatal("destination race was overwritten")
	}
	data, readErr := os.ReadFile(foreign)
	if readErr != nil || string(data) != "foreign destination" {
		t.Fatalf("foreign destination changed: data=%q err=%v", data, readErr)
	}
	if _, statErr := os.Lstat(session.paths.staging); statErr != nil {
		t.Fatalf("audited staging disappeared after destination race: %v", statErr)
	}
	if _, statErr := os.Lstat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("destination race produced report: %v", statErr)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhaseAudited)
}

func TestFinalizeRecoveryDestinationRefusesReportRaceWithoutOverwrite(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("report race"))
	diagnostic, _, plan, _, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	defer diagnostic.Close()
	foreign := []byte("foreign report")
	_, err := diagnostic.finalizeRecoveryDestinationWithOps(context.Background(), session, plan, false, recoveryFinalizeOps{
		beforeReportInstall: func() error {
			if writeErr := os.WriteFile(reportPath, foreign, 0o600); writeErr != nil {
				return writeErr
			}
			return restrictPathPermissions(reportPath, false)
		},
	})
	if err == nil {
		t.Fatal("foreign report race was adopted")
	}
	if _, statErr := os.Lstat(destinationPath); statErr != nil {
		t.Fatalf("audited destination was not promoted before report race: %v", statErr)
	}
	got, readErr := os.ReadFile(reportPath)
	if readErr != nil || string(got) != string(foreign) {
		t.Fatalf("foreign report changed: data=%q err=%v", got, readErr)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhasePromoted)
}

func TestFinalizeRecoveryDestinationRejectsReplacedPlanBeforePromotion(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("plan replacement"))
	diagnostic, _, plan, planPath, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	defer session.Close()
	defer diagnostic.Close()
	if err := os.WriteFile(planPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(planPath, false); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.FinalizeRecoveryDestination(context.Background(), session, plan, false); err == nil {
		t.Fatal("replaced recovery plan was accepted")
	}
	if _, statErr := os.Lstat(destinationPath); !os.IsNotExist(statErr) {
		t.Fatalf("replaced plan exposed destination: %v", statErr)
	}
	if _, statErr := os.Lstat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("replaced plan produced report: %v", statErr)
	}
}

func TestFinalizeRecoveryDestinationCancellationAtFinalRescanIsNonPromoting(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("final cancellation"))
	diagnostic, _, plan, _, destinationPath, _, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	defer session.Close()
	defer diagnostic.Close()
	ctx, cancel := context.WithCancel(context.Background())
	_, err := diagnostic.finalizeRecoveryDestinationWithOps(ctx, session, plan, false, recoveryFinalizeOps{
		beforeFinalSourceCheck: func() error {
			cancel()
			return nil
		},
	})
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("final rescan cancellation error=%v", err)
	}
	if _, statErr := os.Lstat(destinationPath); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled finalization exposed destination: %v", statErr)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhaseAudited)
}

func TestPrepareRecoveryDestinationRejectsReportOnlyCompletedDestination(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("state remains authority"))
	diagnostic, _, plan, planPath, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	if _, err := diagnostic.FinalizeRecoveryDestination(context.Background(), session, plan, false); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(session.paths.state); err != nil {
		t.Fatal(err)
	}
	diagnostic = openRecoveryDiagnosticStore(t, root)
	if _, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destinationPath, reportPath, plan); err == nil {
		t.Fatal("destination+report without promoted plan-bound state was adopted")
	}
}

func TestPrepareRecoveryDestinationRejectsReplacedCompletedReport(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("report replacement"))
	diagnostic, _, plan, planPath, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	if _, err := diagnostic.FinalizeRecoveryDestination(context.Background(), session, plan, false); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("{}\n")
	if err := os.WriteFile(reportPath, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restrictPathPermissions(reportPath, false); err != nil {
		t.Fatal(err)
	}
	diagnostic = openRecoveryDiagnosticStore(t, root)
	if _, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destinationPath, reportPath, plan); err == nil {
		t.Fatal("replaced completed report was accepted")
	}
	got, err := os.ReadFile(reportPath)
	if err != nil || string(got) != string(foreign) {
		t.Fatalf("replaced report was modified: data=%q err=%v", got, err)
	}
}

func TestFinalizeRecoveryDestinationInjectedHookErrorDoesNotClaimRollback(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("post state hook"))
	diagnostic, _, plan, _, destinationPath, reportPath, session := prepareAuditedRecoveryFixtureFromRoot(t, root)
	_, err := diagnostic.finalizeRecoveryDestinationWithOps(context.Background(), session, plan, false, recoveryFinalizeOps{
		afterStatePromoted: func() error { return errors.New("injected state transition boundary failure") },
	})
	if err == nil {
		t.Fatal("injected phase-boundary failure was ignored")
	}
	if _, statErr := os.Lstat(destinationPath); statErr != nil {
		t.Fatalf("promoted destination was falsely rolled back: %v", statErr)
	}
	if _, statErr := os.Lstat(reportPath); !os.IsNotExist(statErr) {
		t.Fatalf("report unexpectedly exists after injected boundary failure: %v", statErr)
	}
	assertRecoveryStatePhase(t, session, RecoveryPhasePromoted)
}
