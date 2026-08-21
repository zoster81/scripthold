package filetoolsserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/deferredoperation"
)

type deferredEngine interface {
	Submit(context.Context, deferredoperation.Request) (deferredoperation.Operation, error)
	Wait(context.Context, string, []string, time.Duration) (deferredoperation.Operation, bool, error)
	Store() *deferredoperation.Store
}

type durableToolEnvelope struct {
	Result json.RawMessage `json:"result"`
	Output json.RawMessage `json:"output"`
}

type deferredHandoffOutput struct {
	Deferred    bool   `json:"deferred"`
	OperationID string `json:"operationId"`
	Status      string `json:"status"`
	PollAfterMS int    `json:"pollAfterMs"`
	Message     string `json:"message"`
}

func deferredFingerprintHandler(h *handler.Handler, cfg *config.Config, engine deferredEngine, directHandler mcp.ToolHandlerFor[handler.FingerprintPathsInput, handler.FingerprintPathsOutput]) mcp.ToolHandlerFor[handler.FingerprintPathsInput, any] {
	return deferredReadOnlyHandler("fingerprint_paths", h, cfg, engine, false, prepareDeferredFingerprintInput, directHandler)
}

func prepareDeferredFingerprintInput(h *handler.Handler, cfg *config.Config, input handler.FingerprintPathsInput) (handler.FingerprintPathsInput, []string, bool) {
	if len(input.Paths) == 0 || len(input.Paths) > cfg.Limits.MaxBatchFiles || (!input.IncludeEntries && input.MaxEntryDetails != 0) || input.MaxEntryDetails < 0 || input.MaxEntryDetails > cfg.Limits.MaxFingerprintEntryDetails {
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

// ExecuteDeferredOperation executes the transport-independent payload admitted
// by the frontend. Only explicitly allowlisted read-only operations belong here.
func ExecuteDeferredOperation(ctx context.Context, request deferredoperation.Request) ([]byte, deferredoperation.ResultMetadata, error) {
	cfg := config.LoadFromEnvironment(func(string) string { return "" })
	cfg.DefaultEncoding = request.Config.DefaultEncoding
	cfg.Limits = request.Config.Limits
	cfg.Source = request.Config.Source
	h := handler.NewHandler(request.AllowedDirectories, handler.WithConfig(cfg))

	switch request.Tool {
	case "fingerprint_paths":
		var input handler.FingerprintPathsInput
		if err := json.Unmarshal(request.Arguments, &input); err != nil {
			return nil, deferredoperation.ResultMetadata{}, err
		}
		return executeDeferredTyped(ctx, input, h.HandleFingerprintPaths)
	case "grep_text_files":
		var input handler.GrepInput
		if err := json.Unmarshal(request.Arguments, &input); err != nil {
			return nil, deferredoperation.ResultMetadata{}, err
		}
		return executeDeferredTyped(ctx, input, h.HandleGrep)
	case "search_files":
		var input handler.SearchFilesInput
		if err := json.Unmarshal(request.Arguments, &input); err != nil {
			return nil, deferredoperation.ResultMetadata{}, err
		}
		return executeDeferredTyped(ctx, input, h.HandleSearchFiles)
	case "tree":
		var input handler.TreeInput
		if err := json.Unmarshal(request.Arguments, &input); err != nil {
			return nil, deferredoperation.ResultMetadata{}, err
		}
		return executeDeferredTyped(ctx, input, h.HandleTree)
	case "source_symbols":
		var input handler.SourceSymbolsInput
		if err := json.Unmarshal(request.Arguments, &input); err != nil {
			return nil, deferredoperation.ResultMetadata{}, err
		}
		return executeDeferredTyped(ctx, input, h.SourceSymbols)
	default:
		return nil, deferredoperation.ResultMetadata{}, fmt.Errorf("unsupported deferred tool %q", request.Tool)
	}
}

func executeDeferredTyped[In, Out any](ctx context.Context, input In, raw mcp.ToolHandlerFor[In, Out]) ([]byte, deferredoperation.ResultMetadata, error) {
	// Match the public handler semantics that occur before SDK output shaping.
	// The detached executor intentionally omits request-bound deadline/logging;
	// its own durable runtime and diagnostics lifecycle own those concerns.
	wrapped := handler.WithStructuredErrorOutput(handler.WithRecovery(raw))
	result, output, err := wrapped(ctx, nil, input)
	if err != nil {
		return nil, deferredoperation.ResultMetadata{}, err
	}
	return encodeDurableToolEnvelope(result, output)
}

func encodeDurableToolEnvelope(result *mcp.CallToolResult, output any) ([]byte, deferredoperation.ResultMetadata, error) {
	if result == nil {
		result = &mcp.CallToolResult{}
	}
	outputBytes, err := canonicalObjectJSON(output)
	if err != nil {
		return nil, deferredoperation.ResultMetadata{}, err
	}
	finalized := *result
	finalized.StructuredContent = json.RawMessage(outputBytes)
	if finalized.Content == nil {
		finalized.Content = []mcp.Content{&mcp.TextContent{Text: string(outputBytes)}}
	}
	resultBytes, err := json.Marshal(&finalized)
	if err != nil {
		return nil, deferredoperation.ResultMetadata{}, err
	}
	payload, err := json.Marshal(durableToolEnvelope{Result: resultBytes, Output: outputBytes})
	if err != nil {
		return nil, deferredoperation.ResultMetadata{}, err
	}
	return payload, deferredoperation.ResultMetadata{OriginalIsError: finalized.IsError, ErrorCode: resultErrorCode(&finalized)}, nil
}

func canonicalObjectJSON(value any) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var object map[string]any
	if err := json.Unmarshal(encoded, &object); err != nil || object == nil {
		if err != nil {
			return nil, fmt.Errorf("deferred tool output must be an object: %w", err)
		}
		return nil, fmt.Errorf("deferred tool output must be a non-null object")
	}
	return json.Marshal(object)
}

func compactDeferredError(code, message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Meta:    mcp.Meta{handler.ErrorCodeMetaKey: code},
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
	}
}
