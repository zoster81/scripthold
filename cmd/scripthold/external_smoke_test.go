package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	externalSmokeExecutableEnv = "MCP_EXTERNAL_SMOKE_EXECUTABLE"
	externalSmokeArgsEnv       = "MCP_EXTERNAL_SMOKE_ARGS_JSON"
	externalSmokeVersionEnv    = "MCP_EXTERNAL_SMOKE_EXPECTED_VERSION"
	externalSmokeRootEnv       = "MCP_EXTERNAL_SMOKE_EXPECTED_ROOT"
	externalSmokeTempRoot      = "{TEMP_ROOT}"
)

func TestExternalStdioBinarySmoke(t *testing.T) {
	executable := os.Getenv(externalSmokeExecutableEnv)
	if executable == "" {
		t.Skipf("%s is not configured", externalSmokeExecutableEnv)
	}

	var args []string
	if raw := os.Getenv(externalSmokeArgsEnv); raw != "" {
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatalf("decode %s: %v", externalSmokeArgsEnv, err)
		}
	}

	expectedRoot := os.Getenv(externalSmokeRootEnv)
	var tempRoot string
	for index, arg := range args {
		if !strings.Contains(arg, externalSmokeTempRoot) {
			continue
		}
		if tempRoot == "" {
			tempRoot = t.TempDir()
			if err := os.Chmod(tempRoot, 0o755); err != nil {
				t.Fatalf("make temporary smoke root traversable: %v", err)
			}
		}
		args[index] = strings.ReplaceAll(arg, externalSmokeTempRoot, tempRoot)
	}
	if expectedRoot == externalSmokeTempRoot {
		if tempRoot == "" {
			t.Fatal("expected temporary root requested without a temporary-root argument")
		}
		expectedRoot = tempRoot
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, executable, args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "external-smoke", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{
		Command:           command,
		TerminateDuration: 5 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("connect to external MCP process: %v; stderr: %s", err, stderr.String())
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("close external MCP session: %v; stderr: %s", err, stderr.String())
		}
	}()

	initialize := session.InitializeResult()
	if initialize == nil || initialize.ServerInfo == nil {
		t.Fatal("external MCP process returned no server information")
	}
	if expected := os.Getenv(externalSmokeVersionEnv); expected != "" && initialize.ServerInfo.Version != expected {
		t.Errorf("server version = %q, want %q", initialize.ServerInfo.Version, expected)
	}

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if got := len(tools.Tools); got != 35 {
		t.Fatalf("tool count = %d, want 35", got)
	}

	roots, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "list_allowed_directories"})
	if err != nil {
		t.Fatalf("call list_allowed_directories: %v", err)
	}
	if roots.IsError {
		t.Fatalf("list_allowed_directories returned an error: %#v", roots.Content)
	}

	if expectedRoot == "" {
		return
	}
	structured, ok := roots.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("list_allowed_directories structured content type = %T", roots.StructuredContent)
	}
	directories, ok := structured["directories"].([]any)
	if !ok {
		t.Fatalf("directories field type = %T", structured["directories"])
	}
	foundRoot := false
	for _, value := range directories {
		actualRoot, ok := value.(string)
		if ok && equivalentSmokeRoot(actualRoot, expectedRoot) {
			foundRoot = true
			break
		}
	}
	if !foundRoot {
		t.Fatalf("allowed directories = %#v, want %q", directories, expectedRoot)
	}

	if tempRoot == "" {
		return
	}
	sourcePath := filepath.Join(tempRoot, "r25-smoke.go")
	if err := os.WriteFile(sourcePath, []byte("package smoke\nfunc Work() {}\n"), 0o644); err != nil {
		t.Fatalf("write R25 stdio smoke source: %v", err)
	}
	sourceRequestPath := smokeSourceRequestPath(tempRoot, expectedRoot, filepath.Base(sourcePath))
	sourceResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "source_symbols",
		Arguments: map[string]any{
			"operation":         "outline",
			"paths":             []string{sourceRequestPath},
			"language":          "go",
			"encoding":          "utf-8",
			"includeSignatures": true,
		},
	})
	if err != nil {
		t.Fatalf("call source_symbols: %v", err)
	}
	if sourceResult.IsError {
		t.Fatalf("source_symbols returned an error: %#v", sourceResult.Content)
	}
	encodedSource, err := json.Marshal(sourceResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal source_symbols output: %v", err)
	}
	var sourceOutput struct {
		Operation        string `json:"operation"`
		FilesParsed      int    `json:"filesParsed"`
		SymbolCount      int    `json:"symbolCount"`
		CoverageComplete bool   `json:"coverageComplete"`
	}
	if err := json.Unmarshal(encodedSource, &sourceOutput); err != nil {
		t.Fatalf("decode source_symbols output: %v", err)
	}
	if sourceOutput.Operation != "outline" || sourceOutput.FilesParsed != 1 || sourceOutput.SymbolCount < 2 || !sourceOutput.CoverageComplete {
		t.Fatalf("unexpected source_symbols stdio smoke output: %#v", sourceOutput)
	}
}

