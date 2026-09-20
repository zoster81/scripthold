package updater

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/security"
)

// StandaloneInspection is read-only evidence that one executable currently has
// a topology suitable for explicit self-update adoption.
type StandaloneInspection struct {
	ExecutablePath     string
	ParentPath         string
	ExecutableIdentity filesystem.ObjectIdentity
	ParentIdentity     filesystem.ObjectIdentity
}

// InspectCurrentStandaloneExecutable inspects the currently running executable
// without creating self-update state or changing the installation.
func InspectCurrentStandaloneExecutable() (*StandaloneInspection, error) {
	path, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve running executable: %w", err)
	}
	return inspectStandaloneExecutable(path, runtime.GOOS)
}

func inspectStandaloneExecutable(path, goos string) (*StandaloneInspection, error) {
	if path == "" || strings.IndexByte(path, 0) >= 0 {
		return nil, errors.New("executable path is empty or invalid")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("make executable path absolute: %w", err)
	}
	absolute = filepath.Clean(absolute)

	initialInfo, err := os.Lstat(absolute)
	if err != nil {
		return nil, fmt.Errorf("inspect executable path: %w", err)
	}
	if initialInfo.Mode()&os.ModeSymlink != 0 || !initialInfo.Mode().IsRegular() {
		return nil, errors.New("self-update adoption requires a direct regular executable file")
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve executable path: %w", err)
	}
	resolved = filepath.Clean(resolved)
	if !security.PathsEqual(absolute, resolved) {
		return nil, errors.New("self-update adoption rejects symlink, junction, or aliased executable paths")
	}

	parent := filepath.Dir(resolved)
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return nil, fmt.Errorf("inspect executable parent: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return nil, errors.New("self-update adoption requires a direct real parent directory")
	}
	if err := rejectKnownMCPBLayout(resolved, goos); err != nil {
		return nil, err
	}

	targetIdentity, err := filesystem.CaptureSingleLinkFileIdentity(resolved)
	if err != nil {
		return nil, fmt.Errorf("capture standalone executable identity: %w", err)
	}
	parentIdentity, err := filesystem.CaptureObjectIdentity(parent)
	if err != nil {
		return nil, fmt.Errorf("capture executable parent identity: %w", err)
	}
	if !parentIdentity.IsDirectory() {
		return nil, errors.New("executable parent identity is not a directory")
	}
	sameVolume, err := targetIdentity.SameVolume(parentIdentity)
	if err != nil {
		return nil, fmt.Errorf("compare executable and parent volume: %w", err)
	}
	if !sameVolume {
		return nil, errors.New("executable and parent directory do not share a stable filesystem volume")
	}

	// Revalidate after every topology check so returned evidence describes the
	// same single-link target and parent observed at the start of inspection.
	reResolved, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		return nil, fmt.Errorf("re-resolve executable path: %w", err)
	}
	if !security.PathsEqual(resolved, reResolved) {
		return nil, errors.New("executable path topology changed during inspection")
	}
	revalidatedTarget, err := filesystem.CaptureSingleLinkFileIdentity(resolved)
	if err != nil {
		return nil, fmt.Errorf("revalidate standalone executable identity: %w", err)
	}
	if !targetIdentity.Equal(revalidatedTarget) {
		return nil, errors.New("executable identity changed during standalone inspection")
	}
	targetMatches, err := targetIdentity.Matches(resolved)
	if err != nil {
		return nil, fmt.Errorf("revalidate executable path identity: %w", err)
	}
	if !targetMatches {
		return nil, errors.New("executable path changed during standalone inspection")
	}
	parentMatches, err := parentIdentity.Matches(parent)
	if err != nil {
		return nil, fmt.Errorf("revalidate executable parent identity: %w", err)
	}
	if !parentMatches {
		return nil, errors.New("executable parent changed during standalone inspection")
	}
	if err := rejectKnownMCPBLayout(resolved, goos); err != nil {
		return nil, err
	}

	return &StandaloneInspection{
		ExecutablePath:     resolved,
		ParentPath:         parent,
		ExecutableIdentity: targetIdentity,
		ParentIdentity:     parentIdentity,
	}, nil
}

func rejectKnownMCPBLayout(executablePath, goos string) error {
	if !platformPathNameEqual(filepath.Base(executablePath), standaloneBinaryName(goos), goos) {
		return nil
	}
	serverDirectory := filepath.Dir(executablePath)
	if !platformPathNameEqual(filepath.Base(serverDirectory), "server", goos) {
		return nil
	}
	manifestPath := filepath.Join(filepath.Dir(serverDirectory), "manifest.json")
	_, err := os.Lstat(manifestPath)
	switch {
	case err == nil:
		return errors.New("self-update adoption is unavailable for MCPB-managed installations")
	case errors.Is(err, os.ErrNotExist):
		return nil
	default:
		return fmt.Errorf("inspect possible MCPB manifest: %w", err)
	}
}

func standaloneBinaryName(goos string) string {
	if goos == "windows" {
		return "scripthold.exe"
	}
	return "scripthold"
}

func platformPathNameEqual(left, right, goos string) bool {
	if goos == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
