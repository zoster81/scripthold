package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func encodeUTF16WithoutBOM(t *testing.T, charset, content string) []byte {
	t.Helper()

	enc, ok := fileEncoding.Get(charset)
	if !ok {
		t.Fatalf("%s encoding is not registered", charset)
	}
	encoded, err := enc.NewEncoder().Bytes([]byte(content))
	if err != nil {
		t.Fatalf("encode %s fixture: %v", charset, err)
	}
	return encoded
}

func encodeUTF16LEWithBOM(t *testing.T, content string) []byte {
	t.Helper()

	encoded := encodeUTF16WithoutBOM(t, "utf-16-le", content)
	bom := fileEncoding.BOMBytesFor("utf-16-le")
	result := make([]byte, 0, len(bom)+len(encoded))
	result = append(result, bom...)
	result = append(result, encoded...)
	return result
}

func TestHandleReadTextFileAutoDetectsBOMlessUTF16(t *testing.T) {
	content := "title = encoding acceptance\r\nLatin: Città\r\nCyrillic: Привет\r\nCJK: 中文\r\nEmoji: 🌍\r\n"

	for _, charset := range []string{"utf-16-le", "utf-16-be"} {
		t.Run(charset, func(t *testing.T) {
			tempDir := t.TempDir()
			h := NewHandler([]string{tempDir})
			path := filepath.Join(tempDir, "multilingual.random")
			if err := os.WriteFile(path, encodeUTF16WithoutBOM(t, charset, content), 0644); err != nil {
				t.Fatal(err)
			}

			result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("expected success, got %v", result.Content)
			}
			if output.Content != content || output.DetectedEncoding != charset || output.EncodingConfidence < fileEncoding.HighConfidenceThreshold {
				t.Fatalf("unexpected output: %+v", output)
			}
			if output.HasBOM || output.BOMType != "" {
				t.Fatalf("unexpected BOM metadata: %+v", output)
			}
		})
	}
}

func TestHandleReadTextFileStripsUTF16TransportBOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "multilingual.data")
	content := "title = encoding acceptance\r\nstring label = \"Città\";"

	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, content), 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.Content != content {
		t.Fatalf("content = %q, want %q", output.Content, content)
	}
	if strings.HasPrefix(output.Content, "\uFEFF") {
		t.Fatal("transport BOM leaked into read content")
	}
	if !output.HasBOM || output.BOMType != "utf-16-le" {
		t.Fatalf("BOM metadata = hasBOM %v, type %q; want true, utf-16-le", output.HasBOM, output.BOMType)
	}
}

func TestHandleReadTextFilePreservesLeadingTextBOMCodePoint(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "leading-feff.data")
	content := "\uFEFFalpha\r\nbeta"

	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, content), 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path:     path,
		Encoding: "utf-16-le",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.Content != content {
		t.Fatalf("content = %q, want meaningful leading U+FEFF preserved as %q", output.Content, content)
	}
}

func TestHandleReadTextFileStripsUTF8TransportBOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "utf8-bom.txt")
	content := "hello\nworld"
	data := append(append([]byte(nil), fileEncoding.BOMBytesFor("utf-8")...), []byte(content)...)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.Content != content {
		t.Fatalf("content = %q, want %q", output.Content, content)
	}
	if !output.HasBOM || output.BOMType != "utf-8" {
		t.Fatalf("BOM metadata = hasBOM %v, type %q; want true, utf-8", output.HasBOM, output.BOMType)
	}
}

func TestHandleReadTextFilePaginatesDecodedUTF16Content(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "pagination.data")

	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, "line1\r\nline2\r\nline3"), 0644); err != nil {
		t.Fatal(err)
	}

	offset := 2
	limit := 1
	result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path:   path,
		Offset: &offset,
		Limit:  &limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.Content != "line2" || output.TotalLines != 3 || output.StartLine != 2 || output.EndLine != 2 {
		t.Fatalf("unexpected paginated output: %+v", output)
	}
	if !output.HasBOM || output.BOMType != "utf-16-le" {
		t.Fatalf("unexpected BOM metadata: %+v", output)
	}
}

func TestReadSingleAndMultipleUseEquivalentDocumentPipeline(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "shared.data")
	content := "// Привет 🌍\r\nint value = 42;"

	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, content), 0644); err != nil {
		t.Fatal(err)
	}

	singleResult, single, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if singleResult.IsError {
		t.Fatalf("single read failed: %v", singleResult.Content)
	}

	multipleResult, multiple, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{Paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if multipleResult.IsError || multiple.ErrorCount != 0 || len(multiple.Results) != 1 {
		t.Fatalf("multiple read failed: result=%v output=%+v", multipleResult, multiple)
	}
	batch := multiple.Results[0]

	if batch.Content != single.Content {
		t.Fatalf("batch content = %q, single content = %q", batch.Content, single.Content)
	}
	if batch.DetectedEncoding != single.DetectedEncoding || batch.EncodingConfidence != single.EncodingConfidence {
		t.Fatalf("detection metadata differs: batch=%q/%d single=%q/%d", batch.DetectedEncoding, batch.EncodingConfidence, single.DetectedEncoding, single.EncodingConfidence)
	}
	if batch.HasBOM != single.HasBOM || batch.BOMType != single.BOMType {
		t.Fatalf("BOM metadata differs: batch=%v/%q single=%v/%q", batch.HasBOM, batch.BOMType, single.HasBOM, single.BOMType)
	}
}

func TestHandleReadTextFileRejectsBOMEncodingConflict(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "conflict.data")

	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, "alpha"), 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path:     path,
		Encoding: "utf-16-be",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected BOM/encoding conflict error")
	}
	if text := extractTextFromResultRead(result.Content); !strings.Contains(text, "BOM indicates utf-16-le") {
		t.Fatalf("unexpected error: %q", text)
	}
}

func TestReadTextDocumentReportsDecodedLineEndings(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "line-endings.data")

	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, "alpha\r\nbeta\r\n"), 0644); err != nil {
		t.Fatal(err)
	}

	document, _, err := h.readTextDocumentWithData(context.Background(), path, "")
	if err != nil {
		t.Fatal(err)
	}
	if document.Charset != "utf-16-le" {
		t.Fatalf("charset = %q, want utf-16-le", document.Charset)
	}
	if document.LineEndings.Style != LineEndingCRLF || document.LineEndings.CRLFCount != 2 {
		t.Fatalf("line endings = %+v, want CRLF with two endings", document.LineEndings)
	}
}

func TestHandleReadMultipleFilesClassifiesBOMConflictAsEncodingError(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "batch-conflict.data")

	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, "alpha"), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{
		Paths:    []string{path},
		Encoding: "utf-16-be",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(output.Results) != 1 || output.Results[0].ErrorCode != ErrCodeEncoding {
		t.Fatalf("batch result = %+v, want ENCODING error", output.Results)
	}
}
