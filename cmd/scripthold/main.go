package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/deferredoperation"
	"github.com/zoster81/scripthold/internal/diagnostics"
	"github.com/zoster81/scripthold/internal/security"
	"github.com/zoster81/scripthold/internal/taskstore"
)

// version is set at build time via ldflags.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(runCommand(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

func runCommand(ctx context.Context, args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	// Keep the legacy exported version synchronized for existing embedders while
	// the explicit server options remain authoritative for this process.
	filetoolsserver.Version = version

	if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
		fmt.Fprintln(stdout, version)
		return 0
	}

	diagnosticManager, err := diagnostics.Open(stderr, getenv, diagnosticRole(args))
	if err != nil {
		fmt.Fprintln(stderr, "Error: diagnostics logging configuration is invalid or unavailable")
		return 1
	}
	defer diagnosticManager.Close()
	previousLogger := slog.Default()
	slog.SetDefault(diagnosticManager.Server())
	defer slog.SetDefault(previousLogger)
	if len(args) > 0 && args[0] == "task-worker" {
		return runTaskWorkerCommand(ctx, args[1:], stderr, getenv)
	}
	if len(args) > 0 && args[0] == "task-supervisor" {
		return runTaskSupervisorCommand(ctx, args[1:], stderr, getenv)
	}
	if len(args) > 0 && args[0] == "_task-exec" {
		return runTaskExecutorCommand(ctx, args[1:], stderr, getenv)
	}
	if len(args) > 0 && args[0] == "_deferred-exec" {
		return runDeferredExecutorCommand(ctx, args[1:], stderr)
	}

	diagnosticOptions, diagnosticCommand, err := parseBackupDiagnosticCommand(args)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if diagnosticCommand {
		return runBackupDiagnosticCommand(ctx, diagnosticOptions, stdout, stderr, getenv)
	}

	recoveryOptions, recoveryCommand, err := parseBackupRecoveryCommand(args)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if recoveryCommand {
		return runBackupRecoveryCommand(ctx, recoveryOptions, stdout, stderr)
	}

	options, err := parseCommandOptions(args, loadCommandDefaults(getenv))
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	normalized, err := security.NormalizeAllowedDirs(options.allowedDirectories)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if len(normalized) > 0 {
		slog.Debug("normalized allowed directories", "dirs", normalized)
	}

	applicationConfig := config.LoadFromEnvironment(getenv)
	if err := validatePrivateStoreSeparation(applicationConfig); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	selection, err := selectRunnerWithAccessLogger(options.transport, getenv, applicationConfig.Limits.MaxSessions, diagnosticManager.HTTPAccess())
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	var store *backupstore.Store
	var tasks *taskstore.Store
	var deferredStore *deferredoperation.Store
	var deferredEngine *deferredoperation.Engine
	protectedDirectories := []string(nil)
	if applicationConfig.Backup.Enabled() {
		store, err = backupstore.Open(backupstore.Options{
			Directory:                applicationConfig.Backup.StoreDir,
			PublicAllowedDirectories: options.allowedDirectories,
			Limits:                   backupStoreLimits(applicationConfig.Backup.Limits),
		})
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			return 1
		}
		protectedDirectories = []string{store.Root()}
	}
	if applicationConfig.Tasks.Enabled() {
		tasks, err = taskstore.Open(applicationConfig.Tasks.StoreDir, options.allowedDirectories, taskStoreLimits(applicationConfig.Tasks))
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			if store != nil {
				_ = store.Close()
			}
			return 1
		}
		protectedDirectories = append(protectedDirectories, tasks.Root())
	}
	if applicationConfig.Reliability.Enabled() {
		otherPrivateRoots := make([]string, 0, 2)
		if store != nil {
			otherPrivateRoots = append(otherPrivateRoots, store.Root())
		}
		if tasks != nil {
			otherPrivateRoots = append(otherPrivateRoots, tasks.Root())
		}
		deferredStore, err = deferredoperation.Initialize(
			applicationConfig.Reliability.StoreDir,
			normalized,
			otherPrivateRoots,
			deferredStoreLimits(applicationConfig.Reliability, applicationConfig.Limits.MaxOutputBytes),
		)
		if err != nil {
			fmt.Fprintf(stderr, "Error: %v\n", err)
			if store != nil {
				_ = store.Close()
			}
			return 1
		}
		executable, executableErr := os.Executable()
		if executableErr != nil {
			fmt.Fprintln(stderr, "Error: deferred operation executable is unavailable")
			if store != nil {
				_ = store.Close()
			}
			return 1
		}
		deferredEngine, err = deferredoperation.NewEngine(deferredStore, executable)
		if err != nil {
			fmt.Fprintln(stderr, "Error: deferred operation engine is unavailable")
			if store != nil {
				_ = store.Close()
			}
			return 1
		}
		protectedDirectories = append(protectedDirectories, deferredStore.Root())
	}

	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:                version,
		AllowedDirectories:     normalized,
		ProtectedDirectories:   protectedDirectories,
		BackupStore:            store,
		TaskStore:              tasks,
		DeferredEngine:         deferredEngine,
		ToolLogger:             diagnosticManager.Server(),
		Config:                 applicationConfig,
		ExecutionPolicy:        selection.executionPolicy,
		EnableClientRoots:      selection.enableClientRoots,
		DisableModernDiscovery: selection.disableModernDiscovery,
		LifecycleContext:       ctx,
	})

	runErr := selection.runner.Run(ctx, server)
	var closeErr error
	if store != nil {
		closeErr = store.Close()
	}
	if runErr != nil {
		if ctx.Err() != nil && errors.Is(runErr, ctx.Err()) && closeErr == nil {
			return 0
		}
		fmt.Fprintf(stderr, "Server error: %v\n", errors.Join(runErr, closeErr))
		return 1
	}
	if closeErr != nil {
		fmt.Fprintf(stderr, "Server error: backup store lock could not be released\n")
		return 1
	}
	return 0
}

