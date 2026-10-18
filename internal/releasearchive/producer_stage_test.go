package releasearchive

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func producerSourceFixture(t *testing.T) (string, string, string, []byte) {
	t.Helper()
	dist := filepath.Join(t.TempDir(), "dist")
	if err := os.Mkdir(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	build := "scripthold_linux_amd64_v1"
	if err := os.Mkdir(filepath.Join(dist, build), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("original Linux GoReleaser executable bytes")
	if err := os.WriteFile(filepath.Join(dist, build, "scripthold"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	stage, err := createPrivateRawStagingDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dist, build, stage, body
}

func TestProducerRootedSourceStagesNestedAndFlatOriginals(t *testing.T) {
	dist, build, stage, body := producerSourceFixture(t)
	const output = "raw-normal-9001-2-scripthold_linux_amd64"
	if err := stageVerifiedProducerSourceFile(dist, build, "scripthold", stage, output,
		flatExpected("scripthold_linux_amd64", body)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(stage, output))
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("nested original bytes changed: %v", err)
	}
	checksums := []byte(strings.Repeat("a", 64) + "  scripthold_linux_amd64\n")
	if err := os.WriteFile(filepath.Join(dist, "checksums.txt"), checksums, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stageVerifiedProducerSourceFile(dist, "", "checksums.txt", stage,
		"raw-normal-9001-2-checksums.txt", flatExpected("checksums.txt", checksums)); err != nil {
		t.Fatal(err)
	}
	if err := verifyFlatDirectory(stage, []ExpectedFile{
		flatExpected(output, body),
		flatExpected("raw-normal-9001-2-checksums.txt", checksums),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProducerRootedSourceRejectsUnsafeDirectoriesBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		name string
		bad  func(t *testing.T, dist, build string) (string, string)
	}{
		{"relative root", func(_ *testing.T, _, build string) (string, string) { return "dist", build }},
		{"parent traversal", func(_ *testing.T, dist, _ string) (string, string) { return dist, "../outside" }},
		{"nested separators", func(_ *testing.T, dist, _ string) (string, string) { return dist, "a/b" }},
		{"windows alias", func(_ *testing.T, dist, _ string) (string, string) { return dist, "RUNNER~1" }},
		{"wrong directory", func(_ *testing.T, dist, _ string) (string, string) { return dist, "missing" }},
		{"source root symlink", func(t *testing.T, dist, build string) (string, string) {
			alias := filepath.Join(t.TempDir(), "alias")
			if err := os.Symlink(dist, alias); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			return alias, build
		}},
		{"nested parent symlink", func(t *testing.T, dist, build string) (string, string) {
			outside := filepath.Join(t.TempDir(), "outside")
			if err := os.Mkdir(outside, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(dist, build)); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(dist, build)); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			return dist, build
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dist, build, stage, body := producerSourceFixture(t)
			src, sub := tc.bad(t, dist, build)
			if err := stageVerifiedProducerSourceFile(src, sub, "scripthold", stage, "raw-original",
				flatExpected("scripthold_linux_amd64", body)); err == nil {
				t.Fatal("unsafe producer path accepted")
			}
			if items, err := os.ReadDir(stage); err != nil || len(items) != 0 {
				t.Fatalf("unsafe path wrote to output: %v, %v", items, err)
			}
		})
	}
}

func TestProducerRootedSourceRejectsSourceTamperingAndNoOverwrite(t *testing.T) {
	for _, change := range []struct {
		name string
		fn   func(t *testing.T, source string, bytes []byte)
	}{
		{"wrong checksum", func(_ *testing.T, _ string, _ []byte) {}},
		{"hardlinked source", func(t *testing.T, source string, _ []byte) {
			if err := os.Link(source, filepath.Join(t.TempDir(), "outside")); err != nil {
				t.Skipf("hard link unavailable: %v", err)
			}
		}},
		{"symlink source leaf", func(t *testing.T, source string, bytes []byte) {
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			}
			other := filepath.Join(t.TempDir(), "original")
			if err := os.WriteFile(other, bytes, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(other, source); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			dist, build, stage, body := producerSourceFixture(t)
			change.fn(t, filepath.Join(dist, build, "scripthold"), body)
			expected := flatExpected("scripthold_linux_amd64", body)
			if change.name == "wrong checksum" {
				expected = flatExpected("scripthold_linux_amd64", []byte("wrong signed bytes"))
			}
			if err := stageVerifiedProducerSourceFile(dist, build, "scripthold", stage, "raw-original",
				expected); err == nil {
				t.Fatal("untrusted producer bytes accepted")
			}
			if items, err := os.ReadDir(stage); err != nil || len(items) != 0 {
				t.Fatalf("failed copy left staged original: %v %v", items, err)
			}
		})
	}
	// A second invocation must never replace a previously staged object.
	dist, build, stage, body := producerSourceFixture(t)
	if err := stageVerifiedProducerSourceFile(dist, build, "scripthold", stage, "existing",
		flatExpected("scripthold_linux_amd64", body)); err != nil {
		t.Fatal(err)
	}
	if err := stageVerifiedProducerSourceFile(dist, build, "scripthold", stage, "existing",
		flatExpected("scripthold_linux_amd64", body)); err == nil {
		t.Fatal("existing output silently overwritten")
	}
}
