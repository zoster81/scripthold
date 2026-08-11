package handler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestHandleGrepBoundsAggregateOutput(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits:          config.Limits{MaxOutputBytes: 10},
	}))
	path := filepath.Join(tempDir, "large-match.txt")
	if err := os.WriteFile(path, []byte("12345678901\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern:  "123",
		Paths:    []string{path},
		Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected grep output budget error")
	}
	if message := extractTextFromResultRead(result.Content); !strings.Contains(message, "grep output budget") {
		t.Fatalf("unexpected error: %q", message)
	}
}

func TestHandleGrep_SimpleMatch(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	content := "line one\nline two with pattern\nline three"
	os.WriteFile(testFile, []byte(content), 0644)

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "pattern",
		Paths:   []string{testFile},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.TotalMatches != 1 || len(output.Matches) != 1 {
		t.Fatalf("expected 1 match, got total=%d matches=%d", output.TotalMatches, len(output.Matches))
	}
	if output.Matches[0].Line != 2 {
		t.Errorf("expected match on line 2, got %d", output.Matches[0].Line)
	}
}

func TestHandleGrep_CaseInsensitive(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "test.txt")
	content := "Hello World\nHELLO WORLD\nhello world"
	os.WriteFile(testFile, []byte(content), 0644)

	caseSensitive := false
	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern:       "hello",
		Paths:         []string{testFile},
		CaseSensitive: &caseSensitive,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.TotalMatches != 3 {
		t.Errorf("expected 3 matches (case-insensitive), got %d", output.TotalMatches)
	}
}

func TestHandleGrep_WithContext(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "test.txt")
	content := "line 1\nline 2\nMATCH\nline 4\nline 5"
	os.WriteFile(testFile, []byte(content), 0644)

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern:       "MATCH",
		Paths:         []string{testFile},
		ContextBefore: 2,
		ContextAfter:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if len(output.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(output.Matches))
	}

	match := output.Matches[0]
	if len(match.Before) != 2 {
		t.Errorf("expected 2 lines before, got %d", len(match.Before))
	}
	if len(match.After) != 2 {
		t.Errorf("expected 2 lines after, got %d", len(match.After))
	}
}

func TestHandleGrep_DirectorySearch(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Create multiple files
	os.WriteFile(filepath.Join(tempDir, "file1.txt"), []byte("findme here"), 0644)
	os.WriteFile(filepath.Join(tempDir, "file2.txt"), []byte("no match"), 0644)
	os.WriteFile(filepath.Join(tempDir, "file3.txt"), []byte("also findme"), 0644)

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "findme",
		Paths:   []string{tempDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.TotalMatches != 2 {
		t.Errorf("expected 2 matches across files, got %d", output.TotalMatches)
	}
	if output.FilesSearched != 3 {
		t.Errorf("expected 3 files searched, got %d", output.FilesSearched)
	}
	if output.FilesMatched != 2 {
		t.Errorf("expected 2 files matched, got %d", output.FilesMatched)
	}
}

func TestHandleGrep_IncludePattern(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("findme"), 0644)
	os.WriteFile(filepath.Join(tempDir, "file.pas"), []byte("findme"), 0644)

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "findme",
		Paths:   []string{tempDir},
		Include: "*.pas",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.TotalMatches != 1 {
		t.Errorf("expected 1 match (only .pas), got %d", output.TotalMatches)
	}
}

func TestHandleGrep_ExcludePattern(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	os.WriteFile(filepath.Join(tempDir, "file.txt"), []byte("findme"), 0644)
	os.WriteFile(filepath.Join(tempDir, "file.bak"), []byte("findme"), 0644)

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "findme",
		Paths:   []string{tempDir},
		Exclude: "*.bak",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.TotalMatches != 1 {
		t.Errorf("expected 1 match (excluded .bak), got %d", output.TotalMatches)
	}
}

