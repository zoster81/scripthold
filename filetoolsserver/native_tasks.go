package filetoolsserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/deferredoperation"
)

const (
	tasksExtensionID                     = "io.modelcontextprotocol/tasks"
	tasksClientCapabilitiesMetaKey       = "io.modelcontextprotocol/clientCapabilities"
	tasksGetMethod                       = "tasks/get"
	tasksUpdateMethod                    = "tasks/update"
	tasksCancelMethod                    = "tasks/cancel"
	tasksMissingCapabilityCode     int64 = -32003
	tasksPollIntervalMS                  = 1000
	tasksMinimumProtocolVersion          = "2026-07-28"
)

type nativeTaskParams struct {
	mcp.ParamsBase
	TaskID string `json:"taskId"`
}

type nativeUpdateTaskParams struct {
	mcp.ParamsBase
	TaskID         string                     `json:"taskId"`
	InputResponses map[string]json.RawMessage `json:"inputResponses"`
}

type nativeTaskFields struct {
	TaskID         string `json:"taskId"`
	Status         string `json:"status"`
	StatusMessage  string `json:"statusMessage,omitempty"`
	CreatedAt      string `json:"createdAt"`
	LastUpdatedAt  string `json:"lastUpdatedAt"`
	TTLMS          int64  `json:"ttlMs"`
	PollIntervalMS int    `json:"pollIntervalMs,omitempty"`
}

type nativeCreateTaskResult struct {
	mcp.ResultBase
	ResultType string `json:"resultType"`
	nativeTaskFields
}

type nativeTaskExecutionError struct {
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type nativeGetTaskResult struct {
	mcp.ResultBase
	ResultType string `json:"resultType"`
	nativeTaskFields
	Result json.RawMessage           `json:"result,omitempty"`
	Error  *nativeTaskExecutionError `json:"error,omitempty"`
}

type nativeUpdateTaskResult struct {
	mcp.ResultBase
	ResultType string `json:"resultType"`
}

type nativeCancelTaskResult struct {
	mcp.ResultBase
	ResultType string `json:"resultType"`
}

type nativeTasksAdapter struct {
	store              *deferredoperation.Store
	allowedDirectories func() []string
}

func registerNativeTasks(server *mcp.Server, adapter nativeTasksAdapter) error {
	if server == nil || adapter.store == nil || adapter.allowedDirectories == nil {
		return errors.New("native tasks adapter is not configured")
	}
	if err := mcp.AddReceivingCustomMethod(server, tasksGetMethod, adapter.get); err != nil {
		return fmt.Errorf("register %s: %w", tasksGetMethod, err)
	}
	if err := mcp.AddReceivingCustomMethod(server, tasksUpdateMethod, adapter.update); err != nil {
		return fmt.Errorf("register %s: %w", tasksUpdateMethod, err)
	}
	if err := mcp.AddReceivingCustomMethod(server, tasksCancelMethod, adapter.cancel); err != nil {
		return fmt.Errorf("register %s: %w", tasksCancelMethod, err)
	}
	return nil
}

func createNativeTasksDiscoveryMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, req)
			if err != nil || method != methodDiscover {
				return result, err
			}
			discovery, ok := result.(*mcp.DiscoverResult)
			if !ok || discovery.Capabilities == nil {
				return result, nil
			}
			capabilities := *discovery.Capabilities
			capabilities.Extensions = cloneExtensions(capabilities.Extensions)
			if capabilities.Extensions == nil {
				capabilities.Extensions = make(map[string]any, 1)
			}
			capabilities.Extensions[tasksExtensionID] = map[string]any{}
			discovery.Capabilities = &capabilities
			return discovery, nil
		}
	}
}

func createNativeTasksMiddleware(store *deferredoperation.Store, allowedDirectories func() []string) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			protocolSupported := nativeTasksProtocolSupported(req)
			if method == tasksGetMethod || method == tasksUpdateMethod || method == tasksCancelMethod {
				if !protocolSupported {
					return nil, &jsonrpc.Error{Code: jsonrpc.CodeMethodNotFound, Message: "method not found"}
				}
				return next(ctx, method, req)
			}
			if method != "tools/call" || !protocolSupported || store == nil || allowedDirectories == nil {
				return next(ctx, method, req)
			}
			params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok || !hasNativeTasksCapability(params.Meta) {
				return next(ctx, method, req)
			}

			result, err := next(ctx, method, req)
			if err != nil {
				return nil, err
			}
			callResult, ok := result.(*mcp.CallToolResult)
			if !ok || callResult.Meta == nil {
				return result, nil
			}
			deferred, _ := callResult.Meta["deferred"].(bool)
			operationID, _ := callResult.Meta["operationId"].(string)
			if !deferred || !deferredoperation.ValidOperationID(operationID) {
				return result, nil
			}

			operation, getErr := store.GetContext(ctx, operationID, allowedDirectories())
			if getErr != nil {
				return nil, nativeTaskStoreError("create task", getErr)
			}
			return &nativeCreateTaskResult{
				ResultType:       "task",
				nativeTaskFields: nativeTaskFieldsFromOperation(store, operation),
			}, nil
		}
	}
}

