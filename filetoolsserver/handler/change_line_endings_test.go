package handler

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/textstream"
)

func TestHandleChangeLineEndings_CRLFtoLF(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("line1\r\nline2\r\nline3\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	input := ChangeLineEndingsInput{Path: testFile, Style: "lf"}
	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	if output.OriginalStyle != "crlf" {
		t.Errorf("expected originalStyle=crlf, got %s", output.OriginalStyle)
	}
	if output.NewStyle != "lf" {
		t.Errorf("expected newStyle=lf, got %s", output.NewStyle)
	}
	if output.LinesChanged != 3 {
		t.Errorf("expected linesChanged=3, got %d", output.LinesChanged)
	}

	// Verify file content
	data, _ := os.ReadFile(testFile)
	if string(data) != "line1\nline2\nline3\n" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

func TestHandleChangeLineEndings_LFtoCRLF(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	input := ChangeLineEndingsInput{Path: testFile, Style: "crlf"}
	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	if output.OriginalStyle != "lf" {
		t.Errorf("expected originalStyle=lf, got %s", output.OriginalStyle)
	}
	if output.LinesChanged != 3 {
		t.Errorf("expected linesChanged=3, got %d", output.LinesChanged)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "line1\r\nline2\r\nline3\r\n" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

func TestHandleChangeLineEndings_MixedToLF(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// Mix of CRLF and LF
	if err := os.WriteFile(testFile, []byte("line1\r\nline2\nline3\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	input := ChangeLineEndingsInput{Path: testFile, Style: "lf"}
	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	if output.OriginalStyle != "mixed" {
		t.Errorf("expected originalStyle=mixed, got %s", output.OriginalStyle)
	}
	if output.LinesChanged != 2 {
		t.Errorf("expected linesChanged=2 (CRLF lines), got %d", output.LinesChanged)
	}

	data, _ := os.ReadFile(testFile)
	if string(data) != "line1\nline2\nline3\n" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

func TestHandleChangeLineEndings_AlreadyCorrect(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("line1\nline2\n"), 0644); err != nil {
		t.Fatal(err)
	}

	input := ChangeLineEndingsInput{Path: testFile, Style: "lf"}
	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	if output.LinesChanged != 0 {
		t.Errorf("expected linesChanged=0 for no-op, got %d", output.LinesChanged)
	}
	if output.OriginalStyle != "lf" {
		t.Errorf("expected originalStyle=lf, got %s", output.OriginalStyle)
	}
}

func representativeTextForEncoding(t *testing.T, encodingName string) string {
	t.Helper()

	if fileEncoding.IsUTF8(encodingName) {
		return "Text © 中文 Привет"
	}
	registered, ok := fileEncoding.Get(encodingName)
	if !ok {
		t.Fatalf("encoding %q is not registered", encodingName)
	}

	// Keep this helper registry-scalable: choose the first non-ASCII sample the
	// actual encoder can represent instead of maintaining a second charset list.
	candidates := []string{
		"日本語", "한국어", "中文", "Привет", "Привіт", "Ελλάδα", "שלום", "مرحبا", "ไทย",
		"Český", "Āžuolas", "Türkçe", "Viêt Nam", "café €", "café", "\uf780", "Text",
	}
	for _, candidate := range candidates {
		if _, err := registered.NewEncoder().Bytes([]byte(candidate)); err == nil {
			return candidate
		}
	}
	t.Fatalf("encoding %q cannot represent any test sample", encodingName)
	return ""
}

func transformLineEndingFixture(t *testing.T, encodingName string, data []byte, target string) ([]byte, textstream.LineEndingStats) {
	t.Helper()

	validated, err := fileEncoding.NewValidatingReader(bytes.NewReader(data), encodingName)
	if err != nil {
		t.Fatalf("initialize %s validation: %v", encodingName, err)
	}
	profile, ok := fileEncoding.LineEndingProfileFor(encodingName)
	if !ok {
		t.Fatalf("line-ending profile unavailable for %s", encodingName)
	}

	var transformed *textstream.LineEndingReader
	switch profile.Kind {
	case fileEncoding.LineEndingUTF16LE, fileEncoding.LineEndingUTF16BE:
		transformed, err = textstream.NewUTF16LineEndingReader(validated, target, profile.Kind == fileEncoding.LineEndingUTF16LE)
	case fileEncoding.LineEndingUTF32LE, fileEncoding.LineEndingUTF32BE:
		transformed, err = textstream.NewUTF32LineEndingReader(validated, target, profile.Kind == fileEncoding.LineEndingUTF32LE)
	case fileEncoding.LineEndingByte:
		if len(profile.CarriageReturn) != 1 || len(profile.LineFeed) != 1 {
			t.Fatalf("invalid single-byte line-ending profile for %s", encodingName)
		}
		transformed, err = textstream.NewSingleByteLineEndingReader(validated, target, profile.CarriageReturn[0], profile.LineFeed[0])
	case fileEncoding.LineEndingHZ:
		transformed, err = textstream.NewHZLineEndingReader(validated, target)
	default:
		t.Fatalf("unsupported line-ending profile %q for %s", profile.Kind, encodingName)
	}
	if err != nil {
		t.Fatalf("initialize line-ending transformer for %s: %v", encodingName, err)
	}
	output, err := io.ReadAll(transformed)
	if err != nil {
		t.Fatalf("transform line endings for %s: %v", encodingName, err)
	}
	return output, transformed.Stats()
}

func decodeLineEndingFixture(t *testing.T, encodingName string, data []byte) string {
	t.Helper()

	if result, found := fileEncoding.DetectBOM(data); found {
		if result.Charset != canonicalBOMEncoding(encodingName) {
			t.Fatalf("BOM = %s, want %s", result.Charset, canonicalBOMEncoding(encodingName))
		}
		data = data[fileEncoding.BOMSize(result.Charset):]
	}
	if fileEncoding.IsUTF8(encodingName) {
		return string(data)
	}

	enc, ok := fileEncoding.Get(encodingName)
	if !ok {
		t.Fatalf("encoding %q is not registered", encodingName)
	}
	decoded, err := enc.NewDecoder().Bytes(data)
	if err != nil {
		t.Fatalf("failed to decode %s fixture: %v", encodingName, err)
	}
	return string(decoded)
}

func TestLineEndingPipeline_AllSupportedEncodings(t *testing.T) {
	tests := []struct {
		name         string
		input        func(string) string
		target       string
		want         func(string) string
		wantOriginal string
		wantChanged  int
	}{
		{
			name:         "crlf to lf",
			input:        func(s string) string { return s + "\r\n" + s + "\r\n" },
			target:       LineEndingLF,
			want:         func(s string) string { return s + "\n" + s + "\n" },
			wantOriginal: LineEndingCRLF,
			wantChanged:  2,
		},
		{
			name:         "lf to crlf",
			input:        func(s string) string { return s + "\n" + s + "\n" },
			target:       LineEndingCRLF,
			want:         func(s string) string { return s + "\r\n" + s + "\r\n" },
			wantOriginal: LineEndingLF,
			wantChanged:  2,
		},
		{
			name:         "mixed to lf",
			input:        func(s string) string { return s + "\r\n" + s + "\n" + s + "\r\n" },
			target:       LineEndingLF,
			want:         func(s string) string { return s + "\n" + s + "\n" + s + "\n" },
			wantOriginal: LineEndingMixed,
			wantChanged:  2,
		},
		{
			name:         "mixed to crlf",
			input:        func(s string) string { return s + "\r\n" + s + "\n" + s + "\r\n" },
			target:       LineEndingCRLF,
			want:         func(s string) string { return s + "\r\n" + s + "\r\n" + s + "\r\n" },
			wantOriginal: LineEndingMixed,
			wantChanged:  1,
		},
	}

	for _, encodingInfo := range fileEncoding.ListEncodings() {
		encodingInfo := encodingInfo
		t.Run(encodingInfo.Name, func(t *testing.T) {
			representative := representativeTextForEncoding(t, encodingInfo.Name)
			for _, testCase := range tests {
				testCase := testCase
				t.Run(testCase.name, func(t *testing.T) {
					inputText := testCase.input(representative)
					data := encodeLineEndingFixture(t, encodingInfo.Name, inputText, false)
					converted, stats := transformLineEndingFixture(t, encodingInfo.Name, data, testCase.target)
					if got := determineStyle(stats.CRLFCount, stats.LFCount); got != testCase.wantOriginal {
						t.Errorf("OriginalStyle = %q, want %q", got, testCase.wantOriginal)
					}
					linesChanged := stats.LFCount
					if testCase.target == LineEndingLF {
						linesChanged = stats.CRLFCount
					}
					if linesChanged != testCase.wantChanged {
						t.Errorf("LinesChanged = %d, want %d", linesChanged, testCase.wantChanged)
					}
					if got, want := decodeLineEndingFixture(t, encodingInfo.Name, converted), testCase.want(representative); got != want {
						t.Errorf("decoded content = %q, want %q", got, want)
					}
				})
			}
		})
	}
}

func TestLineEndingPipeline_AllSupportedEncodingsNoOpIsByteIdentical(t *testing.T) {
	for _, encodingInfo := range fileEncoding.ListEncodings() {
		encodingInfo := encodingInfo
		t.Run(encodingInfo.Name, func(t *testing.T) {
			representative := representativeTextForEncoding(t, encodingInfo.Name)
			original := encodeLineEndingFixture(t, encodingInfo.Name, representative+"\r\n", false)
			actual, stats := transformLineEndingFixture(t, encodingInfo.Name, original, LineEndingCRLF)
			if stats.CRLFCount != 1 || stats.LFCount != 0 {
				t.Fatalf("no-op stats = %+v, want one CRLF", stats)
			}
			if !bytes.Equal(actual, original) {
				t.Fatal("no-op changed encoded bytes")
			}
		})
	}
}

func TestHandleChangeLineEndings_PreservesNonLineEndingBytes(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	tests := []struct {
		encoding string
		withBOM  bool
	}{
		{encoding: "utf-8"},
		{encoding: "utf-16-le", withBOM: true},
		{encoding: "utf-16-be", withBOM: true},
		{encoding: "windows-1252"},
	}

	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.encoding, func(t *testing.T) {
			representative := representativeTextForEncoding(t, testCase.encoding)
			originalText := representative + "\r\n" + representative + "\n"
			original := encodeLineEndingFixture(t, testCase.encoding, originalText, testCase.withBOM)
			testFile := filepath.Join(tempDir, testCase.encoding+"_bytes.txt")
			if err := os.WriteFile(testFile, original, 0644); err != nil {
				t.Fatal(err)
			}

			result, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path:     testFile,
				Style:    LineEndingLF,
				Encoding: testCase.encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatal("expected success")
			}

			actual, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatal(err)
			}
			expected := encodeLineEndingFixture(t, testCase.encoding, ConvertLineEndings(originalText, LineEndingLF), testCase.withBOM)
			if !bytes.Equal(actual, expected) {
				t.Fatalf("bytes = %x, want %x", actual, expected)
			}
		})
	}
}

func TestHandleChangeLineEndings_NoOpPreservesMTime(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "noop-mtime.txt")
	original := []byte("line1\r\nline2\r\n")
	if err := os.WriteFile(testFile, original, 0644); err != nil {
		t.Fatal(err)
	}
	fixedTime := time.Unix(1_700_000_000, 0)
	if err := os.Chtimes(testFile, fixedTime, fixedTime); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(testFile)
	if err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
		Path:  testFile,
		Style: LineEndingCRLF,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || output.LinesChanged != 0 {
		t.Fatalf("unexpected no-op result: %+v", output)
	}

	after, err := os.Stat(testFile)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatal("no-op changed file bytes")
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("mtime changed from %v to %v", before.ModTime(), after.ModTime())
	}
}

