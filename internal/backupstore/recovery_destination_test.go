package backupstore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthorizeRecoveryApplyPathsIsDeterministicAndSeparated(t *testing.T) {
	diagnostic, plan, planPath, destination, report, _ := recoveryApplyFixture(t)

	first, err := diagnostic.AuthorizeRecoveryApplyPaths(planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	second, err := diagnostic.AuthorizeRecoveryApplyPaths(planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("recovery path authorization is nondeterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first.destination != destination || first.plan != planPath || first.report != report || !validHexIdentifier(first.destinationKey) {
		t.Fatalf("authorized paths=%#v", first)
	}
	if filepath.Dir(first.staging) != filepath.Dir(destination) || filepath.Dir(first.state) != filepath.Dir(destination) {
		t.Fatalf("staging/state are not same-parent: %#v", first)
	}
	for name, args := range map[string][3]string{
		"report equals plan":       {planPath, destination, planPath},
		"destination source child": {planPath, filepath.Join(diagnostic.root, "recovered"), report},
		"relative report":          {planPath, destination, "report.json"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := diagnostic.AuthorizeRecoveryApplyPaths(args[0], args[1], args[2], plan); err == nil {
				t.Fatalf("unsafe recovery paths accepted: %#v", args)
			}
		})
	}
}

func TestPrepareRecoveryDestinationCreatesFreshStagingAndResumesExactState(t *testing.T) {
	diagnostic, plan, planPath, destination, report, before := recoveryApplyFixture(t)

	first, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	if first.resumed {
		t.Fatal("fresh recovery destination reported resume")
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("requested destination was exposed before audit: %v", err)
	}
	assertOwnerOnlyPermissions(t, first.paths.staging, true)
	assertOwnerOnlyPermissions(t, first.paths.state, false)
	if first.state.PlanID != plan.PlanID || first.state.DestinationKey != first.paths.destinationKey ||
		!validHexIdentifier(first.state.DestinationStoreID) || first.state.Phase != RecoveryPhaseBuilding {
		t.Fatalf("fresh recovery state=%#v", first.state)
	}
	if first.store.descriptor.StoreID != first.state.DestinationStoreID || first.store.descriptor.StoreID == plan.SourceStoreID {
		t.Fatalf("destination StoreID not fresh/bound: destination=%q source=%q state=%#v", first.store.descriptor.StoreID, plan.SourceStoreID, first.state)
	}
	staging := first.paths.staging
	statePath := first.paths.state
	storeID := first.state.DestinationStoreID
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	if !second.resumed || second.paths.staging != staging || second.paths.state != statePath || second.state.DestinationStoreID != storeID {
		t.Fatalf("resume mismatch: %#v", second)
	}
	if second.store.descriptor.StoreID != storeID {
		t.Fatalf("resumed StoreID=%q want=%q", second.store.descriptor.StoreID, storeID)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, diagnostic.root, before)
}

func TestPrepareRecoveryDestinationRefusesWrongPlanAndUnknownResidue(t *testing.T) {
	t.Run("wrong plan state", func(t *testing.T) {
		diagnostic, plan, planPath, destination, report, _ := recoveryApplyFixture(t)
		session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
		if err != nil {
			t.Fatal(err)
		}
		statePath := session.paths.state
		staging := session.paths.staging
		state := session.state
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
		state.PlanID = strings.Repeat("f", 64)
		data, err := EncodeRecoveryState(state)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(statePath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan); err == nil {
			t.Fatal("wrong-plan recovery state was adopted")
		}
		if _, err := os.Lstat(staging); err != nil {
			t.Fatalf("foreign staging was removed: %v", err)
		}
		if _, err := os.Lstat(statePath); err != nil {
			t.Fatalf("foreign state was removed: %v", err)
		}
	})

	t.Run("unpaired staging residue", func(t *testing.T) {
		diagnostic, plan, planPath, destination, report, _ := recoveryApplyFixture(t)
		paths, err := diagnostic.AuthorizeRecoveryApplyPaths(planPath, destination, report, plan)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(paths.staging, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := restrictPathPermissions(paths.staging, true); err != nil {
			t.Fatal(err)
		}
		if _, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan); err == nil {
			t.Fatal("unpaired staging residue was adopted")
		}
		if _, err := os.Lstat(paths.staging); err != nil {
			t.Fatalf("unknown staging residue was removed: %v", err)
		}
		if _, err := os.Lstat(paths.state); !os.IsNotExist(err) {
			t.Fatalf("state was synthesized for unknown residue: %v", err)
		}
	})
}

func TestPrepareRecoveryDestinationRejectsPreexistingDestinationAndParentAlias(t *testing.T) {
	t.Run("preexisting destination", func(t *testing.T) {
		diagnostic, plan, planPath, destination, report, _ := recoveryApplyFixture(t)
		paths, err := diagnostic.AuthorizeRecoveryApplyPaths(planPath, destination, report, plan)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(destination, 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan); err == nil {
			t.Fatal("preexisting destination was accepted")
		}
		if _, err := os.Lstat(paths.staging); !os.IsNotExist(err) {
			t.Fatalf("staging was created after destination conflict: %v", err)
		}
		if _, err := os.Lstat(paths.state); !os.IsNotExist(err) {
			t.Fatalf("state was created after destination conflict: %v", err)
		}
	})

	t.Run("aliased parent", func(t *testing.T) {
		diagnostic, plan, planPath, destination, _, _ := recoveryApplyFixture(t)
		base := filepath.Dir(destination)
		realParent := filepath.Join(base, "real-recovery-parent")
		aliasParent := filepath.Join(base, "alias-recovery-parent")
		if err := os.Mkdir(realParent, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realParent, aliasParent); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := diagnostic.AuthorizeRecoveryApplyPaths(
			planPath,
			filepath.Join(aliasParent, "destination"),
			filepath.Join(realParent, "report.json"),
			plan,
		); err == nil {
			t.Fatal("aliased destination parent was accepted")
		}
	})
}

func TestPrepareRecoveryDestinationRejectsHardLinkedPlanAndState(t *testing.T) {
	t.Run("hard-linked plan", func(t *testing.T) {
		diagnostic, plan, planPath, destination, report, _ := recoveryApplyFixture(t)
		linkedPlan := filepath.Join(filepath.Dir(planPath), "linked-reviewed-plan.json")
		if err := os.Link(planPath, linkedPlan); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		if _, err := diagnostic.AuthorizeRecoveryApplyPaths(linkedPlan, destination, report, plan); err == nil {
			t.Fatal("hard-linked recovery plan was accepted")
		}
	})

	t.Run("hard-linked state", func(t *testing.T) {
		diagnostic, plan, planPath, destination, report, _ := recoveryApplyFixture(t)
		session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
		if err != nil {
			t.Fatal(err)
		}
		statePath := session.paths.state
		staging := session.paths.staging
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
		linkedState := statePath + ".link"
		if err := os.Link(statePath, linkedState); err != nil {
			t.Skipf("hard links unavailable: %v", err)
		}
		if _, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan); err == nil {
			t.Fatal("hard-linked recovery state was adopted")
		}
		if _, err := os.Lstat(staging); err != nil {
			t.Fatalf("staging was removed after hard-link conflict: %v", err)
		}
		if _, err := os.Lstat(statePath); err != nil {
			t.Fatalf("state was removed after hard-link conflict: %v", err)
		}
	})
}

