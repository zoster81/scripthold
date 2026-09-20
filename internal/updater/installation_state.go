package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/security"
)

const (
	installationStateFormatVersion = 1
	installationStateFileName      = "state.json"
	maxInstallationStateBytes      = 64 << 10
	maxIdentityFieldBytes          = 1024
)

type installationState struct {
	FormatVersion int                      `json:"formatVersion"`
	Target        installationTargetState  `json:"target"`
	Current       installationCurrentState `json:"current"`
}

type installationTargetState struct {
	Path           string `json:"path"`
	Identity       string `json:"identity"`
	Volume         string `json:"volume"`
	ParentIdentity string `json:"parentIdentity"`
	ParentVolume   string `json:"parentVolume"`
}

type installationCurrentState struct {
	Version string                    `json:"version"`
	SHA256  string                    `json:"sha256"`
	Build   installationBuildIdentity `json:"build"`
}

type installationBuildIdentity struct {
	Module   string `json:"module"`
	GOOS     string `json:"goos"`
	GOARCH   string `json:"goarch"`
	VCS      string `json:"vcs"`
	Revision string `json:"revision"`
	VCSClean bool   `json:"vcsClean"`
}

func encodeInstallationState(state installationState) ([]byte, error) {
	if err := validateInstallationStateSyntax(state); err != nil {
		return nil, err
	}
	payload, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	if len(payload) > maxInstallationStateBytes {
		return nil, errors.New("installation state exceeds its size limit")
	}
	return payload, nil
}

func decodeInstallationState(payload []byte) (installationState, error) {
	if len(payload) == 0 || len(payload) > maxInstallationStateBytes {
		return installationState{}, errors.New("installation state size is outside the allowed range")
	}
	var state installationState
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return installationState{}, fmt.Errorf("decode installation state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return installationState{}, errors.New("installation state contains multiple JSON values")
		}
		return installationState{}, fmt.Errorf("decode installation state trailer: %w", err)
	}
	if err := validateInstallationStateSyntax(state); err != nil {
		return installationState{}, err
	}
	return state, nil
}

func validateInstallationStateSyntax(state installationState) error {
	if state.FormatVersion != installationStateFormatVersion {
		return fmt.Errorf("unsupported installation state format version %d", state.FormatVersion)
	}
	if state.Target.Path == "" || strings.IndexByte(state.Target.Path, 0) >= 0 ||
		!filepath.IsAbs(state.Target.Path) || filepath.Clean(state.Target.Path) != state.Target.Path {
		return errors.New("installation target path is not canonical absolute form")
	}
	for label, value := range map[string]string{
		"target identity": state.Target.Identity,
		"target volume":   state.Target.Volume,
		"parent identity": state.Target.ParentIdentity,
		"parent volume":   state.Target.ParentVolume,
	} {
		if value == "" || len(value) > maxIdentityFieldBytes {
			return fmt.Errorf("%s is missing or oversized", label)
		}
	}
	if strings.HasPrefix(state.Current.Version, "v") {
		return errors.New("current version must use canonical semantic version form without a v prefix")
	}
	version, ok := parseSemanticVersion(state.Current.Version)
	if !ok || len(version.prerelease) != 0 {
		return fmt.Errorf("current version %q is not stable semantic version", state.Current.Version)
	}
	if _, err := parseGitHubSHA256("sha256:" + state.Current.SHA256); err != nil {
		return fmt.Errorf("current SHA-256 is invalid: %w", err)
	}
	build := state.Current.Build
	if build.Module != officialModulePath {
		return fmt.Errorf("current build module is not %s", officialModulePath)
	}
	if _, err := expectedBinaryAssetName(build.GOOS, build.GOARCH); err != nil {
		return err
	}
	if build.VCS != "git" {
		return fmt.Errorf("current build VCS is %q, expected git", build.VCS)
	}
	if err := validateGitObjectSHA(build.Revision); err != nil {
		return fmt.Errorf("current build revision is invalid: %w", err)
	}
	if !build.VCSClean {
		return errors.New("current build does not provide affirmative clean VCS evidence")
	}
	return nil
}

func validateInstallationState(state installationState, inspection *StandaloneInspection) error {
	if err := validateInstallationStateSyntax(state); err != nil {
		return err
	}
	if inspection == nil {
		return errors.New("standalone inspection evidence is required")
	}
	if !security.PathsEqual(state.Target.Path, inspection.ExecutablePath) {
		return errors.New("installation target path does not match inspected executable")
	}
	if state.Target.Identity != inspection.ExecutableIdentity.StableKey() ||
		state.Target.Volume != inspection.ExecutableIdentity.VolumeKey() ||
		state.Target.ParentIdentity != inspection.ParentIdentity.StableKey() ||
		state.Target.ParentVolume != inspection.ParentIdentity.VolumeKey() {
		return errors.New("installation target identity evidence does not match inspected executable")
	}
	if state.Current.Build.GOOS != runtime.GOOS || state.Current.Build.GOARCH != runtime.GOARCH {
		return errors.New("installation build platform does not match the running platform")
	}
	return nil
}

func persistInitialInstallationState(boundary *InstallationBoundary, inspection *StandaloneInspection, state installationState) error {
	return persistInstallationState(boundary, inspection, state, true)
}

