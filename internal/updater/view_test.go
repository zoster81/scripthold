package updater

import (
	"context"
	"errors"
	"testing"
)

func TestSelfUpdateViewUsesProcessVersionWhenNotAdopted(t *testing.T) {
	var checkedVersion string
	view, err := observeSelfUpdateViewWith(
		context.Background(),
		"3.2.1",
		false,
		selfUpdateViewDeps{
			observeInstallation: func(context.Context) (InstallationStatus, error) {
				return InstallationStatus{Adopted: false}, nil
			},
			checkRelease: func(_ context.Context, version string, force bool) ReleaseCheckResult {
				checkedVersion = version
				if force {
					t.Fatal("force unexpectedly changed")
				}
				return ReleaseCheckResult{
					CurrentVersion:  version,
					LatestVersion:   "3.3.0",
					UpdateAvailable: true,
					UpdateMessage:   updateMessage(version, "3.3.0"),
				}
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if checkedVersion != "3.2.1" || view.ReleaseComparisonVersion != "3.2.1" ||
		view.Release.CurrentVersion != "3.2.1" || !view.Release.UpdateAvailable {
		t.Fatalf("unexpected unadopted view: %#v", view)
	}
}

func TestSelfUpdateViewUsesVerifiedInstalledVersionForAdoptedStates(t *testing.T) {
	tests := []struct {
		name         string
		status       InstallationStatus
		wantVersion  string
		wantRecovery bool
	}{
		{
			name: "stable",
			status: InstallationStatus{
				Adopted:          true,
				State:            ReconciliationStable,
				CurrentVersion:   "3.2.1",
				InstalledVersion: "3.2.1",
			},
			wantVersion: "3.2.1",
		},
		{
			name: "prepared",
			status: InstallationStatus{
				Adopted:           true,
				State:             ReconciliationPrepared,
				CurrentVersion:    "3.2.1",
				InstalledVersion:  "3.2.1",
				CandidateVersion:  "3.3.0",
				RecoveryAvailable: true,
			},
			wantVersion:  "3.2.1",
			wantRecovery: true,
		},
		{
			name: "committed",
			status: InstallationStatus{
				Adopted:           true,
				State:             ReconciliationCommitted,
				CurrentVersion:    "3.2.1",
				InstalledVersion:  "3.3.0",
				CandidateVersion:  "3.3.0",
				RecoveryAvailable: true,
			},
			wantVersion:  "3.3.0",
			wantRecovery: true,
		},
		{
			name: "rolled_back",
			status: InstallationStatus{
				Adopted:           true,
				State:             ReconciliationRolledBack,
				CurrentVersion:    "3.2.1",
				InstalledVersion:  "3.2.1",
				CandidateVersion:  "3.3.0",
				RecoveryAvailable: true,
			},
			wantVersion:  "3.2.1",
			wantRecovery: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var checkedVersion string
			view, err := observeSelfUpdateViewWith(
				context.Background(),
				"9.9.9",
				true,
				selfUpdateViewDeps{
					observeInstallation: func(context.Context) (InstallationStatus, error) {
						return test.status, nil
					},
					checkRelease: func(_ context.Context, version string, force bool) ReleaseCheckResult {
						checkedVersion = version
						if !force {
							t.Fatal("force was not preserved")
						}
						return ReleaseCheckResult{CurrentVersion: version}
					},
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if checkedVersion != test.wantVersion || view.ReleaseComparisonVersion != test.wantVersion ||
				view.Release.CurrentVersion != test.wantVersion {
				t.Fatalf("wrong release comparison version: %#v", view)
			}
			if view.Installation.RecoveryAvailable != test.wantRecovery {
				t.Fatalf("recovery changed during composition: %#v", view.Installation)
			}
		})
	}
}

func TestSelfUpdateViewSkipsRemoteCheckForUnverifiedAdoptedTarget(t *testing.T) {
	status := InstallationStatus{
		Adopted:           true,
		State:             ReconciliationRecoveryRequired,
		CurrentVersion:    "3.2.1",
		InstalledVersion:  "",
		CandidateVersion:  "3.3.0",
		RecoveryAvailable: true,
		Problem:           "installed candidate bytes failed full verification",
	}
	called := false
	view, err := observeSelfUpdateViewWith(
		context.Background(),
		"9.9.9",
		true,
		selfUpdateViewDeps{
			observeInstallation: func(context.Context) (InstallationStatus, error) {
				return status, nil
			},
			checkRelease: func(context.Context, string, bool) ReleaseCheckResult {
				called = true
				return ReleaseCheckResult{}
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("remote release check ran against an unverified installed version")
	}
	if view.ReleaseComparisonVersion != "" || view.Release != (ReleaseCheckResult{}) {
		t.Fatalf("unexpected remote result for unverified target: %#v", view)
	}
	if !view.Installation.RecoveryAvailable {
		t.Fatal("remote-check suppression changed local recovery availability")
	}
}

func TestSelfUpdateViewLocalObservationFailureDoesNotCallRemote(t *testing.T) {
	called := false
	injected := errors.New("local status unavailable")
	_, err := observeSelfUpdateViewWith(
		context.Background(),
		"3.2.1",
		false,
		selfUpdateViewDeps{
			observeInstallation: func(context.Context) (InstallationStatus, error) {
				return InstallationStatus{}, injected
			},
			checkRelease: func(context.Context, string, bool) ReleaseCheckResult {
				called = true
				return ReleaseCheckResult{}
			},
		},
	)
	if !errors.Is(err, injected) {
		t.Fatalf("error=%v, want injected local failure", err)
	}
	if called {
		t.Fatal("remote release check ran after local observation failure")
	}
}

func TestSelfUpdateViewNormalizesNilContext(t *testing.T) {
	var sawNonNil bool
	view, err := observeSelfUpdateViewWith(
		//lint:ignore SA1012 This test intentionally verifies nil-context normalization.
		nil,
		"3.2.1",
		false,
		selfUpdateViewDeps{
			observeInstallation: func(ctx context.Context) (InstallationStatus, error) {
				sawNonNil = ctx != nil
				return InstallationStatus{Adopted: false}, nil
			},
			checkRelease: func(ctx context.Context, version string, force bool) ReleaseCheckResult {
				if ctx == nil {
					t.Fatal("nil context reached release checker")
				}
				return ReleaseCheckResult{CurrentVersion: version}
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !sawNonNil || view.ReleaseComparisonVersion != "3.2.1" {
		t.Fatalf("nil context was not normalized: %#v", view)
	}
}