func TestPrepareRecoveryDestinationRefusesStateOnlyCrashResidue(t *testing.T) {
	diagnostic, plan, planPath, destination, report, _ := recoveryApplyFixture(t)
	session, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan)
	if err != nil {
		t.Fatal(err)
	}
	staging := session.paths.staging
	statePath := session.paths.state
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(staging); err != nil {
		t.Fatal(err)
	}
	if _, err := diagnostic.PrepareRecoveryDestination(context.Background(), planPath, destination, report, plan); err == nil {
		t.Fatal("state-only crash residue was adopted")
	}
	if _, err := os.Lstat(statePath); err != nil {
		t.Fatalf("state-only crash residue was removed: %v", err)
	}
	if _, err := os.Lstat(staging); !os.IsNotExist(err) {
		t.Fatalf("staging was recreated for state-only residue: %v", err)
	}
}
func recoveryApplyFixture(t *testing.T) (*DiagnosticStore, RecoveryPlan, string, string, string, map[string]diagnosticTreeEntry) {
	t.Helper()
	root, _ := createRecoveryScanStore(t, []byte("recovery destination fixture"))
	before := snapshotDiagnosticTree(t, root)
	diagnostic := openRecoveryDiagnosticStore(t, root)
	plan, err := diagnostic.CreateRecoveryPlan(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "reviewed-plan.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	return diagnostic, plan, planPath, filepath.Join(parent, "recovered-store"), filepath.Join(parent, "recovery-report.json"), before
}