func TestSmokeSourceRequestPath(t *testing.T) {
	tempRoot := t.TempDir()
	resolvedRoot, err := filepath.EvalSymlinks(tempRoot)
	if err != nil {
		t.Fatalf("resolve temp root: %v", err)
	}
	if got, want := smokeSourceRequestPath(tempRoot, tempRoot, "sample.go"), filepath.Join(filepath.Clean(resolvedRoot), "sample.go"); got != want {
		t.Fatalf("same-root source request path = %q, want %q", got, want)
	}
	if got, want := smokeSourceRequestPath(tempRoot, "/data", "sample.go"), "/data/sample.go"; got != want {
		t.Fatalf("child-root source request path = %q, want %q", got, want)
	}
}

func smokeSourceRequestPath(tempRoot, expectedRoot, name string) string {
	if equivalentSmokeRoot(tempRoot, expectedRoot) {
		root := tempRoot
		if resolved, err := filepath.EvalSymlinks(tempRoot); err == nil {
			root = filepath.Clean(resolved)
		}
		return filepath.Join(root, name)
	}
	return childVisibleSmokePath(expectedRoot, name)
}

func TestChildVisibleSmokePath(t *testing.T) {
	tests := map[string]struct {
		root string
		name string
		want string
	}{
		"POSIX root":          {root: "/data", name: "sample.go", want: "/data/sample.go"},
		"POSIX slash root":    {root: "/", name: "sample.go", want: "/sample.go"},
		"Windows backslashes": {root: `C:\data`, name: "sample.go", want: `C:\data\sample.go`},
		"Windows slashes":     {root: "C:/data", name: "sample.go", want: "C:/data/sample.go"},
	}
	for name, testCase := range tests {
		t.Run(name, func(t *testing.T) {
			if got := childVisibleSmokePath(testCase.root, testCase.name); got != testCase.want {
				t.Fatalf("childVisibleSmokePath(%q, %q) = %q, want %q", testCase.root, testCase.name, got, testCase.want)
			}
		})
	}
}

func childVisibleSmokePath(root, name string) string {
	trimmed := strings.TrimRight(root, `/\`)
	if trimmed == "" && strings.HasPrefix(root, "/") {
		return "/" + name
	}
	if strings.Contains(root, `\`) && !strings.Contains(root, "/") {
		return trimmed + `\` + name
	}
	return trimmed + "/" + name
}

func equivalentSmokeRoot(actual, expected string) bool {
	if actual == expected {
		return true
	}
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Clean(actual), filepath.Clean(expected)) {
		return true
	}
	actualInfo, actualErr := os.Stat(actual)
	expectedInfo, expectedErr := os.Stat(expected)
	return actualErr == nil && expectedErr == nil && os.SameFile(actualInfo, expectedInfo)
}
