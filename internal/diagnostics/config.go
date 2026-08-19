package diagnostics

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	envLogLevel         = "MCP_LOG_LEVEL"
	envLogDir           = "MCP_LOG_DIR"
	envLogMaxFileBytes  = "MCP_LOG_MAX_FILE_BYTES"
	envLogMaxTotalBytes = "MCP_LOG_MAX_TOTAL_BYTES"
	envLogRetentionDays = "MCP_LOG_RETENTION_DAYS"

	defaultMaxFileBytes  = int64(8 * 1024 * 1024)
	defaultMaxTotalBytes = int64(128 * 1024 * 1024)
	defaultRetention     = 7 * 24 * time.Hour

	hardMaxFileBytes  = int64(256 * 1024 * 1024)
	hardMaxTotalBytes = int64(8 * 1024 * 1024 * 1024)
	hardRetentionDays = 3650
)

type config struct {
	Level         slog.Level
	Directory     string
	MaxFileBytes  int64
	MaxTotalBytes int64
	Retention     time.Duration
}

func loadConfig(getenv func(string) string) (config, error) {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	cfg := config{
		Level:         logLevel(getenv(envLogLevel)),
		Directory:     strings.TrimSpace(getenv(envLogDir)),
		MaxFileBytes:  defaultMaxFileBytes,
		MaxTotalBytes: defaultMaxTotalBytes,
		Retention:     defaultRetention,
	}
	var err error
	if cfg.MaxFileBytes, err = positiveBoundedInt64(getenv(envLogMaxFileBytes), cfg.MaxFileBytes, hardMaxFileBytes); err != nil {
		return config{}, fmt.Errorf("%s: %w", envLogMaxFileBytes, err)
	}
	if cfg.MaxTotalBytes, err = positiveBoundedInt64(getenv(envLogMaxTotalBytes), cfg.MaxTotalBytes, hardMaxTotalBytes); err != nil {
		return config{}, fmt.Errorf("%s: %w", envLogMaxTotalBytes, err)
	}
	retentionDays, err := positiveBoundedInt(getenv(envLogRetentionDays), int(defaultRetention/(24*time.Hour)), hardRetentionDays)
	if err != nil {
		return config{}, fmt.Errorf("%s: %w", envLogRetentionDays, err)
	}
	cfg.Retention = time.Duration(retentionDays) * 24 * time.Hour
	if cfg.MaxTotalBytes < 4*cfg.MaxFileBytes {
		return config{}, fmt.Errorf("%s must be at least four times %s", envLogMaxTotalBytes, envLogMaxFileBytes)
	}
	return cfg, nil
}

func logLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func positiveBoundedInt64(value string, fallback, maximum int64) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 || parsed > maximum {
		return 0, fmt.Errorf("expected integer in range 1..%d", maximum)
	}
	return parsed, nil
}

func positiveBoundedInt(value string, fallback, maximum int) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || parsed > maximum {
		return 0, fmt.Errorf("expected integer in range 1..%d", maximum)
	}
	return parsed, nil
}
