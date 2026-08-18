package handler

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func TestHandleConvertEncoding_UTF8ToCP1251(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "test.txt")
	// UTF-8 content with Cyrillic
	utf8Content := "Привет мир" // "Hello world" in Russian
	os.WriteFile(testFile, []byte(utf8Content), 0644)

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: testFile,
		From: "utf-8",
		To:   "cp1251",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.SourceEncoding != "utf-8" {
		t.Errorf("expected source encoding utf-8, got %s", output.SourceEncoding)
	}
	if output.TargetEncoding != "windows-1251" {
		t.Errorf("expected canonical target encoding windows-1251, got %s", output.TargetEncoding)
	}

	// Verify file was converted (CP1251 bytes are different from UTF-8)
	converted, _ := os.ReadFile(testFile)
	if string(converted) == utf8Content {
		t.Error("file content should be different after conversion")
	}
}

func TestHandleConvertEncoding_CP1251ToUTF8(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "test.txt")
	// CP1251 bytes for "Привет" (Russian "Hello")
	cp1251Bytes := []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}
	os.WriteFile(testFile, cp1251Bytes, 0644)

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: testFile,
		From: "cp1251",
		To:   "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Errorf("expected success, got error")
	}
	if output.TargetEncoding != "utf-8" {
		t.Errorf("expected target encoding utf-8, got %s", output.TargetEncoding)
	}

	// Verify file is now valid UTF-8
	converted, _ := os.ReadFile(testFile)
	expected := "Привет"
	if string(converted) != expected {
		t.Errorf("expected %q, got %q", expected, string(converted))
	}
}

func TestHandleConvertEncoding_WithBackup(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "test.txt")
	originalContent := []byte("Привет")
	os.WriteFile(testFile, originalContent, 0644)

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path:   testFile,
		From:   "utf-8",
		To:     "cp1251",
		Backup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.BackupPath == "" {
		t.Fatal("expected backup path to be set")
	}

	// Verify backup file exists with original content
	backupContent, err := os.ReadFile(output.BackupPath)
	if err != nil {
		t.Fatalf("backup file should exist: %v", err)
	}
	if string(backupContent) != string(originalContent) {
		t.Fatalf("backup = %q, want original %q", backupContent, originalContent)
	}
}

func TestHandleConvertEncodingBatchEncodingStatusAndBoundedSummary(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	goodPath := filepath.Join(tempDir, "good.txt")
	ambiguousPath := filepath.Join(tempDir, "ambiguous.data")
	malformedPath := filepath.Join(tempDir, "malformed.txt")
	if err := os.WriteFile(goodPath, []byte("plain text\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ambiguousPath, []byte{0xCF, 0xF0, 0xE8, 0xE2, 0xE5, 0xF2}, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(malformedPath, []byte{0xC3, 0x28}, 0644); err != nil {
		t.Fatal(err)
	}

	_, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Paths: []string{goodPath, ambiguousPath}, To: "utf-8", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.SuccessCount != 1 || output.ErrorCount != 1 || len(output.Results) != 2 {
		t.Fatalf("partial conversion = %+v", output)
	}
	if output.Results[1].ErrorCode != ErrCodeEncodingAmbiguous || output.Results[1].EncodingErrorCode != EncodingErrorAmbiguous {
		t.Fatalf("ambiguous conversion = %+v", output.Results[1])
	}

	_, malformed, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Paths: []string{goodPath, malformedPath}, From: "utf-8", To: "utf-8", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if malformed.SuccessCount != 1 || malformed.ErrorCount != 1 || malformed.Results[1].ErrorCode != ErrCodeEncoding || malformed.Results[1].EncodingErrorCode != EncodingErrorMalformed {
		t.Fatalf("malformed conversion = %+v", malformed)
	}

	missing := make([]string, 70)
	for index := range missing {
		missing[index] = filepath.Join(tempDir, fmt.Sprintf("missing-%02d.txt", index))
	}
	_, bounded, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Paths: missing, From: "utf-8", To: "utf-8", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if bounded.ErrorCount != 70 || len(bounded.Results) != 70 {
		t.Fatalf("bounded conversion counts = errors:%d results:%d", bounded.ErrorCount, len(bounded.Results))
	}
	if len(bounded.Errors) != maxPartialFailureDetails || !bounded.ErrorsTruncated || bounded.ErrorsOmitted != 70-maxPartialFailureDetails {
		t.Fatalf("bounded conversion summary = len:%d truncated:%v omitted:%d", len(bounded.Errors), bounded.ErrorsTruncated, bounded.ErrorsOmitted)
	}
}

func TestHandleConvertEncoding_MissingTo(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("content"), 0644)

	result, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: testFile,
		From: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing 'to' parameter")
	}
}

func TestHandleConvertEncoding_OutsideAllowed(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	result, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: "/some/random/file.txt",
		To:   "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for path outside allowed directories")
	}
}

