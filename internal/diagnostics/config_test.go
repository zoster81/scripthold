package diagnostics

import (
	"log/slog"
	"testing"
	"time"
)

func TestLoadConfigDefaultsToStderrOnly(t *testing.T) {
	cfg, err := loadConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Level != slog.LevelInfo {
		t.Fatalf("Level = %v, want info", cfg.Level)
	}
	if cfg.Directory != "" {
		t.Fatalf("Directory = %q, want disabled file logging", cfg.Directory)
	}
	if cfg.MaxFileBytes != defaultMaxFileBytes || cfg.MaxTotalBytes != defaultMaxTotalBytes {
		t.Fatalf("unexpected default byte limits: file=%d total=%d", cfg.MaxFileBytes, cfg.MaxTotalBytes)
	}
	if cfg.Retention != defaultRetention {
		t.Fatalf("Retention = %v, want %v", cfg.Retention, defaultRetention)
	}
}

func TestLoadConfigParsesFilePolicy(t *testing.T) {
	values := map[string]string{
		envLogLevel:         "debug",
		envLogDir:           `C:\logs`,
		envLogMaxFileBytes:  "4096",
		envLogMaxTotalBytes: "32768",
		envLogRetentionDays: "3",
	}
	cfg, err := loadConfig(func(name string) string { return values[name] })
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Level != slog.LevelDebug || cfg.Directory != `C:\logs` || cfg.MaxFileBytes != 4096 || cfg.MaxTotalBytes != 32768 || cfg.Retention != 72*time.Hour {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadConfigRejectsInvalidBoundedFilePolicy(t *testing.T) {
	tests := []map[string]string{
		{envLogDir: `C:\logs`, envLogMaxFileBytes: "0"},
		{envLogDir: `C:\logs`, envLogMaxFileBytes: "abc"},
		{envLogDir: `C:\logs`, envLogMaxFileBytes: "4096", envLogMaxTotalBytes: "4096"},
		{envLogDir: `C:\logs`, envLogRetentionDays: "0"},
		{envLogDir: `C:\logs`, envLogRetentionDays: "99999"},
	}
	for _, values := range tests {
		if _, err := loadConfig(func(name string) string { return values[name] }); err == nil {
			t.Fatalf("loadConfig(%v) succeeded, want error", values)
		}
	}
}

func TestLoadConfigPreservesLegacyUnknownLevelFallback(t *testing.T) {
	cfg, err := loadConfig(func(name string) string {
		if name == envLogLevel {
			return "verbose"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.Level != slog.LevelInfo {
		t.Fatalf("Level = %v, want compatibility fallback to info", cfg.Level)
	}
}
