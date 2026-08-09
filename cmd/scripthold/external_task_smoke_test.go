package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestExternalDurableTaskLifecycle is an opt-in release gate using the exact
// built binary. It proves queue recovery, frontend independence, worker-crash
// survival, parallel dispatch, logical-lock serialization, logs, and cancel.
func TestExternalDurableTaskLifecycle(t *testing.T) {
	executable := os.Getenv(externalSmokeExecutableEnv)
	if executable == "" {
		t.Skipf("%s is not configured", externalSmokeExecutableEnv)
	}
	base := t.TempDir()
	publicRoot := filepath.Join(base, "public")
	storeRoot := filepath.Join(base, "private", "tasks")
	if err := os.MkdirAll(publicRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	environment := append(os.Environ(),
		"MCP_TASK_STORE_DIR="+storeRoot,
		"MCP_ENABLE_EXECUTION=1",
		"MCP_TASK_MAX_CONCURRENCY=2",
		"MCP_TASK_MAX_QUEUED=16",
		"MCP_TASK_MAX_LOG_BYTES_PER_STREAM=65536",
		"MCP_TASK_RETENTION_DAYS=1",
		"MCP_TASK_MAX_TERMINAL=100",
		"MCP_TASK_MAX_TOTAL_BYTES=16777216",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	worker := startExternalTaskWorker(t, ctx, executable, publicRoot, environment, storeRoot)
	defer func() { stopExternalProcess(worker) }()

	// Closing and recreating the MCP stdio process must not affect the task.
	session, command, stderr := connectExternalTaskFrontend(t, ctx, executable, publicRoot, environment)
	taskID := submitExternalTask(t, ctx, session, "frontend-restart", sleepThenOutputCommand(2, "frontend-survived"), nil)
	_ = session.Close()
	_ = command.Wait()
	if stderr.Len() != 0 && strings.Contains(strings.ToLower(stderr.String()), "error") {
		t.Fatalf("frontend stderr: %s", stderr.String())
	}
	session, command, _ = connectExternalTaskFrontend(t, ctx, executable, publicRoot, environment)
	waitExternalTaskStatus(t, ctx, session, taskID, "succeeded", 15*time.Second)
	assertExternalTaskLog(t, ctx, session, taskID, "frontend-survived")
	_ = session.Close()
	_ = command.Wait()

	// A helper already running must survive a hard worker exit.
	session, command, _ = connectExternalTaskFrontend(t, ctx, executable, publicRoot, environment)
	crashTask := submitExternalTask(t, ctx, session, "worker-crash", sleepThenOutputCommand(3, "worker-survived"), nil)
	waitExternalTaskStatus(t, ctx, session, crashTask, "running", 10*time.Second)
	stopExternalProcess(worker)
	worker = nil
	waitExternalTaskStatus(t, ctx, session, crashTask, "succeeded", 12*time.Second)
	assertExternalTaskLog(t, ctx, session, crashTask, "worker-survived")

	// Admission while the worker is offline remains queued and is recovered.
	queuedTask := submitExternalTask(t, ctx, session, "offline-queue", outputCommand("queue-recovered"), nil)
	waitExternalTaskStatus(t, ctx, session, queuedTask, "queued", 3*time.Second)
	worker = startExternalTaskWorker(t, ctx, executable, publicRoot, environment, storeRoot)
	waitExternalTaskStatus(t, ctx, session, queuedTask, "succeeded", 10*time.Second)

	// Two unlocked tasks should overlap under concurrency=2.
	parallelA := submitExternalTask(t, ctx, session, "parallel-a", sleepThenOutputCommand(2, "a"), nil)
	parallelB := submitExternalTask(t, ctx, session, "parallel-b", sleepThenOutputCommand(2, "b"), nil)
	a := waitExternalTaskStatus(t, ctx, session, parallelA, "succeeded", 12*time.Second)
	b := waitExternalTaskStatus(t, ctx, session, parallelB, "succeeded", 12*time.Second)
	if a.StartedAt == nil || b.StartedAt == nil || absDuration(a.StartedAt.Sub(*b.StartedAt)) > 1500*time.Millisecond {
		t.Fatalf("parallel tasks did not overlap: %#v %#v", a, b)
	}

	// A shared lock key must serialize otherwise parallel tasks.
	lockedA := submitExternalTask(t, ctx, session, "locked-a", sleepThenOutputCommand(1, "la"), []string{"workspace"})
	lockedB := submitExternalTask(t, ctx, session, "locked-b", sleepThenOutputCommand(1, "lb"), []string{"workspace"})
	la := waitExternalTaskStatus(t, ctx, session, lockedA, "succeeded", 12*time.Second)
	lb := waitExternalTaskStatus(t, ctx, session, lockedB, "succeeded", 12*time.Second)
	if la.FinishedAt == nil || lb.StartedAt == nil || lb.StartedAt.Before(*la.FinishedAt) {
		t.Fatalf("logical lock did not serialize tasks: %#v %#v", la, lb)
	}

	cancelTask := submitExternalTask(t, ctx, session, "cancel", sleepThenOutputCommand(30, "never"), nil)
	waitExternalTaskStatus(t, ctx, session, cancelTask, "running", 10*time.Second)
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "task_cancel", Arguments: map[string]any{"taskId": cancelTask, "reason": "integration test"}})
	if err != nil || result.IsError {
		t.Fatalf("task_cancel result=%#v err=%v", result, err)
	}
	waitExternalTaskStatus(t, ctx, session, cancelTask, "cancelled", 12*time.Second)
	_ = session.Close()
	_ = command.Wait()
}

