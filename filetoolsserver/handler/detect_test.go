package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Helper to extract text from MCP content
func extractTextFromResult(content []mcp.Content) string {
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestHandleDetectEncoding_UTF8(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")
	content := "Hello, World! This is UTF-8 text."

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := DetectEncodingInput{
		Path: testFile,
	}

	result, output, err := h.HandleDetectEncoding(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	if !strings.Contains(strings.ToLower(output.Encoding), "utf-8") && !strings.Contains(strings.ToLower(output.Encoding), "ascii") {
		t.Errorf("expected UTF-8 or ASCII detection, got %q", output.Encoding)
	}
}

func TestHandleDetectEncoding_UTF8WithBOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// UTF-8 BOM + content
	bom := []byte{0xEF, 0xBB, 0xBF}
	content := "Hello with BOM"
	data := append(bom, []byte(content)...)

	if err := os.WriteFile(testFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	input := DetectEncodingInput{
		Path: testFile,
	}

	result, output, err := h.HandleDetectEncoding(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if !strings.Contains(strings.ToLower(output.Encoding), "utf-8") {
		t.Errorf("expected UTF-8 detection, got %q", output.Encoding)
	}
	if !output.HasBOM {
		t.Errorf("expected BOM indicator to be true, got false")
	}
}

func TestHandleDetectEncoding_CP1251(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// CP1251 bytes for "Привет" (Russian "Hello")
	cp1251Bytes := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}

	if err := os.WriteFile(testFile, cp1251Bytes, 0644); err != nil {
		t.Fatal(err)
	}

	input := DetectEncodingInput{
		Path: testFile,
	}

	result, output, err := h.HandleDetectEncoding(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("detect_encoding must report ambiguity as structured output: %v", result.Content)
	}
	if !output.Ambiguous || output.Encoding != "" || output.Assumed {
		t.Fatalf("short CP1251 must remain explicitly ambiguous, got %+v", output)
	}
}

func TestHandleDetectEncoding_FileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Try to access a file outside allowed directories
	input := DetectEncodingInput{
		Path: filepath.Join(tempDir, "..", "..", "nonexistent", "file.txt"),
	}

	result, _, err := h.HandleDetectEncoding(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for file outside allowed directories")
	}

	text := extractTextFromResult(result.Content)
	// Path validation happens first, so we get "access denied" not "failed to read file"
	if !strings.Contains(text, "access denied") {
		t.Errorf("expected 'access denied' message, got %q", text)
	}
}

func TestHandleDetectEncoding_EmptyPath(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	input := DetectEncodingInput{
		Path: "",
	}

	result, _, err := h.HandleDetectEncoding(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for empty path")
	}

	text := extractTextFromResult(result.Content)
	if !strings.Contains(text, "path is required") {
		t.Errorf("expected 'path is required' message, got %q", text)
	}
}

func TestHandleDetectEncoding_ModeChunked(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")
	content := "Hello, World! This is UTF-8 text for chunked mode testing."

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := DetectEncodingInput{
		Path: testFile,
		Mode: "chunked",
	}

	result, output, err := h.HandleDetectEncoding(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	if !strings.Contains(strings.ToLower(output.Encoding), "utf-8") && !strings.Contains(strings.ToLower(output.Encoding), "ascii") {
		t.Errorf("expected UTF-8 or ASCII detection in chunked mode, got %q", output.Encoding)
	}
}

func TestHandleDetectEncoding_EmptyFileAssumesUTF8(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "empty")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleDetectEncoding(context.Background(), nil, DetectEncodingInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.Encoding != "utf-8" || output.Confidence != 0 || !output.Assumed || output.Ambiguous || output.HasBOM {
		t.Fatalf("unexpected empty-file result: %+v", output)
	}
}

func TestHandleDetectEncoding_BinaryInputIsAmbiguous(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "binary.data")
	data := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R'}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleDetectEncoding(context.Background(), nil, DetectEncodingInput{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("detect_encoding must report ambiguity as structured output: %v", result.Content)
	}
	if !output.Ambiguous || output.Assumed {
		t.Fatalf("unexpected binary detection result: %+v", output)
	}
}

func TestHandleDetectEncoding_InvalidMode(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	input := DetectEncodingInput{
		Path: testFile,
		Mode: "invalid",
	}

	result, _, err := h.HandleDetectEncoding(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for invalid mode")
	}

	text := extractTextFromResult(result.Content)
	if !strings.Contains(text, "invalid mode") {
		t.Errorf("expected 'invalid mode' message, got %q", text)
	}
}
