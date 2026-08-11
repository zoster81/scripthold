package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

const realLineEndingFixtureDir = "testdata/line_endings_real"

type realLineEndingManifest struct {
	SchemaVersion int                     `json:"schema_version"`
	Description   string                  `json:"description"`
	Fixtures      []realLineEndingFixture `json:"fixtures"`
}

type realLineEndingFixture struct {
	Encoding           string `json:"encoding"`
	File               string `json:"file"`
	SourceProject      string `json:"source_project"`
	SourceRevision     string `json:"source_revision"`
	SourcePath         string `json:"source_path"`
	SourceURL          string `json:"source_url"`
	LicenseFile        string `json:"license_file"`
	SHA256             string `json:"sha256"`
	ByteLength         int    `json:"byte_length"`
	BOM                string `json:"bom"`
	ExpectedStyle      string `json:"expected_style"`
	CRLFCount          int    `json:"crlf_count"`
	LFCount            int    `json:"lf_count"`
	ExpectedTotalLines int    `json:"expected_total_lines"`
}

func loadRealLineEndingManifest(t *testing.T) realLineEndingManifest {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(realLineEndingFixtureDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read real fixture manifest: %v", err)
	}

	var manifest realLineEndingManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode real fixture manifest: %v", err)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatalf("manifest schema version = %d, want 1", manifest.SchemaVersion)
	}
	if len(manifest.Fixtures) == 0 {
		t.Fatal("real fixture manifest is empty")
	}
	return manifest
}

func readAndVerifyRealFixture(t *testing.T, fixture realLineEndingFixture) []byte {
	t.Helper()

	path := filepath.Join(realLineEndingFixtureDir, fixture.File)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixture.File, err)
	}
	if len(data) != fixture.ByteLength {
		t.Fatalf("fixture %s byte length = %d, want %d", fixture.File, len(data), fixture.ByteLength)
	}

	sum := sha256.Sum256(data)
	actualHash := hex.EncodeToString(sum[:])
	if actualHash != fixture.SHA256 {
		t.Fatalf("fixture %s SHA-256 = %s, want %s", fixture.File, actualHash, fixture.SHA256)
	}
	return data
}

func fixturePayloadWithoutBOM(t *testing.T, fixture realLineEndingFixture, data []byte) []byte {
	t.Helper()

	result, found := fileEncoding.DetectBOM(data)
	if fixture.BOM == "none" {
		if found {
			t.Fatalf("fixture %s has unexpected %s BOM", fixture.File, result.Charset)
		}
		return data
	}
	if !found {
		t.Fatalf("fixture %s is missing expected %s BOM", fixture.File, fixture.BOM)
	}
	if result.Charset != fixture.BOM {
		t.Fatalf("fixture %s BOM = %s, want %s", fixture.File, result.Charset, fixture.BOM)
	}
	return data[fileEncoding.BOMSize(result.Charset):]
}

func verifyRealFixtureDecoding(t *testing.T, fixture realLineEndingFixture, data []byte) {
	t.Helper()

	payload := fixturePayloadWithoutBOM(t, fixture, data)
	var decoded string
	if fileEncoding.IsUTF8(fixture.Encoding) {
		if !utf8.Valid(payload) {
			t.Fatalf("fixture %s is not valid UTF-8", fixture.File)
		}
		decoded = string(payload)
	} else {
		enc, ok := fileEncoding.Get(fixture.Encoding)
		if !ok {
			t.Fatalf("fixture encoding %q is not registered", fixture.Encoding)
		}
		utf8Bytes, err := enc.NewDecoder().Bytes(payload)
		if err != nil {
			t.Fatalf("decode fixture %s as %s: %v", fixture.File, fixture.Encoding, err)
		}
		decoded = string(utf8Bytes)
	}

	if decoded == "" {
		t.Fatalf("fixture %s decoded to empty text", fixture.File)
	}
	if strings.ContainsRune(decoded, '\uFFFD') {
		t.Fatalf("fixture %s decoded with replacement characters", fixture.File)
	}
	hasNonASCII := false
	for _, r := range decoded {
		if r > 0x7F {
			hasNonASCII = true
			break
		}
	}
	if !hasNonASCII {
		t.Fatalf("fixture %s does not exercise non-ASCII content", fixture.File)
	}
}

