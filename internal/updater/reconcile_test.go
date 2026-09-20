package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func fixtureTargetObserver(sourceBytes, candidateBytes []byte) func(context.Context, *StandaloneInspection, string, string) (installationCurrentState, error) {
	return func(_ context.Context, inspection *StandaloneInspection, _, _ string) (installationCurrentState, error) {
		data, err := os.ReadFile(inspection.ExecutablePath)
		if err != nil {
			return installationCurrentState{}, err
		}
		switch string(data) {
		case string(sourceBytes):
			return installationCurrentState{
				Version: "3.2.1",
				SHA256:  bytesSHA256ForTest(sourceBytes),
				Build: installationBuildIdentity{
					Module: officialModulePath, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
					VCS: "git", Revision: strings.Repeat("b", 40), VCSClean: true,
				},
			}, nil
		case string(candidateBytes):
			return installationCurrentState{
				Version: "3.3.0",
				SHA256:  bytesSHA256ForTest(candidateBytes),
				Build: installationBuildIdentity{
					Module: officialModulePath, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
					VCS: "git", Revision: strings.Repeat("c", 40), VCSClean: true,
				},
			}, nil
		default:
			return installationCurrentState{}, errors.New("unknown target bytes")
		}
	}
}

func bytesSHA256ForTest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func prepareReconciliationFixture(t *testing.T) (*InstallationBoundary, *StandaloneInspection, []byte, []byte, reconciliationDeps) {
	t.Helper()
	boundary, inspection, admission, candidate, installedDeps, sourceBytes, candidateBytes := pendingPreparationFixture(t)
	if _, err := preparePendingTransactionWith(
		context.Background(), boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence: installedDeps,
			validateCandidate: func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:  func() (string, error) { return strings.Repeat("1", 64), nil },
		},
	); err != nil {
		t.Fatal(err)
	}
	return boundary, inspection, sourceBytes, candidateBytes, reconciliationDeps{
		observeTarget: fixtureTargetObserver(sourceBytes, candidateBytes),
	}
}

func TestReconcilePendingPreparedIsReadOnly(t *testing.T) {
	boundary, inspection, _, _, deps := prepareReconciliationFixture(t)
	before := snapshotReconciliationFilesForTest(t, boundary, inspection.ExecutablePath)

	result, err := reconcileInstallationWith(context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationPrepared {
		t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationPrepared, result.Problem)
	}
	after := snapshotReconciliationFilesForTest(t, boundary, inspection.ExecutablePath)
	if !reconciliationSnapshotsEqualForTest(before, after) {
		t.Fatal("reconciliation modified target/state/artifacts")
	}
}

