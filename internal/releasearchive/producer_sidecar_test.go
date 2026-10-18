package releasearchive

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func separateProducerSidecars(t *testing.T) (string, string, string, string, []NormalProducerSource, []ExpectedFile) {
	t.Helper()
	dist, parent, plan, trusted := producerHandoffFixture(t)
	bundleDir, intotoDir := t.TempDir(), t.TempDir()
	for _, item := range plan {
		if !strings.HasSuffix(item.Original.Name, "-provenance.sigstore.json") &&
			!strings.HasSuffix(item.Original.Name, "-provenance.intoto.jsonl") {
			continue
		}
		to := bundleDir
		if strings.HasSuffix(item.Original.Name, ".intoto.jsonl") {
			to = intotoDir
		}
		if err := os.Rename(filepath.Join(dist, item.SourceLeaf), filepath.Join(to, item.SourceLeaf)); err != nil {
			t.Fatal(err)
		}
	}
	return dist, bundleDir, intotoDir, parent, plan, trusted
}

func TestStageNormalProducerOriginalsWithSidecarsComplete(t *testing.T) {
	dist, bundleDir, intotoDir, parent, plan, trusted := separateProducerSidecars(t)
	stage, err := stageNormalProducerOriginalsWithSidecars("v3.3.0", dist, bundleDir, intotoDir, parent, 901, 3, plan, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if stage == "" {
		t.Fatal("expected complete stage")
	}
	if err := verifyFlatDirectory(stage, expectedPhysicalNormalFiles(trusted, 901, 3)); err != nil {
		t.Fatalf("separately rooted sidecars must preserve all 15 original bytes: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dist, plan[13].SourceLeaf)); !os.IsNotExist(err) {
		t.Fatalf("bundle must not be copied into GoReleaser dist: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dist, plan[14].SourceLeaf)); !os.IsNotExist(err) {
		t.Fatalf("in-toto must not be copied into GoReleaser dist: %v", err)
	}
}

func TestStageNormalProducerOriginalsWithSidecarsInvalidRoots(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(t *testing.T, dist, bundle, intoto, stage string) (string, string, string, string)
	}{
		{"relative bundle", func(t *testing.T, d, b, i, s string) (string, string, string, string) {
			return d, "relative", i, s
		}},
		{"missing in-toto directory", func(t *testing.T, d, b, i, s string) (string, string, string, string) {
			return d, b, filepath.Join(i, "missing"), s
		}},
		{"dist source equals stage parent", func(t *testing.T, d, b, i, s string) (string, string, string, string) {
			return d, b, i, d
		}},
		{"stage inside dist", func(t *testing.T, d, b, i, s string) (string, string, string, string) {
			child := filepath.Join(d, "stage-parent")
			if err := os.Mkdir(child, 0o700); err != nil {
				t.Fatal(err)
			}
			return d, b, i, child
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dist, bundleDir, intotoDir, parent, plan, trusted := separateProducerSidecars(t)
			dist, bundleDir, intotoDir, parent = tc.change(t, dist, bundleDir, intotoDir, parent)
			stage, err := stageNormalProducerOriginalsWithSidecars("v3.3.0", dist, bundleDir, intotoDir, parent, 901, 3, plan, trusted)
			if err == nil || stage != "" {
				t.Fatalf("invalid roots created a stage: %q %v", stage, err)
			}
		})
	}
}

func TestStageNormalProducerOriginalsWithSidecarsCanShareStagingParent(t *testing.T) {
	dist, bundleDir, intotoDir, parent, plan, trusted := separateProducerSidecars(t)
	for _, item := range []struct{ from, name string }{
		{bundleDir, plan[13].SourceLeaf}, {intotoDir, plan[14].SourceLeaf},
	} {
		if err := os.Rename(filepath.Join(item.from, item.name), filepath.Join(parent, item.name)); err != nil {
			t.Fatal(err)
		}
	}
	stage, err := stageNormalProducerOriginalsWithSidecars("v3.3.0", dist, parent, parent, parent, 902, 5, plan, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyFlatDirectory(stage, expectedPhysicalNormalFiles(trusted, 902, 5)); err != nil {
		t.Fatalf("staging under the provenance source parent changed originals: %v", err)
	}
	for _, name := range []string{plan[13].SourceLeaf, plan[14].SourceLeaf} {
		if _, err := os.Stat(filepath.Join(parent, name)); err != nil {
			t.Fatalf("source sidecar must remain in place: %v", err)
		}
	}
}

func TestStageNormalProducerOriginalsWithSidecarsLateFailureIsPartial(t *testing.T) {
	dist, bundleDir, intotoDir, parent, plan, trusted := separateProducerSidecars(t)
	// The second sidecar fails after the original binary/archive/checksum files are copied.
	if err := os.WriteFile(filepath.Join(intotoDir, plan[14].SourceLeaf), []byte("altered"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage, err := stageNormalProducerOriginalsWithSidecars("v3.3.0", dist, bundleDir, intotoDir, parent, 901, 3, plan, trusted)
	if err == nil || stage == "" {
		t.Fatalf("late mismatch must return a nonpublishable partial stage: %q %v", stage, err)
	}
	if verifyFlatDirectory(stage, expectedPhysicalNormalFiles(trusted, 901, 3)) == nil {
		t.Fatal("altered provenance must not qualify a complete raw stage")
	}
}

func TestStageNormalProducerOriginalsWithSidecarsRejectsSymlink(t *testing.T) {
	dist, bundleDir, intotoDir, parent, plan, trusted := separateProducerSidecars(t)
	leaf := plan[13].SourceLeaf
	original := filepath.Join(bundleDir, leaf)
	if err := os.Rename(original, original+".real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original+".real", original); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	stage, err := stageNormalProducerOriginalsWithSidecars("v3.3.0", dist, bundleDir, intotoDir, parent, 901, 3, plan, trusted)
	if err == nil || stage == "" {
		t.Fatalf("linked sidecar must reject the partial stage: %q %v", stage, err)
	}
}

func TestStageNormalProducerOriginalsWithSidecarsLinuxOnly(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("public entrypoint requires native Linux")
	}
	dist, bundleDir, intotoDir, parent, plan, trusted := separateProducerSidecars(t)
	stage, err := StageNormalProducerOriginalsWithSidecars("v3.3.0", dist, bundleDir, intotoDir, parent, 901, 3, plan, trusted)
	if err == nil || stage != "" {
		t.Fatalf("non-Linux call must be disabled: %q %v", stage, err)
	}
}
