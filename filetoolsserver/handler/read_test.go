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
	"github.com/zoster81/scripthold/internal/encoding"
)

// Helper to extract text from MCP content
func extractTextFromResultRead(content []mcp.Content) string {
	for _, c := range content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func TestHandleReadTextFile_UTF8(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")
	content := "Hello, World!"

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := ReadTextFileInput{
		Path:     testFile,
		Encoding: "utf-8",
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	if output.Content != content {
		t.Errorf("expected %q, got %q", content, output.Content)
	}
}

func TestHandleReadTextFile_CP1251(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// CP1251 bytes for "Здравей свят!" (Bulgarian "Hello world!")
	// Encode "Здравей свят!" in CP1251 first
	enc, _ := encoding.Get("cp1251")
	encoder := enc.NewEncoder()
	cp1251Bytes, err := encoder.Bytes([]byte("Здравей свят!"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(testFile, cp1251Bytes, 0644); err != nil {
		t.Fatal(err)
	}

	input := ReadTextFileInput{
		Path:     testFile,
		Encoding: "cp1251",
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if !strings.Contains(output.Content, "Здравей свят!") {
		t.Errorf("expected 'Здравей свят!', got %q", output.Content)
	}
}

func TestHandleReadTextFile_AutoDetectUTF8(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")
	// Use plain ASCII content - chardet will detect as "Ascii" which we map to UTF-8
	content := "Hello, this is plain ASCII content for testing auto-detection."

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// No encoding specified - should auto-detect
	input := ReadTextFileInput{
		Path: testFile,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	// Verify content is correct
	if output.Content != content {
		t.Errorf("expected %q, got %q", content, output.Content)
	}

	// Verify auto-detection info is present
	if output.DetectedEncoding == "" {
		t.Errorf("expected DetectedEncoding to be set when auto-detecting")
	}
}

func TestHandleReadTextFile_AmbiguousCP1251RequiresExplicitEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	cyrillicText := "Здравей свят! Това е тест за автоматично разпознаване на кодирането."
	enc, ok := encoding.Get("cp1251")
	if !ok {
		t.Fatal("cp1251 encoding not found")
	}
	cp1251Bytes, err := enc.NewEncoder().Bytes([]byte(cyrillicText))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testFile, cp1251Bytes, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: testFile})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeEncodingAmbiguous {
		t.Fatalf("result = %+v, want %s", result, ErrCodeEncodingAmbiguous)
	}
	if output.Content != "" || output.DetectedEncoding != "" {
		t.Fatalf("ambiguous read returned decoded content: %+v", output)
	}

	// Explicit selection remains fully supported and lossless.
	result, output, err = h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: testFile, Encoding: "cp1251"})
	if err != nil || result.IsError {
		t.Fatalf("explicit CP1251 read failed: result=%+v err=%v", result, err)
	}
	if output.Content != cyrillicText {
		t.Fatalf("explicit CP1251 content = %q, want %q", output.Content, cyrillicText)
	}
}

func TestHandleReadTextFile_ExplicitEncodingNoDetectionInfo(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")
	content := "Hello, World!"

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Explicitly specify encoding
	input := ReadTextFileInput{
		Path:     testFile,
		Encoding: "utf-8",
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error: %v", result.Content)
	}

	// When encoding is explicitly specified, detection info should NOT be present
	if output.DetectedEncoding != "" {
		t.Errorf("expected DetectedEncoding to be empty when encoding is explicitly specified, got %q", output.DetectedEncoding)
	}

	if output.EncodingConfidence != 0 {
		t.Errorf("expected EncodingConfidence to be 0 when encoding is explicitly specified, got %d", output.EncodingConfidence)
	}
}

func TestHandleReadTextFile_InvalidEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	input := ReadTextFileInput{
		Path:     testFile,
		Encoding: "invalid-encoding",
	}

	result, _, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for invalid encoding")
	}

	text := extractTextFromResultRead(result.Content)
	if !strings.Contains(text, "unsupported encoding") {
		t.Errorf("expected 'unsupported encoding' message, got %q", text)
	}
}

