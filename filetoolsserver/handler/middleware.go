package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WithRecovery turns a panic into an error result instead of crashing the server.
func WithRecovery[In, Out any](handler mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	return func(ctx context.Context, req *mcp.CallToolRequest, args In) (result *mcp.CallToolResult, output Out, err error) {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in tool handler", "panic", r, "stack", string(debug.Stack()))
				result = errorResultWithCode(ErrCodeInternal, fmt.Sprintf("internal error: panic in tool handler: %v", r))
			}
		}()
		return handler(ctx, req, args)
	}
}

// WithLogging logs the tool name and outcome of each call.
func WithLogging[In, Out any](logger *slog.Logger, toolName string, handler mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	if logger == nil {
		return handler
	}
	return func(ctx context.Context, req *mcp.CallToolRequest, args In) (*mcp.CallToolResult, Out, error) {
		logger.Debug("tool_call_start", "tool", toolName)

		result, output, err := handler(ctx, req, args)

		if err != nil {
			logger.Error("tool_call_error", "tool", toolName, "error", err)
		} else if result != nil && result.IsError {
			var errMsg string
			if len(result.Content) > 0 {
				if tc, ok := result.Content[0].(*mcp.TextContent); ok {
					errMsg = tc.Text
				}
			}
			logger.Warn("tool_call_failed", "tool", toolName, "message", errMsg)
		} else {
			logger.Debug("tool_call_success", "tool", toolName)
		}

		return result, output, err
	}
}

// Wrap applies recovery (outermost) and optional logging to a tool handler.
func Wrap[In, Out any](logger *slog.Logger, toolName string, handler mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, Out] {
	wrapped := WithRecovery(handler)
	if logger != nil {
		wrapped = WithLogging(logger, toolName, wrapped)
	}
	return wrapped
}

// RepairStringifiedArrayArgs decodes declared array/object tool args that some
// MCP clients send as a JSON-encoded string, so schema validation succeeds.
// String-valued fields are never inferred from their contents: valid JSON text
// remains text unless the exact tool schema declares that top-level field as an
// array or object.
func RepairStringifiedArrayArgs(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if r, ok := req.(*mcp.CallToolRequest); ok && r.Params != nil {
			r.Params.Arguments = unstringifyJSONArgs(r.Params.Name, r.Params.Arguments)
		}
		return next(ctx, method, req)
	}
}

// stringifiedJSONArgumentFields lists the public top-level array/object fields
// for which compatibility repair is safe. The tool name is part of the key so
// an identically named string field in another schema cannot be reinterpreted.
var stringifiedJSONArgumentFields = map[string]map[string]struct{}{
	"read_multiple_files": {"paths": {}},
	"grep_text_files": {
		"patterns": {}, "paths": {}, "includes": {}, "excludes": {},
	},
	"tree":              {"exclude": {}},
	"search_files":      {"excludePatterns": {}},
	"fingerprint_paths": {"paths": {}},
	"verify_state":      {"checks": {}},
	"edit_file":         {"edits": {}},
	"patch_package":     {"manifest": {}},
	"convert_encoding":  {"paths": {}},
	"task_run":          {"args": {}, "tags": {}, "lockKeys": {}},
	"task_list":         {"statuses": {}, "kinds": {}, "tags": {}},
}

// unstringifyJSONArgs decodes only declared top-level array/object fields whose
// value is a JSON string wrapping an array or object. It returns input unchanged
// if the tool has no compatible fields or nothing needs repair.
func unstringifyJSONArgs(toolName string, raw json.RawMessage) json.RawMessage {
	allowedFields := stringifiedJSONArgumentFields[toolName]
	if len(allowedFields) == 0 {
		return raw
	}

	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return raw
	}

	changed := false
	for name, val := range fields {
		if _, allowed := allowedFields[name]; !allowed {
			continue
		}
		var s string
		if json.Unmarshal(val, &s) != nil {
			continue // not a JSON string
		}
		if t := strings.TrimSpace(s); len(t) == 0 || (t[0] != '[' && t[0] != '{') {
			continue // not a wrapped array/object
		}
		if !json.Valid([]byte(s)) {
			continue
		}
		fields[name] = json.RawMessage(s)
		changed = true
	}

	if !changed {
		return raw
	}
	if repaired, err := json.Marshal(fields); err == nil {
		return repaired
	}
	return raw
}

// WrapContentOnly drops StructuredContent, returning only the handler's text (e.g. a diff).
func WrapContentOnly[In, Out any](logger *slog.Logger, toolName string, handler mcp.ToolHandlerFor[In, Out]) mcp.ToolHandlerFor[In, any] {
	wrapped := Wrap(logger, toolName, handler)
	return func(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, any, error) {
		result, _, err := wrapped(ctx, req, input)
		return result, nil, err
	}
}