func TestHandleChangeLineEndings_PreservesUnicodeBOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	for _, encodingName := range []string{"utf-8", "utf-16-le", "utf-16-be"} {
		encodingName := encodingName
		t.Run(encodingName, func(t *testing.T) {
			testFile := filepath.Join(tempDir, encodingName+"_bom.txt")
			text := representativeTextForEncoding(t, encodingName) + "\r\n"
			original := encodeLineEndingFixture(t, encodingName, text, true)
			if err := os.WriteFile(testFile, original, 0644); err != nil {
				t.Fatal(err)
			}

			result, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path:  testFile,
				Style: LineEndingLF,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatal("expected success")
			}

			actual, err := os.ReadFile(testFile)
			if err != nil {
				t.Fatal(err)
			}
			resultBOM, found := fileEncoding.DetectBOM(actual)
			if !found || resultBOM.Charset != encodingName {
				t.Fatalf("BOM = %v, found=%v; want %s", resultBOM, found, encodingName)
			}
			if got, want := decodeLineEndingFixture(t, encodingName, actual), representativeTextForEncoding(t, encodingName)+"\n"; got != want {
				t.Errorf("decoded content = %q, want %q", got, want)
			}
		})
	}
}

func TestHandleChangeLineEndings_RejectsUnmappedLegacyBytesWithoutMutation(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "unmapped-cp1252.txt")
	original := []byte{0x81, '\r', '\n', 0x8D, '\r', '\n'}
	if err := os.WriteFile(testFile, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
		Path:     testFile,
		Style:    LineEndingLF,
		Encoding: "windows-1252",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected strict decoding failure")
	}
	if result.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
		t.Fatalf("error code = %v, want %s", result.Meta[ErrorCodeMetaKey], ErrCodeEncoding)
	}

	actual, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatalf("strict decoding failure mutated bytes: got %x want %x", actual, original)
	}
}

