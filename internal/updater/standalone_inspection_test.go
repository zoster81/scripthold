package updater

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInspectStandaloneExecutableAcceptsOrdinaryStandalone(t *testing.T) {
	directory := canonicalTempDir(t)
	path := filepath.Join(directory, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	inspection, err := inspectStandaloneExecutable(path, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.ExecutablePath == "" || inspection.ParentPath != filepath.Dir(inspection.ExecutablePath) {
		t.Fatalf("unexpected inspection: %#v", inspection)
	}
	if matches, matchErr := inspection.ExecutableIdentity.Matches(inspection.ExecutablePath); matchErr != nil || !matches {
		t.Fatalf("target identity match = %v, %v", matches, matchErr)
	}
	if matches, matchErr := inspection.ParentIdentity.Matches(inspection.ParentPath); matchErr != nil || !matches {
		t.Fatalf("parent identity match = %v, %v", matches, matchErr)
	}
}

func TestInspectStandaloneExecutableRejectsKnownMCPBLayout(t *testing.T) {
	for _, goos := range []string{"windows", "linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			root := canonicalTempDir(t)
			server := filepath.Join(root, "server")
			if err := os.Mkdir(server, 0o700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(server, standaloneBinaryName(goos))
			if err := os.WriteFile(path, []byte("managed"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte("{malformed"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := inspectStandaloneExecutable(path, goos); err == nil {
				t.Fatal("known MCPB layout must fail closed")
			}
		})
	}
}

func TestInspectStandaloneExecutableDoesNotGuessFromNearbyManifest(t *testing.T) {
	root := canonicalTempDir(t)
	path := filepath.Join(root, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "manifest.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := inspectStandaloneExecutable(path, runtime.GOOS); err != nil {
		t.Fatalf("ordinary standalone beside unrelated manifest was rejected: %v", err)
	}
}

func TestInspectStandaloneExecutableRejectsHardLink(t *testing.T) {
	root := canonicalTempDir(t)
	path := filepath.Join(root, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, filepath.Join(root, "alias")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, err := inspectStandaloneExecutable(path, runtime.GOOS); err == nil {
		t.Fatal("hard-linked executable must fail closed")
	}
}

func TestInspectStandaloneExecutableRejectsAliasedPathComponents(t *testing.T) {
	root := canonicalTempDir(t)
	realDirectory := filepath.Join(root, "real")
	if err := os.Mkdir(realDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(realDirectory, standaloneBinaryName(runtime.GOOS))
	if err := os.WriteFile(path, []byte("standalone"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(realDirectory, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("directory symlinks unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if _, err := inspectStandaloneExecutable(filepath.Join(alias, filepath.Base(path)), runtime.GOOS); err == nil {
		t.Fatal("executable reached through an aliased path component must fail closed")
	}
}

func TestStandaloneBinaryName(t *testing.T) {
	if got := standaloneBinaryName("windows"); got != "scripthold.exe" {
		t.Fatalf("windows name = %q", got)
	}
	for _, goos := range []string{"linux", "darwin"} {
		if got := standaloneBinaryName(goos); got != "scripthold" {
			t.Fatalf("%s name = %q", goos, got)
		}
	}
}
