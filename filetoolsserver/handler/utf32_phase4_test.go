package handler

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
)

func TestPhase4UTF32PublicTextPipeline(t *testing.T) {
	const content = "alpha\nbeta\nCittà Привет 中文 🌍\n"
	const editedContent = "alpha\ngamma\nCittà Привет 中文 🌍\n"

	for _, charset := range []string{"utf-32-le", "utf-32-be"} {
		t.Run(charset, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "unicode.data")
			h := NewHandler([]string{root})

			writeResult, writeOutput, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
				Path: path, Content: content, Encoding: charset,
			})
			if err != nil || writeResult.IsError {
				t.Fatalf("write result=%+v output=%+v err=%v", writeResult, writeOutput, err)
			}
			if writeOutput.Encoding != charset || !writeOutput.HasBOM || writeOutput.BOMType != charset || writeOutput.BOMPolicy != "auto" {
				t.Fatalf("write metadata=%+v", writeOutput)
			}
			written, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			bom := fileEncoding.BOMBytesFor(charset)
			if !bytes.HasPrefix(written, bom) || bytes.HasPrefix(written[len(bom):], bom) {
				t.Fatalf("UTF-32 BOM missing or duplicated: %x", written[:min(len(written), len(bom)*2)])
			}
			if got := decodeWrittenText(t, charset, written); got != content {
				t.Fatalf("decoded written text = %q, want %q", got, content)
			}

			readResult, readOutput, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
			if err != nil || readResult.IsError {
				t.Fatalf("read result=%+v output=%+v err=%v", readResult, readOutput, err)
			}
			if readOutput.Content != content || readOutput.DetectedEncoding != charset || !readOutput.HasBOM || readOutput.BOMType != charset {
				t.Fatalf("read output=%+v", readOutput)
			}

			editResult, edited, err := h.HandleEditFile(context.Background(), nil, EditFileInput{
				Path: path, Edits: []EditOperation{{OldText: "beta", NewText: "gamma"}},
			})
			if err != nil || editResult.IsError || !edited.Changed || edited.Encoding != charset || !edited.HasBOM || edited.BOMType != charset {
				t.Fatalf("edit result=%+v output=%+v err=%v", editResult, edited, err)
			}
			readResult, readOutput, err = h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
			if err != nil || readResult.IsError || readOutput.Content != editedContent {
				t.Fatalf("read after edit result=%+v output=%+v err=%v", readResult, readOutput, err)
			}

			grepResult, grepOutput, err := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "Привет", Paths: []string{path}})
			if err != nil || grepResult.IsError || grepOutput.TotalMatches != 1 || len(grepOutput.Matches) != 1 || grepOutput.Matches[0].Encoding != charset {
				t.Fatalf("grep result=%+v output=%+v err=%v", grepResult, grepOutput, err)
			}

			detectResult, detected, err := h.HandleDetectLineEndings(context.Background(), nil, DetectLineEndingsInput{Path: path})
			if err != nil || detectResult.IsError || detected.Style != LineEndingLF {
				t.Fatalf("detect line endings result=%+v output=%+v err=%v", detectResult, detected, err)
			}

			changeResult, changed, err := h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{Path: path, Style: LineEndingCRLF})
			if err != nil || changeResult.IsError || changed.NewStyle != LineEndingCRLF || changed.LinesChanged == 0 {
				t.Fatalf("change line endings result=%+v output=%+v err=%v", changeResult, changed, err)
			}
			wantCRLF := ConvertLineEndings(editedContent, LineEndingCRLF)
			readResult, readOutput, err = h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
			if err != nil || readResult.IsError || readOutput.Content != wantCRLF {
				t.Fatalf("read after line conversion result=%+v output=%+v err=%v", readResult, readOutput, err)
			}

			verifyResult, verification, err := h.HandleVerifyState(context.Background(), nil, VerifyStateInput{Checks: []VerificationCheck{{
				Type: VerifyCheckText,
				Text: &TextVerificationCheck{Path: path, BOM: charset, LineEndings: LineEndingCRLF, TrailingWhitespace: "none"},
			}}})
			if err != nil || verifyResult.IsError || !verification.Passed || verification.PassedCount != 1 {
				t.Fatalf("verify result=%+v output=%+v err=%v", verifyResult, verification, err)
			}
			if got := verification.Results[0]; got.Encoding != charset || !got.HasBOM || got.BOMType != charset || got.LineEndingStyle != LineEndingCRLF {
				t.Fatalf("verify text result=%+v", got)
			}

			convertResult, converted, err := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
				Path: path, To: "utf-8",
			})
			if err != nil || convertResult.IsError || converted.TargetEncoding != "utf-8" || converted.HasBOM {
				t.Fatalf("UTF-32 -> UTF-8 result=%+v output=%+v err=%v", convertResult, converted, err)
			}
			utf8Data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(utf8Data) != wantCRLF {
				t.Fatalf("UTF-8 converted bytes = %q, want %q", utf8Data, wantCRLF)
			}

			convertResult, converted, err = h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
				Path: path, From: "utf-8", To: charset,
			})
			if err != nil || convertResult.IsError || converted.TargetEncoding != charset || !converted.HasBOM || converted.BOMType != charset {
				t.Fatalf("UTF-8 -> UTF-32 result=%+v output=%+v err=%v", convertResult, converted, err)
			}
		})
	}
}

