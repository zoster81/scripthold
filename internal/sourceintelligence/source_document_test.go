package sourceintelligence

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sourceencoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/textstream"
)

func TestOpenSourceDocumentEncodingAndCoordinates(t *testing.T) {
	text := "alpha café\r\nbeta résumé\n"
	tests := []struct {
		name     string
		encoding string
		bom      bool
	}{
		{name: "utf8", encoding: "utf-8"},
		{name: "utf8bom", encoding: "utf-8", bom: true},
		{name: "utf16le", encoding: "utf-16le", bom: true},
		{name: "utf16be", encoding: "utf-16be", bom: true},
		{name: "utf32le", encoding: "utf-32le", bom: true},
		{name: "utf32be", encoding: "utf-32be", bom: true},
		{name: "windows1252", encoding: "windows-1252"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "sample.txt")
			raw := encodeSourceFixture(t, testCase.encoding, text, testCase.bom)
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}

			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
				RequestedEncoding:    testCase.encoding,
				MaxFileBytes:         1024 * 1024,
				MaxDecodedCharacters: 4096,
			})
			if err != nil {
				t.Fatal(err)
			}
			if document.Text != text {
				t.Fatalf("decoded text = %q, want %q", document.Text, text)
			}
			canonical, ok := sourceencoding.CanonicalName(testCase.encoding)
			if !ok {
				t.Fatalf("test encoding %s is not registered", testCase.encoding)
			}
			if document.Encoding != canonical || document.AutoDetected {
				t.Fatalf("encoding metadata = %+v, want explicit %s", document, canonical)
			}
			if document.FileSizeBytes != int64(len(raw)) {
				t.Fatalf("file size = %d, want %d", document.FileSizeBytes, len(raw))
			}
			if document.SourceFingerprint != filesystem.FingerprintRegularFileData(raw) {
				t.Fatalf("fingerprint = %q, want content-v1 fingerprint", document.SourceFingerprint)
			}
			if document.BOM.HasBOM != testCase.bom {
				t.Fatalf("BOM = %+v, want present=%v", document.BOM, testCase.bom)
			}

			accentOffset := strings.Index(text, "é")
			position, err := document.PositionAtUTF8Offset(accentOffset)
			if err != nil {
				t.Fatal(err)
			}
			if position != (Position{Line: 1, Column: 10}) {
				t.Fatalf("accent position = %+v, want 1:10", position)
			}
			afterAccent, err := document.PositionAtUTF8Offset(accentOffset + len("é"))
			if err != nil {
				t.Fatal(err)
			}
			if afterAccent != (Position{Line: 1, Column: 11}) {
				t.Fatalf("after accent = %+v, want 1:11", afterAccent)
			}

			resumeOffset := strings.Index(text, "résumé")
			rangeValue, err := document.RangeFromUTF8Offsets(resumeOffset, resumeOffset+len("résumé"))
			if err != nil {
				t.Fatal(err)
			}
			wantRange := Range{Start: Position{Line: 2, Column: 6}, End: Position{Line: 2, Column: 12}}
			if rangeValue != wantRange {
				t.Fatalf("résumé range = %+v, want %+v", rangeValue, wantRange)
			}
			if _, err := document.PositionAtUTF8Offset(accentOffset + 1); err == nil {
				t.Fatal("accepted UTF-8 offset inside a multibyte scalar")
			}
		})
	}
}

func TestOpenSourceDocumentAutoDetectsUTF32BOM(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	text := "package main\nfunc main() {}\n"
	raw := encodeSourceFixture(t, "utf-32le", text, true)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
		MaxFileBytes:         1024 * 1024,
		MaxDecodedCharacters: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	canonical, ok := sourceencoding.CanonicalName("utf-32le")
	if !ok {
		t.Fatal("utf-32le alias is not registered")
	}
	if !document.AutoDetected || document.Encoding != canonical || document.DetectedEncoding != canonical {
		t.Fatalf("auto-detection metadata = %+v, want canonical %s", document, canonical)
	}
	if !document.BOM.HasBOM || document.BOM.Type != canonical {
		t.Fatalf("BOM metadata = %+v, want canonical %s", document.BOM, canonical)
	}
}

