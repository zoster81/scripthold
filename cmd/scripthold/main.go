package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/internal/backupstore"
	"github.com/zoster81/scripthold/internal/config"
	"github.com/zoster81/scripthold/internal/security"
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
	selection, err := selectRunner(options.transport, getenv, applicationConfig.Limits.MaxSessions)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	var store *backupstore.Store
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

	server := filetoolsserver.BuildServer(filetoolsserver.ServerOptions{
		Version:                version,
		AllowedDirectories:     normalized,
		ProtectedDirectories:   protectedDirectories,
		BackupStore:            store,
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