func TestHandleReadTextFile_FileNotFound(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Try to access a file outside allowed directories
	input := ReadTextFileInput{
		Path:     filepath.Join(tempDir, "..", "..", "nonexistent", "file.txt"),
		Encoding: "utf-8",
	}

	result, _, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for file outside allowed directories")
	}

	text := extractTextFromResultRead(result.Content)
	// Path validation happens first, so we get "access denied" not "failed to read file"
	if !strings.Contains(text, "access denied") {
		t.Errorf("expected 'access denied' message, got %q", text)
	}
}

func TestHandleReadTextFile_EmptyPath(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	input := ReadTextFileInput{
		Path: "",
	}

	result, _, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if !result.IsError {
		t.Errorf("expected error for empty path")
	}

	text := extractTextFromResultRead(result.Content)
	if !strings.Contains(text, "path is required") {
		t.Errorf("expected 'path is required' message, got %q", text)
	}
}

func TestHandleReadTextFile_OffsetLimit(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// Create a file with 10 lines
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	offset := 3
	limit := 4
	input := ReadTextFileInput{
		Path:   testFile,
		Offset: &offset,
		Limit:  &limit,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error")
	}

	// Should return lines 3-6
	expected := "line3\nline4\nline5\nline6"
	if output.Content != expected {
		t.Errorf("expected %q, got %q", expected, output.Content)
	}

	if output.TotalLines != 10 {
		t.Errorf("expected TotalLines=10, got %d", output.TotalLines)
	}

	if output.StartLine != 3 {
		t.Errorf("expected StartLine=3, got %d", output.StartLine)
	}

	if output.EndLine != 6 {
		t.Errorf("expected EndLine=6, got %d", output.EndLine)
	}
}

func TestHandleReadTextFile_OffsetOnly(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	offset := 3
	input := ReadTextFileInput{
		Path:   testFile,
		Offset: &offset,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error")
	}

	// Should return lines 3 to end
	expected := "line3\nline4\nline5"
	if output.Content != expected {
		t.Errorf("expected %q, got %q", expected, output.Content)
	}

	if output.StartLine != 3 {
		t.Errorf("expected StartLine=3, got %d", output.StartLine)
	}

	if output.EndLine != 5 {
		t.Errorf("expected EndLine=5, got %d", output.EndLine)
	}
}

func TestHandleReadTextFile_LimitOnly(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	content := "line1\nline2\nline3\nline4\nline5"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	limit := 2
	input := ReadTextFileInput{
		Path:  testFile,
		Limit: &limit,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error")
	}

	// Should return first 2 lines
	expected := "line1\nline2"
	if output.Content != expected {
		t.Errorf("expected %q, got %q", expected, output.Content)
	}

	if output.StartLine != 1 {
		t.Errorf("expected StartLine=1, got %d", output.StartLine)
	}

	if output.EndLine != 2 {
		t.Errorf("expected EndLine=2, got %d", output.EndLine)
	}
}

func TestHandleReadTextFile_FileSizeBytes(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")
	content := "Hello, World!"

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := ReadTextFileInput{Path: testFile}
	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if output.FileSizeBytes != int64(len(content)) {
		t.Errorf("expected FileSizeBytes=%d, got %d", len(content), output.FileSizeBytes)
	}
	if output.Truncated {
		t.Errorf("expected Truncated=false")
	}
}

