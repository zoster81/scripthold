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

func TestTryRunSelfUpdateCommandUpdate(t *testing.T) {
	tests := []struct {
		name       string
		result     updater.UpdateLaunchResult
		err        error
		wantCode   int
		wantStdout []string
		wantStderr []string
	}{
		{
			name: "current",
			result: updater.UpdateLaunchResult{
				Status:         updater.UpdateLaunchCurrent,
				CurrentVersion: "3.2.1",
			},
			wantStdout: []string{"already up to date", "3.2.1"},
		},
		{
			name: "dispatched",
			result: updater.UpdateLaunchResult{
				Status:           updater.UpdateLaunchDispatched,
				CurrentVersion:   "3.2.1",
				CandidateVersion: "3.3.0",
			},
			wantStdout: []string{"helper has started", "3.2.1", "3.3.0", "update status"},
		},
		{
			name:       "pre-dispatch failure",
			err:        errors.New("private path detail"),
			wantCode:   1,
			wantStderr: []string{"failed safely", "no update helper was started", "update status"},
		},
		{
			name: "post-dispatch failure",
			result: updater.UpdateLaunchResult{
				Status:           updater.UpdateLaunchDispatched,
				CurrentVersion:   "3.2.1",
				CandidateVersion: "3.3.0",
			},
			err:        errors.New("private path detail"),
			wantCode:   1,
			wantStderr: []string{"helper has started", "launcher cleanup", "update status", "3.2.1", "3.3.0"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			updateCalls := 0
			code, matched := tryRunSelfUpdateCommandWithDeps(
				context.Background(), []string{"update"}, &stdout, &stderr, "3.2.1",
				selfUpdateCommandDeps{
					update: func(context.Context) (updater.UpdateLaunchResult, error) {
						updateCalls++
						return test.result, test.err
					},
				},
			)
			if !matched || code != test.wantCode || updateCalls != 1 {
				t.Fatalf("matched=%v code=%d updateCalls=%d stdout=%q stderr=%q", matched, code, updateCalls, stdout.String(), stderr.String())
			}
			for _, expected := range test.wantStdout {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("stdout %q missing %q", stdout.String(), expected)
				}
			}
			for _, expected := range test.wantStderr {
				if !strings.Contains(stderr.String(), expected) {
					t.Fatalf("stderr %q missing %q", stderr.String(), expected)
				}
			}
			if strings.Contains(stdout.String(), "private path detail") || strings.Contains(stderr.String(), "private path detail") {
				t.Fatalf("private error detail leaked: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestTryRunSelfUpdateCommandRejectsInvalidUpdateResult(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code, matched := tryRunSelfUpdateCommandWithDeps(
		context.Background(), []string{"update"}, &stdout, &stderr, "3.2.1",
		selfUpdateCommandDeps{
			update: func(context.Context) (updater.UpdateLaunchResult, error) {
				return updater.UpdateLaunchResult{Status: updater.UpdateLaunchStatus("unexpected")}, nil
			},
		},
	)
	if !matched || code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "failed safely") {
		t.Fatalf("matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}
}

func TestTryRunSelfUpdateCommandAdopt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	adoptCalls := 0
	code, matched := tryRunSelfUpdateCommandWithDeps(
		context.Background(), []string{"update", "--adopt"}, &stdout, &stderr, "3.2.1",
		selfUpdateCommandDeps{
			adopt: func(context.Context) error {
				adoptCalls++
				return nil
			},
		},
	)
	if !matched || code != 0 || adoptCalls != 1 || stderr.Len() != 0 {
		t.Fatalf("matched=%v code=%d adoptCalls=%d stdout=%q stderr=%q", matched, code, adoptCalls, stdout.String(), stderr.String())
	}
	for _, expected := range []string{
		"Adoption complete.",
		"No update was downloaded or installed.",
		"must have been stopped before adoption began",
		"must remain stopped until this command exits",
	} {
		if !strings.Contains(stdout.String(), expected) {
			t.Fatalf("adoption output %q missing %q", stdout.String(), expected)
		}
	}

	stdout.Reset()
	stderr.Reset()
	code, matched = tryRunSelfUpdateCommandWithDeps(
		context.Background(), []string{"update", "--adopt"}, &stdout, &stderr, "3.2.1",
		selfUpdateCommandDeps{adopt: func(context.Context) error { return errors.New("private path detail") }},
	)
	if !matched || code != 1 || stdout.Len() != 0 ||
		!strings.Contains(stderr.String(), "adoption failed safely") || strings.Contains(stderr.String(), "private path detail") {
		t.Fatalf("failure matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}
}

func TestTryRunSelfUpdateCommandRecover(t *testing.T) {
	tests := []struct {
		name       string
		started    bool
		err        error
		wantCode   int
		wantStdout []string
		wantStderr []string
	}{
		{
			name:       "dispatched",
			started:    true,
			wantStdout: []string{"recovery helper has started", "update status"},
		},
		{
			name:       "pre-dispatch failure",
			err:        errors.New("private path detail"),
			wantCode:   1,
			wantStderr: []string{"recovery failed safely", "no recovery helper was started", "update status"},
		},
		{
			name:       "post-dispatch failure",
			started:    true,
			err:        errors.New("private path detail"),
			wantCode:   1,
			wantStderr: []string{"recovery helper has started", "launcher cleanup", "update status"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			recoverCalls := 0
			code, matched := tryRunSelfUpdateCommandWithDeps(
				context.Background(), []string{"update", "recover"}, &stdout, &stderr, "3.2.1",
				selfUpdateCommandDeps{
					recover: func(context.Context) (bool, error) {
						recoverCalls++
						return test.started, test.err
					},
				},
			)
			if !matched || code != test.wantCode || recoverCalls != 1 {
				t.Fatalf("matched=%v code=%d recoverCalls=%d stdout=%q stderr=%q", matched, code, recoverCalls, stdout.String(), stderr.String())
			}
			for _, expected := range test.wantStdout {
				if !strings.Contains(stdout.String(), expected) {
					t.Fatalf("stdout %q missing %q", stdout.String(), expected)
				}
			}
			for _, expected := range test.wantStderr {
				if !strings.Contains(stderr.String(), expected) {
					t.Fatalf("stderr %q missing %q", stderr.String(), expected)
				}
			}
			if strings.Contains(stdout.String(), "private path detail") || strings.Contains(stderr.String(), "private path detail") {
				t.Fatalf("private error detail leaked: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}

	var stdout, stderr bytes.Buffer
	code, matched := tryRunSelfUpdateCommandWithDeps(
		context.Background(), []string{"update", "recover"}, &stdout, &stderr, "3.2.1",
		selfUpdateCommandDeps{recover: func(context.Context) (bool, error) { return false, nil }},
	)
	if !matched || code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "invalid internal result") {
		t.Fatalf("invalid result matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
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
		context.Background(), []string{"update", "unsupported"}, &stdout, &stderr, "3.2.1", nil,
	)
	if !matched || code != 1 || !strings.Contains(stderr.String(), selfUpdateUsage) {
		t.Fatalf("unsupported matched=%v code=%d stdout=%q stderr=%q", matched, code, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	code, matched = tryRunSelfUpdateCommandWith(
		context.Background(), []string{"update", "--help"}, &stdout, &stderr, "3.2.1", nil,
	)
	if !matched || code != 0 || stdout.String() != selfUpdateUsage+"\n" || stderr.Len() != 0 ||
		!strings.Contains(stdout.String(), "scripthold update\n") ||
		!strings.Contains(stdout.String(), "scripthold update --adopt") ||
		!strings.Contains(stdout.String(), "scripthold update recover") ||
		!strings.Contains(stdout.String(), "stop every other Scripthold process") {
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
