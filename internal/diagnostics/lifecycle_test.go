package diagnostics

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestOpenStderrOnlyUsesRedactedServerAndHTTPChannels(t *testing.T) {
	var stderr bytes.Buffer
	manager, err := Open(&stderr, func(name string) string {
		if name == envLogLevel {
			return "debug"
		}
		return ""
	}, "frontend")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer manager.Close()
	if manager.file != nil {
		t.Fatal("stderr-only logging unexpectedly opened a file sink")
	}

	manager.Server().Info("server_event", "path", `C:\secret\source.go`, "tool", "read_text_file")
	manager.HTTPAccess().Info("http_request", "method", "POST", "route", "mcp", "status", 200)
	text := stderr.String()
	if strings.Contains(text, `C:\secret\source.go`) {
		t.Fatalf("server log leaked path: %s", text)
	}
	for _, expected := range []string{"channel=server", "channel=http_access", "role=frontend", "tool=read_text_file", "route=mcp"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("log missing %q: %s", expected, text)
		}
	}
}

func TestFileLifecycleRotatesCompressesAndStaysWithinAggregateBudget(t *testing.T) {
	dir := t.TempDir()
	manager := openTestManager(t, dir, 512, 8192)
	for index := 0; index < 200; index++ {
		manager.Server().Info("bounded_event", "tool", "read_text_file", "count", index, "path", filepath.Join(dir, "secret.txt"))
	}
	manager.Close()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	var archives []string
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
		if strings.HasSuffix(entry.Name(), ".active.log") {
			t.Fatalf("active log survived clean close: %s", entry.Name())
		}
		if strings.HasSuffix(entry.Name(), ".log.gz") {
			archives = append(archives, filepath.Join(dir, entry.Name()))
		}
	}
	if len(archives) < 2 {
		var names []string
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("expected multiple compressed archives, got %d; entries=%v", len(archives), names)
	}
	if total > 8192 {
		t.Fatalf("diagnostic directory uses %d bytes, budget 8192", total)
	}
	data := readGzipFile(t, archives[0])
	if strings.Contains(data, dir) || strings.Contains(data, "secret.txt") {
		t.Fatalf("archive leaked a clear path: %s", data)
	}
	if !strings.Contains(data, "channel=server") {
		t.Fatalf("archive missing server channel: %s", data)
	}
}

func TestFileLifecycleSupportsConcurrentProcessOwners(t *testing.T) {
	dir := t.TempDir()
	first := openTestManager(t, dir, 1024, 16384)
	second := openTestManager(t, dir, 1024, 16384)
	if first.file == nil || second.file == nil {
		t.Fatal("expected two file sinks")
	}
	if first.file.slot == second.file.slot {
		t.Fatalf("concurrent managers share slot %d", first.file.slot)
	}

	var group sync.WaitGroup
	for _, manager := range []*Manager{first, second} {
		group.Add(1)
		go func(manager *Manager) {
			defer group.Done()
			for index := 0; index < 100; index++ {
				manager.Server().Info("concurrent_event", "count", index)
			}
		}(manager)
	}
	group.Wait()
	first.Close()
	second.Close()

	if active, _ := filepath.Glob(filepath.Join(dir, "*.active.log")); len(active) != 0 {
		t.Fatalf("active logs survived close: %v", active)
	}
	if total := directoryBytes(t, dir); total > 16384 {
		t.Fatalf("diagnostic directory uses %d bytes, budget 16384", total)
	}
}

