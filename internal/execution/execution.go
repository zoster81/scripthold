package execution

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultTimeoutSeconds   = 60
	MaximumTimeoutSeconds   = 600
	DefaultOutputLimitBytes = 256 * 1024
	DefaultTimeout          = DefaultTimeoutSeconds * time.Second
	processWaitDelay        = 2 * time.Second
	windowsTreeKillTimeout  = time.Second
)

// Request contains only process-level settings. Callers remain responsible for
// applying their own authorization policy before preparing a process.
type Request struct {
	Program          string
	Args             []string
	WorkingDirectory string
	TimeoutSeconds   int
	OutputLimitBytes int
	Environment      []string
}

// Result is the common structured output returned by execution tools.
type Result struct {
	WorkingDirectory   string `json:"workingDirectory"`
	ExitCode           int    `json:"exitCode"`
	Stdout             string `json:"stdout,omitempty"`
	Stderr             string `json:"stderr,omitempty"`
	TimedOut           bool   `json:"timedOut,omitempty"`
	OutputTruncated    bool   `json:"outputTruncated,omitempty"`
	DurationMillis     int64  `json:"durationMillis"`
	ExecutionCancelled bool   `json:"executionCancelled,omitempty"`
}

// Plan is an immutable, validated process launch plan.
type Plan struct {
	program     string
	args        []string
	cwd         string
	timeout     time.Duration
	outputLimit int
	environment []string
}

// Prepare validates process-level invariants without applying tool-specific
// authorization. In particular, it does not decide which executable, script,
// command text, or working directory a caller is allowed to use.
func Prepare(request Request) (Plan, error) {
	program := strings.TrimSpace(request.Program)
	if program == "" {
		return Plan{}, errors.New("program is required")
	}

	cwd, err := ValidateWorkingDirectory(request.WorkingDirectory)
	if err != nil {
		return Plan{}, err
	}

	timeout, err := resolveTimeout(request.TimeoutSeconds)
	if err != nil {
		return Plan{}, err
	}
	outputLimit := request.OutputLimitBytes
	if outputLimit == 0 {
		outputLimit = DefaultOutputLimitBytes
	}
	if outputLimit < 0 {
		return Plan{}, errors.New("output limit must be positive")
	}

	return Plan{
		program:     program,
		args:        append([]string(nil), request.Args...),
		cwd:         cwd,
		timeout:     timeout,
		outputLimit: outputLimit,
		environment: append([]string(nil), request.Environment...),
	}, nil
}

// ValidateWorkingDirectory verifies the process-level cwd invariant without
// applying any authorization policy and returns its cleaned absolute path.
func ValidateWorkingDirectory(cwd string) (string, error) {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return "", errors.New("working directory is required")
	}
	if !filepath.IsAbs(cwd) {
		return "", fmt.Errorf("working directory must be absolute: %s", cwd)
	}
	cwd = filepath.Clean(cwd)
	info, err := os.Stat(cwd)
	if err != nil {
		return "", fmt.Errorf("failed to inspect working directory: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("working directory is not a directory: %s", cwd)
	}
	return cwd, nil
}

// ValidateTimeoutSeconds checks the public execution timeout bounds.
func ValidateTimeoutSeconds(seconds int) error {
	_, err := resolveTimeout(seconds)
	return err
}

func resolveTimeout(seconds int) (time.Duration, error) {
	if seconds == 0 {
		return DefaultTimeout, nil
	}
	if seconds < 1 || seconds > MaximumTimeoutSeconds {
		return 0, fmt.Errorf("timeoutSeconds must be between 1 and %d", MaximumTimeoutSeconds)
	}
	return time.Duration(seconds) * time.Second, nil
}

// Run revalidates caller-owned security assumptions immediately before launch,
// then executes the prepared process with bounded output and cancellation.
func (plan Plan) Run(parent context.Context, revalidate func() error) (Result, error) {
	started := time.Now()
	if parent == nil {
		parent = context.Background()
	}
	if err := parent.Err(); err != nil {
		return cancelledResult(plan.cwd, started, err), nil
	}
	if revalidate != nil {
		if err := revalidate(); err != nil {
			return Result{}, err
		}
	}
	if err := parent.Err(); err != nil {
		return cancelledResult(plan.cwd, started, err), nil
	}

	ctx, cancel := context.WithTimeout(parent, plan.timeout)
	defer cancel()

	stdout := newLimitedBuffer(plan.outputLimit)
	stderr := newLimitedBuffer(plan.outputLimit)
	cmd := exec.Command(plan.program, plan.args...)
	cmd.Dir = plan.cwd
	if plan.environment != nil {
		cmd.Env = append([]string(nil), plan.environment...)
	}
	cmd.Stdin = nil
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = processWaitDelay

	if err := cmd.Start(); err != nil {
		return Result{}, fmt.Errorf("failed to start process: %w", err)
	}

	waitResult := make(chan error, 1)
	go func() {
		waitResult <- cmd.Wait()
	}()

	var runErr error
	timedOut := false
	cancelled := false
	select {
	case runErr = <-waitResult:
	case <-ctx.Done():
		timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
		cancelled = !timedOut
		terminateProcessTree(cmd)
		runErr = <-waitResult
	}

	exitCode := 0
	if runErr != nil {
		var exitError *exec.ExitError
		if errors.As(runErr, &exitError) {
			exitCode = exitError.ExitCode()
		} else if timedOut || cancelled {
			exitCode = -1
		} else {
			return Result{}, fmt.Errorf("failed while waiting for process: %w", runErr)
		}
	}

	return Result{
		WorkingDirectory:   plan.cwd,
		ExitCode:           exitCode,
		Stdout:             stdout.String(),
		Stderr:             stderr.String(),
		TimedOut:           timedOut,
		OutputTruncated:    stdout.Truncated() || stderr.Truncated(),
		DurationMillis:     time.Since(started).Milliseconds(),
		ExecutionCancelled: cancelled,
	}, nil
}

func cancelledResult(cwd string, started time.Time, err error) Result {
	return Result{
		WorkingDirectory:   cwd,
		ExitCode:           -1,
		TimedOut:           errors.Is(err, context.DeadlineExceeded),
		ExecutionCancelled: !errors.Is(err, context.DeadlineExceeded),
		DurationMillis:     time.Since(started).Milliseconds(),
	}
}

func terminateProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if runtime.GOOS == "windows" {
		ctx, cancel := context.WithTimeout(context.Background(), windowsTreeKillTimeout)
		killer := exec.CommandContext(ctx, "taskkill", "/PID", strconv.Itoa(cmd.Process.Pid), "/T", "/F")
		_ = killer.Run()
		cancel()
	}
	_ = cmd.Process.Kill()
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	return &limitedBuffer{limit: limit}
}

func (buffer *limitedBuffer) Write(payload []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	originalLength := len(payload)
	remaining := buffer.limit - buffer.buffer.Len()
	if remaining > 0 {
		writeLength := len(payload)
		if writeLength > remaining {
			writeLength = remaining
		}
		_, _ = buffer.buffer.Write(payload[:writeLength])
	}
	if originalLength > remaining {
		buffer.truncated = true
	}
	return originalLength, nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func (buffer *limitedBuffer) Truncated() bool {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.truncated
}