func TestHandleChangeLineEndings_BOMEncodingConflictLeavesFileUnchanged(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "conflict.txt")
	original := encodeLineEndingFixture(t, "utf-16-le", "line1\r\nline2", true)
	if err := os.WriteFile(testFile, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
		Path:     testFile,
		Style:    LineEndingLF,
		Encoding: "utf-16-be",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected BOM/encoding conflict error")
	}

	actual, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(original) {
		t.Fatal("conflict changed file bytes")
	}
}

func TestHandleChangeLineEndings_UnsupportedExplicitEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "unsupported.txt")
	if err := os.WriteFile(testFile, []byte("line1\r\nline2"), 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
		Path:     testFile,
		Style:    LineEndingLF,
		Encoding: "not-an-encoding",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected unsupported encoding error")
	}
}

func TestHandleChangeLineEndings_UTF16LEWithBOM_CRLFToLF(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "multilingual.data")
	originalText := "// Copyright © Example\r\nstring message = \"caffè\";\r\n"
	originalData := encodeLineEndingFixture(t, "utf-16-le", originalText, true)
	if err := os.WriteFile(testFile, originalData, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
		Path:     testFile,
		Style:    LineEndingLF,
		Encoding: "utf-16-le",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal("expected success")
	}
	if output.OriginalStyle != LineEndingCRLF {
		t.Errorf("OriginalStyle = %q, want %q", output.OriginalStyle, LineEndingCRLF)
	}
	if output.LinesChanged != 2 {
		t.Errorf("LinesChanged = %d, want 2", output.LinesChanged)
	}

	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xFE {
		t.Fatalf("UTF-16 LE BOM was not preserved: %x", data[:min(len(data), 4)])
	}
	enc, _ := fileEncoding.Get("utf-16-le")
	decoded, err := enc.NewDecoder().Bytes(data[2:])
	if err != nil {
		t.Fatal(err)
	}
	wantText := "// Copyright © Example\nstring message = \"caffè\";\n"
	if string(decoded) != wantText {
		t.Errorf("decoded content = %q, want %q", string(decoded), wantText)
	}
}

func TestHandleChangeLineEndings_InvalidStyle(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	input := ChangeLineEndingsInput{Path: testFile, Style: "mac"}
	result, _, err := h.HandleChangeLineEndings(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Errorf("expected error for invalid style")
	}
}

func TestHandleChangeLineEndings_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("no newlines here"), 0644); err != nil {
		t.Fatal(err)
	}

	input := ChangeLineEndingsInput{Path: testFile, Style: "lf"}
	result, output, err := h.HandleChangeLineEndings(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error")
	}

	if output.LinesChanged != 0 {
		t.Errorf("expected linesChanged=0 for file with no line endings, got %d", output.LinesChanged)
	}
}
