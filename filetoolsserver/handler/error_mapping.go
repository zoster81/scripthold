package handler

import (
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/operation"
)

type operationErrorMapping struct {
	Message   string
	BatchCode string
}

// mapOperationError is the single compatibility boundary for converting
// domain failures into public batch messages and machine-readable codes.
func mapOperationError(err error, path string) operationErrorMapping {
	if err == nil {
		return operationErrorMapping{}
	}

	mapping := operationErrorMapping{Message: err.Error(), BatchCode: ErrCodeIO}
	kind := operation.KindOf(err)
	switch kind {
	case operation.KindInvalidPath:
		mapping.BatchCode = ErrCodeInvalidPath
	case operation.KindAccessDenied:
		mapping.BatchCode = ErrCodeAccessDenied
	case operation.KindSymlinkEscape:
		mapping.BatchCode = ErrCodeSymlinkEscape
	case operation.KindNotFound:
		mapping.BatchCode = ErrCodeNotFound
		if path != "" {
			mapping.Message = fmt.Sprintf("file not found: %s", path)
		}
	case operation.KindPermission:
		mapping.BatchCode = ErrCodePermission
		if path != "" {
			mapping.Message = fmt.Sprintf("permission denied: %s", path)
		}
	case operation.KindEncoding, operation.KindDecoding, operation.KindEncodingOutput:
		mapping.BatchCode = ErrCodeEncoding
		if errors.Is(err, ErrEncodingAmbiguous) {
			mapping.BatchCode = ErrCodeEncodingAmbiguous
		}
	case operation.KindInvalidInput:
		mapping.BatchCode = ErrCodeInvalidInput
	case operation.KindConflict:
		mapping.BatchCode = ErrCodeConflict
	case operation.KindPartialCommit:
		mapping.BatchCode = ErrCodePartialCommit
	case operation.KindUnsupported:
		mapping.BatchCode = ErrCodeUnsupported
	case operation.KindCancelled:
		mapping.BatchCode = ErrCodeCancelled
		mapping.Message = "operation cancelled"
	case operation.KindLimit:
		mapping.BatchCode = ErrCodeLimit
	case operation.KindFilesystem, operation.KindUnknown:
		mapping.BatchCode = ErrCodeIO
	}
	return mapping
}

// errorResultFromError converts an operation error to the standard MCP error
// envelope while preserving its public message.
func errorResultFromError(err error) *mcp.CallToolResult {
	mapping := mapOperationError(err, "")
	return errorResultWithCode(mapping.BatchCode, mapping.Message)
}
