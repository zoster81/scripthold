package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/encoding"
)

func TestHandleReadMultipleFilesBoundsAggregateContent(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits:          config.Limits{MaxOutputBytes: 10},
	}))
	first := filepath.Join(tempDir, "first.txt")
	second := filepath.Join(tempDir, "second.txt")
	if err := os.WriteFile(first, []byte("123456"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("abcdef"), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{Paths: []string{first, second}, Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if output.SuccessCount != 1 || output.ErrorCount != 1 {
		t.Fatalf("output = %+v, want one success and one limit error", output)
	}
	if output.Results[0].Content != "123456" {
		t.Fatalf("first content = %q", output.Results[0].Content)
	}
	if output.Results[1].Content != "" || !strings.Contains(output.Results[1].Error, "batch output budget") {
		t.Fatalf("second result = %+v", output.Results[1])
	}
	if got := len(output.Results[0].Content) + len(output.Results[1].Content); got > 10 {
		t.Fatalf("aggregate content bytes = %d, want <= 10", got)
	}
}

func TestHandleReadMultipleFiles_Success(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")
	os.WriteFile(file1, []byte("content1"), 0644)
	os.WriteFile(file2, []byte("content2"), 0644)

	input := ReadMultipleFilesInput{Paths: []string{file1, file2}}
	result, output, err := h.HandleReadMultipleFiles(context.Background(), nil, input)

	if err != nil || result.IsError {
		t.Fatal("expected success")
	}
	if output.SuccessCount != 2 || output.ErrorCount != 0 {
		t.Errorf("expected 2 successes, got %d successes, %d errors", output.SuccessCount, output.ErrorCount)
	}
	if output.Results[0].Content != "content1" || output.Results[1].Content != "content2" {
		t.Errorf("unexpected content")
	}
}

func TestHandleReadMultipleFiles_PartialFailure(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	file1 := filepath.Join(tempDir, "exists.txt")
	file2 := filepath.Join(tempDir, "nonexistent.txt")
	os.WriteFile(file1, []byte("content1"), 0644)

	input := ReadMultipleFilesInput{Paths: []string{file1, file2}}
	result, output, _ := h.HandleReadMultipleFiles(context.Background(), nil, input)

	if result.IsError {
		t.Error("expected partial success, not tool error")
	}
	if output.SuccessCount != 1 || output.ErrorCount != 1 {
		t.Errorf("expected 1 success, 1 error, got %d/%d", output.SuccessCount, output.ErrorCount)
	}
}

func TestHandleReadMultipleFiles_Phase8EncodingStatusAndBoundedSummary(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	goodPath := filepath.Join(tempDir, "good.txt")
	ambiguousPath := filepath.Join(tempDir, "ambiguous.data")
	malformedPath := filepath.Join(tempDir, "malformed.txt")
	if err := os.WriteFile(goodPath, []byte("content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ambiguousPath, []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(malformedPath, []byte{0xC3, 0x28}, 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{Paths: []string{goodPath, ambiguousPath}})
	if err != nil {
		t.Fatal(err)
	}
	if output.SuccessCount != 1 || output.ErrorCount != 1 || output.Results[0].Content != "content\n" {
		t.Fatalf("partial batch = %+v", output)
	}
	if output.Results[1].ErrorCode != ErrCodeEncodingAmbiguous || output.Results[1].EncodingErrorCode != EncodingErrorAmbiguous {
		t.Fatalf("ambiguous result = %+v", output.Results[1])
	}

	_, malformed, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{
		Paths: []string{goodPath, malformedPath}, Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if malformed.SuccessCount != 1 || malformed.ErrorCount != 1 || malformed.Results[1].ErrorCode != ErrCodeEncoding || malformed.Results[1].EncodingErrorCode != EncodingErrorMalformed {
		t.Fatalf("malformed batch = %+v", malformed)
	}

	missing := make([]string, 70)
	for index := range missing {
		missing[index] = filepath.Join(tempDir, fmt.Sprintf("missing-%02d.txt", index))
	}
	_, bounded, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{Paths: missing, Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if bounded.ErrorCount != 70 || len(bounded.Results) != 70 {
		t.Fatalf("bounded batch counts = errors:%d results:%d", bounded.ErrorCount, len(bounded.Results))
	}
	if len(bounded.Errors) != maxPartialFailureDetails || !bounded.ErrorsTruncated || bounded.ErrorsOmitted != 70-maxPartialFailureDetails {
		t.Fatalf("bounded summary = len:%d truncated:%v omitted:%d", len(bounded.Errors), bounded.ErrorsTruncated, bounded.ErrorsOmitted)
	}
}

func TestHandleReadMultipleFiles_Phase8ErrorSummaryYieldsToContentBudget(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits:          config.Limits{MaxOutputBytes: 4096},
	}))
	missingPath := filepath.Join(tempDir, "00-missing.txt")
	goodPath := filepath.Join(tempDir, "01-good.txt")
	if err := os.WriteFile(goodPath, []byte(strings.Repeat("x", 4096)), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{
		Paths: []string{missingPath, goodPath}, Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.SuccessCount != 1 || output.ErrorCount != 1 || len(output.Results) != 2 {
		t.Fatalf("unexpected output: %+v", output)
	}
	if len(output.Errors) != 0 || !output.ErrorsTruncated || output.ErrorsOmitted != 1 {
		t.Fatalf("error summary should yield to content budget: %+v", output)
	}
}

func TestHandleReadMultipleFiles_EmptyPaths(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	result, _, _ := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{Paths: []string{}})
	if !result.IsError {
		t.Error("expected error for empty paths")
	}
}

func TestHandleReadMultipleFiles_WithEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	enc, _ := encoding.Get("cp1251")
	cp1251Bytes, _ := enc.NewEncoder().Bytes([]byte("Здравей свят!"))
	file1 := filepath.Join(tempDir, "cyrillic.txt")
	os.WriteFile(file1, cp1251Bytes, 0644)

	input := ReadMultipleFilesInput{Paths: []string{file1}, Encoding: "cp1251"}
	_, output, _ := h.HandleReadMultipleFiles(context.Background(), nil, input)

	if !strings.Contains(output.Results[0].Content, "Здравей свят!") {
		t.Errorf("expected Cyrillic content, got %q", output.Results[0].Content)
	}
}

func TestHandleReadMultipleFiles_PathOutsideAllowed(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	input := ReadMultipleFilesInput{Paths: []string{filepath.Join(tempDir, "..", "..", "etc", "passwd")}}
	_, output, _ := h.HandleReadMultipleFiles(context.Background(), nil, input)

	if !strings.Contains(output.Results[0].Error, "access denied") {
		t.Errorf("expected 'access denied' error, got %q", output.Results[0].Error)
	}
}

func TestHandleReadMultipleFiles_ErrorCodes(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Create one existing file
	existingFile := filepath.Join(tempDir, "exists.txt")
	os.WriteFile(existingFile, []byte("content"), 0644)

	// Non-existent file
	nonExistentFile := filepath.Join(tempDir, "not_found.txt")

	// Path outside allowed
	outsidePath := filepath.Join(tempDir, "..", "..", "etc", "passwd")

	input := ReadMultipleFilesInput{
		Paths: []string{existingFile, nonExistentFile, outsidePath},
	}
	_, output, _ := h.HandleReadMultipleFiles(context.Background(), nil, input)

	// Check success
	if output.Results[0].ErrorCode != "" {
		t.Errorf("expected no error code for success, got %q", output.Results[0].ErrorCode)
	}

	// Check NOT_FOUND error code
	if output.Results[1].ErrorCode != ErrCodeNotFound {
		t.Errorf("expected NOT_FOUND error code, got %q", output.Results[1].ErrorCode)
	}
	if !strings.Contains(output.Results[1].Error, "file not found") {
		t.Errorf("expected 'file not found' message, got %q", output.Results[1].Error)
	}

	// Check ACCESS_DENIED error code
	if output.Results[2].ErrorCode != ErrCodeAccessDenied {
		t.Errorf("expected ACCESS_DENIED error code, got %q", output.Results[2].ErrorCode)
	}
}

func TestHandleReadMultipleFiles_EncodingErrorUsesCentralMapping(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleReadMultipleFiles(context.Background(), nil, ReadMultipleFilesInput{
		Paths:    []string{path},
		Encoding: "not-an-encoding",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := output.Results[0].ErrorCode; got != ErrCodeEncoding {
		t.Fatalf("error code = %q, want %q", got, ErrCodeEncoding)
	}
	if got := output.Results[0].EncodingErrorCode; got != EncodingErrorUnsupported {
		t.Fatalf("encoding error code = %q, want %q", got, EncodingErrorUnsupported)
	}
	if !strings.Contains(output.Results[0].Error, "unsupported encoding") {
		t.Fatalf("error = %q, want unsupported encoding message", output.Results[0].Error)
	}
}

func TestHandleReadMultipleFiles_CancellationUsesCentralMapping(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "file.txt")
	if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, output, err := h.HandleReadMultipleFiles(ctx, nil, ReadMultipleFilesInput{Paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if got := output.Results[0].ErrorCode; got != ErrCodeCancelled {
		t.Fatalf("error code = %q, want %q", got, ErrCodeCancelled)
	}
	if got := output.Results[0].Error; got != "operation cancelled" {
		t.Fatalf("error = %q, want operation cancelled", got)
	}
}

func TestHandleReadMultipleFiles_ErrorsSummary(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Create one existing file
	existingFile := filepath.Join(tempDir, "exists.txt")
	os.WriteFile(existingFile, []byte("content"), 0644)

	// Non-existent files
	nonExistent1 := filepath.Join(tempDir, "missing1.txt")
	nonExistent2 := filepath.Join(tempDir, "missing2.txt")

	input := ReadMultipleFilesInput{
		Paths: []string{existingFile, nonExistent1, nonExistent2},
	}
	_, output, _ := h.HandleReadMultipleFiles(context.Background(), nil, input)

	// Check counts
	if output.SuccessCount != 1 {
		t.Errorf("expected 1 success, got %d", output.SuccessCount)
	}
	if output.ErrorCount != 2 {
		t.Errorf("expected 2 errors, got %d", output.ErrorCount)
	}

	// Check errors summary is populated
	if len(output.Errors) != 2 {
		t.Errorf("expected 2 errors in summary, got %d", len(output.Errors))
	}

	// Check error summary contains file paths
	for _, errMsg := range output.Errors {
		if !strings.Contains(errMsg, "missing") {
			t.Errorf("expected error summary to contain file path, got %q", errMsg)
		}
	}
}
