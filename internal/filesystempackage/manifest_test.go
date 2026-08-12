package filesystempackage

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestManifestV1AcceptsExactlySevenOperationShapes(t *testing.T) {
	operations := []string{
		"{\"type\":\"mkdir\",\"path\":\"root/dir\"}",
		"{\"type\":\"createFile\",\"path\":\"root/file.bin\",\"contentBase64\":\"AAEC\"}",
		"{\"type\":\"copyFile\",\"source\":\"root/a\",\"destination\":\"root/b\"}",
		"{\"type\":\"copyDirectory\",\"source\":\"root/a\",\"destination\":\"root/b\"}",
		"{\"type\":\"move\",\"source\":\"root/a\",\"destination\":\"root/b\"}",
		"{\"type\":\"deleteFile\",\"path\":\"root/file.bin\"}",
		"{\"type\":\"deleteDirectory\",\"path\":\"root/dir\"}",
	}
	for _, rawOperation := range operations {
		raw := "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[" + rawOperation + "]}"
		var manifest Manifest
		if err := json.Unmarshal([]byte(raw), &manifest); err != nil {
			t.Fatalf("decode %s: %v", rawOperation, err)
		}
		if err := ValidateManifest(manifest, testManifestLimits()); err != nil {
			t.Fatalf("validate %s: %v", rawOperation, err)
		}
	}
}

func TestManifestV1RejectsMalformedAndAmbiguousOperations(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "unknown manifest field", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"mkdir\",\"path\":\"x\"}],\"extra\":true}"},
		{name: "unknown operation", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"chmod\",\"path\":\"x\"}]}"},
		{name: "unknown operation field", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"mkdir\",\"path\":\"x\",\"extra\":true}]}"},
		{name: "foreign field", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"deleteFile\",\"path\":\"x\",\"source\":\"y\"}]}"},
		{name: "invalid base64", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"createFile\",\"path\":\"x\",\"contentBase64\":\"%%%\"}]}"},
		{name: "non-canonical base64", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"createFile\",\"path\":\"x\",\"contentBase64\":\"AAE=\\n\"}]}"},
		{name: "missing path", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"mkdir\"}]}"},
		{name: "missing source", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"copyFile\",\"destination\":\"x\"}]}"},
		{name: "missing destination", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"move\",\"source\":\"x\"}]}"},
		{name: "wrong version", raw: "{\"formatVersion\":\"filesystem-package-v2\",\"operations\":[{\"type\":\"mkdir\",\"path\":\"x\"}]}"},
		{name: "empty operations", raw: "{\"formatVersion\":\"filesystem-package-v1\",\"operations\":[]}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var manifest Manifest
			err := json.Unmarshal([]byte(tt.raw), &manifest)
			if err == nil {
				err = ValidateManifest(manifest, testManifestLimits())
			}
			if err == nil {
				t.Fatalf("malformed manifest unexpectedly accepted: %s", tt.raw)
			}
		})
	}
}

func TestManifestV1EnforcesPackageAndContentBounds(t *testing.T) {
	limits := testManifestLimits()
	limits.MaxOperations = 1
	manifest := Manifest{
		FormatVersion: FormatV1,
		Operations: []Operation{
			{Type: OperationMkdir, Path: "a"},
			{Type: OperationMkdir, Path: "b"},
		},
	}
	if err := ValidateManifest(manifest, limits); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("operation limit error = %v, want LIMIT", err)
	}

	limits = testManifestLimits()
	limits.MaxPathBytes = 3
	manifest = Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationMkdir, Path: "long"}}}
	if err := ValidateManifest(manifest, limits); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("path limit error = %v, want LIMIT", err)
	}

	limits = testManifestLimits()
	limits.MaxFileBytes = 2
	manifest = Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationCreateFile, Path: "x", Content: []byte("abc")}}}
	if err := ValidateManifest(manifest, limits); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("content limit error = %v, want LIMIT", err)
	}

	limits = testManifestLimits()
	limits.MaxManifestBytes = 32
	manifest = Manifest{FormatVersion: FormatV1, Operations: []Operation{{Type: OperationMkdir, Path: strings.Repeat("x", 20)}}}
	if err := ValidateManifest(manifest, limits); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("manifest limit error = %v, want LIMIT", err)
	}
}

func TestManifestV1EnforcesConfiguredRawJSONByteLimit(t *testing.T) {
	raw := []byte("{" + strings.Repeat(" ", 256) + "\"formatVersion\":\"filesystem-package-v1\",\"operations\":[{\"type\":\"mkdir\",\"path\":\"x\"}]}")
	var manifest Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	limits := testManifestLimits()
	limits.MaxManifestBytes = int64(len(raw) - 1)
	if err := ValidateManifest(manifest, limits); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("raw manifest byte limit error = %v, want LIMIT", err)
	}
}

func testManifestLimits() Limits {
	return Limits{
		MaxOperations:       8,
		MaxManifestBytes:    4096,
		MaxPathBytes:        1024,
		MaxFileBytes:        1024,
		MaxRecursiveEntries: 128,
		MaxRecursiveDepth:   16,
		MaxAggregateBytes:   1 << 20,
		MaxStagingBytes:     1 << 20,
	}
}
