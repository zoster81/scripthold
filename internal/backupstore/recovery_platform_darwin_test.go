//go:build darwin

package backupstore

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryDarwinVarAliasCannotBecomeSourceOrDestinationAuthority(t *testing.T) {
	root, _ := createRecoveryScanStore(t, []byte("macOS alias recovery"))
	if !strings.HasPrefix(root, "/private/var/") {
		t.Skipf("canonical temporary store is not under /private/var: %q", root)
	}
	aliasRoot := strings.TrimPrefix(root, "/private")
	before := snapshotDiagnosticTree(t, root)
	if opened, err := OpenExistingForDiagnosis(DiagnosticOpenOptions{Directory: aliasRoot}); err == nil {
		_ = opened.Close()
		t.Fatal("macOS /var alias was accepted as an existing recovery source root")
	}

	diagnostic := openRecoveryDiagnosticStore(t, root)
	plan, err := diagnostic.CreateRecoveryPlan(context.Background(), RecoveryBounds{})
	if err != nil {
		t.Fatal(err)
	}
	parent := filepath.Dir(root)
	planPath := filepath.Join(parent, "darwin-alias-plan.json")
	report := filepath.Join(parent, "darwin-alias-report.json")
	if err := diagnostic.WriteRecoveryPlan(context.Background(), planPath, plan, false); err != nil {
		t.Fatal(err)
	}
	aliasParent := strings.TrimPrefix(parent, "/private")
	if _, err := diagnostic.AuthorizeRecoveryApplyPaths(
		planPath,
		filepath.Join(aliasParent, "darwin-aliased-destination"),
		report,
		plan,
	); err == nil {
		t.Fatal("macOS /var destination parent alias was accepted for recovery")
	}
	if err := diagnostic.Close(); err != nil {
		t.Fatal(err)
	}
	assertDiagnosticTreeUnchanged(t, root, before)
}