func TestPhase4UTF32BOMlessAutoDetectionAndExplicitRead(t *testing.T) {
	const content = "UTF-32 without BOM still needs strong structural evidence.\nSecond printable line.\n"
	for _, charset := range []string{"utf-32-le", "utf-32-be"} {
		t.Run(charset, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "bomless.data")
			h := NewHandler([]string{root})
			result, output, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
				Path: path, Content: content, Encoding: charset, BOM: "never",
			})
			if err != nil || result.IsError || output.HasBOM {
				t.Fatalf("bomless write result=%+v output=%+v err=%v", result, output, err)
			}

			result, readOutput, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
			if err != nil || result.IsError || readOutput.Content != content || readOutput.DetectedEncoding != charset || readOutput.HasBOM {
				t.Fatalf("bomless auto read result=%+v output=%+v err=%v", result, readOutput, err)
			}

			result, readOutput, err = h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path, Encoding: charset})
			if err != nil || result.IsError || readOutput.Content != content {
				t.Fatalf("bomless explicit read result=%+v output=%+v err=%v", result, readOutput, err)
			}
		})
	}
}

func TestPhase4UTF32MalformedInputFailsClosedWithoutMutation(t *testing.T) {
	for _, testCase := range []struct {
		charset string
		order   binary.ByteOrder
	}{
		{charset: "utf-32-le", order: binary.LittleEndian},
		{charset: "utf-32-be", order: binary.BigEndian},
	} {
		t.Run(testCase.charset, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "malformed.data")
			var illegal [4]byte
			testCase.order.PutUint32(illegal[:], 0x110000)
			original := append(append([]byte(nil), fileEncoding.BOMBytesFor(testCase.charset)...), illegal[:]...)
			if err := os.WriteFile(path, original, 0644); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})

			result, _, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: path})
			if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
				t.Fatalf("malformed read result=%+v err=%v", result, err)
			}
			result, _, err = h.HandleChangeLineEndings(context.Background(), nil, ChangeLineEndingsInput{Path: path, Style: LineEndingCRLF})
			if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
				t.Fatalf("malformed line-ending result=%+v err=%v", result, err)
			}
			actual, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(actual, original) {
				t.Fatalf("malformed operation mutated target: got %x want %x", actual, original)
			}
		})
	}
}

func TestPhase4UTF32GenericAndBOMConflictFailClosed(t *testing.T) {
	root := t.TempDir()
	h := NewHandler([]string{root})
	genericPath := filepath.Join(root, "generic.data")
	result, _, err := h.HandleWriteWholeFile(context.Background(), nil, WriteWholeFileInput{
		Path: genericPath, Content: "text", Encoding: "utf-32",
	})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
		t.Fatalf("generic UTF-32 result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(genericPath); !os.IsNotExist(err) {
		t.Fatalf("generic UTF-32 unexpectedly created target: %v", err)
	}

	conflictPath := filepath.Join(root, "conflict.data")
	original := append(fileEncoding.BOMBytesFor("utf-32-le"), encodePhase3Text(t, "utf-32-le", "text")...)
	if err := os.WriteFile(conflictPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	result, _, err = h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{Path: conflictPath, Encoding: "utf-32-be"})
	if err != nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeEncoding {
		t.Fatalf("UTF-32 BOM conflict result=%+v err=%v", result, err)
	}
}
