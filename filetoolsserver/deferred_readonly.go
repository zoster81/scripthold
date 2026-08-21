package filetoolsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/deferredoperation"
)

type deferredInputPreparer[In any] func(*handler.Handler, *config.Config, In) (In, []string, bool)

// deferredReadOnlyHandler starts eligible work under independent ownership from
// the beginning, waits only for the configured synchronous grace period, and
// exposes an operation ID only after the durable started marker exists.
func deferredReadOnlyHandler[In, Out any](toolName string, h *handler.Handler, cfg *config.Config, engine deferredEngine, sourceBound bool, prepare deferredInputPreparer[In], directHandler mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, any] {
	direct := func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		result, output, err := directHandler(ctx, req, input)
		return result, output, err
	}
	if h == nil || cfg == nil || engine == nil || engine.Store() == nil || prepare == nil {
		return direct
	}
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		canonical, origins, ok := prepare(h, cfg, input)
		if !ok {
			return direct(ctx, req, input)
		}
		arguments, err := json.Marshal(canonical)
		if err != nil {
			return compactDeferredError(handler.ErrCodeInternal, "read-only request could not be encoded for deferred execution"), map[string]any{"errorCode": handler.ErrCodeInternal}, nil
		}
		allowed := h.ResolvedAllowedDirs()
		request := deferredoperation.Request{
			Tool:               toolName,
			Arguments:          arguments,
			AllowedDirectories: allowed,
			OriginPaths:        origins,
			Config: deferredoperation.ExecutionConfig{
				DefaultEncoding: cfg.DefaultEncoding,
				Limits:          cfg.Limits,
				Source:          cfg.Source,
			},
			MaxRuntimeSeconds: deferredRuntimeSeconds(cfg, sourceBound),
		}
		operation, err := engine.Submit(ctx, request)
		if err != nil {
			code := handler.ErrCodeOperationFailed
			if errors.Is(err, deferredoperation.ErrCapacity) {
				code = handler.ErrCodeLimit
			}
			return compactDeferredError(code, "deferred read-only operation could not be admitted"), map[string]any{"errorCode": code}, nil
		}
		observed, finished, waitErr := engine.Wait(ctx, operation.OperationID, allowed, time.Duration(cfg.Reliability.DeferredSyncWaitSeconds)*time.Second)
		if waitErr != nil {
			cancelDeferredBestEffort(engine.Store(), operation.OperationID, allowed)
			if errors.Is(waitErr, context.Canceled) || errors.Is(waitErr, context.DeadlineExceeded) {
				return compactDeferredError(handler.ErrCodeCancelled, "read-only operation was cancelled before handoff"), map[string]any{"errorCode": handler.ErrCodeCancelled}, nil
			}
			return compactDeferredError(handler.ErrCodeOperationFailed, "deferred read-only operation could not be observed"), map[string]any{"errorCode": handler.ErrCodeOperationFailed}, nil
		}
		if finished {
			return completedDeferredReadOnly[Out](ctx, engine.Store(), observed, allowed)
		}
		if !observed.Started {
			cancelDeferredBestEffort(engine.Store(), operation.OperationID, allowed)
			return compactDeferredError(handler.ErrCodeOperationFailed, "deferred executor did not start within the synchronous handoff window"), map[string]any{"errorCode": handler.ErrCodeOperationFailed}, nil
		}
		observed, err = engine.Store().MarkExposed(ctx, operation.OperationID, allowed)
		if err != nil {
			cancelDeferredBestEffort(engine.Store(), operation.OperationID, allowed)
			return compactDeferredError(handler.ErrCodeOperationFailed, "deferred read-only operation could not be exposed safely"), map[string]any{"errorCode": handler.ErrCodeOperationFailed}, nil
		}
		return &mcp.CallToolResult{Meta: mcp.Meta{"operationId": observed.OperationID, "deferred": true}}, deferredHandoffOutput{
			Deferred: true, OperationID: observed.OperationID, Status: "working", PollAfterMS: 1000,
			Message: "The read-only operation is still running independently. Retrieve it with deferred_operation.",
		}, nil
	}
}

func cancelDeferredBestEffort(store *deferredoperation.Store, operationID string, allowed []string) {
	if store == nil {
		return
	}
	// Cleanup must outlive the frontend request context so an abandoned handoff
	// still records a cooperative cancellation request when possible.
	_, _ = store.Cancel(context.Background(), operationID, allowed)
}

