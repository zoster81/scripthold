package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

// UpdateLaunchStatus describes whether an update launch was unnecessary or dispatched.
type UpdateLaunchStatus string

const (
	UpdateLaunchCurrent    UpdateLaunchStatus = "current"
	UpdateLaunchDispatched UpdateLaunchStatus = "dispatched"
)

// UpdateLaunchResult reports the verified source/candidate versions and launch disposition.
type UpdateLaunchResult struct {
	Status           UpdateLaunchStatus
	CurrentVersion   string
	CandidateVersion string
}

type currentUpdateDeps struct {
	installedEvidence installedEvidenceDeps
	prepareCandidate  func(context.Context, string, string) (*PreparedCandidate, error)
	launch            func(context.Context, *InstallationBoundary, *StandaloneInspection, *ProcessAdmission, *PreparedCandidate) (bool, error)
}

// LaunchCurrentUpdate verifies the current adopted installation, prepares the
// next official release when one exists, and dispatches the detached switch
// helper. A dispatched result means the helper process was started, not that
// the replacement has already completed.
func LaunchCurrentUpdate(ctx context.Context) (UpdateLaunchResult, error) {
	path, err := os.Executable()
	if err != nil {
		return UpdateLaunchResult{}, fmt.Errorf("resolve running executable for self-update: %w", err)
	}
	return launchCurrentUpdateWith(ctx, path, runtime.GOOS, runtime.GOARCH, currentUpdateDeps{})
}

func launchCurrentUpdateWith(
	ctx context.Context,
	executablePath, goos, goarch string,
	deps currentUpdateDeps,
) (result UpdateLaunchResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return UpdateLaunchResult{}, err
	}
	if deps.prepareCandidate == nil {
		deps.prepareCandidate = PrepareCandidate
	}
	if deps.launch == nil {
		deps.launch = PreparePendingTransactionAndLaunch
	}

	inspection, err := inspectStandaloneExecutable(executablePath, goos)
	if err != nil {
		return UpdateLaunchResult{}, err
	}
	boundary, err := openInstallationBoundary(inspection, false)
	if err != nil {
		return UpdateLaunchResult{}, fmt.Errorf("open adopted installation state: %w", err)
	}
	admission, err := admitStableProcessWith(ctx, boundary, inspection, goos, goarch, deps.installedEvidence)
	if err != nil {
		return UpdateLaunchResult{}, fmt.Errorf("admit current installation for update: %w", err)
	}
	defer func() {
		err = errors.Join(err, admission.Close())
	}()

	state, err := readInstallationState(boundary, inspection)
	if err != nil {
		return UpdateLaunchResult{}, fmt.Errorf("read stable installation state: %w", err)
	}
	if state.Pending != nil {
		return UpdateLaunchResult{}, errors.New("an update transaction is already pending")
	}
	if state.Current.Version == "" {
		return UpdateLaunchResult{}, errors.New("stable installation version is unavailable")
	}
	result.CurrentVersion = state.Current.Version

	stagingDirectory, err := createCurrentUpdateStagingDirectory(inspection.ParentPath)
	if err != nil {
		return result, err
	}
	defer func() {
		err = errors.Join(err, removeCurrentUpdateStagingDirectory(stagingDirectory))
	}()

	candidate, err := deps.prepareCandidate(ctx, state.Current.Version, stagingDirectory)
	if errors.Is(err, errNoUpdateAvailable) {
		result.Status = UpdateLaunchCurrent
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("prepare self-update candidate: %w", err)
	}
	if candidate == nil {
		return result, errors.New("candidate preparation returned no candidate")
	}
	defer func() {
		err = errors.Join(err, candidate.Cleanup())
	}()

	result.CandidateVersion = candidate.Version
	if err := ctx.Err(); err != nil {
		return result, err
	}
	started, launchErr := deps.launch(ctx, boundary, inspection, admission, candidate)
	if started {
		result.Status = UpdateLaunchDispatched
	}
	if launchErr != nil {
		return result, fmt.Errorf("dispatch self-update helper: %w", launchErr)
	}
	if !started {
		return result, errors.New("self-update helper was not reported as started")
	}
	return result, nil
}

func createCurrentUpdateStagingDirectory(parent string) (string, error) {
	directory, err := os.MkdirTemp(parent, ".scripthold-update-stage-*")
	if err != nil {
		return "", fmt.Errorf("create self-update staging directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(directory)
		}
	}()
	if err := filesystem.RestrictOwnerOnlyPath(directory, true); err != nil {
		return "", fmt.Errorf("restrict self-update staging directory: %w", err)
	}
	if err := filesystem.ValidateOwnerOnlyPath(directory, true); err != nil {
		return "", fmt.Errorf("validate self-update staging directory: %w", err)
	}
	cleanup = false
	return directory, nil
}

func removeCurrentUpdateStagingDirectory(directory string) error {
	if directory == "" {
		return nil
	}
	if _, err := os.Lstat(directory); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect self-update staging directory: %w", err)
	}
	if err := filesystem.ValidateOwnerOnlyPath(directory, true); err != nil {
		return fmt.Errorf("validate self-update staging directory before cleanup: %w", err)
	}
	if err := os.Remove(directory); err != nil {
		return fmt.Errorf("remove self-update staging directory: %w", err)
	}
	return nil
}
