package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zoster81/scripthold/internal/filesystem"
)

type ReconciliationStatus string

const (
	ReconciliationStable           ReconciliationStatus = "stable"
	ReconciliationPrepared         ReconciliationStatus = "prepared"
	ReconciliationCommitted        ReconciliationStatus = "committed"
	ReconciliationRolledBack       ReconciliationStatus = "rolled_back"
	ReconciliationRecoveryRequired ReconciliationStatus = "recovery_required"
)

type ReconciliationResult struct {
	Status  ReconciliationStatus
	Problem string

	rollbackPrepared        bool
	candidateBytesInstalled bool
}

type reconciliationDeps struct {
	observeTarget func(context.Context, *StandaloneInspection, string, string) (installationCurrentState, error)
}

type artifactObservation int

const (
	artifactMissing artifactObservation = iota
	artifactValid
	artifactInvalid
)

type candidateArtifactObservation int

const (
	candidateArtifactMissing candidateArtifactObservation = iota
	candidateArtifactUpdate
	candidateArtifactRollbackSource
	candidateArtifactInvalid
)

func ReconcileInstallation(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
) (ReconciliationResult, error) {
	return reconcileInstallationWith(ctx, boundary, inspection, runtime.GOOS, runtime.GOARCH, reconciliationDeps{})
}

func reconcileInstallationWith(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	goos, goarch string,
	deps reconciliationDeps,
) (_ ReconciliationResult, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateInstallationBoundaryBinding(boundary, inspection); err != nil {
		return ReconciliationResult{}, err
	}
	control, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		return ReconciliationResult{}, fmt.Errorf("acquire installation control lock: %w", err)
	}
	defer func() {
		if closeErr := control.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return ReconciliationResult{}, fmt.Errorf("validate installation control lock: %w", err)
	}
	return reconcileInstallationLocked(ctx, boundary, inspection, goos, goarch, deps, control)
}

