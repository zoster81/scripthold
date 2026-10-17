package releasearchive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func normalHandoffFixture(t *testing.T) (string, string, []RawNormalFile) {
	t.Helper()
	source := t.TempDir()
	parent := t.TempDir()
	if err := os.Chmod(source, 0o700); err != nil {
		t.Fatal(err)
	}
	assets := make([]RawNormalFile, 15)
	for index := range assets {
		name := fmt.Sprintf("original-%02d", index+1)
		downloadName := fmt.Sprintf("raw-9001-2-%02d", index+1)
		body := []byte("synthetic original for " + name)
		digest := sha256.Sum256(body)
		if err := os.WriteFile(filepath.Join(source, downloadName), body, 0o600); err != nil {
			t.Fatal(err)
		}
		assets[index] = RawNormalFile{
			DownloadName: downloadName,
			Original: ExpectedFile{Name: name, Size: int64(len(body)),
				SHA256: hex.EncodeToString(digest[:])},
		}
	}
	return source, parent, assets
}

func TestRawNormalFileJSONStagePlanContract(t *testing.T) {
	record := fmt.Sprintf(`[{"downloadName":"raw-normal-9001-2-original-01","original":{"name":"original-01","size":12,"sha256":"%s"}}]`,
		strings.Repeat("a", 64))
	var files []RawNormalFile
	decoder := json.NewDecoder(strings.NewReader(record))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&files); err != nil {
		t.Fatalf("Go rejected canonical lowercase staging plan: %v", err)
	}
	if len(files) != 1 || files[0].DownloadName != "raw-normal-9001-2-original-01" ||
		files[0].Original.Name != "original-01" || files[0].Original.Size != 12 ||
		files[0].Original.SHA256 != strings.Repeat("a", 64) {
		t.Fatalf("Go staging contract changed: %+v", files)
	}
	for _, unexpected := range []string{
		`[{"downloadName":"raw-file","path":"untrusted","original":{"name":"a","size":1,"sha256":"aa"}}]`,
		`[{"downloadName":"raw-file","original":{"name":"a","size":1,"sha256":"aa","path":"untrusted"}}]`,
	} {
		var rejected []RawNormalFile
		decoder := json.NewDecoder(strings.NewReader(unexpected))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&rejected); err == nil {
			t.Fatal("accepted unknown path authority in raw stage mapping")
		}
	}
}

func TestStageRawNormalHandoffRejectsUnsupportedPlatform(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("Linux API is exercised on the native runner")
	}
	source, parent, assets := normalHandoffFixture(t)
	stage, err := StageRawNormalHandoff(source, parent, assets, trustedNormalOriginals(assets))
	if err == nil || stage != "" {
		t.Fatalf("non-Linux runner accepted publication staging: stage=%q err=%v", stage, err)
	}
}

