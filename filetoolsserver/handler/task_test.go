package handler

import (
	"context"
	"testing"

	"github.com/zoster81/scripthold/internal/taskstore"
)

func TestWithTaskStoreTreatsTypedNilAsUnavailable(t *testing.T) {
	var store *taskstore.Store
	handler := NewHandler([]string{t.TempDir()}, WithTaskStore(store))
	if handler.taskStore != nil {
		t.Fatalf("typed nil task store remained configured as %T", handler.taskStore)
	}
	result, _, err := handler.HandleTaskList(context.Background(), nil, TaskListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError {
		t.Fatalf("disabled task list result = %#v", result)
	}
}

func TestTaskRunRejectsUnsupportedShellBeforeAdmission(t *testing.T) {
	handler := NewHandler([]string{t.TempDir()}, WithExecutionPolicy(ExecutionPolicy{AllowShell: true}))
	for _, shell := range []string{"fish", "powershell.exe"} {
		t.Run(shell, func(t *testing.T) {
			result, _, err := handler.HandleTaskRun(context.Background(), nil, TaskRunInput{
				Kind:           "shell",
				IdempotencyKey: "unsupported-shell-" + shell,
				Command:        "echo ok",
				Shell:          shell,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
				t.Fatalf("unsupported shell result = %#v", result)
			}
		})
	}
}

func TestTaskRunRejectsCrossKindPathWithStableCode(t *testing.T) {
	handler := NewHandler([]string{t.TempDir()}, WithExecutionPolicy(ExecutionPolicy{AllowShell: true}))
	result, _, err := handler.HandleTaskRun(context.Background(), nil, TaskRunInput{Kind: "shell", Path: `C:\unexpected.ps1`})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("cross-kind path result = %#v", result)
	}
}
