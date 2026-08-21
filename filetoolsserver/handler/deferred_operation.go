package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/deferredoperation"
	"github.com/zoster81/scripthold/internal/responsecontinuation"
)

type DeferredOperationInput struct {
	Operation   string `json:"operation"`
	OperationID string `json:"operationId"`
	Offset      int    `json:"offset,omitempty"`
	LimitBytes  int    `json:"limitBytes,omitempty"`
}

type DeferredOperationOutput struct {
	OperationID     string `json:"operationId"`
	Status          string `json:"status"`
	Started         bool   `json:"started,omitempty"`
	Exposed         bool   `json:"exposed,omitempty"`
	ResultAvailable bool   `json:"resultAvailable,omitempty"`
	OriginalIsError bool   `json:"originalIsError,omitempty"`
	ErrorCode       string `json:"errorCode,omitempty"`
	TotalBytes      int64  `json:"totalBytes"`
	Offset          int64  `json:"offset"`
	NextOffset      int64  `json:"nextOffset"`
	Data            string `json:"data"`
	Complete        bool   `json:"complete"`
	PollAfterMS     int    `json:"pollAfterMs,omitempty"`
	Message         string `json:"message,omitempty"`
}

func (h *Handler) HandleDeferredOperation(ctx context.Context, _ *mcp.CallToolRequest, input DeferredOperationInput) (*mcp.CallToolResult, DeferredOperationOutput, error) {
	if err := ctx.Err(); err != nil {
		return errorResultWithCode(ErrCodeCancelled, "operation cancelled"), DeferredOperationOutput{}, nil
	}
	operation := strings.TrimSpace(input.Operation)
	if operation != "get" && operation != "cancel" {
		return errorResultWithCode(ErrCodeInvalidInput, "operation must be get or cancel"), DeferredOperationOutput{}, nil
	}
	operationID := strings.TrimSpace(input.OperationID)
	if operationID == "" || input.Offset < 0 || input.LimitBytes < 0 {
		return errorResultWithCode(ErrCodeInvalidInput, "deferred operation request is invalid"), DeferredOperationOutput{}, nil
	}

	if h != nil && h.deferredOperations != nil {
		output, found, result := h.handleDurableDeferredOperation(ctx, operation, operationID, input.Offset, input.LimitBytes)
		if found {
			return result, output, nil
		}
	}
	if operation == "cancel" {
		return errorResultWithCode(ErrCodeNotFound, "deferred operation was not found or is no longer retained"), DeferredOperationOutput{}, nil
	}
	if h == nil || h.responseContinuations == nil {
		return errorResultWithCode(ErrCodeOperationFailed, "deferred operation store is unavailable"), DeferredOperationOutput{}, nil
	}
	chunk, err := h.responseContinuations.Get(operationID, input.Offset, input.LimitBytes)
	if err != nil {
		switch {
		case errors.Is(err, responsecontinuation.ErrNotFound):
			return errorResultWithCode(ErrCodeNotFound, "deferred operation was not found or is no longer retained"), DeferredOperationOutput{}, nil
		case errors.Is(err, responsecontinuation.ErrInvalidInput):
			return errorResultWithCode(ErrCodeInvalidInput, "deferred operation request is invalid"), DeferredOperationOutput{}, nil
		default:
			return errorResultWithCode(ErrCodeOperationFailed, "deferred operation could not be read"), DeferredOperationOutput{}, nil
		}
	}
	return &mcp.CallToolResult{}, DeferredOperationOutput{
		OperationID:     chunk.OperationID,
		Status:          chunk.Status,
		ResultAvailable: true,
		OriginalIsError: chunk.OriginalIsError,
		ErrorCode:       chunk.ErrorCode,
		TotalBytes:      int64(chunk.TotalBytes),
		Offset:          int64(chunk.Offset),
		NextOffset:      int64(chunk.NextOffset),
		Data:            chunk.Data,
		Complete:        chunk.Complete,
	}, nil
}

func (h *Handler) handleDurableDeferredOperation(ctx context.Context, action, operationID string, offset, limitBytes int) (DeferredOperationOutput, bool, *mcp.CallToolResult) {
	var (
		operation deferredoperation.Operation
		err       error
	)
	if action == "cancel" {
		operation, err = h.deferredOperations.Cancel(ctx, operationID, h.ResolvedAllowedDirs())
	} else {
		operation, err = h.deferredOperations.Get(operationID, h.ResolvedAllowedDirs())
	}
	if err != nil {
		switch {
		case errors.Is(err, deferredoperation.ErrNotFound):
			return DeferredOperationOutput{}, false, nil
		case errors.Is(err, deferredoperation.ErrInvalidInput):
			return DeferredOperationOutput{}, true, errorResultWithCode(ErrCodeInvalidInput, "deferred operation request is invalid")
		case errors.Is(err, deferredoperation.ErrAccessDenied):
			return DeferredOperationOutput{}, true, errorResultWithCode(ErrCodeAccessDenied, "deferred operation is no longer authorized by the current filesystem roots")
		default:
			return DeferredOperationOutput{}, true, errorResultWithCode(ErrCodeOperationFailed, "deferred operation could not be read")
		}
	}

	output := DeferredOperationOutput{
		OperationID:     operation.OperationID,
		Status:          string(operation.Status),
		Started:         operation.Started,
		Exposed:         operation.Exposed,
		ResultAvailable: operation.ResultAvailable,
		OriginalIsError: operation.OriginalIsError,
		ErrorCode:       operation.ErrorCode,
		TotalBytes:      operation.TotalBytes,
		Complete:        operation.Status.Terminal(),
		Message:         operation.Message,
	}
	if !operation.Status.Terminal() {
		output.Complete = false
		output.PollAfterMS = 1000
		return output, true, &mcp.CallToolResult{}
	}
	if operation.Status != deferredoperation.StatusCompleted || !operation.ResultAvailable {
		return output, true, &mcp.CallToolResult{}
	}
	chunk, err := h.deferredOperations.ResultChunk(operationID, h.ResolvedAllowedDirs(), int64(offset), limitBytes)
	if err != nil {
		if errors.Is(err, deferredoperation.ErrInvalidInput) {
			return DeferredOperationOutput{}, true, errorResultWithCode(ErrCodeInvalidInput, "deferred operation request is invalid")
		}
		if errors.Is(err, deferredoperation.ErrAccessDenied) {
			return DeferredOperationOutput{}, true, errorResultWithCode(ErrCodeAccessDenied, "deferred operation is no longer authorized by the current filesystem roots")
		}
		return DeferredOperationOutput{}, true, errorResultWithCode(ErrCodeOperationFailed, "deferred operation result could not be read")
	}
	output.Offset = chunk.Offset
	output.NextOffset = chunk.NextOffset
	output.Data = chunk.Data
	output.Complete = chunk.Complete
	return output, true, &mcp.CallToolResult{}
}
