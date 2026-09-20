package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/zoster81/scripthold/internal/security"
)

// AdmitCurrentProcessIfAdopted acquires normal-process shared use authority
// only when self-update state already exists beside the running executable.
// Missing state is treated as an unadopted installation and is never created.
func AdmitCurrentProcessIfAdopted(ctx context.Context) (*ProcessAdmission, error) {
	executablePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve running executable for self-update admission: %w", err)
	}
	return admitProcessIfStatePresent(
		ctx,
		executablePath,
		runtime.GOOS,
		runtime.GOARCH,
		installedEvidenceDeps{},
	)
}

func admitProcessIfStatePresent(
	ctx context.Context,
	executablePath, goos, goarch string,
	deps installedEvidenceDeps,
) (*ProcessAdmission, error) {
	present, err := installationStateRootPresent(executablePath)
	if err != nil {
		return nil, err
	}
	if !present {
		return nil, nil
	}

	inspection, err := inspectStandaloneExecutable(executablePath, goos)
	if err != nil {
		return nil, err
	}
	boundary, err := openInstallationBoundary(inspection, false)
	if err != nil {
		return nil, err
	}
	return admitStableProcessWith(ctx, boundary, inspection, goos, goarch, deps)
}

func installationStateRootPresent(executablePath string) (bool, error) {
	if executablePath == "" {
		return false, errors.New("running executable path is empty")
	}
	absolute, err := filepath.Abs(executablePath)
	if err != nil {
		return false, fmt.Errorf("make running executable path absolute: %w", err)
	}
	absolute = filepath.Clean(absolute)

	candidates := []string{filepath.Dir(absolute)}
	if resolved, resolveErr := filepath.EvalSymlinks(absolute); resolveErr == nil {
		resolvedParent := filepath.Dir(filepath.Clean(resolved))
		if !security.PathsEqual(candidates[0], resolvedParent) {
			candidates = append(candidates, resolvedParent)
		}
	}

	for _, parent := range candidates {
		stateRoot := filepath.Join(parent, installationStateDirectoryName)
		_, err := os.Lstat(stateRoot)
		switch {
		case err == nil:
			return true, nil
		case errors.Is(err, os.ErrNotExist):
			continue
		default:
			return false, fmt.Errorf("inspect self-update state root: %w", err)
		}
	}
	return false, nil
}