func taskStoreLimits(cfg config.TaskConfig) taskstore.Limits {
	return taskstore.Limits{
		MaxConcurrency: cfg.MaxConcurrency, MaxQueued: cfg.MaxQueued,
		MaxLogBytesPerStream: cfg.MaxLogBytesPerStream, MaxRuntimeSeconds: cfg.MaxRuntimeSeconds,
		RetentionDays: cfg.RetentionDays, MaxTerminal: cfg.MaxTerminal, MaxTotalBytes: cfg.MaxTotalBytes,
	}
}

func validatePrivateStoreSeparation(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	roots := make([]string, 0, 3)
	if cfg.Tasks.Enabled() {
		roots = append(roots, cfg.Tasks.StoreDir)
	}
	if cfg.Backup.Enabled() {
		roots = append(roots, cfg.Backup.StoreDir)
	}
	if cfg.Reliability.Enabled() {
		roots = append(roots, cfg.Reliability.StoreDir)
	}
	absolute := make([]string, len(roots))
	for index, root := range roots {
		value, err := filepath.Abs(root)
		if err != nil {
			return errors.New("private store paths are invalid")
		}
		absolute[index] = filepath.Clean(value)
	}
	for first := 0; first < len(absolute); first++ {
		for second := first + 1; second < len(absolute); second++ {
			if security.PathsOverlap(absolute[first], absolute[second]) {
				return errors.New("private stores must use separate non-overlapping directories")
			}
		}
	}
	return nil
}

func runTaskWorkerCommand(ctx context.Context, directories []string, stderr io.Writer, getenv func(string) string) int {
	directories = trimInternalArgumentSeparator(directories)
	cfg := config.LoadFromEnvironment(getenv)
	if err := validatePrivateStoreSeparation(cfg); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if !cfg.Tasks.Enabled() {
		fmt.Fprintln(stderr, "Error: MCP_TASK_STORE_DIR is required for task-worker")
		return 1
	}
	normalized, err := security.NormalizeAllowedDirs(directories)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	store, err := taskstore.Initialize(cfg.Tasks.StoreDir, normalized, taskStoreLimits(cfg.Tasks))
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	policy := handler.ExecutionPolicyFromEnvironment(getenv)
	worker, err := taskstore.NewWorker(store, executable, normalized, taskstore.WorkerPolicy{AllowShell: policy.AllowShell, AllowRunScript: policy.AllowRunScript}, slog.Default())
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := worker.Run(ctx); err != nil {
		fmt.Fprintf(stderr, "Task worker error: %v\n", err)
		return 1
	}
	return 0
}

func runTaskSupervisorCommand(ctx context.Context, directories []string, stderr io.Writer, getenv func(string) string) int {
	directories = trimInternalArgumentSeparator(directories)
	cfg := config.LoadFromEnvironment(getenv)
	if err := validatePrivateStoreSeparation(cfg); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if !cfg.Tasks.Enabled() {
		fmt.Fprintln(stderr, "Error: MCP_TASK_STORE_DIR is required for task-supervisor")
		return 1
	}
	normalized, err := security.NormalizeAllowedDirs(directories)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	store, err := taskstore.Initialize(cfg.Tasks.StoreDir, normalized, taskStoreLimits(cfg.Tasks))
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if err := taskstore.RunSupervisor(ctx, store, executable, normalized, slog.Default()); err != nil {
		fmt.Fprintf(stderr, "Task supervisor error: %v\n", err)
		return 1
	}
	return 0
}

