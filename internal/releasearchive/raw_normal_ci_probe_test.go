package releasearchive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestCITransferredNormalHandoff is enabled only in the nonpublishing
// GitHub raw-artifact transfer probe, after its 22-byte-stream oracle passes.
// The expected 15 originals are independently derived from known fixture
// bytes rather than producer-supplied digests or mutable artifact display names.
func TestCITransferredNormalHandoff(t *testing.T) {
	if os.Getenv("RAW_NORMAL_PROBE") != "1" {
		t.Skip("requires actual GitHub raw-download consumer job")
	}
	if runtime.GOOS != "linux" {
		t.Fatal("raw normal handoff must execute on a Linux runner")
	}
	runID := os.Getenv("GITHUB_RUN_ID")
	attempt := os.Getenv("NORMAL_ATTEMPT")
	for _, value := range []string{runID, attempt} {
		if len(value) == 0 || len(value) > 20 || value[0] == '0' {
			t.Fatal("missing or invalid producer run identity")
		}
		for i := 0; i < len(value); i++ {
			if value[i] < '0' || value[i] > '9' {
				t.Fatal("invalid producer run identity")
			}
		}
	}
	temp := os.Getenv("RUNNER_TEMP")
	if temp == "" || !filepath.IsAbs(temp) {
		t.Fatal("runner temporary directory is not absolute")
	}
	// The 15 normals are downloaded separately because the strict private
	// staging source must contain exactly 15 entries, never all 22 assets.
	sourceDir := filepath.Join(temp, "raw-normal-probe")
	parent := t.TempDir()
	zipOriginal, err := hex.DecodeString("504b0506000000000000000000000000000000000000")
	if err != nil || len(zipOriginal) != 22 {
		t.Fatal("invalid independent ZIP byte oracle")
	}
	plan := make([]RawNormalFile, 0, 15)
	trusted := make([]ExpectedFile, 0, 15)
	for i := 1; i <= 15; i++ {
		ext := "dat"
		original := []byte(fmt.Sprintf("raw-probe:normal:%02d\n", i))
		if i == 12 {
			ext = "zip"
			original = zipOriginal
		}
		physical := fmt.Sprintf("probe-%s-%s-normal-%02d.%s", runID, attempt, i, ext)
		canonical := fmt.Sprintf("synthetic-normal-%02d.%s", i, ext)
		sum := sha256.Sum256(original)
		expected := ExpectedFile{
			Name: canonical, Size: int64(len(original)), SHA256: hex.EncodeToString(sum[:]),
		}
		plan = append(plan, RawNormalFile{DownloadName: physical, Original: expected})
		trusted = append(trusted, expected)
	}
	stage, err := StageRawNormalHandoff(sourceDir, parent, plan, trusted)
	if err != nil {
		t.Fatalf("cannot stage authenticated transferred originals: %v (partial=%t)", err, stage != "")
	}
	if err := VerifyFlatDirectory(stage, trusted); err != nil {
		t.Fatalf("consumer cannot verify the 15 private staged originals: %v", err)
	}
	bytesFromStage, err := os.ReadFile(filepath.Join(stage, "synthetic-normal-12.zip"))
	if err != nil || !bytes.Equal(bytesFromStage, zipOriginal) {
		t.Fatalf("the original raw ZIP bytes did not survive the private handoff: %v", err)
	}
	t.Log("15/15 transferred originals verified in private Linux staging, including original ZIP bytes")
}