func TestStageRawNormalHandoffUsesVerifiedOriginalCopies(t *testing.T) {
	source, parent, assets := normalHandoffFixture(t)
	stage, err := stageRawNormalHandoff(source, parent, assets)
	if err != nil {
		t.Fatal(err)
	}
	canonical := make([]ExpectedFile, len(assets))
	for i, asset := range assets {
		canonical[i] = asset.Original
	}
	if err := verifyFlatDirectory(stage, canonical); err != nil {
		t.Fatalf("consumer output rejected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(source, assets[0].DownloadName),
		[]byte("producer was replaced"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyFlatDirectory(stage, canonical); err != nil {
		t.Fatalf("private staged originals changed after producer edit: %v", err)
	}
}

func TestStageRawNormalHandoffRefusesUntrustedEvidenceBeforeMutation(t *testing.T) {
	cases := []struct {
		name   string
		change func([]RawNormalFile) []RawNormalFile
	}{
		{"missing", func(a []RawNormalFile) []RawNormalFile { return a[:14] }},
		{"extra", func(a []RawNormalFile) []RawNormalFile { return append(a, a[0]) }},
		{"duplicate source", func(a []RawNormalFile) []RawNormalFile {
			a[1].DownloadName = a[0].DownloadName
			return a
		}},
		{"casefold duplicate original", func(a []RawNormalFile) []RawNormalFile {
			a[1].Original.Name = strings.ToUpper(a[0].Original.Name)
			return a
		}},
		{"malformed physical basename", func(a []RawNormalFile) []RawNormalFile {
			a[0].DownloadName = "../escape"
			return a
		}},
		{"windows alias", func(a []RawNormalFile) []RawNormalFile {
			a[0].DownloadName = "RUNNER~1"
			return a
		}},
		{"oversized", func(a []RawNormalFile) []RawNormalFile {
			a[0].Original.Size = maxAssetBytes + 1
			return a
		}},
		{"invalid digest", func(a []RawNormalFile) []RawNormalFile {
			a[0].Original.SHA256 = strings.ToUpper(a[0].Original.SHA256)
			return a
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, parent, assets := normalHandoffFixture(t)
			stage, err := stageRawNormalHandoff(source, parent, tc.change(assets))
			if err == nil || stage != "" {
				t.Fatalf("invalid authority created a staging directory: stage=%q err=%v", stage, err)
			}
			items, err := os.ReadDir(parent)
			if err != nil || len(items) != 0 {
				t.Fatalf("preflight wrote to destination parent: entries=%d err=%v", len(items), err)
			}
		})
	}
}

func TestStageRawNormalHandoffNeverStagesInsideItsOwnDownloadRoot(t *testing.T) {
	source, _, assets := normalHandoffFixture(t)
	stage, err := stageRawNormalHandoff(source, source, assets)
	if err == nil || stage != "" {
		t.Fatalf("overlapping input/output authority accepted: stage=%q err=%v", stage, err)
	}
	entries, err := os.ReadDir(source)
	if err != nil || len(entries) != 15 {
		t.Fatalf("failed preflight mutated raw input inventory: count=%d err=%v", len(entries), err)
	}
}

func TestStageRawNormalHandoffRejectsRelativeDirectories(t *testing.T) {
	source, parent, assets := normalHandoffFixture(t)
	for _, tc := range []struct {
		name, source, parent string
	}{
		{"relative download root", ".", parent},
		{"relative staging parent", source, "."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stage, err := stageRawNormalHandoff(tc.source, tc.parent, assets)
			if err == nil || stage != "" {
				t.Fatalf("accepted untrusted relative directory: stage=%q err=%v", stage, err)
			}
			items, err := os.ReadDir(parent)
			if err != nil || len(items) != 0 {
				t.Fatalf("invalid directory authority mutated parent: entries=%d err=%v", len(items), err)
			}
		})
	}
}

func TestStageRawNormalHandoffChecksAllSourcesBeforeCopying(t *testing.T) {
	cases := []struct {
		name   string
		change func(string, []RawNormalFile) error
	}{
		{"altered last source", func(source string, a []RawNormalFile) error {
			return os.WriteFile(filepath.Join(source, a[14].DownloadName),
				[]byte("tampered last file"), 0o600)
		}},
		{"missing last source", func(source string, a []RawNormalFile) error {
			return os.Remove(filepath.Join(source, a[14].DownloadName))
		}},
		{"undeclared source entry", func(source string, _ []RawNormalFile) error {
			return os.WriteFile(filepath.Join(source, "extra"), []byte("not trusted"), 0o600)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, parent, assets := normalHandoffFixture(t)
			if err := tc.change(source, assets); err != nil {
				t.Fatal(err)
			}
			stage, err := stageRawNormalHandoff(source, parent, assets)
			if err == nil || stage != "" {
				t.Fatalf("untrusted source created stage: %q %v", stage, err)
			}
			items, err := os.ReadDir(parent)
			if err != nil || len(items) != 0 {
				t.Fatalf("source preflight created destination: entries=%d err=%v", len(items), err)
			}
		})
	}
}