func reconcileInstallationLocked(
	ctx context.Context,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	goos, goarch string,
	deps reconciliationDeps,
	control *filesystem.OwnerOnlyFileLock,
) (ReconciliationResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if control == nil {
		return ReconciliationResult{}, errors.New("installation control lock is required")
	}
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return ReconciliationResult{}, fmt.Errorf("validate installation control lock: %w", err)
	}
	if err := validateInstallationBoundaryBinding(boundary, inspection); err != nil {
		return ReconciliationResult{}, err
	}
	if deps.observeTarget == nil {
		deps.observeTarget = func(ctx context.Context, inspection *StandaloneInspection, goos, goarch string) (installationCurrentState, error) {
			return observeInstalledBinary(ctx, inspection, goos, goarch, installedEvidenceDeps{})
		}
	}

	state, stateErr := readInstallationStateForReconciliationLocked(boundary, inspection)
	if stateErr != nil {
		return recoveryRequired("installation state cannot be safely validated"), nil
	}
	if state.Pending == nil {
		if err := validateInstallationState(state, inspection); err != nil {
			return recoveryRequired("stable target identity does not match installation state"), nil
		}
		observed, observeErr := deps.observeTarget(ctx, inspection, goos, goarch)
		if observeErr != nil {
			if ctx.Err() != nil {
				return ReconciliationResult{}, ctx.Err()
			}
			return recoveryRequired("stable target cannot be fully verified"), nil
		}
		if observed != state.Current {
			return recoveryRequired("stable target does not match current state"), nil
		}
		return finishReconciliation(control, boundary, inspection, ReconciliationResult{Status: ReconciliationStable})
	}

	knownGood := observeFixedArtifact(boundary, knownGoodArtifactName, state.Pending.SourceSHA256)
	helper := observeFixedArtifact(boundary, helperArtifactName, state.Pending.SourceSHA256)
	candidate := observeCandidateSlot(
		boundary,
		candidateArtifactName,
		state.Pending.CandidateSHA256,
		state.Pending.SourceSHA256,
	)
	if knownGood != artifactValid {
		return recoveryRequired("known-good artifact is missing or invalid"), nil
	}
	if helper != artifactValid {
		return recoveryRequired("helper artifact is missing or invalid"), nil
	}
	if candidate == candidateArtifactInvalid {
		return recoveryRequired("candidate artifact is invalid"), nil
	}

	observed, observeErr := deps.observeTarget(ctx, inspection, goos, goarch)
	if observeErr != nil {
		if ctx.Err() != nil {
			return ReconciliationResult{}, ctx.Err()
		}
		targetDigest, digestErr := observeTargetDigest(inspection)
		if digestErr != nil {
			return recoveryRequired("installed target bytes cannot be safely observed"), nil
		}
		candidateBytesInstalled := targetDigest == state.Pending.CandidateSHA256 &&
			(candidate == candidateArtifactMissing || candidate == candidateArtifactRollbackSource)
		if candidateBytesInstalled {
			return finishReconciliation(control, boundary, inspection, ReconciliationResult{
				Status:                  ReconciliationRecoveryRequired,
				Problem:                 "installed candidate bytes failed full verification",
				rollbackPrepared:        candidate == candidateArtifactRollbackSource,
				candidateBytesInstalled: true,
			})
		}
		return recoveryRequired("installed target cannot be fully verified"), nil
	}
	candidateBytesInstalled := observed.SHA256 == state.Pending.CandidateSHA256 &&
		(candidate == candidateArtifactMissing || candidate == candidateArtifactRollbackSource)
	candidateState := installationCurrentState{
		Version: state.Pending.CandidateVersion,
		SHA256:  state.Pending.CandidateSHA256,
		Build:   state.Pending.CandidateBuild,
	}

	var result ReconciliationResult
	switch {
	case observed == state.Current && candidate == candidateArtifactUpdate:
		result = ReconciliationResult{Status: ReconciliationPrepared}
	case observed == candidateState && candidate == candidateArtifactMissing:
		result = ReconciliationResult{Status: ReconciliationCommitted}
	case observed == candidateState && candidate == candidateArtifactRollbackSource:
		result = ReconciliationResult{Status: ReconciliationCommitted, rollbackPrepared: true}
	case observed == state.Current && candidate == candidateArtifactMissing:
		result = ReconciliationResult{Status: ReconciliationRolledBack}
	default:
		if candidateBytesInstalled {
			return finishReconciliation(control, boundary, inspection, ReconciliationResult{
				Status:                  ReconciliationRecoveryRequired,
				Problem:                 "installed candidate bytes do not match candidate verification evidence",
				rollbackPrepared:        candidate == candidateArtifactRollbackSource,
				candidateBytesInstalled: true,
			})
		}
		return recoveryRequired("target and candidate artifact combination is ambiguous or unsupported"), nil
	}
	return finishReconciliation(control, boundary, inspection, result)
}

func readInstallationStateForReconciliationLocked(
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
) (installationState, error) {
	if err := validateInstallationBoundaryBinding(boundary, inspection); err != nil {
		return installationState{}, err
	}
	statePath := filepath.Join(boundary.Directory, installationStateFileName)
	payload, err := filesystem.ReadOwnerOnlyFileBounded(statePath, maxInstallationStateBytes)
	if err != nil {
		return installationState{}, fmt.Errorf("read installation state: %w", err)
	}
	state, err := decodeInstallationState(payload)
	if err != nil {
		return installationState{}, err
	}
	if err := validateInstallationStateBinding(state, inspection, false); err != nil {
		return installationState{}, err
	}
	return state, nil
}