func TestHandleReadTextFile_MaxCharacters(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// Create a file with known content
	content := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	maxChars := 20
	input := ReadTextFileInput{
		Path:          testFile,
		MaxCharacters: &maxChars,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if !output.Truncated {
		t.Errorf("expected Truncated=true")
	}

	// Content should start with the first 20 chars of original content
	if !strings.HasPrefix(output.Content, content[:maxChars]) {
		t.Errorf("expected content to start with first %d chars, got %q", maxChars, output.Content[:50])
	}

	// Should contain truncation notice
	if !strings.Contains(output.Content, "[TRUNCATED") {
		t.Errorf("expected truncation notice in content")
	}

	// FileSizeBytes should still be the full file size
	if output.FileSizeBytes != int64(len(content)) {
		t.Errorf("expected FileSizeBytes=%d, got %d", len(content), output.FileSizeBytes)
	}

	// TotalLines should reflect the full file
	if output.TotalLines != 10 {
		t.Errorf("expected TotalLines=10, got %d", output.TotalLines)
	}
}

func TestHandleReadTextFile_MaxCharactersNoTruncation(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")
	content := "short"

	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	maxChars := 1000
	input := ReadTextFileInput{
		Path:          testFile,
		MaxCharacters: &maxChars,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if output.Truncated {
		t.Errorf("expected Truncated=false when content fits within maxCharacters")
	}
	if output.Content != content {
		t.Errorf("expected %q, got %q", content, output.Content)
	}
}

func TestHandleReadTextFile_MaxCharactersCyrillic(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// UTF-8 Cyrillic: each char is 2 bytes. 10 runes = 20 bytes.
	content := "Здравейте!" // 10 Cyrillic/punctuation characters
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Truncate at 5 characters (runes), not 5 bytes
	maxChars := 5
	input := ReadTextFileInput{
		Path:          testFile,
		MaxCharacters: &maxChars,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if !output.Truncated {
		t.Errorf("expected Truncated=true")
	}

	// The truncated content should start with exactly 5 Cyrillic runes
	if !strings.HasPrefix(output.Content, "Здрав") {
		t.Errorf("expected content to start with 'Здрав' (5 runes), got %q", output.Content)
	}

	// Should NOT start with only 5 bytes (which would be 2.5 Cyrillic chars = corrupted)
	if strings.HasPrefix(output.Content, string([]byte(content)[:5])) && !strings.HasPrefix(output.Content, "Здрав") {
		t.Errorf("truncation used bytes instead of runes")
	}
}

func TestHandleReadTextFile_MaxCharactersWithOffsetLimit(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	// Create a file with many lines
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 50) // 50 chars per line
	}
	content := strings.Join(lines, "\n")
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	offset := 10
	limit := 20
	maxChars := 100
	input := ReadTextFileInput{
		Path:          testFile,
		Offset:        &offset,
		Limit:         &limit,
		MaxCharacters: &maxChars,
	}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if !output.Truncated {
		t.Errorf("expected Truncated=true when offset/limit content exceeds maxCharacters")
	}

	if !strings.Contains(output.Content, "[TRUNCATED") {
		t.Errorf("expected truncation notice")
	}
}

func TestHandleReadTextFileBoundsDefaultOutput(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits:          config.Limits{MaxOutputBytes: 8},
	}))
	path := filepath.Join(tempDir, "output-limit.txt")
	if err := os.WriteFile(path, []byte("123456789"), 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path, Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected read output limit error")
	}
	if message := extractTextFromResultRead(result.Content); !strings.Contains(message, "8-byte read output limit") {
		t.Fatalf("unexpected error: %q", message)
	}

	maxCharacters := 4
	result, output, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path:          path,
		Encoding:      "utf-8",
		MaxCharacters: &maxCharacters,
	})
	if err != nil || result.IsError {
		t.Fatalf("bounded read failed: result=%v err=%v", result, err)
	}
	if !output.Truncated || !strings.HasPrefix(output.Content, "1234") {
		t.Fatalf("unexpected bounded output: %+v", output)
	}
}

func TestHandleReadTextFile_RejectsOversizedLine(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "oversized-line.txt")
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, 16*1024*1024+1), 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path, Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected explicit line-length limit error")
	}
	if message := extractTextFromResultRead(result.Content); !strings.Contains(message, "line 1 exceeds") {
		t.Fatalf("unexpected error: %q", message)
	}
}

func TestHandleReadTextFile_TotalLinesReturned(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	testFile := filepath.Join(tempDir, "test.txt")

	content := "line1\nline2\nline3"
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	input := ReadTextFileInput{Path: testFile}

	result, output, err := h.HandleReadTextFile(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError {
		t.Errorf("expected success, got error")
	}

	if output.TotalLines != 3 {
		t.Errorf("expected TotalLines=3, got %d", output.TotalLines)
	}

	if output.StartLine != 1 {
		t.Errorf("expected StartLine=1, got %d", output.StartLine)
	}

	if output.EndLine != 3 {
		t.Errorf("expected EndLine=3, got %d", output.EndLine)
	}
}