func trimInternalArgumentSeparator(arguments []string) []string {
	if len(arguments) > 0 && arguments[0] == "--" {
		return arguments[1:]
	}
	return arguments
}

func runTaskExecutorCommand(ctx context.Context, args []string, stderr io.Writer, getenv func(string) string) int {
	if len(args) != 2 {
		fmt.Fprintln(stderr, "Error: invalid internal task executor invocation")
		return 1
	}
	cfg := config.LoadFromEnvironment(getenv)
	if err := validatePrivateStoreSeparation(cfg); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	configuredStore, pathErr := filepath.Abs(cfg.Tasks.StoreDir)
	requestedStore, requestedErr := filepath.Abs(args[0])
	if !cfg.Tasks.Enabled() || pathErr != nil || requestedErr != nil || !security.PathsEqual(configuredStore, requestedStore) {
		fmt.Fprintln(stderr, "Error: task executor store mismatch")
		return 1
	}
	store, err := taskstore.OpenExecutor(cfg.Tasks.StoreDir, taskStoreLimits(cfg.Tasks))
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	token := getenv("MCP_TASK_EXECUTOR_TOKEN")
	_ = os.Unsetenv("MCP_TASK_EXECUTOR_TOKEN")
	if err := taskstore.RunExecutor(ctx, store, args[1], token); err != nil {
		fmt.Fprintf(stderr, "Task executor error: %v\n", err)
		return 1
	}
	return 0
}

func deferredStoreLimits(cfg config.ReliabilityConfig, maxOutputBytes int64) deferredoperation.Limits {
	maxResultBytes := maxOutputBytes
	if maxResultBytes > 0 && maxResultBytes <= (1<<62) {
		maxResultBytes *= 2
	}
	if maxResultBytes <= 0 || maxResultBytes > cfg.DeferredMaxTotalBytes {
		maxResultBytes = cfg.DeferredMaxTotalBytes
	}
	return deferredoperation.Limits{
		MaxConcurrency:    cfg.DeferredMaxConcurrency,
		MaxQueued:         cfg.DeferredMaxQueued,
		MaxRuntimeSeconds: cfg.DeferredMaxRuntimeSeconds,
		RetentionSeconds:  cfg.DeferredRetentionSeconds,
		MaxTerminal:       max(64, cfg.DeferredMaxQueued),
		MaxTotalBytes:     cfg.DeferredMaxTotalBytes,
		MaxResultBytes:    maxResultBytes,
		MaxChunkBytes:     cfg.ResponseChunkBytes,
	}
}

func runDeferredExecutorCommand(ctx context.Context, args []string, stderr io.Writer) int {
	if len(args) != 2 || !deferredoperation.ValidOperationID(args[1]) {
		fmt.Fprintln(stderr, "Error: invalid internal deferred executor invocation")
		return 1
	}
	store, err := deferredoperation.OpenExecutor(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "Error: deferred operation store is unavailable")
		return 1
	}
	if err := store.Execute(ctx, args[1], filetoolsserver.ExecuteDeferredOperation); err != nil {
		if errors.Is(err, deferredoperation.ErrTerminal) || errors.Is(err, deferredoperation.ErrAlreadyStarted) {
			return 0
		}
		fmt.Fprintln(stderr, "Deferred operation executor failed")
		return 1
	}
	return 0
}

func backupStoreLimits(limits config.BackupLimits) backupstore.Limits {
	return backupstore.Limits{
		MaxTotalBytes:        limits.MaxTotalBytes,
		MaxObjectBytes:       limits.MaxObjectBytes,
		MaxManifests:         limits.MaxManifests,
		MaxVersionsPerTarget: limits.MaxVersionsPerTarget,
		MaxPinned:            limits.MaxPinned,
		RetentionDays:        limits.RetentionDays,
		PlanTTLSeconds:       limits.PlanTTLSeconds,
	}
}

func diagnosticRole(args []string) string {
	if len(args) == 0 {
		return "server"
	}
	switch args[0] {
	case "task-worker":
		return "task-worker"
	case "task-supervisor":
		return "task-supervisor"
	case "_task-exec":
		return "task-exec"
	case "_deferred-exec":
		return "deferred-exec"
	default:
		return "server"
	}
}
