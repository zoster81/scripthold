package handler

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/operation"
)

func markdownPhysicalResult(document textDocument, originalData, sourceUTF8, resultUTF8 []byte) ([]byte, error) {
	if !markdownPhysicalReencodingSupported(document.Charset) {
		return nil, operation.New(
			operation.KindUnsupported,
			fmt.Sprintf("byte-preserving markdown mutation is not proven for encoding %s", document.Charset),
		)
	}
	if !utf8.Valid(sourceUTF8) || !utf8.Valid(resultUTF8) {
		return nil, operation.New(operation.KindEncodingOutput, "markdown mutation requires valid UTF-8 semantic bytes")
	}

	roundTrippedSource, err := encodeTextDocument(document, string(sourceUTF8), bomPreserve)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(originalData, roundTrippedSource) {
		return nil, operation.New(operation.KindConflict, "markdown source bytes no longer match the semantic snapshot")
	}
	if bytes.Equal(sourceUTF8, resultUTF8) {
		return append([]byte(nil), originalData...), nil
	}
	return encodeTextDocument(document, string(resultUTF8), bomPreserve)
}

func markdownPhysicalReencodingSupported(charset string) bool {
	return fileEncoding.IsUTF8(charset) || fileEncoding.IsUTF16(charset) || fileEncoding.IsUTF32(charset)
}
