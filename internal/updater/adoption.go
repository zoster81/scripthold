package updater

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/zoster81/scripthold/internal/filesystem"
)

type installedEvidenceDeps struct {
	readBuildInfo func(io.ReaderAt) (*debug.BuildInfo, error)
	smokeVersion  func(context.Context, string) (string, error)
}

// InitializeStableAdoption verifies the currently running standalone binary and
// creates its initial stable self-update state without downloading or switching anything.
func InitializeStableAdoption(ctx context.Context) error {
	path, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve running executable: %w", err)
	}
	_, _, _, err = initializeStableAdoptionWith(ctx, path, runtime.GOOS, runtime.GOARCH, installedEvidenceDeps{})
	return err
}

func observeInstalledBinary(
	ctx context.Context,
	inspection *StandaloneInspection,
	goos, goarch string,
	deps installedEvidenceDeps,
) (installationCurrentState, error) {
	if inspection == nil {
		return installationCurrentState{}, errors.New("standalone inspection evidence is required")
	}
	if deps.readBuildInfo == nil {
		deps.readBuildInfo = buildinfo.Read
	}
	if deps.smokeVersion == nil {
		deps.smokeVersion = readVersionSmoke
	}

	file, err := filesystem.OpenVerifiedSingleLinkFile(inspection.ExecutablePath, inspection.ExecutableIdentity)
	if err != nil {
		return installationCurrentState{}, fmt.Errorf("open inspected executable: %w", err)
	}
	defer file.Close()

	size, digest, err := hashInstalledFile(file)
	if err != nil {
		return installationCurrentState{}, err
	}
	info, err := deps.readBuildInfo(file)
	if err != nil {
		return installationCurrentState{}, fmt.Errorf("read installed Go build information: %w", err)
	}
	build, err := installationBuildIdentityFromInfo(info, goos, goarch)
	if err != nil {
		return installationCurrentState{}, err
	}
	if err := revalidateInspectedExecutable(inspection); err != nil {
		return installationCurrentState{}, err
	}

	version, err := deps.smokeVersion(ctx, inspection.ExecutablePath)
	if err != nil {
		return installationCurrentState{}, fmt.Errorf("installed version smoke failed: %w", err)
	}
	if strings.HasPrefix(version, "v") {
		return installationCurrentState{}, errors.New("installed version must use canonical semantic version form without a v prefix")
	}
	parsed, ok := parseSemanticVersion(version)
	if !ok || len(parsed.prerelease) != 0 {
		return installationCurrentState{}, fmt.Errorf("installed version %q is not stable semantic version", version)
	}

	secondSize, secondDigest, err := hashInstalledFile(file)
	if err != nil {
		return installationCurrentState{}, err
	}
	if secondSize != size || secondDigest != digest {
		return installationCurrentState{}, errors.New("installed executable bytes changed during adoption inspection")
	}
	if err := revalidateInspectedExecutable(inspection); err != nil {
		return installationCurrentState{}, err
	}

	return installationCurrentState{
		Version: version,
		SHA256:  digest,
		Build:   build,
	}, nil
}

