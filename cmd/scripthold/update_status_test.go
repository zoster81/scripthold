package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/updater"
)

func TestTryRunSelfUpdateCommandStatus(t *testing.T) {
	view := updater.SelfUpdateView{
		Installation: updater.InstallationStatus{
			Adopted:           true,
			State:             updater.ReconciliationCommitted,
			CurrentVersion:    "3.2.1",
			InstalledVersion:  "3.3.0",
			CandidateVersion:  "3.3.0",
			RecoveryAvailable: true,
		},
		ReleaseComparisonVersion: "3.3.0",
		Release: updater.ReleaseCheckResult{
			CurrentVersion:  "3.3.0",
			LatestVersion:   "3.4.0",
			UpdateAvailable: true,
		},
	}
	var stdout, stderr bytes.Buffer
	code, matched := tryRunSelfUpdateCommandWith(
		context.Background(),
		[]string{"update", "status"},
		&stdout,
		&stderr,
		"9.9.9",
		func(context.Context, string, bool) (updater.SelfUpdateView, error) {
			return view, nil
		},
	)
	if !matched || code != 0 || stderr.Len() != 0 {
		t.Fatalf("matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}
	for _, expected := range []string{
		"Adopted: yes",
		"State: committed",
		"Process version: 9.9.9",
		"Durable current version: 3.2.1",
		"Verified installed version: 3.3.0",
		"Release comparison version: 3.3.0",
		"Latest version: 3.4.0",
		"Update available: yes",
		"Recovery available: yes",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("status output %q missing %q", stdout.String(), expected)
		}
	}
}

func TestTryRunSelfUpdateCommandShowsSuppressedRemoteCheck(t *testing.T) {
	view := updater.SelfUpdateView{
		Installation: updater.InstallationStatus{
			Adopted:           true,
			State:             updater.ReconciliationRecoveryRequired,
			CurrentVersion:    "3.2.1",
			CandidateVersion:  "3.3.0",
			RecoveryAvailable: true,
			Problem:           "installed candidate bytes failed full verification",
		},
	}
	var stdout, stderr bytes.Buffer
	code, matched := tryRunSelfUpdateCommandWith(
		context.Background(), []string{"update", "status"}, &stdout, &stderr, "3.2.1",
		func(context.Context, string, bool) (updater.SelfUpdateView, error) { return view, nil },
	)
	if !matched || code != 0 || stderr.Len() != 0 {
		t.Fatalf("matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}
	for _, expected := range []string{
		"State: recovery_required",
		"Verified installed version: unverified",
		"Release comparison version: suppressed",
		"Latest version: unknown",
		"Update available: unknown",
		"Recovery available: yes",
		"Problem: installed candidate bytes failed full verification",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("status output %q missing %q", stdout.String(), expected)
		}
	}
}

func TestTryRunSelfUpdateCommandObservedStateAlwaysExitsZero(t *testing.T) {
	for _, state := range []updater.ReconciliationStatus{
		updater.ReconciliationStable,
		updater.ReconciliationPrepared,
		updater.ReconciliationCommitted,
		updater.ReconciliationRolledBack,
		updater.ReconciliationRecoveryRequired,
	} {
		t.Run(string(state), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code, matched := tryRunSelfUpdateCommandWith(
				context.Background(), []string{"update", "status"}, &stdout, &stderr, "3.2.1",
				func(context.Context, string, bool) (updater.SelfUpdateView, error) {
					return updater.SelfUpdateView{Installation: updater.InstallationStatus{Adopted: true, State: state}}, nil
				},
			)
			if !matched || code != 0 || stderr.Len() != 0 {
				t.Fatalf("state=%s matched=%v code=%d stderr=%q", state, matched, code, stderr.String())
			}
		})
	}
}

func TestTryRunSelfUpdateCommandFailureAndUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, matched := tryRunSelfUpdateCommandWith(
		context.Background(), []string{"update", "status"}, &stdout, &stderr, "3.2.1",
		func(context.Context, string, bool) (updater.SelfUpdateView, error) {
			return updater.SelfUpdateView{}, errors.New("private path detail")
		},
	)
	if !matched || code != 1 || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "could not be observed safely") ||
		strings.Contains(stderr.String(), "private path detail") {
		t.Fatalf("failure matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code, matched = tryRunSelfUpdateCommandWith(
		context.Background(), []string{"update", "recover"}, &stdout, &stderr, "3.2.1", nil,
	)
	if !matched || code != 1 || !strings.Contains(stderr.String(), selfUpdateStatusUsage) {
		t.Fatalf("unsupported matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code, matched = tryRunSelfUpdateCommandWith(
		context.Background(), []string{"update", "--help"}, &stdout, &stderr, "3.2.1", nil,
	)
	if !matched || code != 0 || stdout.String() != selfUpdateStatusUsage+"\n" || stderr.Len() != 0 {
		t.Fatalf("help matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}
}

func TestTryRunSelfUpdateCommandDoesNotClaimOtherArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, matched := tryRunSelfUpdateCommandWith(
		context.Background(), []string{"--", "update"}, &stdout, &stderr, "3.2.1", nil,
	)
	if matched || code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("separator directory was claimed: matched=%v code=%d", matched, code)
	}
}

func TestRunCommandUpdateStatusRoutesBeforeServerConfiguration(t *testing.T) {
	originalVersion := version
	version = "dev"
	t.Cleanup(func() { version = originalVersion })

	var stdout, stderr bytes.Buffer
	code := runCommand(
		context.Background(),
		[]string{"update", "status"},
		&stdout,
		&stderr,
		func(name string) string {
			if name == envTransport {
				return "unsupported"
			}
			return ""
		},
	)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "State: not_adopted") ||
		!strings.Contains(stdout.String(), "Process version: dev") {
		t.Fatalf("unexpected status output: %q", stdout.String())
	}
}