func TestHandleGrep_MaxMatches(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Create file with many matches
	content := ""
	for i := 0; i < 100; i++ {
		content += "match line\n"
	}
	os.WriteFile(filepath.Join(tempDir, "test.txt"), []byte(content), 0644)

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern:    "match",
		Paths:      []string{tempDir},
		MaxMatches: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.TotalMatches != 10 {
		t.Errorf("expected 10 matches (max), got %d", output.TotalMatches)
	}
	if !output.Truncated {
		t.Error("expected truncated to be true")
	}
}

func TestHandleGrep_CP1251Encoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "test.txt")
	// CP1251 bytes for "Привет" (Russian "Hello")
	cp1251Bytes := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	os.WriteFile(testFile, cp1251Bytes, 0644)

	// CP1251 is intentionally ambiguous under conservative phase-7 detection;
	// explicit codec selection remains fully supported. Phase 8 will make
	// auto-detection skips visible in directory/batch grep reporting.
	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern:  "Привет",
		Paths:    []string{testFile},
		Encoding: "cp1251",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.TotalMatches != 1 {
		t.Errorf("expected 1 explicit CP1251 match, got %d", output.TotalMatches)
	}
}

func TestHandleGrep_Phase8ReportsBoundedEncodingSkips(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	goodPath := filepath.Join(tempDir, "00-good.txt")
	if err := os.WriteFile(goodPath, []byte("needle\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 70; index++ {
		path := filepath.Join(tempDir, fmt.Sprintf("%02d-ambiguous.data", index+1))
		if err := os.WriteFile(path, []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}, 0644); err != nil {
			t.Fatal(err)
		}
	}

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern:    "needle",
		Paths:      []string{tempDir},
		MaxMatches: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected partial success, got %v", result.Content)
	}
	if output.TotalMatches != 1 || len(output.Matches) != 1 || !sameExistingTestFile(t, output.Matches[0].Path, goodPath) {
		t.Fatalf("valid grep result was not preserved: %+v", output)
	}
	if output.CoverageComplete {
		t.Fatal("coverageComplete = true with skipped encoding failures")
	}
	if output.FilesScanned != 1 || output.FilesSkipped != 70 {
		t.Fatalf("filesScanned/filesSkipped = %d/%d, want 1/70", output.FilesScanned, output.FilesSkipped)
	}
	if len(output.SkippedFiles) != maxPartialFailureDetails {
		t.Fatalf("retained skipped files = %d, want %d", len(output.SkippedFiles), maxPartialFailureDetails)
	}
	if !output.SkippedFilesTruncated || output.SkippedFilesOmitted != 70-maxPartialFailureDetails {
		t.Fatalf("skip truncation metadata = truncated:%v omitted:%d", output.SkippedFilesTruncated, output.SkippedFilesOmitted)
	}
	for index, skipped := range output.SkippedFiles {
		if skipped.ErrorCode != ErrCodeEncodingAmbiguous || skipped.EncodingErrorCode != EncodingErrorAmbiguous {
			t.Fatalf("skip[%d] = %+v, want ambiguous encoding codes", index, skipped)
		}
		wantBase := fmt.Sprintf("%02d-ambiguous.data", index+1)
		if filepath.Base(skipped.Path) != wantBase {
			t.Fatalf("skip[%d].path = %q, want deterministic %q", index, skipped.Path, wantBase)
		}
	}
}

func TestHandleGrep_Phase8MalformedExplicitEncodingIsVisible(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	goodPath := filepath.Join(tempDir, "a-good.txt")
	badPath := filepath.Join(tempDir, "b-malformed.txt")
	if err := os.WriteFile(goodPath, []byte("needle\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte{0xC3, 0x28}, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "needle", Paths: []string{tempDir}, Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected partial success, got %v", result.Content)
	}
	if output.TotalMatches != 1 || output.FilesSkipped != 1 || len(output.SkippedFiles) != 1 {
		t.Fatalf("unexpected partial grep output: %+v", output)
	}
	skipped := output.SkippedFiles[0]
	if !sameExistingTestFile(t, skipped.Path, badPath) || skipped.ErrorCode != ErrCodeEncoding || skipped.EncodingErrorCode != EncodingErrorMalformed {
		t.Fatalf("malformed skip = %+v", skipped)
	}
}

