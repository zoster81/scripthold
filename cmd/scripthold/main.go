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
	"strings"
	"syscall"

	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/filetoolsserver/handler"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
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
	configureLogging(stderr, getenv)

	// Keep the legacy exported version synchronized for existing embedders while
	// the explicit server options remain authoritative for this process.
	filetoolsserver.Version = version

	if len(args) == 1 && (args[0] == "--version" || args[0] == "-v") {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if len(args) > 0 && args[0] == "task-worker" {
		return runTaskWorkerCommand(ctx, args[1:], stderr, getenv)
	}
	if len(args) > 0 && args[0] == "task-supervisor" {
		return runTaskSupervisorCommand(ctx, args[1:], stderr, getenv)
	}
	if len(args) > 0 && args[0] == "_task-exec" {
		return runTaskExecutorCommand(ctx, args[1:], stderr, getenv)
	}

	diagnosticOptions, diagnosticCommand, err := parseBackupDiagnosticCommand(args)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	if diagnosticCommand {
		return runBackupDiagnosticCommand(ctx, diagnosticOptions, stdout, stderr, getenv)
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
	selection, err := selectRunner(options.transport, getenv, applicationConfig.Limits.MaxSessions)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	var store *backupstore.Store
	var tasks *taskstore.Store
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

	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:                version,
		AllowedDirectories:     normalized,
		ProtectedDirectories:   protectedDirectories,
		BackupStore:            store,
		TaskStore:              tasks,
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
	if cfg == nil || !cfg.Tasks.Enabled() || !cfg.Backup.Enabled() {
		return nil
	}
	taskRoot, taskErr := filepath.Abs(cfg.Tasks.StoreDir)
	backupRoot, backupErr := filepath.Abs(cfg.Backup.StoreDir)
	if taskErr != nil || backupErr != nil {
		return errors.New("private store paths are invalid")
	}
	if security.PathsOverlap(taskRoot, backupRoot) {
		return errors.New("task store and backup store must be separate non-overlapping directories")
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

func configureLogging(stderr io.Writer, getenv func(string) string) {
	// Protocol output remains reserved for stdout; all logs use stderr.
	level := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(getenv("MCP_LOG_LEVEL"))) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: level})))
}