func (adapter nativeTasksAdapter) get(ctx context.Context, _ *mcp.ServerSession, params *nativeTaskParams) (*nativeGetTaskResult, error) {
	if err := validateNativeTaskRequest(params); err != nil {
		return nil, err
	}
	operation, err := adapter.store.GetContext(ctx, params.TaskID, adapter.allowedDirectories())
	if err != nil {
		return nil, nativeTaskStoreError("get task", err)
	}
	result := &nativeGetTaskResult{
		ResultType:       "complete",
		nativeTaskFields: nativeTaskFieldsFromOperation(adapter.store, operation),
	}
	switch operation.Status {
	case deferredoperation.StatusCompleted:
		payload, observed, readErr := adapter.store.ReadResultContext(ctx, operation.OperationID, adapter.allowedDirectories())
		if readErr != nil {
			return nil, nativeTaskStoreError("read task result", readErr)
		}
		var envelope durableToolEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil || len(envelope.Result) == 0 || !json.Valid(envelope.Result) {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "Task result is unavailable"}
		}
		result.nativeTaskFields = nativeTaskFieldsFromOperation(adapter.store, observed)
		result.Result = append(json.RawMessage(nil), envelope.Result...)
	case deferredoperation.StatusFailed, deferredoperation.StatusTimedOut, deferredoperation.StatusInterrupted:
		result.Error = &nativeTaskExecutionError{Code: jsonrpc.CodeInternalError, Message: nativeTaskFailureMessage(operation)}
	}
	return result, nil
}

func (adapter nativeTasksAdapter) update(ctx context.Context, _ *mcp.ServerSession, params *nativeUpdateTaskParams) (*nativeUpdateTaskResult, error) {
	if params == nil || !hasNativeTasksCapability(params.Meta) {
		return nil, missingNativeTasksCapabilityError()
	}
	if !deferredoperation.ValidOperationID(params.TaskID) || params.InputResponses == nil {
		return nil, &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Invalid tasks/update params"}
	}
	if _, err := adapter.store.GetContext(ctx, params.TaskID, adapter.allowedDirectories()); err != nil {
		return nil, nativeTaskStoreError("update task", err)
	}
	// This engine never enters input_required. SEP-2663 requires unknown or
	// already-satisfied inputResponses to be ignored, so a known task is acked
	// without changing durable execution state.
	return &nativeUpdateTaskResult{ResultType: "complete"}, nil
}

func (adapter nativeTasksAdapter) cancel(ctx context.Context, _ *mcp.ServerSession, params *nativeTaskParams) (*nativeCancelTaskResult, error) {
	if err := validateNativeTaskRequest(params); err != nil {
		return nil, err
	}
	if _, err := adapter.store.Cancel(ctx, params.TaskID, adapter.allowedDirectories()); err != nil {
		return nil, nativeTaskStoreError("cancel task", err)
	}
	return &nativeCancelTaskResult{ResultType: "complete"}, nil
}

func nativeTasksProtocolSupported(req mcp.Request) bool {
	serverRequest, ok := req.(interface{ ProtocolVersion() string })
	return ok && serverRequest.ProtocolVersion() >= tasksMinimumProtocolVersion
}

func validateNativeTaskRequest(params *nativeTaskParams) error {
	if params == nil || !hasNativeTasksCapability(params.Meta) {
		return missingNativeTasksCapabilityError()
	}
	if !deferredoperation.ValidOperationID(params.TaskID) {
		return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Invalid taskId"}
	}
	return nil
}

func hasNativeTasksCapability(meta mcp.Meta) bool {
	clientCapabilities, ok := meta[tasksClientCapabilitiesMetaKey].(map[string]any)
	if !ok {
		return false
	}
	extensions, ok := clientCapabilities["extensions"].(map[string]any)
	if !ok {
		return false
	}
	settings, ok := extensions[tasksExtensionID]
	if !ok {
		return false
	}
	_, ok = settings.(map[string]any)
	return ok
}

func missingNativeTasksCapabilityError() error {
	return &jsonrpc.Error{
		Code:    tasksMissingCapabilityCode,
		Message: "Missing required client capability",
		Data:    json.RawMessage(`{"requiredCapabilities":{"extensions":{"io.modelcontextprotocol/tasks":{}}}}`),
	}
}

func nativeTaskStoreError(action string, err error) error {
	switch {
	case errors.Is(err, deferredoperation.ErrInvalidInput), errors.Is(err, deferredoperation.ErrNotFound), errors.Is(err, deferredoperation.ErrAccessDenied), errors.Is(err, deferredoperation.ErrResultMissing):
		return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: "Invalid or unavailable taskId"}
	default:
		return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: action + " failed"}
	}
}

func nativeTaskFieldsFromOperation(store *deferredoperation.Store, operation deferredoperation.Operation) nativeTaskFields {
	fields := nativeTaskFields{
		TaskID:         operation.OperationID,
		Status:         nativeTaskStatus(operation.Status),
		CreatedAt:      operation.CreatedAt.UTC().Format(time.RFC3339Nano),
		LastUpdatedAt:  operation.UpdatedAt.UTC().Format(time.RFC3339Nano),
		PollIntervalMS: tasksPollIntervalMS,
	}
	if store != nil {
		fields.TTLMS = int64(store.Limits().RetentionSeconds) * 1000
	}
	if operation.Message != "" && operation.Status.Terminal() {
		fields.StatusMessage = operation.Message
	}
	return fields
}

func nativeTaskStatus(status deferredoperation.Status) string {
	switch status {
	case deferredoperation.StatusCompleted:
		return "completed"
	case deferredoperation.StatusCancelled:
		return "cancelled"
	case deferredoperation.StatusFailed, deferredoperation.StatusTimedOut, deferredoperation.StatusInterrupted:
		return "failed"
	default:
		return "working"
	}
}

func nativeTaskFailureMessage(operation deferredoperation.Operation) string {
	if operation.Message != "" {
		return operation.Message
	}
	switch operation.Status {
	case deferredoperation.StatusTimedOut:
		return "Task execution timed out"
	case deferredoperation.StatusInterrupted:
		return "Task execution was interrupted"
	default:
		return "Task execution failed"
	}
}

func cloneExtensions(extensions map[string]any) map[string]any {
	if len(extensions) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(extensions)+1)
	for key, value := range extensions {
		cloned[key] = value
	}
	return cloned
}