func TestHandleGrep_Phase8EncodingSkipsVisibleInAllOutputModes(t *testing.T) {
	for _, mode := range []string{"content", "files_with_matches", "count"} {
		t.Run(mode, func(t *testing.T) {
			tempDir := t.TempDir()
			h := NewHandler([]string{tempDir})
			if err := os.WriteFile(filepath.Join(tempDir, "a-good.txt"), []byte("needle\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(tempDir, "b-ambiguous.data"), []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}, 0644); err != nil {
				t.Fatal(err)
			}

			result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
				Pattern: "needle", Paths: []string{tempDir}, OutputMode: mode, MaxMatches: 10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError || output.FilesSkipped != 1 || output.CoverageComplete {
				t.Fatalf("mode %s hid partial coverage: result=%+v output=%+v", mode, result, output)
			}
			if len(output.SkippedFiles) != 1 || output.SkippedFiles[0].ErrorCode != ErrCodeEncodingAmbiguous {
				t.Fatalf("mode %s skipped files = %+v", mode, output.SkippedFiles)
			}
		})
	}
}

func TestHandleGrep_Phase8DiagnosticsYieldToMatchOutputBudget(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir}, WithConfig(&config.Config{
		DefaultEncoding: "utf-8",
		Limits:          config.Limits{MaxOutputBytes: 4096},
	}))
	goodPath := filepath.Join(tempDir, "a-good.txt")
	badPath := filepath.Join(tempDir, "b-ambiguous.data")
	line := "needle" + strings.Repeat("x", 3550) + "\n"
	if err := os.WriteFile(goodPath, []byte(line), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "needle", Paths: []string{tempDir}, Encoding: "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(output.Matches) != 1 || output.FilesSkipped != 1 {
		t.Fatalf("unexpected result=%+v output=%+v", result, output)
	}
	if len(output.SkippedFiles) != 0 || !output.SkippedFilesTruncated || output.SkippedFilesOmitted != 1 {
		t.Fatalf("diagnostics should yield to match budget: %+v", output)
	}
}

func TestHandleGrep_Phase8CancellationIsTerminal(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "cancel.txt")
	if err := os.WriteFile(path, []byte("needle\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, _, err := h.HandleGrep(ctx, nil, GrepInput{Pattern: "needle", Paths: []string{path}, Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancelled grep result = %+v, want %s", result, ErrCodeCancelled)
	}
}

func TestHandleGrep_MaxMatchesMultipleFiles(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Create many files, each with a match
	for i := 0; i < 50; i++ {
		path := filepath.Join(tempDir, fmt.Sprintf("file%03d.txt", i))
		os.WriteFile(path, []byte("match line\n"), 0644)
	}

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern:    "match",
		Paths:      []string{tempDir},
		MaxMatches: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Error("expected success")
	}
	if output.TotalMatches != 5 {
		t.Errorf("expected 5 matches (max), got %d", output.TotalMatches)
	}
	if !output.Truncated {
		t.Error("expected truncated to be true")
	}
}

func TestHandleGrep_InvalidRegex(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	result, _, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "[invalid",
		Paths:   []string{tempDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for invalid regex")
	}
}

func TestHandleGrep_MissingPattern(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	result, _, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Paths: []string{tempDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing pattern")
	}
}

func TestHandleGrep_MissingPaths(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	result, _, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing paths")
	}
}

func TestHandleGrep_SkipsBinaryFiles(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	// Create a PNG-shaped binary payload containing the search text.
	binaryContent := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), []byte("findme")...)
	os.WriteFile(filepath.Join(tempDir, "binary.png"), binaryContent, 0644)

	// Create text file
	os.WriteFile(filepath.Join(tempDir, "text.txt"), []byte("findme"), 0644)

	result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "findme",
		Paths:   []string{tempDir},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	// Should only find in text file, not binary
	if output.TotalMatches != 1 {
		t.Errorf("expected 1 match (skipping binary), got %d", output.TotalMatches)
	}
}

