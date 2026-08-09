package handler

import (
	"os"
	"strings"
)

// ExecutionPolicy is an immutable transport-specific execution decision.
type ExecutionPolicy struct {
	AllowRunScript bool
	AllowShell     bool
}

// ExecutionPolicyFromEnvironment snapshots the existing execution flags.
func ExecutionPolicyFromEnvironment(getenv func(string) string) ExecutionPolicy {
	if getenv == nil {
		return ExecutionPolicy{}
	}
	combined := environmentFlagValue(getenv("MCP_ENABLE_EXECUTION"))
	return ExecutionPolicy{
		AllowRunScript: combined || environmentFlagValue(getenv("MCP_ENABLE_RUN_SCRIPT")),
		AllowShell:     combined || environmentFlagValue(getenv("MCP_ENABLE_SHELL")),
	}
}

func (h *Handler) executionAllowed(specificVariable string) bool {
	if h.executionPolicy == nil {
		return executionFeatureEnabled(specificVariable)
	}
	switch specificVariable {
	case "MCP_ENABLE_RUN_SCRIPT":
		return h.executionPolicy.AllowRunScript
	case "MCP_ENABLE_SHELL":
		return h.executionPolicy.AllowShell
	default:
		return false
	}
}

func executionFeatureEnabled(specificVariable string) bool {
	return environmentFlagValue(os.Getenv("MCP_ENABLE_EXECUTION")) || environmentFlagValue(os.Getenv(specificVariable))
}

func (h *Handler) executionDisabledMessage(toolName string) string {
	if h.executionPolicy == nil {
		switch toolName {
		case "task_run script":
			return "task_run kind=script is disabled; set MCP_ENABLE_RUN_SCRIPT=1 or MCP_ENABLE_EXECUTION=1 before starting the server"
		case "task_run shell":
			return "task_run kind=shell is disabled; set MCP_ENABLE_SHELL=1 or MCP_ENABLE_EXECUTION=1 before starting the server"
		}
	}
	switch toolName {
	case "task_run script":
		return "task_run kind=script is disabled by server policy; HTTP requires MCP_HTTP_ENABLE_EXECUTION=1 plus MCP_ENABLE_RUN_SCRIPT=1 or MCP_ENABLE_EXECUTION=1"
	case "task_run shell":
		return "task_run kind=shell is disabled by server policy; HTTP requires MCP_HTTP_ENABLE_EXECUTION=1 plus MCP_ENABLE_SHELL=1 or MCP_ENABLE_EXECUTION=1"
	default:
		return "execution is disabled by server policy"
	}
}

func environmentFlagValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