func completedDeferredReadOnly[Out any](ctx context.Context, store *deferredoperation.Store, operation deferredoperation.Operation, allowed []string) (*mcp.CallToolResult, any, error) {
	if operation.Status != deferredoperation.StatusCompleted {
		code := operation.ErrorCode
		if code == "" {
			code = handler.ErrCodeOperationFailed
		}
		message := operation.Message
		if message == "" {
			message = fmt.Sprintf("deferred read-only operation ended with status %s", operation.Status)
		}
		return compactDeferredError(code, message), map[string]any{"operationId": operation.OperationID, "status": operation.Status, "errorCode": code}, nil
	}
	payload, _, err := store.ReadResultContext(ctx, operation.OperationID, allowed)
	if err != nil {
		return compactDeferredError(handler.ErrCodeOperationFailed, "deferred read-only result could not be read"), map[string]any{"errorCode": handler.ErrCodeOperationFailed}, nil
	}
	result, output, err := decodeDurableToolEnvelope[Out](payload)
	if err != nil {
		return compactDeferredError(handler.ErrCodeInternal, "deferred read-only result is invalid"), map[string]any{"errorCode": handler.ErrCodeInternal}, nil
	}
	return result, output, nil
}

func decodeDurableToolEnvelope[Out any](payload []byte) (*mcp.CallToolResult, Out, error) {
	var zero Out
	var envelope durableToolEnvelope
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return nil, zero, err
	}
	var result mcp.CallToolResult
	if err := json.Unmarshal(envelope.Result, &result); err != nil {
		return nil, zero, err
	}
	var output Out
	if err := json.Unmarshal(envelope.Output, &output); err != nil {
		return nil, zero, err
	}
	return &result, output, nil
}

func deferredRuntimeSeconds(cfg *config.Config, sourceBound bool) int {
	runtimeSeconds := cfg.Reliability.DeferredMaxRuntimeSeconds
	if sourceBound && cfg.Source.MaxRequestSeconds > 0 && (runtimeSeconds <= 0 || cfg.Source.MaxRequestSeconds < runtimeSeconds) {
		runtimeSeconds = cfg.Source.MaxRequestSeconds
	}
	return runtimeSeconds
}

func prepareDeferredGrep(h *handler.Handler, cfg *config.Config, input handler.GrepInput) (handler.GrepInput, []string, bool) {
	if len(input.Paths) == 0 || len(input.Paths) > cfg.Limits.MaxBatchFiles {
		return input, nil, false
	}
	canonical := input
	canonical.Paths = make([]string, len(input.Paths))
	for index, path := range input.Paths {
		validated := h.ValidatePath(path)
		if !validated.Ok() {
			return input, nil, false
		}
		canonical.Paths[index] = validated.Path
	}
	return canonical, append([]string(nil), canonical.Paths...), true
}

func prepareDeferredSearchFiles(h *handler.Handler, _ *config.Config, input handler.SearchFilesInput) (handler.SearchFilesInput, []string, bool) {
	if input.Path == "" {
		return input, nil, false
	}
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return input, nil, false
	}
	input.Path = validated.Path
	return input, []string{validated.Path}, true
}

func prepareDeferredTree(h *handler.Handler, _ *config.Config, input handler.TreeInput) (handler.TreeInput, []string, bool) {
	if input.Path == "" {
		return input, nil, false
	}
	validated := h.ValidatePath(input.Path)
	if !validated.Ok() {
		return input, nil, false
	}
	input.Path = validated.Path
	return input, []string{validated.Path}, true
}

func prepareDeferredSourceSymbols(h *handler.Handler, cfg *config.Config, input handler.SourceSymbolsInput) (handler.SourceSymbolsInput, []string, bool) {
	if input.Operation == "show" || input.Operation == "SHOW" {
		if input.Path == "" {
			return input, nil, false
		}
		validated := h.ValidatePath(input.Path)
		if !validated.Ok() {
			return input, nil, false
		}
		input.Path = validated.Path
		return input, []string{validated.Path}, true
	}
	if len(input.Paths) == 0 || len(input.Paths) > cfg.Source.MaxInputPaths {
		return input, nil, false
	}
	input.Paths = append([]string(nil), input.Paths...)
	for index, path := range input.Paths {
		validated := h.ValidatePath(path)
		if !validated.Ok() {
			return input, nil, false
		}
		input.Paths[index] = validated.Path
	}
	return input, append([]string(nil), input.Paths...), true
}
