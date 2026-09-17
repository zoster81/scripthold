package handler

import (
	"bytes"
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

func TestMarkdownPhysicalResultFailsClosedForNonUTF8UntilBytePreservationIsProvable(t *testing.T) {
	document := textDocument{Charset: "windows-1252"}
	_, err := markdownPhysicalResult(document, []byte("# Old\n"), []byte("# Old\n"), []byte("# New\n"))
	if err == nil || operation.KindOf(err) != operation.KindUnsupported {
		t.Fatalf("error=%v, want unsupported", err)
	}
}
