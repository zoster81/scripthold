package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/filetoolsserver"
	"github.com/zoster81/scripthold/internal/config"
)

func TestRunCommandVersionWritesOnlyVersion(t *testing.T) {
	originalVersion := version
	originalServerVersion := filetoolsserver.Version
	version = "test-version"
	t.Cleanup(func() {
		version = originalVersion
		filetoolsserver.Version = originalServerVersion
	})

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{"--version"}, &stdout, &stderr, func(string) string { return "" })
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if stdout.String() != "test-version\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestRunCommandRejectsUnsupportedTransportBeforeStartup(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), nil, &stdout, &stderr, func(name string) string {
		if name == envTransport {
			return "websocket"
		}
		return ""
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "unsupported transport") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunCommandRejectsHTTPWithoutTokenBeforeStartup(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{"--transport=streamable-http"}, &stdout, &stderr, func(string) string { return "" })
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "MCP_HTTP_TOKEN") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestBackupStoreLimitsPreserveConfiguredValues(t *testing.T) {
	configured := config.BackupLimits{
		MaxTotalBytes:        11,
		MaxObjectBytes:       12,
		MaxManifests:         13,
		MaxVersionsPerTarget: 14,
		MaxPinned:            15,
		RetentionDays:        16,
		PlanTTLSeconds:       17,
	}
	mapped := backupStoreLimits(configured)
	if mapped.MaxTotalBytes != configured.MaxTotalBytes || mapped.MaxObjectBytes != configured.MaxObjectBytes ||
		mapped.MaxManifests != configured.MaxManifests || mapped.MaxVersionsPerTarget != configured.MaxVersionsPerTarget ||
		mapped.MaxPinned != configured.MaxPinned || mapped.RetentionDays != configured.RetentionDays ||
		mapped.PlanTTLSeconds != configured.PlanTTLSeconds {
		t.Fatalf("mapped backup limits = %#v", mapped)
	}
}

func TestRunCommandRejectsOverlappingBackupStoreBeforeStartup(t *testing.T) {
	publicRoot := canonicalBackupTestTempDir(t)
	storeDir := filepath.Join(publicRoot, "backups")
	values := map[string]string{
		"MCP_BACKUP_STORE_DIR": storeDir,
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := runCommand(context.Background(), []string{publicRoot}, &stdout, &stderr, func(name string) string {
		return values[name]
	})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), "backup store") || !strings.Contains(stderr.String(), "overlap") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if strings.Contains(stderr.String(), storeDir) || strings.Contains(stderr.String(), publicRoot) {
		t.Fatalf("stderr exposed a configured path: %q", stderr.String())
	}
}
