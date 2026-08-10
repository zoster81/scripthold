package handler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

const encodingAcceptanceFixtureDir = "testdata/encoding_acceptance"

type encodingAcceptanceFixture struct {
	File      string
	Encoding  string
	Style     string
	FirstLine string
	HasBOM    bool
}

func encodingAcceptanceFixtures() []encodingAcceptanceFixture {
	return []encodingAcceptanceFixture{
		{
			File:      "multilingual_utf16le_crlf.data",
			Encoding:  "utf-16-le",
			Style:     LineEndingCRLF,
			FirstLine: "title = \"encoding acceptance\"",
			HasBOM:    true,
		},
		{
			File:      "multilingual_utf16le_crlf",
			Encoding:  "utf-16-le",
			Style:     LineEndingCRLF,
			FirstLine: "BEGIN GENERIC FIXTURE",
			HasBOM:    true,
		},
		{
			File:      "multilingual_utf8_lf.random",
			Encoding:  "utf-8",
			Style:     LineEndingLF,
			FirstLine: "title = \"encoding acceptance\"",
			HasBOM:    false,
		},
	}
}

func copyEncodingAcceptanceFixture(t *testing.T, source, destination string) []byte {
	t.Helper()

	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read fixture %s: %v", source, err)
	}
	if err := os.WriteFile(destination, data, 0644); err != nil {
		t.Fatalf("copy fixture to %s: %v", destination, err)
	}
	return data
}

func assertExpectedBOM(t *testing.T, data []byte, encodingName string, wantBOM bool) {
	t.Helper()

	result, found := fileEncoding.DetectBOM(data)
	if !wantBOM {
		if found {
			t.Fatalf("unexpected %s BOM", result.Charset)
		}
		return
	}
	if !found {
		t.Fatalf("missing %s BOM", encodingName)
	}
	if result.Charset != canonicalBOMEncoding(encodingName) {
		t.Fatalf("BOM = %s, want %s", result.Charset, canonicalBOMEncoding(encodingName))
	}
	payload := data[fileEncoding.BOMSize(result.Charset):]
	if nested, duplicate := fileEncoding.DetectBOM(payload); duplicate {
		t.Fatalf("duplicate transport BOM detected: %s", nested.Charset)
	}
}

func assertEncodingAcceptanceTextCoverage(t *testing.T, content string) {
	t.Helper()

	for _, want := range []string{"città", "Привет", "中文", "🌍"} {
		if !strings.Contains(content, want) {
			t.Fatalf("fixture content is missing %q", want)
		}
	}
}