func observeTargetDigest(inspection *StandaloneInspection) (string, error) {
	if inspection == nil {
		return "", errors.New("standalone inspection evidence is required")
	}
	file, err := filesystem.OpenVerifiedSingleLinkFile(inspection.ExecutablePath, inspection.ExecutableIdentity)
	if err != nil {
		return "", err
	}
	firstSize, firstDigest, firstErr := hashInstalledFile(file)
	secondSize, secondDigest, secondErr := hashInstalledFile(file)
	closeErr := file.Close()
	if firstErr != nil || secondErr != nil || closeErr != nil {
		return "", errors.Join(firstErr, secondErr, closeErr)
	}
	if firstSize != secondSize || firstDigest != secondDigest {
		return "", errors.New("installed target bytes changed while observing digest")
	}
	if err := revalidateInspectedExecutable(inspection); err != nil {
		return "", err
	}
	return firstDigest, nil
}

func observeCandidateArtifact(boundary *InstallationBoundary, expectedSHA256 string) artifactObservation {
	return observeArtifact(boundary, candidateArtifactName, expectedSHA256, false)
}

func observeCandidateSlot(
	boundary *InstallationBoundary,
	name, updateSHA256, sourceSHA256 string,
) candidateArtifactObservation {
	status, digest := observeArtifactDigest(boundary, name, false)
	switch status {
	case artifactMissing:
		return candidateArtifactMissing
	case artifactInvalid:
		return candidateArtifactInvalid
	}
	switch digest {
	case updateSHA256:
		return candidateArtifactUpdate
	case sourceSHA256:
		return candidateArtifactRollbackSource
	default:
		return candidateArtifactInvalid
	}
}

func observeFixedArtifact(boundary *InstallationBoundary, name, expectedSHA256 string) artifactObservation {
	return observeArtifact(boundary, name, expectedSHA256, true)
}

func observeArtifact(boundary *InstallationBoundary, name, expectedSHA256 string, requireOwnerOnly bool) artifactObservation {
	status, digest := observeArtifactDigest(boundary, name, requireOwnerOnly)
	if status != artifactValid || digest != expectedSHA256 {
		if status == artifactMissing {
			return artifactMissing
		}
		return artifactInvalid
	}
	return artifactValid
}

func observeArtifactDigest(
	boundary *InstallationBoundary,
	name string,
	requireOwnerOnly bool,
) (artifactObservation, string) {
	path := filepath.Join(boundary.Directory, name)
	if _, err := os.Lstat(path); err != nil {
		if os.IsNotExist(err) {
			return artifactMissing, ""
		}
		return artifactInvalid, ""
	}
	if requireOwnerOnly {
		if err := filesystem.ValidateOwnerOnlyExecutable(path); err != nil {
			return artifactInvalid, ""
		}
	}
	identity, err := filesystem.CaptureSingleLinkFileIdentity(path)
	if err != nil {
		return artifactInvalid, ""
	}
	file, err := filesystem.OpenVerifiedSingleLinkFile(path, identity)
	if err != nil {
		return artifactInvalid, ""
	}
	firstSize, firstDigest, firstErr := hashInstalledFile(file)
	secondSize, secondDigest, secondErr := hashInstalledFile(file)
	closeErr := file.Close()
	if firstErr != nil || secondErr != nil || closeErr != nil ||
		firstSize != secondSize || firstDigest != secondDigest {
		return artifactInvalid, ""
	}
	matches, err := identity.Matches(path)
	if err != nil || !matches {
		return artifactInvalid, ""
	}
	directoryMatches, err := boundary.directoryIdentity.Matches(boundary.Directory)
	if err != nil || !directoryMatches {
		return artifactInvalid, ""
	}
	return artifactValid, firstDigest
}

func finishReconciliation(
	control *filesystem.OwnerOnlyFileLock,
	boundary *InstallationBoundary,
	inspection *StandaloneInspection,
	result ReconciliationResult,
) (ReconciliationResult, error) {
	if err := control.Validate(boundary.ControlLockPath); err != nil {
		return ReconciliationResult{}, fmt.Errorf("revalidate installation control lock: %w", err)
	}
	if err := validateInstallationBoundaryBinding(boundary, inspection); err != nil {
		return ReconciliationResult{}, err
	}
	return result, nil
}

func recoveryRequired(problem string) ReconciliationResult {
	return ReconciliationResult{Status: ReconciliationRecoveryRequired, Problem: problem}
}
