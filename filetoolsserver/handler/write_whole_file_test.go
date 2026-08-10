package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

// Helper to extract text from MCP content
func extractTextFromResultWrite(content []mcp.Content) string {
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestHandleWriteWholeFile_UTF8(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "output.txt")
	content := "Hello, World!"

	input := WriteWholeFileInput{
		Path:     testFile,
		Content:  content,
		Encoding: "utf-8",
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	if !strings.Contains(strings.ToLower(output.Message), "success") && !strings.Contains(strings.ToLower(output.Message), "wrote") {
		t.Errorf("expected success message, got %q", output.Message)
	}

	// Verify file content
	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(written) != content {
		t.Errorf("expected %q, got %q", content, string(written))
	}
}

func TestHandleWriteWholeFile_CP1251(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "output.txt")
	content := "Привет" // Russian "Hello"

	input := WriteWholeFileInput{
		Path:     testFile,
		Content:  content,
		Encoding: "cp1251",
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	if !strings.Contains(strings.ToLower(output.Message), "success") && !strings.Contains(strings.ToLower(output.Message), "wrote") {
		t.Errorf("expected success message, got %q", output.Message)
	}

	// Verify CP1251 bytes were written
	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Expected CP1251 bytes for "Привет"
	expectedCP1251 := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	if !bytes.Equal(written, expectedCP1251) {
		t.Errorf("expected CP1251 bytes %v, got %v", expectedCP1251, written)
	}
}

func TestHandleWriteWholeFile_InvalidEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "output.txt")

	input := WriteWholeFileInput{
		Path:     testFile,
		Content:  "test",
		Encoding: "invalid-encoding",
	}

	result, _, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for invalid encoding")
	}

	text := extractTextFromResultWrite(result.Content)
	if !strings.Contains(text, "unsupported encoding") {
		t.Errorf("expected 'unsupported encoding' message, got %q", text)
	}
}

func TestHandleWriteWholeFile_EmptyPath(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	input := WriteWholeFileInput{
		Path:    "",
		Content: "test",
	}

	result, _, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for empty path")
	}

	text := extractTextFromResultWrite(result.Content)
	if !strings.Contains(text, "path is required") {
		t.Errorf("expected 'path is required' message, got %q", text)
	}
}

func TestHandleWriteWholeFile_DefaultEncoding_NewFile(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "new_file.txt")
	content := "Città Привет 中文 🌍"

	// A new file without an explicit encoding must use the modern UTF-8 default.
	input := WriteWholeFileInput{
		Path:    testFile,
		Content: content,
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Fatalf("expected success, got %q", extractTextFromResultWrite(result.Content))
	}
	if output.Encoding != "utf-8" {
		t.Errorf("default encoding = %q, want utf-8", output.Encoding)
	}
	if output.HasBOM || output.BOMType != "" {
		t.Fatalf("default UTF-8 write unexpectedly emitted a BOM: %+v", output)
	}

	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, []byte(content)) {
		t.Errorf("written bytes = % X, want UTF-8 % X", written, []byte(content))
	}
}

func TestHandleWriteWholeFile_ConfiguredLegacyDefault_NewFile(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir}, WithConfig(&config.Config{
		DefaultEncoding: "cp1251",
		Limits:          config.Limits{MaxFileBytes: config.DefaultMaxFileBytes, MaxOutputBytes: config.DefaultMaxOutputBytes},
	}))
	testFile := filepath.Join(tempDir, "legacy-new-file.txt")
	content := "Тест"

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path:    testFile,
		Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %q", extractTextFromResultWrite(result.Content))
	}
	if output.Encoding != "cp1251" {
		t.Errorf("configured encoding = %q, want cp1251", output.Encoding)
	}

	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte{0xD2, 0xE5, 0xF1, 0xF2}
	if !bytes.Equal(written, expected) {
		t.Errorf("written bytes = % X, want CP1251 % X", written, expected)
	}
}

func TestHandleWriteWholeFile_PreservesExistingEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "existing.txt")

	// Create an existing file with UTF-8 content
	utf8Content := "Hello, мир!" // Mixed English and Russian in UTF-8
	if err := os.WriteFile(testFile, []byte(utf8Content), 0644); err != nil {
		t.Fatal(err)
	}

	// Write new content WITHOUT specifying encoding - should preserve UTF-8
	newContent := "Goodbye, мир!"
	input := WriteWholeFileInput{
		Path:    testFile,
		Content: newContent,
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		text := extractTextFromResultWrite(result.Content)
		t.Errorf("expected success, got error: %s", text)
	}

	// Should preserve UTF-8 encoding
	if !strings.Contains(output.Message, "utf-8") && !strings.Contains(output.Message, "UTF-8") {
		t.Errorf("expected preserved UTF-8 encoding in message, got %q", output.Message)
	}

	// Verify UTF-8 bytes were written (not CP1251)
	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(written) != newContent {
		t.Errorf("expected UTF-8 content %q, got %q", newContent, string(written))
	}
}

func TestHandleWriteWholeFile_PreservesExistingCP1251(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "cyrillic.txt")

	// Create an existing file with CP1251 content (Russian text)
	// "Привет мир" in CP1251
	cp1251Content := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2, 0x20, 0xEC, 0xE8, 0xF0}
	if err := os.WriteFile(testFile, cp1251Content, 0644); err != nil {
		t.Fatal(err)
	}

	// Write new content WITHOUT specifying encoding - should preserve CP1251
	newContent := "Пока" // "Bye" in Russian
	input := WriteWholeFileInput{
		Path:    testFile,
		Content: newContent,
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		text := extractTextFromResultWrite(result.Content)
		t.Errorf("expected success, got error: %s", text)
	}

	// Should preserve CP1251 encoding (may be detected as "windows-1251" alias)
	if !strings.Contains(output.Message, "cp1251") && !strings.Contains(output.Message, "windows-1251") {
		t.Errorf("expected preserved CP1251/windows-1251 encoding in message, got %q", output.Message)
	}

	// Verify CP1251 bytes were written
	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Expected CP1251 bytes for "Пока"
	expectedCP1251 := []byte{0xCF, 0xEE, 0xEA, 0xE0}
	if !bytes.Equal(written, expectedCP1251) {
		t.Errorf("expected CP1251 bytes %v, got %v", expectedCP1251, written)
	}
}