func encodeGrepFixture(t *testing.T, charset, content string, withBOM bool) []byte {
	t.Helper()

	enc, ok := fileEncoding.Get(charset)
	if !ok {
		t.Fatalf("encoding %q is not registered", charset)
	}
	encoded, err := enc.NewEncoder().Bytes([]byte(content))
	if err != nil {
		t.Fatalf("encode %s fixture: %v", charset, err)
	}
	if !withBOM {
		return encoded
	}

	bom := fileEncoding.BOMBytesFor(charset)
	result := make([]byte, 0, len(bom)+len(encoded))
	result = append(result, bom...)
	result = append(result, encoded...)
	return result
}

func TestHandleGrep_UTF16Documents(t *testing.T) {
	tests := []struct {
		name              string
		charset           string
		withBOM           bool
		requestedEncoding string
		pattern           string
		wantLine          string
	}{
		{name: "auto detects UTF-16 LE BOM", charset: "utf-16-le", withBOM: true, pattern: "title = encoding acceptance", wantLine: "title = encoding acceptance"},
		{name: "auto detects UTF-16 BE BOM", charset: "utf-16-be", withBOM: true, pattern: "中文注释", wantLine: "// 中文注释"},
		{name: "auto detects UTF-16 LE without BOM", charset: "utf-16-le", pattern: "Привет", wantLine: "// Привет"},
		{name: "auto detects UTF-16 BE without BOM", charset: "utf-16-be", pattern: "Città", wantLine: "// Città"},
		{name: "explicit UTF-16 LE without BOM", charset: "utf-16-le", requestedEncoding: "utf-16-le", pattern: "Привет", wantLine: "// Привет"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			h := NewHandler([]string{tempDir})
			path := filepath.Join(tempDir, "multilingual.data")
			content := "title = encoding acceptance\r\n// Città\r\n// Привет\r\n// 中文注释\r\n"
			if err := os.WriteFile(path, encodeGrepFixture(t, tt.charset, content, tt.withBOM), 0644); err != nil {
				t.Fatal(err)
			}

			result, output, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: tt.pattern, Paths: []string{path}, Encoding: tt.requestedEncoding})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("expected success, got %v", result.Content)
			}
			if output.TotalMatches != 1 {
				t.Fatalf("total matches = %d, want 1", output.TotalMatches)
			}
			match := output.Matches[0]
			if match.Text != tt.wantLine {
				t.Fatalf("match text = %q, want %q", match.Text, tt.wantLine)
			}
			if strings.HasPrefix(match.Text, "\uFEFF") {
				t.Fatal("transport BOM leaked into the first grep line")
			}
			if match.Encoding != tt.charset {
				t.Fatalf("encoding = %q, want %q", match.Encoding, tt.charset)
			}
		})
	}
}

func TestHandleGrep_UTF16MultilingualComments(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "multilingual.random")
	content := "// Città\r\n// Привет\r\n// 中文注释\r\n"
	if err := os.WriteFile(path, encodeGrepFixture(t, "utf-16-le", content, true), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "Città|Привет|中文注释", Paths: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if output.TotalMatches != 3 {
		t.Fatalf("total matches = %d, want 3", output.TotalMatches)
	}
}

func TestHandleGrep_MaxMatchesUsesDeterministicFileOrder(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	slowPath := filepath.Join(tempDir, "a-slow.txt")
	fastPath := filepath.Join(tempDir, "z-fast.txt")
	if err := os.WriteFile(slowPath, []byte(strings.Repeat("x", 8<<20)+"match-a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fastPath, []byte("match-z\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "match-", Paths: []string{tempDir}, MaxMatches: 1})
	if err != nil {
		t.Fatal(err)
	}
	if output.TotalMatches != 1 || !output.Truncated {
		t.Fatalf("unexpected limited output: %+v", output)
	}
	if !sameExistingTestFile(t, output.Matches[0].Path, slowPath) {
		t.Fatalf("first match path = %q, want deterministic file %q", output.Matches[0].Path, slowPath)
	}
}