func hashInstalledFile(file *os.File) (int64, string, error) {
	if file == nil {
		return 0, "", errors.New("installed executable handle is unavailable")
	}
	info, err := file.Stat()
	if err != nil {
		return 0, "", err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > maxSelfUpdateAssetBytes {
		return 0, "", errors.New("installed executable size is outside the allowed range")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", err
	}
	hasher := sha256.New()
	written, err := io.CopyN(hasher, file, info.Size())
	if err != nil {
		return 0, "", err
	}
	if written != info.Size() {
		return 0, "", errors.New("installed executable could not be hashed completely")
	}
	after, err := file.Stat()
	if err != nil {
		return 0, "", err
	}
	if after.Size() != info.Size() {
		return 0, "", errors.New("installed executable size changed while hashing")
	}
	return written, hex.EncodeToString(hasher.Sum(nil)), nil
}

func installationBuildIdentityFromInfo(info *debug.BuildInfo, goos, goarch string) (installationBuildIdentity, error) {
	if info == nil {
		return installationBuildIdentity{}, errors.New("installed Go build information is unavailable")
	}
	if info.Main.Path != officialModulePath || info.Main.Replace != nil {
		return installationBuildIdentity{}, fmt.Errorf("installed main module is not %s", officialModulePath)
	}
	actualOS, err := uniqueBuildSetting(info, "GOOS", true)
	if err != nil {
		return installationBuildIdentity{}, err
	}
	actualArch, err := uniqueBuildSetting(info, "GOARCH", true)
	if err != nil {
		return installationBuildIdentity{}, err
	}
	if actualOS != goos || actualArch != goarch {
		return installationBuildIdentity{}, fmt.Errorf("installed build platform is %s/%s, expected %s/%s", actualOS, actualArch, goos, goarch)
	}
	vcs, err := uniqueBuildSetting(info, "vcs", true)
	if err != nil {
		return installationBuildIdentity{}, err
	}
	if vcs != "git" {
		return installationBuildIdentity{}, fmt.Errorf("installed VCS is %q, expected git", vcs)
	}
	revision, err := uniqueBuildSetting(info, "vcs.revision", true)
	if err != nil {
		return installationBuildIdentity{}, err
	}
	if err := validateGitObjectSHA(revision); err != nil {
		return installationBuildIdentity{}, fmt.Errorf("installed VCS revision is invalid: %w", err)
	}
	modified, err := uniqueBuildSetting(info, "vcs.modified", true)
	if err != nil {
		return installationBuildIdentity{}, err
	}
	if modified != "false" {
		return installationBuildIdentity{}, errors.New("installed build does not provide affirmative clean VCS evidence")
	}
	return installationBuildIdentity{
		Module:   officialModulePath,
		GOOS:     actualOS,
		GOARCH:   actualArch,
		VCS:      vcs,
		Revision: revision,
		VCSClean: true,
	}, nil
}

func revalidateInspectedExecutable(inspection *StandaloneInspection) error {
	current, err := filesystem.CaptureSingleLinkFileIdentity(inspection.ExecutablePath)
	if err != nil {
		return fmt.Errorf("revalidate inspected executable identity: %w", err)
	}
	if !inspection.ExecutableIdentity.Equal(current) {
		return errors.New("inspected executable identity changed")
	}
	parentMatches, err := inspection.ParentIdentity.Matches(inspection.ParentPath)
	if err != nil {
		return err
	}
	if !parentMatches {
		return errors.New("inspected executable parent identity changed")
	}
	return nil
}

func initializeStableAdoptionWith(
	ctx context.Context,
	executablePath, goos, goarch string,
	deps installedEvidenceDeps,
) (*InstallationBoundary, *StandaloneInspection, installationState, error) {
	inspection, err := inspectStandaloneExecutable(executablePath, goos)
	if err != nil {
		return nil, nil, installationState{}, err
	}
	current, err := observeInstalledBinary(ctx, inspection, goos, goarch, deps)
	if err != nil {
		return nil, nil, installationState{}, err
	}
	state := installationState{
		FormatVersion: installationStateFormatVersion,
		Target: installationTargetState{
			Path:           inspection.ExecutablePath,
			Identity:       inspection.ExecutableIdentity.StableKey(),
			Volume:         inspection.ExecutableIdentity.VolumeKey(),
			ParentIdentity: inspection.ParentIdentity.StableKey(),
			ParentVolume:   inspection.ParentIdentity.VolumeKey(),
		},
		Current: current,
	}
	boundary, err := openInstallationBoundary(inspection, true)
	if err != nil {
		return nil, nil, installationState{}, err
	}
	if err := persistInitialInstallationState(boundary, inspection, state); err != nil {
		return nil, nil, installationState{}, err
	}
	return boundary, inspection, state, nil
}
