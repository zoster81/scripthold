package releasearchive

import (
	"reflect"
	"strings"
	"testing"
)

func exampleProducerSourcePlan() ([]NormalProducerSource, []ExpectedFile) {
	assets := []struct {
		name, child, leaf string
	}{
		{"scripthold_darwin_amd64", "scripthold_darwin_amd64_v1", "scripthold"},
		{"scripthold_darwin_amd64.tar.gz", "", "scripthold_darwin_amd64.tar.gz"},
		{"scripthold_darwin_arm64", "scripthold_darwin_arm64_v8.0", "scripthold"},
		{"scripthold_darwin_arm64.tar.gz", "", "scripthold_darwin_arm64.tar.gz"},
		{"scripthold_linux_amd64", "scripthold_linux_amd64_v1", "scripthold"},
		{"scripthold_linux_amd64.tar.gz", "", "scripthold_linux_amd64.tar.gz"},
		{"scripthold_linux_arm64", "scripthold_linux_arm64_v8.0", "scripthold"},
		{"scripthold_linux_arm64.tar.gz", "", "scripthold_linux_arm64.tar.gz"},
		{"scripthold_windows_amd64.exe", "scripthold_windows_amd64_v1", "scripthold.exe"},
		{"scripthold_windows_amd64.zip", "", "scripthold_windows_amd64.zip"},
		{"scripthold_windows_arm64.exe", "scripthold_windows_arm64_v8.0", "scripthold.exe"},
		{"scripthold_windows_arm64.zip", "", "scripthold_windows_arm64.zip"},
		{"checksums.txt", "", "checksums.txt"},
		{"scripthold-v3.3.0-provenance.sigstore.json", "", "scripthold-v3.3.0-provenance.sigstore.json"},
		{"scripthold-v3.3.0-provenance.intoto.jsonl", "", "scripthold-v3.3.0-provenance.intoto.jsonl"},
	}
	plan := make([]NormalProducerSource, 0, len(assets))
	trusted := make([]ExpectedFile, 0, len(assets))
	for _, a := range assets {
		original := ExpectedFile{Name: a.name, Size: 50, SHA256: strings.Repeat("a", 64)}
		plan = append(plan, NormalProducerSource{Original: original, SourceSubdir: a.child, SourceLeaf: a.leaf})
		trusted = append(trusted, original)
	}
	return plan, trusted
}

func TestBindNormalProducerSourcePlanExact15(t *testing.T) {
	plan, trusted := exampleProducerSourcePlan()
	plan[0], plan[14] = plan[14], plan[0]
	result, err := BindNormalProducerSourcePlan("v3.3.0", plan, trusted)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 15 {
		t.Fatalf("expected 15, got %d", len(result))
	}
	if result[0].Original.Name != "scripthold_darwin_amd64" ||
		result[14].Original.Name != "scripthold-v3.3.0-provenance.intoto.jsonl" {
		t.Fatalf("not in canonical output order: first=%q last=%q", result[0].Original.Name, result[14].Original.Name)
	}
	if result[0].SourceSubdir != "scripthold_darwin_amd64_v1" ||
		result[0].SourceLeaf != "scripthold" ||
		result[12].SourceSubdir != "" || result[12].SourceLeaf != "checksums.txt" {
		t.Fatal("source location mismatch")
	}
	result[0].Original.Name = "mutated"
	if plan[14].Original.Name != "scripthold_darwin_amd64" {
		t.Fatal("returned data aliases mutable caller input")
	}
}

func TestBindNormalProducerSourcePlanRejectsTampering(t *testing.T) {
	cases := []struct {
		name string
		edit func([]NormalProducerSource, []ExpectedFile) ([]NormalProducerSource, []ExpectedFile)
	}{
		{"missing", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			return p[:14], t
		}},
		{"extra", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			return append(p, p[0]), t
		}},
		{"duplicate", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[1] = p[0]
			return p, t
		}},
		{"case alias", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].Original.Name = strings.ToUpper(p[0].Original.Name)
			return p, t
		}},
		{"wrong name", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].Original.Name = "scripthold_linux_386"
			return p, t
		}},
		{"swapped target", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].SourceSubdir = p[2].SourceSubdir
			return p, t
		}},
		{"source traversal", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].SourceSubdir = "../outside"
			return p, t
		}},
		{"nested source", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].SourceSubdir = "nested/build"
			return p, t
		}},
		{"unexpected binary leaf", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].SourceLeaf = "other"
			return p, t
		}},
		{"sidecar misplaced", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[14].SourceSubdir = "dist"
			return p, t
		}},
		{"wrong digest", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].Original.SHA256 = strings.Repeat("b", 64)
			return p, t
		}},
		{"wrong size", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[0].Original.Size++
			return p, t
		}},
		{"oversized checksum", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[12].Original.Size = 17 * 1024
			t[12].Size = 17 * 1024
			return p, t
		}},
		{"oversized sidecar", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			p[13].Original.Size = 5 * 1024 * 1024
			t[13].Size = 5 * 1024 * 1024
			return p, t
		}},
		{"bad trusted name", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			t[0].Name = "bad"
			return p, t
		}},
		{"duplicate trusted", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			t[1] = t[0]
			return p, t
		}},
		{"uppercase hash", func(p []NormalProducerSource, t []ExpectedFile) ([]NormalProducerSource, []ExpectedFile) {
			h := strings.Repeat("A", 64)
			p[0].Original.SHA256 = h
			t[0].SHA256 = h
			return p, t
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, trusted := exampleProducerSourcePlan()
			p, trusted = tc.edit(p, trusted)
			if _, err := BindNormalProducerSourcePlan("v3.3.0", p, trusted); err == nil {
				t.Fatal("accepted untrusted original/source mapping")
			}
		})
	}
}

func TestBindNormalProducerSourcePlanRejectsUnsupportedTags(t *testing.T) {
	p, trusted := exampleProducerSourcePlan()
	for _, tag := range []string{"", "3.3.0", "../v3.3.0", "v3.3.0/other", "v3.3.0-", "v3.3.0\n", strings.Repeat("9", 300)} {
		if _, err := BindNormalProducerSourcePlan(tag, p, trusted); err == nil {
			t.Fatalf("accepted invalid tag %q", tag)
		}
	}
	result, err := BindNormalProducerSourcePlan("v3.3.0", p, trusted)
	if err != nil || !reflect.DeepEqual(result[0], p[0]) {
		t.Fatalf("valid source not retained: %v", err)
	}
}