func TestFileLifecycleRecoversStaleActiveLogAfterOwnership(t *testing.T) {
	dir := t.TempDir()
	cfg := config{Level: slog.LevelInfo, Directory: dir, MaxFileBytes: 512, MaxTotalBytes: 4096, Retention: 24 * time.Hour}
	stale := activeLogPath(dir, 0)
	staleData := []byte("time=2026-08-19T00:00:00Z level=INFO msg=stale channel=server\n")
	if err := os.WriteFile(stale, staleData, 0o600); err != nil {
		t.Fatal(err)
	}

	writer, err := openFileSinkWithOps(cfg, io.Discard, defaultSinkOps())
	if err != nil {
		t.Fatalf("openFileSinkWithOps: %v", err)
	}
	if writer == nil {
		t.Fatal("file sink unexpectedly unavailable")
	}
	writer.Close()

	if active, _ := filepath.Glob(filepath.Join(dir, "*.active.log")); len(active) != 0 {
		t.Fatalf("active logs survived recovery and clean close: %v", active)
	}
	archives, _ := filepath.Glob(filepath.Join(dir, "*.log.gz"))
	if len(archives) != 1 {
		t.Fatalf("stale active log produced %d archives, want 1", len(archives))
	}
	if got := readGzipFile(t, archives[0]); got != string(staleData) {
		t.Fatalf("recovered stale bytes changed: got %q want %q", got, staleData)
	}
}

func TestFileLifecycleRemovesExpiredArchive(t *testing.T) {
	dir := t.TempDir()
	cfg := config{Level: slog.LevelInfo, Directory: dir, MaxFileBytes: 512, MaxTotalBytes: 4096, Retention: 24 * time.Hour}
	expired := filepath.Join(dir, "scripthold-archive-20000101T000000.000000000Z-w00-000000.log")
	if err := os.WriteFile(expired, []byte("expired diagnostic archive\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatal(err)
	}

	writer, err := openFileSinkWithOps(cfg, io.Discard, defaultSinkOps())
	if err != nil {
		t.Fatalf("openFileSinkWithOps: %v", err)
	}
	if writer == nil {
		t.Fatal("file sink unexpectedly unavailable")
	}
	writer.Close()
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired archive still exists: %v", err)
	}
}

func TestActiveOpenPermissionFailureFailsFileSinkStartupWithoutPublishingActiveFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config{Level: slog.LevelInfo, Directory: dir, MaxFileBytes: 512, MaxTotalBytes: 4096, Retention: time.Hour}
	ops := defaultSinkOps()
	ops.openActive = func(string) (*os.File, error) { return nil, fs.ErrPermission }

	writer, err := openFileSinkWithOps(cfg, io.Discard, ops)
	if writer != nil {
		writer.Close()
		t.Fatal("permission failure unexpectedly returned a file sink")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("openFileSinkWithOps error = %v, want permission failure", err)
	}
	if active, _ := filepath.Glob(filepath.Join(dir, "*.active.log")); len(active) != 0 {
		t.Fatalf("permission failure published active log: %v", active)
	}
}

func TestRenameFailurePreservesActiveLogAndDegradesToStderr(t *testing.T) {
	dir := t.TempDir()
	cfg := config{Level: slog.LevelInfo, Directory: dir, MaxFileBytes: 256, MaxTotalBytes: 4096, Retention: time.Hour}
	var stderr bytes.Buffer
	ops := defaultSinkOps()
	ops.move = func(string, string) error { return errors.New("injected rename failure") }
	writer, err := openFileSinkWithOps(cfg, &stderr, ops)
	if err != nil {
		t.Fatalf("openFileSinkWithOps: %v", err)
	}
	if writer == nil {
		t.Fatal("file sink unexpectedly unavailable")
	}
	first := []byte(strings.Repeat("a", 180) + "\n")
	second := []byte(strings.Repeat("b", 180) + "\n")
	if _, err := writer.Write(first); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := writer.Write(second); err != nil {
		t.Fatalf("second write: %v", err)
	}
	writer.Close()

	data, err := os.ReadFile(activeLogPath(dir, writer.slot))
	if err != nil {
		t.Fatalf("read preserved active log: %v", err)
	}
	if !bytes.Equal(data, first) {
		t.Fatalf("rename failure changed preserved active bytes: got %d want %d", len(data), len(first))
	}
	if !strings.Contains(stderr.String(), "diagnostics file sink disabled; continuing on stderr") ||
		!strings.Contains(stderr.String(), "diagnostics degraded writer could not finalize active log") {
		t.Fatalf("missing bounded rename-failure diagnostics: %s", stderr.String())
	}
}

