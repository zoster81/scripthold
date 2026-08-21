package filetoolsserver

import (
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/responsecontinuation"
)

const responseEstimateOverheadBytes = 4096

type responseContinuationOutput struct {
	Deferred           bool   `json:"deferred,omitempty"`
	Segmented          bool   `json:"segmented,omitempty"`
	OperationID        string `json:"operationId,omitempty"`
	Status             string `json:"status,omitempty"`
	OperationCompleted bool   `json:"operationCompleted"`
	ResultRetained     bool   `json:"resultRetained"`
	TotalBytes         int    `json:"totalBytes,omitempty"`
	OriginalIsError    bool   `json:"originalIsError,omitempty"`
	ErrorCode          string `json:"errorCode,omitempty"`
	Message            string `json:"message"`
}

type retainedToolResponse struct {
	Meta              mcp.Meta        `json:"_meta,omitempty"`
	Content           json.RawMessage `json:"content,omitempty"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError,omitempty"`
}

func withSafeToolResponse[In any](maxInlineBytes int64, store *responsecontinuation.Store, next mcp.ToolHandlerFor[In, any]) mcp.ToolHandlerFor[In, any] {
	if maxInlineBytes <= 0 || store == nil {
		return next
	}
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		result, output, err := next(ctx, req, input)
		if err != nil {
			return result, output, err
		}
		if result == nil {
			result = &mcp.CallToolResult{}
		}
		structured, marshalErr := json.Marshal(output)
		if marshalErr != nil {
			return compactResponseFailure(result, false, "tool result could not be encoded safely"), responseContinuationOutput{
				OperationCompleted: true,
				ResultRetained:     false,
				OriginalIsError:    result.IsError,
				ErrorCode:          resultErrorCode(result),
				Message:            "The tool operation completed, but its result could not be encoded safely; do not retry automatically.",
			}, nil
		}
		if estimatedToolResponseBytes(result, structured) <= maxInlineBytes {
			return result, output, nil
		}

		content, marshalErr := json.Marshal(result.Content)
		if marshalErr != nil {
			return compactResponseFailure(result, false, "tool result content could not be retained safely"), responseContinuationOutput{
				OperationCompleted: true,
				ResultRetained:     false,
				OriginalIsError:    result.IsError,
				ErrorCode:          resultErrorCode(result),
				Message:            "The tool operation completed, but its oversized result could not be retained safely; do not retry automatically.",
			}, nil
		}
		retained, marshalErr := json.Marshal(retainedToolResponse{
			Meta:              cloneMeta(result.Meta),
			Content:           content,
			StructuredContent: structured,
			IsError:           result.IsError,
		})
		if marshalErr != nil {
			return compactResponseFailure(result, false, "tool result could not be retained safely"), responseContinuationOutput{
				OperationCompleted: true,
				ResultRetained:     false,
				OriginalIsError:    result.IsError,
				ErrorCode:          resultErrorCode(result),
				Message:            "The tool operation completed, but its oversized result could not be retained safely; do not retry automatically.",
			}, nil
		}

		handle, retainErr := store.Retain(retained, responsecontinuation.Metadata{OriginalIsError: result.IsError, ErrorCode: resultErrorCode(result)})
		if retainErr != nil {
			return compactResponseFailure(result, false, "tool result exceeded safe response retention capacity"), responseContinuationOutput{
				OperationCompleted: true,
				ResultRetained:     false,
				OriginalIsError:    result.IsError,
				ErrorCode:          resultErrorCode(result),
				Message:            "The tool operation completed, but its oversized result exceeded response-retention capacity; do not retry automatically.",
			}, nil
		}

		handoff := responseContinuationOutput{
			Deferred:           true,
			Segmented:          true,
			OperationID:        handle.OperationID,
			Status:             handle.Status,
			OperationCompleted: true,
			ResultRetained:     true,
			TotalBytes:         handle.TotalBytes,
			OriginalIsError:    handle.OriginalIsError,
			ErrorCode:          handle.ErrorCode,
			Message:            "The tool operation completed; its oversized result was retained. Retrieve it in bounded segments with deferred_operation.",
		}
		compact := &mcp.CallToolResult{
			Meta:    cloneMeta(result.Meta),
			IsError: result.IsError,
			Content: []mcp.Content{&mcp.TextContent{Text: handoff.Message}},
		}
		if compact.Meta == nil {
			compact.Meta = mcp.Meta{}
		}
		compact.Meta["operationId"] = handle.OperationID
		compact.Meta["resultSegmented"] = true
		return compact, handoff, nil
	}
}

func estimatedToolResponseBytes(result *mcp.CallToolResult, structured []byte) int64 {
	if result == nil {
		result = &mcp.CallToolResult{}
	}
	var contentBytes int
	if len(result.Content) == 0 {
		encodedText, err := json.Marshal(string(structured))
		if err != nil {
			return int64(len(structured) + responseEstimateOverheadBytes)
		}
		contentBytes = len(encodedText)
	} else if encodedContent, err := json.Marshal(result.Content); err == nil {
		contentBytes = len(encodedContent)
	} else {
		contentBytes = len(structured)
	}
	metaBytes := 0
	if encodedMeta, err := json.Marshal(result.Meta); err == nil {
		metaBytes = len(encodedMeta)
	}
	return int64(len(structured) + contentBytes + metaBytes + responseEstimateOverheadBytes)
}

func compactResponseFailure(original *mcp.CallToolResult, preserveOriginalError bool, message string) *mcp.CallToolResult {
	meta := cloneMeta(nil)
	if preserveOriginalError && original != nil {
		meta = cloneMeta(original.Meta)
	}
	if meta == nil {
		meta = mcp.Meta{}
	}
	meta[handler.ErrorCodeMetaKey] = handler.ErrCodeLimit
	meta["operationCompleted"] = true
	meta["resultRetained"] = false
	return &mcp.CallToolResult{
		Meta:    meta,
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}

func resultErrorCode(result *mcp.CallToolResult) string {
	if result == nil || result.Meta == nil {
		return ""
	}
	value, _ := result.Meta[handler.ErrorCodeMetaKey].(string)
	return value
}

func cloneMeta(meta mcp.Meta) mcp.Meta {
	if len(meta) == 0 {
		return nil
	}
	copy := make(mcp.Meta, len(meta))
	for key, value := range meta {
		copy[key] = value
	}
	return copy
}