func persistStableInstallationState(boundary *InstallationBoundary, inspection *StandaloneInspection, state installationState) error {
	return persistInstallationState(boundary, inspection, state, false)
}

func persistInstallationState(boundary *InstallationBoundary, inspection *StandaloneInspection, state installationState, requireMissing bool) error {
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return err
	}
	lock, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		return fmt.Errorf("acquire installation control lock: %w", err)
	}
	defer lock.Close()
	if err := lock.Validate(boundary.ControlLockPath); err != nil {
		return fmt.Errorf("validate installation control lock: %w", err)
	}
	if err := validateInstallationState(state, inspection); err != nil {
		return err
	}
	payload, err := encodeInstallationState(state)
	if err != nil {
		return err
	}

	statePath := filepath.Join(boundary.Directory, installationStateFileName)
	snapshot, err := filesystem.CaptureSnapshot(statePath)
	if err != nil {
		return err
	}
	if snapshot.Exists {
		if requireMissing {
			return errors.New("installation state is already initialized")
		}
		existingPayload, readErr := filesystem.ReadOwnerOnlyFileBounded(statePath, maxInstallationStateBytes)
		if readErr != nil {
			return fmt.Errorf("read existing installation state: %w", readErr)
		}
		existingState, decodeErr := decodeInstallationState(existingPayload)
		if decodeErr != nil {
			return fmt.Errorf("decode existing installation state: %w", decodeErr)
		}
		if validateErr := validateInstallationState(existingState, inspection); validateErr != nil {
			return fmt.Errorf("validate existing installation state: %w", validateErr)
		}
		snapshot, err = filesystem.CaptureSnapshotWithDigest(statePath)
		if err != nil {
			return err
		}
	}
	staged, err := filesystem.StageReplacement(statePath, bytes.NewReader(payload), 0o600, nil)
	if err != nil {
		return err
	}
	defer staged.Cleanup()
	if err := staged.RestrictOwnerOnly(); err != nil {
		return fmt.Errorf("restrict staged installation state: %w", err)
	}
	if _, err := staged.Commit(filesystem.ReplaceOptions{Mode: 0o600, Expected: &snapshot}); err != nil {
		return fmt.Errorf("commit installation state: %w", err)
	}
	if err := filesystem.ValidateOwnerOnlyPath(statePath, false); err != nil {
		return fmt.Errorf("validate committed installation state: %w", err)
	}
	if err := lock.Validate(boundary.ControlLockPath); err != nil {
		return fmt.Errorf("revalidate installation control lock: %w", err)
	}
	return validateInstallationBoundary(boundary, inspection)
}

func readInstallationState(boundary *InstallationBoundary, inspection *StandaloneInspection) (installationState, error) {
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return installationState{}, err
	}
	lock, err := filesystem.TryAcquireOwnerOnlyFileLock(boundary.ControlLockPath, filesystem.LockExclusive, false)
	if err != nil {
		return installationState{}, fmt.Errorf("acquire installation control lock: %w", err)
	}
	defer lock.Close()
	if err := lock.Validate(boundary.ControlLockPath); err != nil {
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
	if err := validateInstallationState(state, inspection); err != nil {
		return installationState{}, err
	}
	if err := lock.Validate(boundary.ControlLockPath); err != nil {
		return installationState{}, err
	}
	if err := validateInstallationBoundary(boundary, inspection); err != nil {
		return installationState{}, err
	}
	return state, nil
}

func validateInstallationBoundary(boundary *InstallationBoundary, inspection *StandaloneInspection) error {
	if boundary == nil || boundary.Directory == "" || boundary.ControlLockPath == "" || boundary.UseLockPath == "" {
		return errors.New("installation boundary is unavailable")
	}
	if inspection == nil {
		return errors.New("standalone inspection evidence is required")
	}
	expectedDirectory := filepath.Join(inspection.ParentPath, installationStateDirectoryName)
	if !security.PathsEqual(boundary.Directory, expectedDirectory) {
		return errors.New("installation state directory is not the canonical sibling of the executable")
	}
	if !security.PathsEqual(boundary.ControlLockPath, filepath.Join(boundary.Directory, controlLockName)) ||
		!security.PathsEqual(boundary.UseLockPath, filepath.Join(boundary.Directory, useLockName)) {
		return errors.New("installation lock paths do not match the canonical state layout")
	}
	if !boundary.directoryIdentity.IsDirectory() {
		return errors.New("installation state directory identity is unavailable")
	}
	matches, err := boundary.directoryIdentity.Matches(boundary.Directory)
	if err != nil {
		return err
	}
	if !matches {
		return errors.New("installation state directory identity changed")
	}
	parentMatches, err := inspection.ParentIdentity.Matches(inspection.ParentPath)
	if err != nil || !parentMatches {
		if err != nil {
			return err
		}
		return errors.New("executable parent identity changed")
	}
	targetMatches, err := inspection.ExecutableIdentity.Matches(inspection.ExecutablePath)
	if err != nil || !targetMatches {
		if err != nil {
			return err
		}
		return errors.New("executable identity changed")
	}
	if err := filesystem.ValidateOwnerOnlyPath(boundary.Directory, true); err != nil {
		return err
	}
	return nil
}