func TestOpenSourceDocumentRequiresExplicitEncodingWhenAmbiguous(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	text := "Здравей свят! Това е тест за автоматично разпознаване на кодирането."
	raw := encodeSourceFixture(t, "cp1251", text, false)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
		MaxFileBytes:         1024 * 1024,
		MaxDecodedCharacters: 4096,
	})
	if !errors.Is(err, textstream.ErrEncodingAmbiguous) {
		t.Fatalf("ambiguous error = %v, want %v", err, textstream.ErrEncodingAmbiguous)
	}

	document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
		RequestedEncoding:    "cp1251",
		MaxFileBytes:         1024 * 1024,
		MaxDecodedCharacters: 4096,
	})
	if err != nil || document.Text != text {
		t.Fatalf("explicit cp1251 document=%+v err=%v", document, err)
	}
}

func TestOpenSourceDocumentRejectsBOMConflictMalformedAndLimits(t *testing.T) {
	t.Run("BOM conflict", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "conflict.txt")
		if err := os.WriteFile(path, encodeSourceFixture(t, "utf-16le", "hello", true), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
			RequestedEncoding:    "utf-8",
			MaxFileBytes:         1024,
			MaxDecodedCharacters: 1024,
		})
		if !errors.Is(err, textstream.ErrBOMEncodingConflict) {
			t.Fatalf("BOM conflict error = %v", err)
		}
	})

	t.Run("malformed UTF-8", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "malformed.txt")
		if err := os.WriteFile(path, []byte{0xc3, 0x28}, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
			RequestedEncoding:    "utf-8",
			MaxFileBytes:         1024,
			MaxDecodedCharacters: 1024,
		})
		if operation.KindOf(err) != operation.KindDecoding {
			t.Fatalf("malformed error = %v, kind=%v", err, operation.KindOf(err))
		}
	})

	t.Run("raw file bytes", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "large.txt")
		if err := os.WriteFile(path, []byte("123456"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
			RequestedEncoding:    "utf-8",
			MaxFileBytes:         5,
			MaxDecodedCharacters: 1024,
		})
		if operation.KindOf(err) != operation.KindLimit {
			t.Fatalf("file limit error = %v, kind=%v", err, operation.KindOf(err))
		}
	})

	t.Run("decoded characters", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "characters.txt")
		if err := os.WriteFile(path, []byte("abcd"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
			RequestedEncoding:    "utf-8",
			MaxFileBytes:         1024,
			MaxDecodedCharacters: 3,
		})
		if operation.KindOf(err) != operation.KindLimit {
			t.Fatalf("character limit error = %v, kind=%v", err, operation.KindOf(err))
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "cancelled.txt")
		if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := OpenSourceDocument(ctx, path, OpenDocumentOptions{
			RequestedEncoding:    "utf-8",
			MaxFileBytes:         1024,
			MaxDecodedCharacters: 1024,
		})
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("cancel error = %v, kind=%v", err, operation.KindOf(err))
		}
	})
}

