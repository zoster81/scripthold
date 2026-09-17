package handler

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	fileEncoding "github.com/zoster81/scripthold/internal/encoding"
	"github.com/zoster81/scripthold/internal/operation"
)

func markdownPhysicalResult(document textDocument, originalData, sourceUTF8, resultUTF8 []byte) ([]byte, error) {
	if !fileEncoding.IsUTF8(document.Charset) {
		return nil, operation.New(
			operation.KindUnsupported,
			fmt.Sprintf("byte-preserving markdown mutation is not proven for encoding %s", document.Charset),
		)
	}
	if !utf8.Valid(sourceUTF8) || !utf8.Valid(resultUTF8) {
		return nil, operation.New(operation.KindEncodingOutput, "markdown mutation requires valid UTF-8 semantic bytes")
	}

	payload := originalData
	if document.BOM.HasBOM {
		if len(document.BOM.Bytes) == 0 || len(originalData) < len(document.BOM.Bytes) || !bytes.Equal(originalData[:len(document.BOM.Bytes)], document.BOM.Bytes) {
			return nil, operation.New(operation.KindConflict, "markdown source BOM changed after preview")
		}
		payload = originalData[len(document.BOM.Bytes):]
	} else if _, found := fileEncoding.DetectBOM(originalData); found {
		return nil, operation.New(operation.KindConflict, "markdown source BOM changed after preview")
	}
	if !bytes.Equal(payload, sourceUTF8) {
		return nil, operation.New(operation.KindConflict, "markdown source bytes no longer match the semantic snapshot")
	}

	result := make([]byte, 0, len(document.BOM.Bytes)+len(resultUTF8))
	if document.BOM.HasBOM {
		result = append(result, document.BOM.Bytes...)
	}
	result = append(result, resultUTF8...)
	return result, nil
}
