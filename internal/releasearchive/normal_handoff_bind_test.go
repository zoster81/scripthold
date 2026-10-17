package releasearchive

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

func trustedNormalOriginals(files []RawNormalFile) []ExpectedFile {
	trusted := make([]ExpectedFile, len(files))
	for index, file := range files {
		trusted[index] = file.Original
	}
	return trusted
}

func TestStageRawNormalHandoffLinuxMatchesIndependentInventory(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the exported private staging entrypoint requires native Linux")
	}
	source, parent, files := normalHandoffFixture(t)
	trusted := trustedNormalOriginals(files)
	invalid := append([]ExpectedFile(nil), trusted...)
	invalid[len(invalid)-1].SHA256 = strings.Repeat("f", 64)

	stage, err := StageRawNormalHandoff(source, parent, files, invalid)
	if err == nil || stage != "" {
		t.Fatalf("invalid independent inventory created staging root: %q, %v", stage, err)
	}
	before, err := os.ReadDir(parent)
	if err != nil || len(before) != 0 {
		t.Fatalf("rejected authority changed destination parent: %d, %v", len(before), err)
	}

	stage, err = StageRawNormalHandoff(source, parent, files, trusted)
	if err != nil || stage == "" {
		t.Fatalf("verified Linux public staging entrypoint failed: %q, %v", stage, err)
	}
	if err := verifyFlatDirectory(stage, trusted); err != nil {
		t.Fatalf("public Linux stage contains incorrect original bytes: %v", err)
	}
}

func TestBindRawNormalHandoffPlanToIndependentOriginals(t *testing.T) {
	_, _, input := normalHandoffFixture(t)
	trusted := trustedNormalOriginals(input)
	for left, right := 0, len(trusted)-1; left < right; left, right = left+1, right-1 {
		trusted[left], trusted[right] = trusted[right], trusted[left]
	}
	bound, err := BindRawNormalHandoffPlan(input, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound) != 15 || bound[0] != input[0] {
		t.Fatal("lost original raw handoff identity")
	}
	input[0].Original.Size++
	// The reversed trusted inventory's last entry describes original-01.
	trusted[len(trusted)-1].SHA256 = strings.Repeat("0", 64)
	if bound[0].Original.Size == input[0].Original.Size ||
		bound[0].Original.SHA256 == trusted[len(trusted)-1].SHA256 {
		t.Fatal("trusted plan result aliases mutable input authority")
	}
}

func TestBindRawNormalHandoffPlanRejectsUntrustedIndependentInventory(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func([]RawNormalFile, []ExpectedFile) ([]RawNormalFile, []ExpectedFile)
	}{
		{"missing trusted original", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			return p, a[:14]
		}},
		{"extra trusted original", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			return p, append(a, a[0])
		}},
		{"duplicate trusted name", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			a[1].Name = a[0].Name
			return p, a
		}},
		{"casefold alias trusted name", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			a[1].Name = strings.ToUpper(a[0].Name)
			return p, a
		}},
		{"unexpected trusted name", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			a[1].Name = "other-original"
			return p, a
		}},
		{"trusted length mismatch", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			a[14].Size++
			return p, a
		}},
		{"trusted digest mismatch", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			a[14].SHA256 = strings.Repeat("a", 64)
			return p, a
		}},
		{"invalid trusted digest", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			a[0].SHA256 = "invalid"
			return p, a
		}},
		{"substituted physical file", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			p[0].DownloadName = p[1].DownloadName
			return p, a
		}},
		{"untrusted reordered digest", func(p []RawNormalFile, a []ExpectedFile) ([]RawNormalFile, []ExpectedFile) {
			p[0].Original.SHA256, p[1].Original.SHA256 =
				p[1].Original.SHA256, p[0].Original.SHA256
			return p, a
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, input := normalHandoffFixture(t)
			trusted := trustedNormalOriginals(input)
			input, trusted = tc.change(input, trusted)
			bound, err := BindRawNormalHandoffPlan(input, trusted)
			if err == nil || bound != nil {
				t.Fatalf("accepted plan that differs from trusted originals: %+v, %v", bound, err)
			}
		})
	}
}
