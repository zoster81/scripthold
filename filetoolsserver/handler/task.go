package handler

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/filesystem"
	"github.com/zoster81/scripthold/internal/taskstore"
)

type TaskRunInput struct {
	Kind              string   `json:"kind"`
	Name              string   `json:"name,omitempty"`
	Description       string   `json:"description,omitempty"`
	Tags              []string `json:"tags,omitempty"`
	IdempotencyKey    string   `json:"idempotencyKey"`
	LockKeys          []string `json:"lockKeys,omitempty"`
	Cwd               string   `json:"cwd,omitempty"`
	Command           string   `json:"command,omitempty"`
	Shell             string   `json:"shell,omitempty"`
	Path              string   `json:"path,omitempty"`
	Args              []string `json:"args,omitempty"`
	MaxRuntimeSeconds int      `json:"maxRuntimeSeconds,omitempty"`
}

type TaskListInput struct {
	Statuses []string `json:"statuses,omitempty"`
	Kinds    []string `json:"kinds,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Cursor   string   `json:"cursor,omitempty"`
	Limit    int      `json:"limit,omitempty"`
}

type TaskGetInput struct {
	TaskID string `json:"taskId"`
}

type TaskLogsInput struct {
	TaskID       string `json:"taskId"`
	StdoutCursor int64  `json:"stdoutCursor,omitempty"`
	StderrCursor int64  `json:"stderrCursor,omitempty"`
	LimitBytes   int    `json:"limitBytes,omitempty"`
}

type TaskCancelInput struct {
	TaskID string `json:"taskId"`
	Reason string `json:"reason,omitempty"`
}

func (h *Handler) HandleTaskRun(ctx context.Context, _ *mcp.CallToolRequest, input TaskRunInput) (*mcp.CallToolResult, taskstore.SubmitResult, error) {
	kind := taskstore.Kind(strings.ToLower(strings.TrimSpace(input.Kind)))
	if kind == taskstore.KindShell && !h.executionAllowed("MCP_ENABLE_SHELL") {
		return errorResult(h.executionDisabledMessage("task_run shell")), taskstore.SubmitResult{}, nil
	}
	if kind == taskstore.KindScript && !h.executionAllowed("MCP_ENABLE_RUN_SCRIPT") {
		return errorResult(h.executionDisabledMessage("task_run script")), taskstore.SubmitResult{}, nil
	}
	if kind != taskstore.KindShell && kind != taskstore.KindScript {
		return errorResultWithCode(ErrCodeInvalidInput, "kind must be shell or script"), taskstore.SubmitResult{}, nil
	}
	if kind == taskstore.KindShell && strings.TrimSpace(input.Path) != "" {
		return errorResultWithCode(ErrCodeInvalidInput, "path is not valid for shell tasks"), taskstore.SubmitResult{}, nil
	}
	if h.taskStore == nil {
		return errorResult("task system is unavailable; configure MCP_TASK_STORE_DIR and start task-worker"), taskstore.SubmitResult{}, nil
	}

	cwd := strings.TrimSpace(input.Cwd)
	request := taskstore.Request{Kind: kind, Name: input.Name, Description: input.Description, Tags: input.Tags, IdempotencyKey: input.IdempotencyKey, LockKeys: input.LockKeys, Command: input.Command, Shell: input.Shell, Args: input.Args, MaxRuntimeSeconds: input.MaxRuntimeSeconds}
	if kind == taskstore.KindScript {
		validated := h.ValidatePath(input.Path)
		if !validated.Ok() {
			return validated.Result, taskstore.SubmitResult{}, nil
		}
		snapshot, err := filesystem.CaptureRegularFileSnapshotBounded(ctx, validated.Path, h.config.Limits.MaxFileBytes)
		if err != nil {
			return errorResultFromError(err), taskstore.SubmitResult{}, nil
		}
		digest, ok := snapshot.ContentDigest()
		if !ok {
			return errorResult("script digest is unavailable"), taskstore.SubmitResult{}, nil
		}
		request.ScriptPath = validated.Path
		request.ScriptDigest = hex.EncodeToString(digest[:])
		request.ScriptSize = snapshot.Size
		if cwd == "" {
			cwd = filepath.Dir(validated.Path)
		}
	} else if cwd == "" {
		roots := h.GetAllowedDirectories()
		if len(roots) == 0 {
			return errorResult("no allowed directories are configured"), taskstore.SubmitResult{}, nil
		}
		cwd = roots[0]
	}
	validatedCwd := h.ValidatePath(cwd)
	if !validatedCwd.Ok() {
		return validatedCwd.Result, taskstore.SubmitResult{}, nil
	}
	request.WorkingDirectory = validatedCwd.Path
	result, err := h.taskStore.Submit(ctx, request)
	if err != nil {
		return errorResultFromError(err), taskstore.SubmitResult{}, nil
	}
	return &mcp.CallToolResult{}, result, nil
}

func (h *Handler) HandleTaskList(ctx context.Context, _ *mcp.CallToolRequest, input TaskListInput) (*mcp.CallToolResult, taskstore.ListResult, error) {
	if h.taskStore == nil {
		return errorResult("task system is unavailable"), taskstore.ListResult{}, nil
	}
	statuses := make([]taskstore.Status, len(input.Statuses))
	for i, value := range input.Statuses {
		statuses[i] = taskstore.Status(value)
	}
	kinds := make([]taskstore.Kind, len(input.Kinds))
	for i, value := range input.Kinds {
		kinds[i] = taskstore.Kind(value)
	}
	result, err := h.taskStore.List(ctx, taskstore.ListOptions{Statuses: statuses, Kinds: kinds, Tags: input.Tags, Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return errorResultFromError(err), taskstore.ListResult{}, nil
	}
	return &mcp.CallToolResult{}, result, nil
}

func (h *Handler) HandleTaskGet(ctx context.Context, _ *mcp.CallToolRequest, input TaskGetInput) (*mcp.CallToolResult, taskstore.Task, error) {
	if h.taskStore == nil {
		return errorResult("task system is unavailable"), taskstore.Task{}, nil
	}
	result, err := h.taskStore.Get(ctx, input.TaskID)
	if err != nil {
		return errorResultFromError(err), taskstore.Task{}, nil
	}
	return &mcp.CallToolResult{}, result, nil
}

func (h *Handler) HandleTaskLogs(ctx context.Context, _ *mcp.CallToolRequest, input TaskLogsInput) (*mcp.CallToolResult, taskstore.LogsResult, error) {
	if h.taskStore == nil {
		return errorResult("task system is unavailable"), taskstore.LogsResult{}, nil
	}
	result, err := h.taskStore.Logs(ctx, input.TaskID, taskstore.LogOptions{StdoutCursor: input.StdoutCursor, StderrCursor: input.StderrCursor, LimitBytes: input.LimitBytes})
	if err != nil {
		return errorResultFromError(err), taskstore.LogsResult{}, nil
	}
	return &mcp.CallToolResult{}, result, nil
}

func (h *Handler) HandleTaskCancel(ctx context.Context, _ *mcp.CallToolRequest, input TaskCancelInput) (*mcp.CallToolResult, taskstore.Task, error) {
	if h.taskStore == nil {
		return errorResult("task system is unavailable"), taskstore.Task{}, nil
	}
	result, err := h.taskStore.Cancel(ctx, input.TaskID, input.Reason)
	if err != nil {
		return errorResultFromError(err), taskstore.Task{}, nil
	}
	return &mcp.CallToolResult{}, result, nil
}