func TestHandleWriteWholeFile_ExplicitEncodingOverridesExisting(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "override.txt")

	// Create an existing file with UTF-8 content
	if err := os.WriteFile(testFile, []byte("Hello UTF-8"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write new content WITH explicit CP1251 encoding - should use CP1251, not UTF-8
	newContent := "Тест" // Russian "Test"
	input := WriteWholeFileInput{
		Path:     testFile,
		Content:  newContent,
		Encoding: "cp1251", // Explicit override
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		text := extractTextFromResultWrite(result.Content)
		t.Errorf("expected success, got error: %s", text)
	}

	// Should use explicit CP1251
	if !strings.Contains(output.Message, "cp1251") {
		t.Errorf("expected explicit CP1251 encoding in message, got %q", output.Message)
	}

	// Verify CP1251 bytes were written
	written, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}

	// Expected CP1251 bytes for "Тест"
	expectedCP1251 := []byte{0xD2, 0xE5, 0xF1, 0xF2}
	if !bytes.Equal(written, expectedCP1251) {
		t.Errorf("expected CP1251 bytes %v, got %v", expectedCP1251, written)
	}
}

func decodeWrittenText(t *testing.T, encodingName string, data []byte) string {
	t.Helper()
	if detected, found := fileEncoding.DetectBOM(data); found {
		data = data[fileEncoding.BOMSize(detected.Charset):]
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
		t.Fatalf("decode %s: %v", encodingName, err)
	}
	return string(decoded)
}

func TestHandleWriteWholeFile_UTF16LEAutoAddsSingleBOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "multilingual.data")
	content := "title = encoding acceptance\r\n// Città Привет 中文\r\n"

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: content, Encoding: "utf-16-le",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %q", extractTextFromResultWrite(result.Content))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bom := fileEncoding.BOMBytesFor("utf-16-le")
	if !bytes.HasPrefix(data, bom) {
		t.Fatalf("missing UTF-16 LE BOM: % X", data[:min(len(data), len(bom))])
	}
	if bytes.HasPrefix(data[len(bom):], bom) {
		t.Fatal("UTF-16 LE BOM was duplicated")
	}
	if got := decodeWrittenText(t, "utf-16-le", data); got != content {
		t.Fatalf("decoded content = %q, want %q", got, content)
	}
	if !output.HasBOM || output.BOMType != "utf-16-le" || output.BOMPolicy != "auto" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleWriteWholeFile_BOMNeverWritesBOMlessUTF16(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "bomless.data")

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "value = 1\r\n", Encoding: "utf-16-le", BOM: "never",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %q", extractTextFromResultWrite(result.Content))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := fileEncoding.DetectBOM(data); found {
		t.Fatal("unexpected BOM")
	}
	if output.HasBOM || output.BOMType != "" || output.BOMPolicy != "never" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleWriteWholeFile_BOMPreserveKeepsUTF8BOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "preserve.txt")
	original := append(append([]byte(nil), fileEncoding.BOMBytesFor("utf-8")...), []byte("old")...)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "new", Encoding: "utf-8", BOM: "preserve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %q", extractTextFromResultWrite(result.Content))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, fileEncoding.BOMBytesFor("utf-8")) {
		t.Fatal("UTF-8 BOM was not preserved")
	}
	if !output.HasBOM || output.BOMType != "utf-8" || output.BOMPolicy != "preserve" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleWriteWholeFile_InvalidBOMPolicyDoesNotMutate(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "unchanged.txt")
	original := []byte("original")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "replacement", Encoding: "utf-8", BOM: "sometimes",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected invalid BOM policy error")
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatalf("file mutated: got %q, want %q", actual, original)
	}
}

func TestHandleWriteWholeFile_BOMAlwaysRejectsLegacyEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "legacy.txt")

	result, _, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "Привет", Encoding: "cp1251", BOM: "always",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected BOM/encoding compatibility error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target should not be created, stat error = %v", err)
	}
}

func TestHandleWriteWholeFile_BOMAlwaysAddsUTF8BOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "utf8-bom.txt")

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "hello", Encoding: "utf-8", BOM: "always",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %q", extractTextFromResultWrite(result.Content))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, append(fileEncoding.BOMBytesFor("utf-8"), []byte("hello")...)) {
		t.Fatalf("unexpected bytes: % X", data)
	}
	if !output.HasBOM || output.BOMType != "utf-8" || output.BOMPolicy != "always" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleWriteWholeFile_UnrepresentableContentDoesNotMutate(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "legacy.txt")
	original := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "earth 🌍", Encoding: "cp1251", BOM: "never",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected encoding error")
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatalf("file mutated: got % X, want % X", actual, original)
	}
}

func TestHandleWriteWholeFile_BOMPreserveTreatsEmptyFileAsBOMless(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "empty.txt")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: path, Content: "hello", Encoding: "utf-8", BOM: "preserve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %q", extractTextFromResultWrite(result.Content))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("content = %q, want hello", data)
	}
	if output.HasBOM || output.BOMType != "" || output.BOMPolicy != "preserve" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}
