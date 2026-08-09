package handler

import "testing"

func TestExecutionFeatureFlagsRemainIndependent(t *testing.T) {
	t.Setenv("MCP_ENABLE_EXECUTION", "")
	t.Setenv("MCP_ENABLE_RUN_SCRIPT", "1")
	t.Setenv("MCP_ENABLE_SHELL", "")
	if !executionFeatureEnabled("MCP_ENABLE_RUN_SCRIPT") {
		t.Fatal("script flag did not enable the script task kind")
	}
	if executionFeatureEnabled("MCP_ENABLE_SHELL") {
		t.Fatal("script flag incorrectly enabled the shell task kind")
	}
}

func TestExecutionPolicyFromEnvironment(t *testing.T) {
	values := map[string]string{"MCP_ENABLE_RUN_SCRIPT": "1", "MCP_ENABLE_SHELL": "0"}
	policy := ExecutionPolicyFromEnvironment(func(name string) string { return values[name] })
	if !policy.AllowRunScript || policy.AllowShell {
		t.Fatalf("policy = %#v", policy)
	}
	values["MCP_ENABLE_EXECUTION"] = "true"
	policy = ExecutionPolicyFromEnvironment(func(name string) string { return values[name] })
	if !policy.AllowRunScript || !policy.AllowShell {
		t.Fatalf("combined flag policy = %#v", policy)
	}
}

func TestExplicitExecutionPolicyOverridesEnvironment(t *testing.T) {
	t.Setenv("MCP_ENABLE_EXECUTION", "1")
	handler := NewHandler(nil, WithExecutionPolicy(ExecutionPolicy{}))
	if handler.executionAllowed("MCP_ENABLE_RUN_SCRIPT") || handler.executionAllowed("MCP_ENABLE_SHELL") {
		t.Fatal("explicit deny policy was bypassed by environment flags")
	}
	handler = NewHandler(nil, WithExecutionPolicy(ExecutionPolicy{AllowShell: true}))
	if handler.executionAllowed("MCP_ENABLE_RUN_SCRIPT") || !handler.executionAllowed("MCP_ENABLE_SHELL") {
		t.Fatal("explicit policy did not remain kind-specific")
	}
}