func TestCompressionFailureKeepsUncompressedArchiveAndContinues(t *testing.T) {
	dir := t.TempDir()
	cfg := config{Level: slog.LevelDebug, Directory: dir, MaxFileBytes: 256, MaxTotalBytes: 4096, Retention: time.Hour}
	var stderr bytes.Buffer
	ops := defaultSinkOps()
	ops.compress = func(string, string, int64) error { return errors.New("injected compression failure") }
	writer, err := openFileSinkWithOps(cfg, &stderr, ops)
	if err != nil {
		t.Fatalf("openFileSinkWithOps: %v", err)
	}
	if writer == nil {
		t.Fatal("file sink unexpectedly unavailable")
	}
	for index := 0; index < 20; index++ {
		_, _ = writer.Write([]byte(strings.Repeat("x", 80) + "\n"))
	}
	writer.Close()

	plain, _ := filepath.Glob(filepath.Join(dir, "*.log"))
	if len(plain) == 0 {
		t.Fatal("compression failure did not preserve an uncompressed archive")
	}
	if !strings.Contains(stderr.String(), "diagnostics compression failed") {
		t.Fatalf("missing bounded compression warning: %s", stderr.String())
	}
	if total := directoryBytes(t, dir); total > cfg.MaxTotalBytes {
		t.Fatalf("diagnostic directory uses %d bytes, budget %d", total, cfg.MaxTotalBytes)
	}
}

func TestMaintenanceFailureFallsBackToStderrWithoutGrowingFiles(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "scripthold-archive-20000101T000000.000000000Z-w00-000000.log")
	if err := os.WriteFile(old, bytes.Repeat([]byte("x"), 3000), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config{Level: slog.LevelInfo, Directory: dir, MaxFileBytes: 512, MaxTotalBytes: 4096, Retention: time.Hour}
	var stderr bytes.Buffer
	ops := defaultSinkOps()
	ops.remove = func(string) error { return errors.New("injected cleanup failure") }
	writer, err := openFileSinkWithOps(cfg, &stderr, ops)
	if err != nil {
		t.Fatalf("maintenance failure must not fail product startup: %v", err)
	}
	if writer != nil {
		writer.Close()
		t.Fatal("file logging should be disabled when aggregate cleanup cannot be enforced")
	}
	if !strings.Contains(stderr.String(), "diagnostics file sink unavailable; continuing on stderr") {
		t.Fatalf("missing bounded fallback warning: %s", stderr.String())
	}
	info, err := os.Stat(old)
	if err != nil || info.Size() != 3000 {
		t.Fatalf("failed cleanup corrupted existing archive: info=%v err=%v", info, err)
	}
}

func TestOpenRejectsMissingConfiguredDirectory(t *testing.T) {
	var stderr bytes.Buffer
	missing := filepath.Join(t.TempDir(), "missing")
	_, err := Open(&stderr, func(name string) string {
		if name == envLogDir {
			return missing
		}
		return ""
	}, "frontend")
	if err == nil {
		t.Fatal("Open succeeded with missing configured log directory")
	}
	if strings.Contains(err.Error(), missing) {
		t.Fatalf("startup error leaked configured path: %v", err)
	}
}

func openTestManager(t *testing.T, dir string, maxFile, maxTotal int64) *Manager {
	t.Helper()
	values := map[string]string{
		envLogLevel:         "debug",
		envLogDir:           dir,
		envLogMaxFileBytes:  itoa64(maxFile),
		envLogMaxTotalBytes: itoa64(maxTotal),
		envLogRetentionDays: "1",
	}
	manager, err := Open(io.Discard, func(name string) string { return values[name] }, "frontend")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return manager
}

func readGzipFile(t *testing.T, path string) string {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func directoryBytes(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	return total
}

func itoa64(value int64) string {
	return strconv.FormatInt(value, 10)
}