// TestExternalTaskSupervisorRecovery uses the exact built executable and the
// same `--` form used by the launchers. It proves worker restart, adoption of a
// worker whose original supervisor died, and task survival across that death.
func TestExternalTaskSupervisorRecovery(t *testing.T) {
	executable := os.Getenv(externalSmokeExecutableEnv)
	if executable == "" {
		t.Skipf("%s is not configured", externalSmokeExecutableEnv)
	}
	base := t.TempDir()
	publicRoot := filepath.Join(base, "public")
	storeRoot := filepath.Join(base, "private", "tasks")
	if err := os.MkdirAll(publicRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	environment := append(os.Environ(),
		"MCP_TASK_STORE_DIR="+storeRoot,
		"MCP_ENABLE_EXECUTION=1",
		"MCP_TASK_MAX_CONCURRENCY=2",
		"MCP_TASK_MAX_QUEUED=16",
		"MCP_TASK_MAX_LOG_BYTES_PER_STREAM=65536",
		"MCP_TASK_RETENTION_DAYS=1",
		"MCP_TASK_MAX_TERMINAL=100",
		"MCP_TASK_MAX_TOTAL_BYTES=16777216",
	)
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	supervisor, workerPID := startExternalTaskSupervisor(t, ctx, executable, publicRoot, environment, storeRoot)
	defer func() {
		stopExternalProcess(supervisor)
		stopExternalPID(workerPID)
	}()

	// Kill the worker itself: the supervisor must create a different one.
	stopExternalPID(workerPID)
	workerPID = waitExternalSupervisorWorker(t, supervisor.Process.Pid, workerPID, storeRoot, 15*time.Second)

	session, frontend, _ := connectExternalTaskFrontend(t, ctx, executable, publicRoot, environment)
	defer func() {
		_ = session.Close()
		_ = frontend.Wait()
	}()
	taskID := submitExternalTask(t, ctx, session, "supervisor-death", sleepThenOutputCommand(2, "supervisor-independent"), nil)
	waitExternalTaskStatus(t, ctx, session, taskID, "running", 10*time.Second)

	// A hard supervisor death must not own or terminate worker/executor lifetime.
	stopExternalProcess(supervisor)
	supervisor = nil
	waitExternalTaskStatus(t, ctx, session, taskID, "succeeded", 12*time.Second)
	assertExternalTaskLog(t, ctx, session, taskID, "supervisor-independent")

	// A new supervisor first adopts the existing worker, then restarts it after
	// failure even though the worker was created by the previous supervisor.
	supervisor = startExternalSupervisorProcess(t, ctx, executable, publicRoot, environment)
	waitExternalFreshFile(t, filepath.Join(storeRoot, supervisorHeartbeatName), 10*time.Second)
	stopExternalPID(workerPID)
	workerPID = waitExternalSupervisorWorker(t, supervisor.Process.Pid, workerPID, storeRoot, 15*time.Second)

	recovered := submitExternalTask(t, ctx, session, "supervisor-recovered", outputCommand("restarted-worker"), nil)
	waitExternalTaskStatus(t, ctx, session, recovered, "succeeded", 10*time.Second)
}

type externalTaskState struct {
	Status                string
	StartedAt, FinishedAt *time.Time
}

func startExternalTaskWorker(t *testing.T, ctx context.Context, executable, root string, environment []string, storeRoot string) *exec.Cmd {
	t.Helper()
	command := exec.CommandContext(ctx, executable, "task-worker", root)
	command.Env = environment
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(filepath.Join(storeRoot, workerHeartbeatName)); err == nil && time.Since(info.ModTime()) < 3*time.Second {
			return command
		}
		if command.ProcessState != nil && command.ProcessState.Exited() {
			t.Fatal("task worker exited before readiness")
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopExternalProcess(command)
	t.Fatal("task worker readiness timeout")
	return nil
}

const workerHeartbeatName = "worker.heartbeat"
const supervisorHeartbeatName = "supervisor.heartbeat"

func startExternalTaskSupervisor(t *testing.T, ctx context.Context, executable, root string, environment []string, storeRoot string) (*exec.Cmd, int) {
	t.Helper()
	supervisor := startExternalSupervisorProcess(t, ctx, executable, root, environment)
	waitExternalFreshFile(t, filepath.Join(storeRoot, supervisorHeartbeatName), 10*time.Second)
	waitExternalFreshFile(t, filepath.Join(storeRoot, workerHeartbeatName), 10*time.Second)
	return supervisor, waitExternalSupervisorWorker(t, supervisor.Process.Pid, 0, storeRoot, 10*time.Second)
}

func startExternalSupervisorProcess(t *testing.T, ctx context.Context, executable, root string, environment []string) *exec.Cmd {
	t.Helper()
	command := exec.CommandContext(ctx, executable, "task-supervisor", "--", root)
	command.Env = environment
	command.Stdout = nil
	command.Stderr = nil
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	return command
}

func waitExternalSupervisorWorker(t *testing.T, supervisorPID, previousPID int, storeRoot string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ids, err := externalChildProcessIDs(supervisorPID)
		if err == nil {
			for _, id := range ids {
				if id > 0 && id != previousPID {
					if info, statErr := os.Stat(filepath.Join(storeRoot, workerHeartbeatName)); statErr == nil && time.Since(info.ModTime()) < 3*time.Second {
						return id
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("supervisor %d did not expose a fresh worker after pid %d", supervisorPID, previousPID)
	return 0
}

func waitExternalFreshFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < 3*time.Second {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("fresh heartbeat not found: %s", path)
}

func connectExternalTaskFrontend(t *testing.T, ctx context.Context, executable, root string, environment []string) (*mcp.ClientSession, *exec.Cmd, *bytes.Buffer) {
	t.Helper()
	command := exec.CommandContext(ctx, executable, root)
	command.Env = environment
	var stderr bytes.Buffer
	command.Stderr = &stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "durable-task-smoke", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command, TerminateDuration: 3 * time.Second}, nil)
	if err != nil {
		t.Fatalf("connect frontend: %v; stderr=%s", err, stderr.String())
	}
	return session, command, &stderr
}

func submitExternalTask(t *testing.T, ctx context.Context, session *mcp.ClientSession, key, command string, locks []string) string {
	t.Helper()
	arguments := map[string]any{"kind": "shell", "idempotencyKey": key, "name": key, "command": command}
	if len(locks) > 0 {
		arguments["lockKeys"] = locks
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "task_run", Arguments: arguments})
	if err != nil || result.IsError {
		t.Fatalf("task_run %s result=%#v err=%v", key, result, err)
	}
	content, ok := result.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("task_run structured content = %T", result.StructuredContent)
	}
	task, ok := content["task"].(map[string]any)
	if !ok {
		t.Fatalf("task field = %T", content["task"])
	}
	id, _ := task["taskId"].(string)
	if id == "" {
		t.Fatalf("task_run returned no taskId: %#v", content)
	}
	return id
}

func waitExternalTaskStatus(t *testing.T, ctx context.Context, session *mcp.ClientSession, id, wanted string, timeout time.Duration) externalTaskState {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last externalTaskState
	for time.Now().Before(deadline) {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "task_get", Arguments: map[string]any{"taskId": id}})
		if err == nil && !result.IsError {
			content, _ := result.StructuredContent.(map[string]any)
			last.Status, _ = content["status"].(string)
			last.StartedAt = parseExternalTime(content["startedAt"])
			last.FinishedAt = parseExternalTime(content["finishedAt"])
			if last.Status == wanted {
				return last
			}
			if isExternalTerminal(last.Status) && last.Status != wanted {
				t.Fatalf("task %s reached %s, want %s", id, last.Status, wanted)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("task %s status=%s, want %s", id, last.Status, wanted)
	return last
}

func assertExternalTaskLog(t *testing.T, ctx context.Context, session *mcp.ClientSession, id, wanted string) {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "task_logs", Arguments: map[string]any{"taskId": id}})
	if err != nil || result.IsError {
		t.Fatalf("task_logs result=%#v err=%v", result, err)
	}
	content, _ := result.StructuredContent.(map[string]any)
	stdout, _ := content["stdout"].(map[string]any)
	data, _ := stdout["data"].(string)
	if !strings.Contains(data, wanted) {
		t.Fatalf("stdout %q does not contain %q", data, wanted)
	}
}

func parseExternalTime(value any) *time.Time {
	text, _ := value.(string)
	if text == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil
	}
	return &parsed
}
func isExternalTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "timed_out", "cancelled", "interrupted":
		return true
	}
	return false
}
func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}
func stopExternalProcess(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = command.Process.Kill()
	_, _ = command.Process.Wait()
}

func stopExternalPID(pid int) {
	if pid <= 0 {
		return
	}
	if process, err := os.FindProcess(pid); err == nil {
		_ = process.Kill()
		_, _ = process.Wait()
	}
}

func sleepThenOutputCommand(seconds int, text string) string {
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("Start-Sleep -Seconds %d; Write-Output '%s'", seconds, text)
	}
	return fmt.Sprintf("sleep %d; printf '%s\\n'", seconds, text)
}
func outputCommand(text string) string {
	if runtime.GOOS == "windows" {
		return "Write-Output '" + text + "'"
	}
	return "printf '" + text + "\\n'"
}
