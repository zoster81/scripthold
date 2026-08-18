package taskstore

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExecutorClassifiesInheritedOutputDrainTimeout(t *testing.T) {
	store := newTestStore(t)
	admitted, err := store.Submit(context.Background(), shellRequest("executor-output-drain-timeout"))
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	pidFile := filepath.Join(tempDir, "output-holder.pid")
	exitMarker := filepath.Join(tempDir, "direct-child-exiting")
	defer killRecordedTaskstoreProcess(t, pidFile)
	token := testRandomHex(t, 32)
	if err := writeJSONExclusive(filepath.Join(store.taskDir(admitted.Task.ID), "launch.json"), launchRecord{
		Program:          os.Args[0],
		Args:             []string{"-test.run=TestTaskstoreWaitDelayHelperProcess", "--", "spawn-output-holder", pidFile, exitMarker},
		WorkingDirectory: os.TempDir(),
		ExecutorToken:    token,
	}); err != nil {
		t.Fatal(err)
	}
	state, err := store.latestState(admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.appendStateUnlocked(admitted.Task.ID, stateRecord{
		Status:        StatusStarting,
		Revision:      state.Revision + 1,
		UpdatedAt:     time.Now().UTC(),
		ExecutorToken: token,
	}); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- RunExecutor(context.Background(), store, admitted.Task.ID, token) }()

	markerDeadline := time.Now().Add(10 * time.Second)
	for !fileExists(exitMarker) && time.Now().Before(markerDeadline) {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			t.Fatal("RunExecutor() returned before the direct child exit marker")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
	if !fileExists(exitMarker) {
		t.Fatal("direct child exit marker was not observed")
	}

	drainStarted := time.Now()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("RunExecutor() did not finish within 10s after the direct child began exiting")
	}
	if elapsed := time.Since(drainStarted); elapsed > 10*time.Second {
		t.Fatalf("RunExecutor() took %v after the direct child began exiting; want <= 10s", elapsed)
	}
	task, err := store.Get(context.Background(), admitted.Task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusFailed || task.Result == nil {
		t.Fatalf("terminal task = %#v, want failed result", task)
	}
	if task.Result.ErrorCode != "PROCESS_OUTPUT_DRAIN_TIMEOUT" {
		t.Fatalf("error code = %q, want PROCESS_OUTPUT_DRAIN_TIMEOUT", task.Result.ErrorCode)
	}
	if task.Result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want direct process exit code 0", task.Result.ExitCode)
	}
}

func killRecordedTaskstoreProcess(t *testing.T, path string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		return
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil || pid <= 0 {
		return
	}
	process, err := os.FindProcess(pid)
	if err == nil {
		_ = process.Kill()
	}
}

func TestTaskstoreWaitDelayHelperProcess(t *testing.T) {
	separator := -1
	for i, arg := range os.Args {
		if arg == "--" {
			separator = i
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		return
	}

	switch os.Args[separator+1] {
	case "spawn-output-holder":
		if separator+3 >= len(os.Args) {
			os.Exit(2)
		}
		cmd := exec.Command(os.Args[0], "-test.run=TestTaskstoreWaitDelayHelperProcess", "--", "hold-output")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Args[separator+2], []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
			_ = cmd.Process.Kill()
			os.Exit(4)
		}
		if err := os.WriteFile(os.Args[separator+3], []byte("exiting"), 0o600); err != nil {
			_ = cmd.Process.Kill()
			os.Exit(5)
		}
		os.Exit(0)
	case "hold-output":
		time.Sleep(15 * time.Second)
		os.Exit(0)
	}
}