func TestReconcilePendingCommittedAfterCandidateConsumption(t *testing.T) {
	boundary, inspection, _, candidateBytes, deps := prepareReconciliationFixture(t)
	candidatePath := filepath.Join(boundary.Directory, candidateArtifactName)
	if err := os.Remove(inspection.ExecutablePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(candidatePath, inspection.ExecutablePath); err != nil {
		t.Fatal(err)
	}
	currentInspection, err := inspectStandaloneExecutable(inspection.ExecutablePath, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(currentInspection.ExecutablePath); err != nil || string(data) != string(candidateBytes) {
		t.Fatalf("target candidate bytes = %q, err=%v", data, err)
	}

	result, err := reconcileInstallationWith(context.Background(), boundary, currentInspection, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationCommitted {
		t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationCommitted, result.Problem)
	}
}

func TestReconcilePendingRolledBackAfterCandidateConsumption(t *testing.T) {
	boundary, inspection, sourceBytes, _, deps := prepareReconciliationFixture(t)
	if err := os.Remove(filepath.Join(boundary.Directory, candidateArtifactName)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(inspection.ExecutablePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inspection.ExecutablePath, sourceBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	currentInspection, err := inspectStandaloneExecutable(inspection.ExecutablePath, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}

	result, err := reconcileInstallationWith(context.Background(), boundary, currentInspection, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationRolledBack {
		t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationRolledBack, result.Problem)
	}
}

func TestReconcilePendingRejectsAmbiguousAndDamagedStates(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *InstallationBoundary, *StandaloneInspection, []byte, []byte) *StandaloneInspection
	}{
		{
			name: "unknown target bytes",
			mutate: func(t *testing.T, _ *InstallationBoundary, inspection *StandaloneInspection, _, _ []byte) *StandaloneInspection {
				if err := os.WriteFile(inspection.ExecutablePath, []byte("unknown"), 0o700); err != nil {
					t.Fatal(err)
				}
				return inspection
			},
		},
		{
			name: "known good missing",
			mutate: func(t *testing.T, boundary *InstallationBoundary, inspection *StandaloneInspection, _, _ []byte) *StandaloneInspection {
				if err := os.Remove(filepath.Join(boundary.Directory, knownGoodArtifactName)); err != nil {
					t.Fatal(err)
				}
				return inspection
			},
		},
		{
			name: "helper corrupt",
			mutate: func(t *testing.T, boundary *InstallationBoundary, inspection *StandaloneInspection, _, _ []byte) *StandaloneInspection {
				if err := os.WriteFile(filepath.Join(boundary.Directory, helperArtifactName), []byte("corrupt-helper"), 0o700); err != nil {
					t.Fatal(err)
				}
				return inspection
			},
		},
		{
			name: "target candidate while candidate still staged",
			mutate: func(t *testing.T, _ *InstallationBoundary, inspection *StandaloneInspection, _, candidate []byte) *StandaloneInspection {
				if err := os.WriteFile(inspection.ExecutablePath, candidate, 0o700); err != nil {
					t.Fatal(err)
				}
				return inspection
			},
		},
		{
			name: "candidate corrupt while source installed",
			mutate: func(t *testing.T, boundary *InstallationBoundary, inspection *StandaloneInspection, _, _ []byte) *StandaloneInspection {
				if err := os.WriteFile(filepath.Join(boundary.Directory, candidateArtifactName), []byte("corrupt-candidate"), 0o700); err != nil {
					t.Fatal(err)
				}
				return inspection
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			boundary, inspection, sourceBytes, candidateBytes, deps := prepareReconciliationFixture(t)
			currentInspection := test.mutate(t, boundary, inspection, sourceBytes, candidateBytes)
			result, err := reconcileInstallationWith(context.Background(), boundary, currentInspection, runtime.GOOS, runtime.GOARCH, deps)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != ReconciliationRecoveryRequired {
				t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationRecoveryRequired, result.Problem)
			}
		})
	}
}

func TestReconcileStableRequiresStrictTargetIdentity(t *testing.T) {
	parent := t.TempDir()
	target := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	sourceBytes := []byte("stable-binary")
	if err := os.WriteFile(target, sourceBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	depsInstalled := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	boundary, _, _, err := initializeStableAdoptionWith(context.Background(), target, runtime.GOOS, runtime.GOARCH, depsInstalled)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, sourceBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	currentInspection, err := inspectStandaloneExecutable(target, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	result, err := reconcileInstallationWith(context.Background(), boundary, currentInspection, runtime.GOOS, runtime.GOARCH, reconciliationDeps{
		observeTarget: fixtureTargetObserver(sourceBytes, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationRecoveryRequired {
		t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationRecoveryRequired, result.Problem)
	}
}

func snapshotReconciliationFilesForTest(t *testing.T, boundary *InstallationBoundary, target string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, path := range []string{
		target,
		filepath.Join(boundary.Directory, installationStateFileName),
		filepath.Join(boundary.Directory, knownGoodArtifactName),
		filepath.Join(boundary.Directory, candidateArtifactName),
		filepath.Join(boundary.Directory, helperArtifactName),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result[path] = string(data)
	}
	return result
}

func reconciliationSnapshotsEqualForTest(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func TestReconcileCorruptStateRequiresRecovery(t *testing.T) {
	boundary, inspection, _, _, deps := prepareReconciliationFixture(t)
	statePath := filepath.Join(boundary.Directory, installationStateFileName)
	if err := os.WriteFile(statePath, []byte("{\"formatVersion\":1,\"broken\":true}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyPath(statePath, false); err != nil {
			t.Fatal(err)
		}
	}
	result, err := reconcileInstallationWith(context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ReconciliationRecoveryRequired {
		t.Fatalf("status = %q, want %q; problem=%q", result.Status, ReconciliationRecoveryRequired, result.Problem)
	}
}