func TestRealLineEndingFixturesRemainSupportedAndVerified(t *testing.T) {
	manifest := loadRealLineEndingManifest(t)
	absoluteFixtureDir, err := filepath.Abs(realLineEndingFixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{absoluteFixtureDir})

	supported := make(map[string]bool)
	for _, item := range fileEncoding.ListEncodings() {
		supported[item.Name] = true
	}
	// Preserve the complete pre-R22 real-world baseline independently from the
	// registry-wide generated operation matrix.
	if len(manifest.Fixtures) != 24 {
		t.Fatalf("real fixture baseline = %d, want 24", len(manifest.Fixtures))
	}

	seen := make(map[string]bool)
	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(fixture.Encoding, func(t *testing.T) {
			if !supported[fixture.Encoding] {
				t.Fatalf("fixture uses unsupported encoding %q", fixture.Encoding)
			}
			if seen[fixture.Encoding] {
				t.Fatalf("duplicate real fixture for %q", fixture.Encoding)
			}
			seen[fixture.Encoding] = true

			if len(fixture.SourceRevision) != 40 || !strings.Contains(fixture.SourceURL, fixture.SourceRevision) {
				t.Fatalf("fixture %s does not use a pinned upstream revision", fixture.File)
			}
			if fixture.SourceProject == "" || fixture.SourcePath == "" || fixture.SourceURL == "" {
				t.Fatalf("fixture %s has incomplete provenance", fixture.File)
			}
			if _, err := os.Stat(filepath.Join(realLineEndingFixtureDir, fixture.LicenseFile)); err != nil {
				t.Fatalf("fixture %s license file: %v", fixture.File, err)
			}

			data := readAndVerifyRealFixture(t, fixture)
			verifyRealFixtureDecoding(t, fixture, data)

			path := filepath.Join(absoluteFixtureDir, fixture.File)
			result, output, err := h.HandleDetectLineEndings(context.Background(), nil, DetectLineEndingsInput{
				Path:     path,
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("detect_line_endings failed for real %s fixture", fixture.Encoding)
			}
			if output.Style != fixture.ExpectedStyle {
				t.Errorf("Style = %q, want %q", output.Style, fixture.ExpectedStyle)
			}
			if output.TotalLines != fixture.ExpectedTotalLines {
				t.Errorf("TotalLines = %d, want %d", output.TotalLines, fixture.ExpectedTotalLines)
			}
			if len(output.InconsistentLines) != 0 {
				t.Errorf("InconsistentLines = %v, want []", output.InconsistentLines)
			}
		})
	}
}

func TestRealLineEndingFixturesChangeRoundTripByteIdentical(t *testing.T) {
	manifest := loadRealLineEndingManifest(t)

	for _, fixture := range manifest.Fixtures {
		fixture := fixture
		t.Run(fixture.Encoding, func(t *testing.T) {
			original := readAndVerifyRealFixture(t, fixture)
			if fixture.ExpectedStyle != LineEndingLF && fixture.ExpectedStyle != LineEndingCRLF {
				t.Fatalf("real fixture style %q is not round-trip testable", fixture.ExpectedStyle)
			}

			tempDir := t.TempDir()
			workingPath := filepath.Join(tempDir, fixture.File)
			if err := os.WriteFile(workingPath, original, 0644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{tempDir})

			targetStyle := LineEndingCRLF
			expectedChanged := fixture.LFCount
			if fixture.ExpectedStyle == LineEndingCRLF {
				targetStyle = LineEndingLF
				expectedChanged = fixture.CRLFCount
			}

			result, output, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path:     workingPath,
				Style:    targetStyle,
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("change_line_endings failed for real %s fixture", fixture.Encoding)
			}
			if output.OriginalStyle != fixture.ExpectedStyle || output.NewStyle != targetStyle {
				t.Errorf("conversion styles = %s -> %s, want %s -> %s", output.OriginalStyle, output.NewStyle, fixture.ExpectedStyle, targetStyle)
			}
			if output.LinesChanged != expectedChanged {
				t.Errorf("LinesChanged = %d, want %d", output.LinesChanged, expectedChanged)
			}

			detectResult, detected, err := h.HandleDetectLineEndings(context.Background(), nil, DetectLineEndingsInput{
				Path:     workingPath,
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if detectResult.IsError || detected.Style != targetStyle {
				t.Fatalf("converted real fixture style = %q, want %q", detected.Style, targetStyle)
			}

			result, _, err = h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path:     workingPath,
				Style:    fixture.ExpectedStyle,
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("reverse change_line_endings failed for real %s fixture", fixture.Encoding)
			}

			roundTripped, err := os.ReadFile(workingPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(roundTripped, original) {
				t.Fatalf("real %s fixture is not byte-identical after line-ending round trip", fixture.Encoding)
			}
		})
	}
}

func TestRealUTF16LEFixtureAutoDetection(t *testing.T) {
	manifest := loadRealLineEndingManifest(t)
	absoluteFixtureDir, err := filepath.Abs(realLineEndingFixtureDir)
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{absoluteFixtureDir})

	for _, fixture := range manifest.Fixtures {
		if fixture.Encoding != "utf-16-le" {
			continue
		}
		result, output, err := h.HandleDetectLineEndings(context.Background(), nil, DetectLineEndingsInput{
			Path: filepath.Join(absoluteFixtureDir, fixture.File),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError {
			t.Fatal("auto-detection failed for real UTF-16 LE fixture")
		}
		if output.Style != fixture.ExpectedStyle || output.TotalLines != fixture.ExpectedTotalLines {
			t.Fatalf("auto-detected UTF-16 LE result = style %q, lines %d; want %q, %d", output.Style, output.TotalLines, fixture.ExpectedStyle, fixture.ExpectedTotalLines)
		}
		return
	}
	t.Fatal("real UTF-16 LE fixture not found")
}
