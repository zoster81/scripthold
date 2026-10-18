package releasearchive

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func producerHandoffFixture(t *testing.T) (string, string, []NormalProducerSource, []ExpectedFile) {
	t.Helper()
	dist := filepath.Join(t.TempDir(), "dist")
	parent := t.TempDir()
	if err := os.Mkdir(dist, 0o700); err != nil {
		t.Fatal(err)
	}
	plan, trusted := exampleProducerSourcePlan()
	for i := range plan {
		item := &plan[i]
		dir := dist
		if item.SourceSubdir != "" {
			dir = filepath.Join(dist, item.SourceSubdir)
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		}
		body := []byte("real-independent-source:" + item.Original.Name + "\n")
		digest := sha256.Sum256(body)
		item.Original.Size = int64(len(body))
		item.Original.SHA256 = hex.EncodeToString(digest[:])
		trusted[i] = item.Original
		if err := os.WriteFile(filepath.Join(dir, item.SourceLeaf), body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dist, parent, plan, trusted
}

func expectedPhysicalNormalFiles(trusted []ExpectedFile, run, attempt int64) []ExpectedFile {
	physical := make([]ExpectedFile, len(trusted))
	for i, item := range trusted {
		physical[i] = item
		physical[i].Name = fmt.Sprintf("scripthold-raw-normal-%d-%d-%s", run, attempt, item.Name)
	}
	return physical
}

func TestStageNormalProducerOriginalsExact15(t *testing.T) {
	dist, parent, plan, trusted := producerHandoffFixture(t)
	plan[0], plan[14] = plan[14], plan[0]
	stage, err := stageNormalProducerOriginals("v3.3.0", dist, parent, 9001, 2, plan, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if stage == "" || !filepath.IsAbs(stage) {
		t.Fatalf("missing isolated private stage %q", stage)
	}
	physical := expectedPhysicalNormalFiles(trusted, 9001, 2)
	if err := verifyFlatDirectory(stage, physical); err != nil {
		t.Fatalf("all 15 original payloads must be byte-identical: %v", err)
	}
	files, err := os.ReadDir(stage)
	if err != nil || len(files) != 15 {
		t.Fatalf("expected 15 staged files: count=%d err=%v", len(files), err)
	}
	if files[0].Name() != "scripthold-raw-normal-9001-2-checksums.txt" {
		t.Fatalf("physical basename changed: %s", files[0].Name())
	}
}

func TestStageNormalProducerOriginalsRejectsInvalidAuthorityBeforeCreatingStage(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(string, string, []NormalProducerSource, []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64)
	}{
		{"zero run", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			return d, p, a, t, 0, 2
		}},
		{"negative attempt", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			return d, p, a, t, 9001, -1
		}},
		{"unsafe run numeric precision", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			return d, p, a, t, 9007199254740992, 1
		}},
		{"incorrect digest", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			a[0].Original.SHA256 = strings.Repeat("f", 64)
			return d, p, a, t, 1, 1
		}},
		{"wrong target folder", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			a[1].SourceSubdir = "unexpected"
			return d, p, a, t, 1, 1
		}},
		{"duplicate original", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			a[14] = a[0]
			return d, p, a, t, 1, 1
		}},
		{"staging equals source", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			return d, d, a, t, 1, 1
		}},
		{"relative staging parent", func(d, p string, a []NormalProducerSource, t []ExpectedFile) (string, string, []NormalProducerSource, []ExpectedFile, int64, int64) {
			return d, "relative", a, t, 1, 1
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dist, parent, plan, trusted := producerHandoffFixture(t)
			dist, parent, plan, trusted, run, attempt := tc.edit(dist, parent, plan, trusted)
			stage, err := stageNormalProducerOriginals("v3.3.0", dist, parent, run, attempt, plan, trusted)
			if err == nil || stage != "" {
				t.Fatalf("invalid input produced staging: %q %v", stage, err)
			}
			if filepath.IsAbs(parent) && parent != dist {
				entries, readErr := os.ReadDir(parent)
				if readErr != nil || len(entries) != 0 {
					t.Fatalf("failed preflight mutated staging parent: %d %v", len(entries), readErr)
				}
			}
		})
	}
}

func TestStageNormalProducerOriginalsFailedCopyLeavesInvalidPartialEvidence(t *testing.T) {
	dist, parent, plan, trusted := producerHandoffFixture(t)
	// Fail after some good originals have been copied.
	missing := plan[12]
	if err := os.Remove(filepath.Join(dist, missing.SourceLeaf)); err != nil {
		t.Fatal(err)
	}
	stage, err := stageNormalProducerOriginals("v3.3.0", dist, parent, 700, 4, plan, trusted)
	if err == nil || stage == "" {
		t.Fatalf("expected partial, rejected stage: %q %v", stage, err)
	}
	entries, readErr := os.ReadDir(stage)
	if readErr != nil || len(entries) == 0 || len(entries) >= 15 {
		t.Fatalf("expected non-publishable partial files, count=%d err=%v", len(entries), readErr)
	}
	if verifyFlatDirectory(stage, expectedPhysicalNormalFiles(trusted, 700, 4)) == nil {
		t.Fatal("partial stage accidentally qualified as complete")
	}
}

func TestStageNormalProducerOriginalsPublicLinuxOnly(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("the public entrypoint is native Linux only")
	}
	dist, parent, plan, trusted := producerHandoffFixture(t)
	if stage, err := StageNormalProducerOriginals("v3.3.0", dist, parent, 700, 4, plan, trusted); err == nil || stage != "" {
		t.Fatalf("non-Linux producer should be disabled: %q %v", stage, err)
	}
}