func TestHandleGrep_MaxMatchesExactDoesNotTruncate(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	if err := os.WriteFile(filepath.Join(tempDir, "a-match.txt"), []byte("match\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "b-no-match.txt"), []byte("other\n"), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "match", Paths: []string{tempDir}, MaxMatches: 1})
	if err != nil {
		t.Fatal(err)
	}
	if output.TotalMatches != 1 {
		t.Fatalf("total matches = %d, want 1", output.TotalMatches)
	}
	if output.Truncated {
		t.Fatal("truncated = true, want false when no match was omitted")
	}
}

func TestHandleGrep_SkipsSymlinkFileEscape(t *testing.T) {
	allowedDir := t.TempDir()
	outsideDir := t.TempDir()
	target := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(target, []byte("findme outside"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(allowedDir, "linked-secret.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink creation is not supported in this environment: %v", err)
	}

	h := NewHandler([]string{allowedDir})
	_, output, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "findme", Paths: []string{allowedDir}})
	if err != nil {
		t.Fatal(err)
	}
	if output.TotalMatches != 0 {
		t.Fatalf("symlink escape returned %d matches", output.TotalMatches)
	}
}

func TestHandleGrep_SkipsDirectoryLinkEscape(t *testing.T) {
	allowedDir := t.TempDir()
	outsideDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("findme outside"), 0644); err != nil {
		t.Fatal(err)
	}
	createDirectoryLinkForTest(t, outsideDir, filepath.Join(allowedDir, "escape"))

	h := NewHandler([]string{allowedDir})
	_, output, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "findme", Paths: []string{allowedDir}})
	if err != nil {
		t.Fatal(err)
	}
	if output.TotalMatches != 0 || output.FilesSearched != 0 {
		t.Fatalf("directory link escape was searched: %+v", output)
	}
}

func TestSearchSingleFile_StopsAtLimit(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "limited.txt")
	if err := os.WriteFile(path, []byte("match one\nmatch two\nmatch three\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result := h.searchSingleFile(context.Background(), path, regexp.MustCompile("match"), GrepInput{}, 2)
	if result.err != nil {
		t.Fatal(result.err)
	}
	if len(result.matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(result.matches))
	}
	if !result.truncated {
		t.Fatal("truncated = false, want true after finding an additional match")
	}
}

func TestHandleGrep_DoesNotTreatSplitUTF8ProbeRuneAsBinary(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "probe-boundary.txt")
	content := strings.Repeat("a", binaryCheckSize-1) + "🌍\nfindme\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "findme", Paths: []string{path}, Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if output.TotalMatches != 1 {
		t.Fatalf("output = %+v, want one match", output)
	}
}

func TestSearchSingleFile_RejectsOversizedLine(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "oversized.txt")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 16*1024*1024+1)), 0644); err != nil {
		t.Fatal(err)
	}

	result := h.searchSingleFile(context.Background(), path, regexp.MustCompile("x"), GrepInput{Encoding: "utf-8"}, 1)
	if operation.KindOf(result.err) != operation.KindLimit {
		t.Fatalf("error = %v, kind=%v; want limit", result.err, operation.KindOf(result.err))
	}
	if len(result.matches) != 0 {
		t.Fatalf("matches = %d, want 0", len(result.matches))
	}
}

func TestSearchSingleFile_CancelledContext(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "cancel.txt")
	if err := os.WriteFile(path, []byte("match\n"), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result := h.searchSingleFile(ctx, path, regexp.MustCompile("match"), GrepInput{}, 1)
	if !errors.Is(result.err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", result.err)
	}
	if len(result.matches) != 0 {
		t.Fatalf("matches = %d, want 0", len(result.matches))
	}
}