func TestHandleConvertEncoding_GBKRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	const chinese = "你好，世界" // "Hello, world"
	testFile := filepath.Join(tempDir, "zh.txt")
	if err := os.WriteFile(testFile, []byte(chinese), 0644); err != nil {
		t.Fatal(err)
	}

	// UTF-8 -> GBK
	_, out, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: testFile, From: "utf-8", To: "gbk",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.TargetEncoding != "gbk" {
		t.Errorf("target encoding = %q, want gbk", out.TargetEncoding)
	}
	if encoded, _ := os.ReadFile(testFile); string(encoded) == chinese {
		t.Error("file should differ from UTF-8 after GBK encoding")
	}

	// GBK -> UTF-8 round-trips back to the original text
	if _, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: testFile, From: "gbk", To: "utf-8",
	}); err != nil {
		t.Fatal(err)
	}
	if back, _ := os.ReadFile(testFile); string(back) != chinese {
		t.Errorf("round-trip mismatch: got %q, want %q", back, chinese)
	}
}

func TestHandleConvertEncoding_GB2312AliasResolves(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})

	testFile := filepath.Join(tempDir, "zh.txt")
	if err := os.WriteFile(testFile, []byte("编码"), 0644); err != nil {
		t.Fatal(err)
	}

	// gb2312 is an alias for gbk; conversion should succeed.
	result, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: testFile, From: "utf-8", To: "gb2312",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Error("expected gb2312 alias to resolve, got error")
	}
}

func TestHandleConvertEncoding_UTF8ToUTF16LEAutoAddsBOMAndPreservesLineEndings(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "mixed.data")
	content := "alpha\r\nbeta\ngamma\rdelta"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "utf-16-le",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bom := fileEncoding.BOMBytesFor("utf-16-le")
	if !bytes.HasPrefix(data, bom) {
		t.Fatal("auto policy did not add UTF-16 LE BOM")
	}
	if got := decodeWrittenText(t, "utf-16-le", data); got != content {
		t.Fatalf("line endings changed: got %q, want %q", got, content)
	}
	if !output.HasBOM || output.BOMType != "utf-16-le" || output.BOMPolicy != "auto" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleConvertEncoding_UTF16LEToUTF8AutoRemovesTransportBOM(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "multilingual.data")
	content := "// Città Привет 中文\r\n"
	if err := os.WriteFile(path, encodeUTF16LEWithBOM(t, content), 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, To: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := fileEncoding.DetectBOM(data); found {
		t.Fatal("auto policy should write UTF-8 without BOM")
	}
	if string(data) != content {
		t.Fatalf("converted content = %q, want %q", data, content)
	}
	if output.HasBOM || output.BOMType != "" || output.BOMPolicy != "auto" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleConvertEncoding_BOMPreserveMapsPresenceToTargetEncoding(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "preserve.txt")
	original := append(append([]byte(nil), fileEncoding.BOMBytesFor("utf-8")...), []byte("hello")...)
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "utf-16-be", BOM: "preserve",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, fileEncoding.BOMBytesFor("utf-16-be")) {
		t.Fatal("preserve policy did not preserve BOM presence in target encoding")
	}
	if !output.HasBOM || output.BOMType != "utf-16-be" || output.BOMPolicy != "preserve" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleConvertEncoding_BOMNeverWritesBOMlessUTF16(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "bomless.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "utf-16-le", BOM: "never",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := fileEncoding.DetectBOM(data); found {
		t.Fatal("unexpected BOM")
	}
	if output.HasBOM || output.BOMPolicy != "never" {
		t.Fatalf("unexpected output metadata: %+v", output)
	}
}

func TestHandleConvertEncoding_InvalidBOMPolicyDoesNotMutate(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "unchanged.txt")
	original := []byte("original")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "utf-16-le", BOM: "sometimes",
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

func TestHandleConvertEncoding_BOMAlwaysRejectsLegacyTargetWithoutMutation(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "legacy.txt")
	original := []byte("hello")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "cp1251", BOM: "always", Backup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected BOM/encoding compatibility error")
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatalf("file mutated: got %q, want %q", actual, original)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should not be created, stat error = %v", err)
	}
}

func TestHandleConvertEncoding_ByteIdenticalNoOpSkipsWriteAndBackup(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "unchanged.txt")
	original := []byte("alpha\r\nbeta\ngamma\rdelta")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	result, output, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "utf-8", BOM: "auto", Backup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("expected success, got %v", result.Content)
	}
	if output.Changed {
		t.Fatalf("changed = true, want false: %+v", output)
	}
	if output.BackupPath != "" {
		t.Fatalf("backupPath = %q, want empty for no-op", output.BackupPath)
	}
	actual, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, original) {
		t.Fatalf("file changed: got %q, want %q", actual, original)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should not be created, stat error = %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("modification time changed: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

func TestHandleConvertEncoding_UnrepresentableContentDoesNotMutateOrBackup(t *testing.T) {
	tempDir := t.TempDir()
	h := NewHandler([]string{tempDir})
	path := filepath.Join(tempDir, "emoji.txt")
	original := []byte("earth 🌍")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}

	result, _, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Path: path, From: "utf-8", To: "cp1251", BOM: "never", Backup: true,
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
		t.Fatalf("file mutated: got %q, want %q", actual, original)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup should not be created, stat error = %v", err)
	}
}
