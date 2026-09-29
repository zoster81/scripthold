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

func pendingPreparationFixture(t *testing.T) (*InstallationBoundary, *StandaloneInspection, *ProcessAdmission, *PreparedCandidate, installedEvidenceDeps, []byte, []byte) {
	t.Helper()
	parent := canonicalTempDir(t)
	installedBytes := []byte("installed-binary")
	target := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(target, installedBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	installedDeps := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	boundary, inspection, _, err := initializeStableAdoptionWith(context.Background(), target, runtime.GOOS, runtime.GOARCH, installedDeps)
	if err != nil {
		t.Fatal(err)
	}

	candidateBytes := []byte("candidate-binary")
	candidatePath := filepath.Join(t.TempDir(), "candidate-source")
	if err := os.WriteFile(candidatePath, candidateBytes, 0o700); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(candidateBytes)
	admission, err := admitStableProcessWith(context.Background(), boundary, inspection, runtime.GOOS, runtime.GOARCH, installedDeps)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admission.Close() })

	candidate := &PreparedCandidate{
		ReleaseID: 42,
		AssetID:   202,
		Tag:       "v3.3.0",
		Version:   "3.3.0",
		Commit:    strings.Repeat("c", 40),
		Size:      int64(len(candidateBytes)),
		SHA256:    hex.EncodeToString(sum[:]),
		path:      candidatePath,
	}
	return boundary, inspection, admission, candidate, installedDeps, installedBytes, candidateBytes
}