func TestSourceDocumentMixedLineEndingsUnicodeAndBoundedSlice(t *testing.T) {
	identifier := "café变量"
	text := "α\u0301 " + identifier + "\rbravo\r\ncharlie\ndelta"
	path := filepath.Join(t.TempDir(), "mixed.txt")
	if err := os.WriteFile(path, encodeSourceFixture(t, "utf-8", text, true), 0o600); err != nil {
		t.Fatal(err)
	}

	document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
		RequestedEncoding:    "utf-8",
		MaxFileBytes:         1024 * 1024,
		MaxDecodedCharacters: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !document.BOM.HasBOM {
		t.Fatal("UTF-8 BOM was not preserved as evidence")
	}

	for label, expectation := range map[string]Position{
		"bravo":   {Line: 2, Column: 1},
		"charlie": {Line: 3, Column: 1},
		"delta":   {Line: 4, Column: 1},
	} {
		offset := strings.Index(text, label)
		position, err := document.PositionAtUTF8Offset(offset)
		if err != nil {
			t.Fatal(err)
		}
		if position != expectation {
			t.Fatalf("%s position = %+v, want %+v", label, position, expectation)
		}
	}

	combiningOffset := strings.Index(text, "\u0301")
	combiningPosition, err := document.PositionAtUTF8Offset(combiningOffset)
	if err != nil {
		t.Fatal(err)
	}
	if combiningPosition != (Position{Line: 1, Column: 2}) {
		t.Fatalf("combining-mark position = %+v, want 1:2", combiningPosition)
	}

	start := strings.Index(text, identifier)
	end := start + len(identifier)
	slice, sliceRange, err := document.SliceUTF8Offsets(start, end, len(identifier))
	if err != nil {
		t.Fatal(err)
	}
	if slice != identifier {
		t.Fatalf("bounded slice = %q, want %q", slice, identifier)
	}
	if sliceRange.Start.Line != 1 || sliceRange.End.Line != 1 {
		t.Fatalf("identifier range crosses lines: %+v", sliceRange)
	}
	if _, _, err := document.SliceUTF8Offsets(start, end, len(identifier)-1); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("oversized slice error = %v, kind=%v", err, operation.KindOf(err))
	}
	midScalar := strings.Index(text, "é") + 1
	if _, _, err := document.SliceUTF8Offsets(midScalar, end, len(identifier)); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("mid-scalar slice error = %v, kind=%v", err, operation.KindOf(err))
	}
}

func TestSourceDocumentVeryLongLineRangeBoundary(t *testing.T) {
	const longRunes = 128 * 1024
	text := strings.Repeat("x", longRunes) + "\nend"
	path := filepath.Join(t.TempDir(), "long.txt")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}

	document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
		RequestedEncoding:    "utf-8",
		MaxFileBytes:         int64(len(text) + 1),
		MaxDecodedCharacters: len(text) + 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	endOfLine, err := document.PositionAtUTF8Offset(longRunes)
	if err != nil {
		t.Fatal(err)
	}
	if endOfLine != (Position{Line: 1, Column: longRunes + 1}) {
		t.Fatalf("long-line boundary = %+v", endOfLine)
	}
	endOfDocument, err := document.PositionAtUTF8Offset(len(text))
	if err != nil {
		t.Fatal(err)
	}
	if endOfDocument != (Position{Line: 2, Column: 4}) {
		t.Fatalf("document end = %+v, want 2:4", endOfDocument)
	}
}

func encodeSourceFixture(t *testing.T, encodingName, text string, withBOM bool) []byte {
	t.Helper()
	canonical, ok := sourceencoding.CanonicalName(encodingName)
	if !ok {
		t.Fatalf("encoding %s is not registered", encodingName)
	}
	var encoded []byte
	if sourceencoding.IsUTF8(canonical) {
		encoded = []byte(text)
	} else {
		registered, ok := sourceencoding.Get(canonical)
		if !ok {
			t.Fatalf("encoding %s has no registered encoder", canonical)
		}
		var err error
		encoded, err = registered.NewEncoder().Bytes([]byte(text))
		if err != nil {
			t.Fatalf("encode %s fixture: %v", canonical, err)
		}
	}
	if !withBOM {
		return encoded
	}
	bom := sourceencoding.BOMBytesFor(canonical)
	if len(bom) == 0 {
		t.Fatalf("encoding %s has no BOM", canonical)
	}
	result := make([]byte, 0, len(bom)+len(encoded))
	result = append(result, bom...)
	result = append(result, encoded...)
	return result
}
