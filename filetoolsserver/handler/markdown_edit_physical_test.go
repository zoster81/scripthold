package handler

import (
	"bytes"
	"fmt"
	"testing"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestMarkdownPhysicalResultPreservesUTF8BOMAndMixedLineEndings(t *testing.T) {
	bom := fileEncoding.BOMBytesFor("utf-8")
	sourceUTF8 := []byte("# Old\r\n\nBody\r\n")
	original := append(append([]byte(nil), bom...), sourceUTF8...)
	document := textDocument{
		Charset:     "utf-8",
		BOM:         bomInfo{HasBOM: true, Type: "utf-8", Bytes: append([]byte(nil), bom...)},
		LineEndings: DetectLineEndings(sourceUTF8),
	}
	resultUTF8 := []byte("# New\r\n\nBody\r\n")

	physical, err := markdownPhysicalResult(document, original, sourceUTF8, resultUTF8)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte(nil), bom...), resultUTF8...)
	if !bytes.Equal(physical, want) {
		t.Fatalf("physical=%x want=%x", physical, want)
	}
}

func TestMarkdownPhysicalResultRejectsMismatchedUTF8Snapshot(t *testing.T) {
	document := textDocument{Charset: "utf-8"}
	_, err := markdownPhysicalResult(document, []byte("# Other\n"), []byte("# Old\n"), []byte("# New\n"))
	if err == nil || operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func TestMarkdownPhysicalResultPreservesFixedWidthUnicodeEncodings(t *testing.T) {
	sourceUTF8 := []byte("# Old\r\n\nCittà 🌍\r\n")
	resultUTF8 := []byte("# New\r\n\nCittà 🌍\r\n")
	for _, testCase := range []struct {
		charset string
		bom     bool
	}{
		{charset: "utf-16-le", bom: true},
		{charset: "utf-16-be", bom: true},
		{charset: "utf-32-le", bom: true},
		{charset: "utf-32-be", bom: true},
		{charset: "utf-16-le", bom: false},
	} {
		t.Run(testCase.charset+"-bom-"+fmt.Sprint(testCase.bom), func(t *testing.T) {
			document := textDocument{Charset: testCase.charset}
			if testCase.bom {
				bom := fileEncoding.BOMBytesFor(testCase.charset)
				document.BOM = bomInfo{HasBOM: true, Type: testCase.charset, Bytes: append([]byte(nil), bom...)}
			}
			original, err := encodeTextDocument(document, string(sourceUTF8), bomPreserve)
			if err != nil {
				t.Fatal(err)
			}
			physical, err := markdownPhysicalResult(document, original, sourceUTF8, resultUTF8)
			if err != nil {
				t.Fatal(err)
			}
			want, err := encodeTextDocument(document, string(resultUTF8), bomPreserve)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(physical, want) {
				t.Fatalf("physical=%x want=%x", physical, want)
			}
		})
	}
}

func TestMarkdownPhysicalResultRejectsMismatchedUTF16Snapshot(t *testing.T) {
	bom := fileEncoding.BOMBytesFor("utf-16-le")
	document := textDocument{
		Charset: "utf-16-le",
		BOM:     bomInfo{HasBOM: true, Type: "utf-16-le", Bytes: append([]byte(nil), bom...)},
	}
	original, err := encodeTextDocument(document, "# Other\n", bomPreserve)
	if err != nil {
		t.Fatal(err)
	}
	_, err = markdownPhysicalResult(document, original, []byte("# Old\n"), []byte("# New\n"))
	if err == nil || operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("error=%v, want conflict", err)
	}
}

func TestMarkdownPhysicalResultFailsClosedForUnprovenLegacyEncoding(t *testing.T) {
	document := textDocument{Charset: "windows-1252"}
	_, err := markdownPhysicalResult(document, []byte("# Old\n"), []byte("# Old\n"), []byte("# New\n"))
	if err == nil || operation.KindOf(err) != operation.KindUnsupported {
		t.Fatalf("error=%v, want unsupported", err)
	}
}