func TestPreparePendingTransactionPublishesArtifactsBeforePendingState(t *testing.T) {
	boundary, inspection, admission, candidate, installedDeps, installedBytes, candidateBytes := pendingPreparationFixture(t)
	hookCalled := false
	state, err := preparePendingTransactionWith(
		context.Background(), boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence: installedDeps,
			validateCandidate: func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:  func() (string, error) { return strings.Repeat("1", 64), nil },
			beforePendingWrite: func() error {
				hookCalled = true
				for _, name := range []string{knownGoodArtifactName, candidateArtifactName, helperArtifactName} {
					if _, statErr := os.Stat(filepath.Join(boundary.Directory, name)); statErr != nil {
						return statErr
					}
				}
				onDisk, readErr := readInstallationStateLocked(boundary, inspection)
				if readErr != nil {
					return readErr
				}
				if onDisk.Pending != nil {
					return errors.New("pending state became durable before artifact preparation completed")
				}
				return nil
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !hookCalled || state.Pending == nil || state.Pending.TransactionID != strings.Repeat("1", 64) {
		t.Fatalf("unexpected pending state: %#v", state.Pending)
	}
	if state.Pending.SourceSHA256 != state.Current.SHA256 || state.Pending.CandidateSHA256 != candidate.SHA256 {
		t.Fatalf("pending digest evidence mismatch: %#v", state.Pending)
	}

	checks := map[string][]byte{
		knownGoodArtifactName: installedBytes,
		helperArtifactName:    installedBytes,
		candidateArtifactName: candidateBytes,
	}
	for name, want := range checks {
		path := filepath.Join(boundary.Directory, name)
		if err := filesystem.ValidateOwnerOnlyExecutable(path); err != nil {
			t.Fatalf("%s permissions: %v", name, err)
		}
		got, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(got) != string(want) {
			t.Fatalf("%s bytes = %q, want %q", name, got, want)
		}
	}
	onDisk, err := readInstallationState(boundary, inspection)
	if err != nil {
		t.Fatal(err)
	}
	if onDisk.Pending == nil || *onDisk.Pending != *state.Pending {
		t.Fatalf("pending state was not persisted: %#v", onDisk.Pending)
	}
}

func TestPreparePendingTransactionRollsBackArtifactsBeforeStatePublication(t *testing.T) {
	boundary, inspection, admission, candidate, installedDeps, _, _ := pendingPreparationFixture(t)
	injected := errors.New("injected before pending state")
	_, err := preparePendingTransactionWith(
		context.Background(), boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence:  installedDeps,
			validateCandidate:  func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:   func() (string, error) { return strings.Repeat("2", 64), nil },
			beforePendingWrite: func() error { return injected },
		},
	)
	if !errors.Is(err, injected) {
		t.Fatalf("error = %v, want injected failure", err)
	}
	for _, name := range []string{knownGoodArtifactName, candidateArtifactName, helperArtifactName} {
		if _, statErr := os.Stat(filepath.Join(boundary.Directory, name)); !os.IsNotExist(statErr) {
			t.Fatalf("%s remains after rollback: %v", name, statErr)
		}
	}
	state, err := readInstallationState(boundary, inspection)
	if err != nil {
		t.Fatal(err)
	}
	if state.Pending != nil {
		t.Fatalf("pending state unexpectedly durable: %#v", state.Pending)
	}
}

func TestPreparePendingTransactionCancellationBeforePendingPublicationRollsBackArtifacts(t *testing.T) {
	boundary, inspection, admission, candidate, installedDeps, _, _ := pendingPreparationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := preparePendingTransactionWith(
		ctx, boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence: installedDeps,
			validateCandidate: func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:  func() (string, error) { return strings.Repeat("5", 64), nil },
			beforePendingWrite: func() error {
				cancel()
				return nil
			},
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	for _, name := range []string{knownGoodArtifactName, candidateArtifactName, helperArtifactName} {
		if _, statErr := os.Stat(filepath.Join(boundary.Directory, name)); !os.IsNotExist(statErr) {
			t.Fatalf("%s remains after cancelled publication: %v", name, statErr)
		}
	}
	state, stateErr := readInstallationState(boundary, inspection)
	if stateErr != nil {
		t.Fatal(stateErr)
	}
	if state.Pending != nil {
		t.Fatalf("cancelled transaction became durable: %#v", state.Pending)
	}
}

func TestPreparePendingTransactionRejectsExistingArtifact(t *testing.T) {
	boundary, inspection, admission, candidate, installedDeps, _, _ := pendingPreparationFixture(t)
	path := filepath.Join(boundary.Directory, knownGoodArtifactName)
	if err := os.WriteFile(path, []byte("preexisting"), 0o700); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := filesystem.RestrictOwnerOnlyExecutable(path); err != nil {
			t.Fatal(err)
		}
	}
	_, err := preparePendingTransactionWith(
		context.Background(), boundary, inspection, admission, candidate, runtime.GOOS, runtime.GOARCH,
		pendingPreparationDeps{
			installedEvidence: installedDeps,
			validateCandidate: func(context.Context, *PreparedCandidate, string, string) error { return nil },
			newTransactionID:  func() (string, error) { return strings.Repeat("3", 64), nil },
		},
	)
	if err == nil {
		t.Fatal("preexisting fixed artifact must fail closed")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "preexisting" {
		t.Fatalf("preexisting artifact was modified: %q", got)
	}
}

func TestPendingStateRejectsInvalidTransactionEvidence(t *testing.T) {
	_, inspection, state := validStableStateForTest(t)
	state.Pending = &installationPendingState{
		TransactionID:    "bad",
		SourceVersion:    state.Current.Version,
		SourceSHA256:     state.Current.SHA256,
		CandidateVersion: "3.3.0",
		CandidateSHA256:  strings.Repeat("c", 64),
		CandidateBuild: installationBuildIdentity{
			Module: officialModulePath, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			VCS: "git", Revision: strings.Repeat("d", 40), VCSClean: true,
		},
		ReleaseID: 42, AssetID: 202, Tag: "v3.3.0", Commit: strings.Repeat("d", 40),
	}
	if err := validateInstallationState(state, inspection); err == nil {
		t.Fatal("invalid transaction ID must fail closed")
	}
}
