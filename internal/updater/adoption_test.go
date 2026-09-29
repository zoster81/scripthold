package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
)

func adoptionBuildInfoForTest() *debug.BuildInfo {
	return &debug.BuildInfo{
		Main: debug.Module{Path: officialModulePath},
		Settings: []debug.BuildSetting{
			{Key: "GOOS", Value: runtime.GOOS},
			{Key: "GOARCH", Value: runtime.GOARCH},
			{Key: "vcs", Value: "git"},
			{Key: "vcs.revision", Value: strings.Repeat("b", 40)},
			{Key: "vcs.modified", Value: "false"},
		},
	}
}

func TestObserveInstalledBinaryEvidence(t *testing.T) {
	path := filepath.Join(canonicalTempDir(t), standaloneBinaryName(runtime.GOOS))
	payload := []byte("installed-binary")
	if err := os.WriteFile(path, payload, 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(path, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	current, err := observeInstalledBinary(context.Background(), inspection, runtime.GOOS, runtime.GOARCH, installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	if current.Version != "3.2.1" || current.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected current evidence: %#v", current)
	}
	if current.Build.Module != officialModulePath || current.Build.VCS != "git" ||
		current.Build.Revision != strings.Repeat("b", 40) || !current.Build.VCSClean {
		t.Fatalf("unexpected build evidence: %#v", current.Build)
	}
}

func TestObserveInstalledBinaryStaticValidationPrecedesSmoke(t *testing.T) {
	path := filepath.Join(canonicalTempDir(t), standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(path, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	var smokeCalls atomic.Int32
	info := adoptionBuildInfoForTest()
	for i := range info.Settings {
		if info.Settings[i].Key == "vcs.modified" {
			info.Settings[i].Value = "true"
		}
	}
	_, err = observeInstalledBinary(context.Background(), inspection, runtime.GOOS, runtime.GOARCH, installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return info, nil },
		smokeVersion: func(context.Context, string) (string, error) {
			smokeCalls.Add(1)
			return "3.2.1", nil
		},
	})
	if err == nil {
		t.Fatal("dirty build must fail closed")
	}
	if smokeCalls.Load() != 0 {
		t.Fatal("smoke ran before static validation succeeded")
	}
}

func TestObserveInstalledBinaryRejectsMutationDuringSmoke(t *testing.T) {
	path := filepath.Join(canonicalTempDir(t), standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(path, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	_, err = observeInstalledBinary(context.Background(), inspection, runtime.GOOS, runtime.GOARCH, installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion: func(context.Context, string) (string, error) {
			file, openErr := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
			if openErr != nil {
				return "", openErr
			}
			if _, writeErr := file.WriteString("mutated-binary"); writeErr != nil {
				_ = file.Close()
				return "", writeErr
			}
			if closeErr := file.Close(); closeErr != nil {
				return "", closeErr
			}
			return "3.2.1", nil
		},
	})
	if err == nil {
		t.Fatal("binary mutation during smoke must fail closed")
	}
}

func TestInitializeStableAdoptionPersistsOnce(t *testing.T) {
	parent := canonicalTempDir(t)
	path := filepath.Join(parent, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("installed-binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps := installedEvidenceDeps{
		readBuildInfo: func(io.ReaderAt) (*debug.BuildInfo, error) { return adoptionBuildInfoForTest(), nil },
		smokeVersion:  func(context.Context, string) (string, error) { return "3.2.1", nil },
	}
	boundary, inspection, state, err := initializeStableAdoptionWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, deps)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readInstallationState(boundary, inspection)
	if err != nil {
		t.Fatal(err)
	}
	if got != state {
		t.Fatalf("persisted state differs: got %#v want %#v", got, state)
	}
	statePath := filepath.Join(boundary.Directory, installationStateFileName)
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := initializeStableAdoptionWith(context.Background(), path, runtime.GOOS, runtime.GOARCH, deps); err == nil {
		t.Fatal("second adoption must fail closed")
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("second adoption modified existing state")
	}
}
