package updater

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/filesystem"
)

func TestLaunchDetachedHelperStartsBeforeReleasingLauncherLocks(t *testing.T) {
	boundary, inspection, admission, transactionID, _, reconcileDeps := helperOwnershipFixture(t)
	released := false
	started := false
	start := func(path string, args []string, environment []string) (func() error, error) {
		started = true
		if path != filepath.Join(boundary.Directory, helperArtifactName) {
			t.Fatalf("helper path = %q", path)
		}
		if len(args) != 2 || args[0] != detachedHelperCommand || args[1] != transactionID {
			t.Fatalf("helper args = %#v", args)
		}
		for _, entry := range environment {
			name, _, _ := strings.Cut(entry, "=")
			switch name {
			case "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR", "TZ":
			default:
				t.Fatalf("unexpected helper environment entry %q", entry)
			}
		}
		if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false); !errors.Is(err, filesystem.ErrFileLockBusy) {
			t.Fatalf("control lock during start = %v, want busy", err)
		}
		if _, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockExclusive, false); !errors.Is(err, filesystem.ErrFileLockBusy) {
			t.Fatalf("use lock during start = %v, want busy", err)
		}
		return func() error {
			released = true
			return nil
		}, nil
	}

	if err := launchDetachedHelperWith(
		context.Background(),
		boundary,
		inspection,
		admission,
		transactionID,
		runtime.GOOS,
		runtime.GOARCH,
		helperLaunchDeps{reconciliation: reconcileDeps, start: start},
	); err != nil {
		t.Fatal(err)
	}
	if !started || !released {
		t.Fatalf("started=%v released=%v", started, released)
	}
	if err := admission.validateFor(boundary); err == nil {
		t.Fatal("successful helper launch did not consume process admission")
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatalf("control lock remained held: %v", err)
	}
	_ = control.Close()
	use, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.UseLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatalf("use lock remained held: %v", err)
	}
	_ = use.Close()
}

func TestLaunchDetachedHelperStartFailureKeepsAdmission(t *testing.T) {
	boundary, inspection, admission, transactionID, _, reconcileDeps := helperOwnershipFixture(t)
	injected := errors.New("start failed")
	err := launchDetachedHelperWith(
		context.Background(),
		boundary,
		inspection,
		admission,
		transactionID,
		runtime.GOOS,
		runtime.GOARCH,
		helperLaunchDeps{
			reconciliation: reconcileDeps,
			start: func(string, []string, []string) (func() error, error) {
				return nil, injected
			},
		},
	)
	if !errors.Is(err, injected) {
		t.Fatalf("launch error = %v, want injected start failure", err)
	}
	if err := admission.validateFor(boundary); err != nil {
		t.Fatalf("failed launch consumed admission: %v", err)
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		t.Fatalf("control lock remained held after start failure: %v", err)
	}
	_ = control.Close()
}
