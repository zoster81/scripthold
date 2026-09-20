package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/filesystem"
)

type helperLaunchDeps struct {
	reconciliation reconciliationDeps
	start          func(string, []string, []string) (func() error, error)
}

// PreparePendingTransactionAndLaunch durably publishes one verified transaction
// and then dispatches its fixed copied helper. On successful dispatch it consumes
// the caller's normal-process admission so the helper can obtain exclusive use.
func PreparePendingTransactionAndLaunch(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	admission *ProcessAdmission,
	candidate *PreparedCandidate,
) error {
	state, err := preparePendingTransactionWith(
		ctx,
		boundary,
		inspection,
		admission,
		candidate,
		runtime.GOOS,
		runtime.GOARCH,
		pendingPreparationDeps{},
	)
	if err != nil {
		return err
	}
	if state.Pending == nil {
		return errors.New("published update transaction has no pending evidence")
	}
	return launchDetachedHelperWith(
		ctx,
		boundary,
		inspection,
		admission,
		state.Pending.TransactionID,
		runtime.GOOS,
		runtime.GOARCH,
		helperLaunchDeps{},
	)
}

func launchDetachedHelperWith(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	admission *ProcessAdmission,
	transactionID, goos, goarch string,
	deps helperLaunchDeps,
) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return err
	}
	if err := admission.validateFor(boundary); err != nil {
		return err
	}
	args, err := DetachedHelperArguments(transactionID)
	if err != nil {
		return err
	}
	if deps.start == nil {
		deps.start = startDetachedSelfUpdateHelper
	}

	control, err := filesystem.TryAcquireOwnerOnlyFileLock(
		boundary.ControlLockPath,
		filesystem.LockExclusive,
		false,
	)
	if err != nil {
		return fmt.Errorf("acquire helper launch control lock: %w", err)
	}
	releaseControl := true
	defer func() {
		if releaseControl {
			err = errors.Join(err, control.Close())
		}
	}()
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate helper launch control lock: %w", err)
	}
	state, err := readInstallationStateForReconciliationLocked(boundary, inspection)
	if err != nil {
		return err
	}
	if state.Pending == nil || state.Pending.TransactionID != transactionID {
		return errors.New("helper launch transaction does not match pending state")
	}
	result, err := reconcileInstallationLocked(
		ctx,
		boundary,
		inspection,
		goos,
		goarch,
		deps.reconciliation,
		control,
	)
	if err != nil {
		return err
	}
	if result.Status != ReconciliationPrepared {
		return fmt.Errorf("helper launch requires prepared transaction, observed %s", result.Status)
	}
	if observeFixedArtifact(boundary, helperArtifactName, state.Pending.SourceSHA256) != artifactValid {
		return errors.New("helper launch artifact no longer matches source evidence")
	}
	helperPath := filepath.Join(boundary.Directory, helperArtifactName)
	if err := filesystem.ValidateOwnerOnlyExecutable(helperPath); err != nil {
		return fmt.Errorf("validate helper launch executable: %w", err)
	}

	releaseProcess, err := deps.start(
		helperPath,
		args,
		minimalSelfUpdateHelperEnvironment(os.Environ()),
	)
	if err != nil {
		return fmt.Errorf("start detached self-update helper: %w", err)
	}
	if releaseProcess == nil {
		return errors.New("detached self-update helper started without a releasable process handle")
	}

	releaseControl = false
	controlErr := control.Close()
	admissionErr := admission.Close()
	processErr := releaseProcess()
	return errors.Join(controlErr, admissionErr, processErr)
}

func startDetachedSelfUpdateHelper(
	executable string,
	args []string,
	environment []string,
) (func() error, error) {
	command := exec.Command(executable, args...)
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	command.Env = append([]string(nil), environment...)
	configureDetachedSelfUpdateHelper(command)
	if err := command.Start(); err != nil {
		return nil, err
	}
	if command.Process == nil {
		return nil, errors.New("detached helper process handle is unavailable")
	}
	return command.Process.Release, nil
}

func minimalSelfUpdateHelperEnvironment(environment []string) []string {
	allowed := []string{"SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR", "TZ"}
	values := make(map[string]string, len(allowed))
	for _, entry := range environment {
		name, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		key := strings.ToUpper(name)
		for _, allowedName := range allowed {
			if key == allowedName {
				values[allowedName] = value
				break
			}
		}
	}
	result := make([]string, 0, len(values))
	for _, name := range allowed {
		if value, ok := values[name]; ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}
