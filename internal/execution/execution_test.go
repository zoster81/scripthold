package execution

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPrepareDefaultsAndCopiesArguments(t *testing.T) {
	args := []string{"-test.run=TestExecutionHelperProcess", "--", "success"}
	environment := []string{"VERIFY_ENV=present"}
	plan, err := Prepare(Request{
		Program:          os.Args[0],
		Args:             args,
		WorkingDirectory: t.TempDir(),
		Environment:      environment,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	args[0] = "modified"
	environment[0] = "VERIFY_ENV=modified"
	if got, want := plan.timeout, DefaultTimeout; got != want {
		t.Fatalf("timeout = %v, want %v", got, want)
	}
	if got, want := plan.outputLimit, DefaultOutputLimitBytes; got != want {
		t.Fatalf("output limit = %d, want %d", got, want)
	}
	if plan.args[0] == "modified" {
		t.Fatal("Prepare() retained the caller's argument slice")
	}
	if plan.environment[0] == "VERIFY_ENV=modified" {
		t.Fatal("Prepare() retained the caller's environment slice")
	}
}

func TestPrepareRejectsInvalidInputs(t *testing.T) {
	file := filepath.Join(t.TempDir(), "not-a-directory.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		request Request
		match   string
	}{
		{name: "empty program", request: Request{WorkingDirectory: t.TempDir()}, match: "program is required"},
		{name: "empty cwd", request: Request{Program: os.Args[0]}, match: "working directory is required"},
		{name: "cwd is file", request: Request{Program: os.Args[0], WorkingDirectory: file}, match: "working directory is not a directory"},
		{name: "timeout too low", request: Request{Program: os.Args[0], WorkingDirectory: t.TempDir(), TimeoutSeconds: -1}, match: "timeoutSeconds must be between"},
		{name: "timeout too high", request: Request{Program: os.Args[0], WorkingDirectory: t.TempDir(), TimeoutSeconds: MaximumTimeoutSeconds + 1}, match: "timeoutSeconds must be between"},
		{name: "negative output limit", request: Request{Program: os.Args[0], WorkingDirectory: t.TempDir(), OutputLimitBytes: -1}, match: "output limit must be positive"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Prepare(test.request)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("Prepare() error = %v, want substring %q", err, test.match)
			}
		})
	}
}

func TestRunRevalidatesBeforeStarting(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "started.txt")
	plan, err := Prepare(Request{
		Program:          os.Args[0],
		Args:             []string{"-test.run=TestExecutionHelperProcess", "--", "marker", marker},
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	guardErr := errors.New("path changed")
	_, err = plan.Run(context.Background(), func() error { return guardErr })
	if !errors.Is(err, guardErr) {
		t.Fatalf("Run() error = %v, want %v", err, guardErr)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("process started despite failed revalidation: stat error = %v", statErr)
	}
}

func TestRunCapturesExitAndTruncation(t *testing.T) {
	plan, err := Prepare(Request{
		Program:          os.Args[0],
		Args:             []string{"-test.run=TestExecutionHelperProcess", "--", "output"},
		WorkingDirectory: t.TempDir(),
		OutputLimitBytes: 16,
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := plan.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
	if !result.OutputTruncated {
		t.Fatal("expected output truncation")
	}
	if len(result.Stdout) != 16 || len(result.Stderr) != 16 {
		t.Fatalf("captured lengths = stdout %d, stderr %d; want 16 each", len(result.Stdout), len(result.Stderr))
	}
}

func TestRunUsesExplicitEnvironment(t *testing.T) {
	plan, err := Prepare(Request{
		Program:          os.Args[0],
		Args:             []string{"-test.run=TestExecutionHelperProcess", "--", "environment"},
		WorkingDirectory: t.TempDir(),
		Environment:      []string{"VERIFY_ENV=present"},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := plan.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Stdout != "present" {
		t.Fatalf("environment result=%+v", result)
	}
}

func TestRunHonorsParentCancellation(t *testing.T) {
	plan, err := Prepare(Request{
		Program:          os.Args[0],
		Args:             []string{"-test.run=TestExecutionHelperProcess", "--", "sleep"},
		WorkingDirectory: t.TempDir(),
		TimeoutSeconds:   5,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := plan.Run(ctx, nil)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.ExecutionCancelled || result.TimedOut {
		t.Fatalf("cancellation flags = cancelled %v, timedOut %v", result.ExecutionCancelled, result.TimedOut)
	}
}

func TestRunBoundsInheritedOutputPipeLeak(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "output-holder.pid")
	plan, err := Prepare(Request{
		Program:          os.Args[0],
		Args:             []string{"-test.run=TestExecutionHelperProcess", "--", "spawn-output-holder", pidFile},
		WorkingDirectory: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = plan.Run(context.Background(), nil)
	killRecordedProcess(t, pidFile)

	if !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("Run() error = %v, want errors.Is(exec.ErrWaitDelay)", err)
	}
}

func killRecordedProcess(t *testing.T, path string) {
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

func TestExecutionHelperProcess(t *testing.T) {
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
	case "success":
		os.Exit(0)
	case "marker":
		if err := os.WriteFile(os.Args[separator+2], []byte("started"), 0o600); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	case "output":
		_, _ = os.Stdout.WriteString(strings.Repeat("o", 64))
		_, _ = os.Stderr.WriteString(strings.Repeat("e", 64))
		os.Exit(7)
	case "sleep":
		time.Sleep(10 * time.Second)
		os.Exit(0)
	case "spawn-output-holder":
		cmd := exec.Command(os.Args[0], "-test.run=TestExecutionHelperProcess", "--", "hold-output")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(3)
		}
		if err := os.WriteFile(os.Args[separator+2], []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
			_ = cmd.Process.Kill()
			os.Exit(4)
		}
		os.Exit(0)
	case "hold-output":
		terminated := make(chan os.Signal, 1)
		signal.Notify(terminated, os.Interrupt)
		defer signal.Stop(terminated)
		<-terminated
		os.Exit(0)
	case "environment":
		_, _ = os.Stdout.WriteString(os.Getenv("VERIFY_ENV"))
		os.Exit(0)
	}
}