func TestEncodingFixturesSharedTextDocumentAcceptance(t *testing.T) {
	for _, fixture := range encodingAcceptanceFixtures() {
		fixture := fixture
		t.Run(fixture.File, func(t *testing.T) {
			sourcePath := filepath.Join(encodingAcceptanceFixtureDir, fixture.File)
			original, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			assertExpectedBOM(t, original, fixture.Encoding, fixture.HasBOM)
			expectedText := decodeLineEndingFixture(t, fixture.Encoding, original)
			assertEncodingAcceptanceTextCoverage(t, expectedText)

			tempDir := t.TempDir()
			h := NewHandler([]string{tempDir})
			workingPath := filepath.Join(tempDir, fixture.File)
			copyEncodingAcceptanceFixture(t, sourcePath, workingPath)

			readResult, readOutput, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: workingPath})
			if err != nil {
				t.Fatal(err)
			}
			if readResult.IsError || readOutput.Content != expectedText {
				t.Fatalf("read_text_file failed: result=%v output=%+v", readResult, readOutput)
			}
			if readOutput.HasBOM != fixture.HasBOM {
				t.Fatalf("read hasBOM = %v, want %v", readOutput.HasBOM, fixture.HasBOM)
			}

			detectResult, detectOutput, err := h.HandleDetectLineEndings(context.Background(), nil, DetectLineEndingsInput{Path: workingPath})
			if err != nil {
				t.Fatal(err)
			}
			if detectResult.IsError || detectOutput.Style != fixture.Style {
				t.Fatalf("detect_line_endings style = %q, want %q", detectOutput.Style, fixture.Style)
			}

			grepResult, grepOutput, err := h.HandleGrep(context.Background(), nil, GrepInput{
				Pattern:  "Привет",
				Paths:    []string{workingPath},
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if grepResult.IsError || grepOutput.TotalMatches != 1 {
				t.Fatalf("grep_text_files matches = %d, want 1", grepOutput.TotalMatches)
			}

			editPath := filepath.Join(tempDir, "edit_"+fixture.File)
			copyEncodingAcceptanceFixture(t, sourcePath, editPath)
			editedFirstLine := fixture.FirstLine + " // edited"
			editResult, _, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
				Path:     editPath,
				Encoding: fixture.Encoding,
				Edits: []EditOperation{{
					OldText: fixture.FirstLine,
					NewText: editedFirstLine,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if editResult.IsError {
				t.Fatal("edit_file failed")
			}
			editedBytes, err := os.ReadFile(editPath)
			if err != nil {
				t.Fatal(err)
			}
			assertExpectedBOM(t, editedBytes, fixture.Encoding, fixture.HasBOM)
			editedText := decodeLineEndingFixture(t, fixture.Encoding, editedBytes)
			if !strings.HasPrefix(editedText, editedFirstLine) {
				t.Fatalf("first-line edit was not applied: %q", editedText)
			}
			if got := DetectLineEndings([]byte(editedText)).Style; got != fixture.Style {
				t.Fatalf("edit changed line-ending style to %q, want %q", got, fixture.Style)
			}

			fixedTime := time.Unix(1_700_000_000, 0)
			if err := os.Chtimes(editPath, fixedTime, fixedTime); err != nil {
				t.Fatal(err)
			}
			beforeNoOpEdit, err := os.ReadFile(editPath)
			if err != nil {
				t.Fatal(err)
			}
			beforeNoOpEditInfo, err := os.Stat(editPath)
			if err != nil {
				t.Fatal(err)
			}
			editResult, _, err = h.HandleEditFile(context.Background(), nil, EditFileInput{
				Path:     editPath,
				Encoding: fixture.Encoding,
				Edits: []EditOperation{{
					OldText: editedFirstLine,
					NewText: editedFirstLine,
				}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if editResult.IsError {
				t.Fatal("no-op edit_file failed")
			}
			afterNoOpEdit, err := os.ReadFile(editPath)
			if err != nil {
				t.Fatal(err)
			}
			afterNoOpEditInfo, err := os.Stat(editPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(afterNoOpEdit, beforeNoOpEdit) || !afterNoOpEditInfo.ModTime().Equal(beforeNoOpEditInfo.ModTime()) {
				t.Fatal("no-op edit changed bytes or mtime")
			}

			writePath := filepath.Join(tempDir, "write_"+fixture.File)
			bomPolicy := "never"
			if fixture.HasBOM {
				bomPolicy = "auto"
			}
			writeResult, _, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
				Path:     writePath,
				Content:  expectedText,
				Encoding: fixture.Encoding,
				BOM:      bomPolicy,
			})
			if err != nil {
				t.Fatal(err)
			}
			if writeResult.IsError {
				t.Fatal("write_whole_file failed")
			}
			written, err := os.ReadFile(writePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(written, original) {
				t.Fatal("write_whole_file did not reproduce fixture bytes")
			}

			convertNoOpPath := filepath.Join(tempDir, "convert_noop_"+fixture.File)
			copyEncodingAcceptanceFixture(t, sourcePath, convertNoOpPath)
			if err := os.Chtimes(convertNoOpPath, fixedTime, fixedTime); err != nil {
				t.Fatal(err)
			}
			beforeConvertInfo, err := os.Stat(convertNoOpPath)
			if err != nil {
				t.Fatal(err)
			}
			convertResult, convertOutput, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
				Path: convertNoOpPath,
				From: fixture.Encoding,
				To:   fixture.Encoding,
				BOM:  "preserve",
			})
			if err != nil {
				t.Fatal(err)
			}
			if convertResult.IsError || convertOutput.Changed {
				t.Fatal("byte-identical conversion was not reported as a no-op")
			}
			afterConvertNoOp, err := os.ReadFile(convertNoOpPath)
			if err != nil {
				t.Fatal(err)
			}
			afterConvertInfo, err := os.Stat(convertNoOpPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(afterConvertNoOp, original) || !afterConvertInfo.ModTime().Equal(beforeConvertInfo.ModTime()) {
				t.Fatal("no-op conversion changed bytes or mtime")
			}

			convertPath := filepath.Join(tempDir, "convert_"+fixture.File)
			copyEncodingAcceptanceFixture(t, sourcePath, convertPath)
			targetEncoding := "utf-8"
			targetBOMPolicy := "never"
			wantConvertedBOM := false
			if fixture.Encoding == "utf-8" {
				targetEncoding = "utf-16-le"
				targetBOMPolicy = "auto"
				wantConvertedBOM = true
			}
			convertResult, convertOutput, err = h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
				Path: convertPath,
				From: fixture.Encoding,
				To:   targetEncoding,
				BOM:  targetBOMPolicy,
			})
			if err != nil {
				t.Fatal(err)
			}
			if convertResult.IsError || !convertOutput.Changed || convertOutput.HasBOM != wantConvertedBOM {
				t.Fatalf("convert_encoding failed: %+v", convertOutput)
			}
			convertedReadResult, convertedRead, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
				Path:     convertPath,
				Encoding: targetEncoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if convertedReadResult.IsError || convertedRead.Content != expectedText {
				t.Fatal("convert_encoding changed decoded text")
			}

			lineNoOpPath := filepath.Join(tempDir, "line_noop_"+fixture.File)
			copyEncodingAcceptanceFixture(t, sourcePath, lineNoOpPath)
			if err := os.Chtimes(lineNoOpPath, fixedTime, fixedTime); err != nil {
				t.Fatal(err)
			}
			beforeLineNoOpInfo, err := os.Stat(lineNoOpPath)
			if err != nil {
				t.Fatal(err)
			}
			changeResult, changeOutput, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path:     lineNoOpPath,
				Style:    fixture.Style,
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if changeResult.IsError || changeOutput.LinesChanged != 0 {
				t.Fatal("no-op change_line_endings failed")
			}
			afterLineNoOp, err := os.ReadFile(lineNoOpPath)
			if err != nil {
				t.Fatal(err)
			}
			afterLineNoOpInfo, err := os.Stat(lineNoOpPath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(afterLineNoOp, original) || !afterLineNoOpInfo.ModTime().Equal(beforeLineNoOpInfo.ModTime()) {
				t.Fatal("no-op line-ending conversion changed bytes or mtime")
			}

			lineChangePath := filepath.Join(tempDir, "line_change_"+fixture.File)
			copyEncodingAcceptanceFixture(t, sourcePath, lineChangePath)
			beforeChangeInfo, err := os.Stat(lineChangePath)
			if err != nil {
				t.Fatal(err)
			}
			targetStyle := LineEndingCRLF
			if fixture.Style == LineEndingCRLF {
				targetStyle = LineEndingLF
			}
			changeResult, changeOutput, err = h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path:     lineChangePath,
				Style:    targetStyle,
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if changeResult.IsError || changeOutput.LinesChanged == 0 {
				t.Fatalf("change_line_endings failed: %+v", changeOutput)
			}
			changedBytes, err := os.ReadFile(lineChangePath)
			if err != nil {
				t.Fatal(err)
			}
			expectedChangedBytes := encodeLineEndingFixture(t, fixture.Encoding, ConvertLineEndings(expectedText, targetStyle), fixture.HasBOM)
			if !bytes.Equal(changedBytes, expectedChangedBytes) {
				t.Fatal("line-ending conversion changed non-line-ending bytes")
			}
			afterChangeInfo, err := os.Stat(lineChangePath)
			if err != nil {
				t.Fatal(err)
			}
			if afterChangeInfo.Mode().Perm() != beforeChangeInfo.Mode().Perm() {
				t.Fatalf("permissions changed from %v to %v", beforeChangeInfo.Mode().Perm(), afterChangeInfo.Mode().Perm())
			}

			changeResult, _, err = h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{
				Path:     lineChangePath,
				Style:    fixture.Style,
				Encoding: fixture.Encoding,
			})
			if err != nil {
				t.Fatal(err)
			}
			if changeResult.IsError {
				t.Fatal("reverse change_line_endings failed")
			}
			roundTripped, err := os.ReadFile(lineChangePath)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(roundTripped, original) {
				t.Fatal("line-ending round trip was not byte-identical")
			}
		})
	}
}
