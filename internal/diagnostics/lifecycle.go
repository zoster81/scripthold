package diagnostics

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zoster81/scripthold/internal/filesystem"
)

const (
	maxWriterSlots      = 8
	maintenanceLockWait = 250 * time.Millisecond
	lockRetryInterval   = 5 * time.Millisecond
)

var errMaintenanceBusy = errors.New("diagnostics maintenance lock is busy")

type Manager struct {
	server *slog.Logger
	access *slog.Logger
	file   *rotatingWriter
	close  sync.Once
}

func Open(stderr io.Writer, getenv func(string) string, role string) (*Manager, error) {
	if stderr == nil {
		stderr = io.Discard
	}
	cfg, err := loadConfig(getenv)
	if err != nil {
		return nil, errors.New("diagnostics logging configuration is invalid")
	}

	var file *rotatingWriter
	if cfg.Directory != "" {
		file, err = openFileSinkWithOps(cfg, stderr, defaultSinkOps())
		if err != nil {
			return nil, errors.New("diagnostics file logging is unavailable")
		}
	}

	output := stderr
	if file != nil {
		output = io.MultiWriter(stderr, file)
	}
	base := slog.New(newRedactingHandler(slog.NewTextHandler(output, &slog.HandlerOptions{Level: cfg.Level})))
	return &Manager{
		server: channelLogger(base, "server", role),
		access: channelLogger(base, "http_access", role),
		file:   file,
	}, nil
}

func (manager *Manager) Server() *slog.Logger {
	if manager == nil {
		return nil
	}
	return manager.server
}

func (manager *Manager) HTTPAccess() *slog.Logger {
	if manager == nil {
		return nil
	}
	return manager.access
}

func (manager *Manager) Close() {
	if manager == nil {
		return
	}
	manager.close.Do(func() {
		if manager.file != nil {
			manager.file.Close()
		}
	})
}

type sinkOps struct {
	now        func() time.Time
	move       func(string, string) error
	remove     func(string) error
	openActive func(string) (*os.File, error)
	compress   func(string, string, int64) error
}

func defaultSinkOps() sinkOps {
	return sinkOps{
		now:    time.Now,
		move:   filesystem.MoveNoReplace,
		remove: os.Remove,
		openActive: func(path string) (*os.File, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		},
		compress: compressArchive,
	}
}

type rotatingWriter struct {
	mu sync.Mutex

	cfg      config
	stderr   io.Writer
	ops      sinkOps
	slot     int
	slotLock *fileLock
	active   string
	file     *os.File
	size     int64
	sequence uint64
	disabled bool
	closed   bool
}

func openFileSinkWithOps(cfg config, stderr io.Writer, ops sinkOps) (*rotatingWriter, error) {
	directory, err := validateLogDirectory(cfg.Directory)
	if err != nil {
		return nil, err
	}
	cfg.Directory = directory
	if err := runMaintenance(cfg, stderr, ops); err != nil {
		warnFixed(stderr, "diagnostics file sink unavailable; continuing on stderr")
		return nil, nil
	}

	slot, lock, err := acquireWriterSlot(cfg.Directory, writerSlotCount(cfg))
	if err != nil {
		return nil, err
	}
	if lock == nil {
		warnFixed(stderr, "diagnostics file writer capacity reached; continuing on stderr")
		return nil, nil
	}

	writer := &rotatingWriter{
		cfg:      cfg,
		stderr:   stderr,
		ops:      ops,
		slot:     slot,
		slotLock: lock,
		active:   activeLogPath(cfg.Directory, slot),
	}
	if err := writer.prepareOwnedSlot(); err != nil {
		_ = lock.Close()
		warnFixed(stderr, "diagnostics file sink unavailable; continuing on stderr")
		return nil, nil
	}
	if err := writer.openActive(); err != nil {
		_ = lock.Close()
		return nil, err
	}
	return writer, nil
}

func validateLogDirectory(path string) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || !filepath.IsAbs(path) {
		return "", errors.New("configured diagnostics directory must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("configured diagnostics directory is unavailable")
	}
	if err := validateLogDirectoryPlatform(info); err != nil {
		return "", errors.New("configured diagnostics directory is unavailable")
	}
	return path, nil
}

func writerSlotCount(cfg config) int {
	count := int(cfg.MaxTotalBytes / (4 * cfg.MaxFileBytes))
	if count < 1 {
		count = 1
	}
	if count > maxWriterSlots {
		count = maxWriterSlots
	}
	return count
}

func archiveBudget(cfg config) int64 {
	return cfg.MaxTotalBytes - int64(writerSlotCount(cfg))*2*cfg.MaxFileBytes
}

func acquireWriterSlot(directory string, count int) (int, *fileLock, error) {
	for slot := 0; slot < count; slot++ {
		lock, acquired, err := tryAcquireFileLock(slotLockPath(directory, slot))
		if err != nil {
			return 0, nil, err
		}
		if acquired {
			return slot, lock, nil
		}
	}
	return 0, nil, nil
}

func (writer *rotatingWriter) prepareOwnedSlot() error {
	return withMaintenanceLock(writer.cfg.Directory, maintenanceLockWait, func() error {
		if err := finalizeStaleActive(writer.active, writer.slot, writer.cfg, writer.stderr, writer.ops); err != nil {
			return err
		}
		return cleanupArchives(writer.cfg, writer.ops)
	})
}

func (writer *rotatingWriter) openActive() error {
	file, err := writer.ops.openActive(writer.active)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = writer.ops.remove(writer.active)
		return err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		_ = writer.ops.remove(writer.active)
		if err != nil {
			return err
		}
		return errors.New("diagnostics active file identity is invalid")
	}
	writer.file = file
	writer.size = 0
	return nil
}

func (writer *rotatingWriter) Write(payload []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.closed || writer.disabled {
		return len(payload), nil
	}
	if int64(len(payload)) > writer.cfg.MaxFileBytes {
		warnFixed(writer.stderr, "diagnostics record exceeded file limit; omitted from file sink")
		return len(payload), nil
	}
	if writer.size > 0 && writer.size+int64(len(payload)) > writer.cfg.MaxFileBytes {
		if err := writer.rotateLocked(true); err != nil {
			writer.disableLocked()
			return len(payload), nil
		}
	}
	written, err := writer.file.Write(payload)
	if err != nil || written != len(payload) {
		writer.disableLocked()
		return len(payload), nil
	}
	writer.size += int64(written)
	return len(payload), nil
}

func (writer *rotatingWriter) Close() {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if writer.closed {
		return
	}
	writer.closed = true
	if !writer.disabled && writer.file != nil {
		if writer.size == 0 {
			_ = writer.file.Close()
			writer.file = nil
			_ = writer.ops.remove(writer.active)
		} else if err := writer.rotateLocked(false); err != nil {
			warnFixed(writer.stderr, "diagnostics final rotation failed; preserving active log")
			if writer.file != nil {
				_ = writer.file.Close()
				writer.file = nil
			}
		}
	} else if writer.disabled {
		if err := writer.finalizeDisabledLocked(); err != nil {
			warnFixed(writer.stderr, "diagnostics degraded writer could not finalize active log")
		}
	}
	writer.releaseSlotLocked()
}

func (writer *rotatingWriter) rotateLocked(reopen bool) error {
	if writer.file == nil {
		return nil
	}
	if err := writer.file.Sync(); err != nil {
		return err
	}
	if err := writer.file.Close(); err != nil {
		return err
	}
	writer.file = nil

	err := withMaintenanceLock(writer.cfg.Directory, maintenanceLockWait, func() error {
		archive, err := moveActiveToArchive(writer.active, writer.slot, &writer.sequence, writer.ops)
		if err != nil {
			return err
		}
		compressed := strings.TrimSuffix(archive, ".log") + ".log.gz"
		if err := writer.ops.compress(archive, compressed, writer.cfg.MaxFileBytes); err != nil {
			warnFixed(writer.stderr, "diagnostics compression failed; retained bounded uncompressed archive")
		}
		return cleanupArchives(writer.cfg, writer.ops)
	})
	if err != nil {
		return err
	}
	writer.size = 0
	if reopen {
		return writer.openActive()
	}
	return nil
}

func (writer *rotatingWriter) disableLocked() {
	if writer.disabled {
		return
	}
	writer.disabled = true
	warnFixed(writer.stderr, "diagnostics file sink disabled; continuing on stderr")
	if writer.file != nil {
		_ = writer.file.Close()
		writer.file = nil
	}
}

func (writer *rotatingWriter) finalizeDisabledLocked() error {
	return withMaintenanceLock(writer.cfg.Directory, maintenanceLockWait, func() error {
		if err := finalizeStaleActive(writer.active, writer.slot, writer.cfg, writer.stderr, writer.ops); err != nil {
			return err
		}
		return cleanupArchives(writer.cfg, writer.ops)
	})
}

func (writer *rotatingWriter) releaseSlotLocked() {
	if writer.slotLock != nil {
		_ = writer.slotLock.Close()
		writer.slotLock = nil
	}
}

func runMaintenance(cfg config, stderr io.Writer, ops sinkOps) error {
	return withMaintenanceLock(cfg.Directory, maintenanceLockWait, func() error {
		for slot := 0; slot < writerSlotCount(cfg); slot++ {
			lock, acquired, err := tryAcquireFileLock(slotLockPath(cfg.Directory, slot))
			if err != nil {
				return err
			}
			if !acquired {
				continue
			}
			active := activeLogPath(cfg.Directory, slot)
			finalizeErr := finalizeStaleActive(active, slot, cfg, stderr, ops)
			_ = lock.Close()
			if finalizeErr != nil {
				return finalizeErr
			}
		}
		return cleanupArchives(cfg, ops)
	})
}

func finalizeStaleActive(path string, slot int, cfg config, stderr io.Writer, ops sinkOps) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("diagnostics active file identity is invalid")
	}
	if info.Size() == 0 {
		return ops.remove(path)
	}
	var sequence uint64
	archive, err := moveActiveToArchive(path, slot, &sequence, ops)
	if err != nil {
		return err
	}
	compressed := strings.TrimSuffix(archive, ".log") + ".log.gz"
	if err := ops.compress(archive, compressed, cfg.MaxFileBytes); err != nil {
		warnFixed(stderr, "diagnostics compression failed; retained bounded uncompressed archive")
	}
	return nil
}

func moveActiveToArchive(active string, slot int, sequence *uint64, ops sinkOps) (string, error) {
	for attempts := 0; attempts < 1024; attempts++ {
		number := *sequence
		(*sequence)++
		stamp := ops.now().UTC().Format("20060102T150405.000000000Z")
		archive := filepath.Join(filepath.Dir(active), fmt.Sprintf("scripthold-archive-%s-w%02d-%06d.log", stamp, slot, number))
		err := ops.move(active, archive)
		if errors.Is(err, filesystem.ErrDestinationExists) {
			continue
		}
		if err != nil {
			return "", err
		}
		return archive, nil
	}
	return "", errors.New("diagnostics archive namespace is exhausted")
}

func cleanupArchives(cfg config, ops sinkOps) error {
	now := ops.now()
	archives, err := listArchives(cfg.Directory)
	if err != nil {
		return err
	}
	cutoff := now.Add(-cfg.Retention)
	retained := archives[:0]
	for _, archive := range archives {
		if archive.modTime.Before(cutoff) {
			if err := ops.remove(archive.path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		retained = append(retained, archive)
	}
	archives = retained
	var total int64
	for _, archive := range archives {
		total += archive.size
	}
	budget := archiveBudget(cfg)
	for _, archive := range archives {
		if total <= budget {
			break
		}
		if err := ops.remove(archive.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		total -= archive.size
	}
	return nil
}

type archiveInfo struct {
	path    string
	size    int64
	modTime time.Time
	name    string
}

func listArchives(directory string) ([]archiveInfo, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	archives := make([]archiveInfo, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "scripthold-archive-") ||
			!(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz") || strings.HasSuffix(name, ".log.gz.tmp")) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("diagnostics archive identity is invalid")
		}
		archives = append(archives, archiveInfo{path: filepath.Join(directory, name), size: info.Size(), modTime: info.ModTime(), name: name})
	}
	sort.Slice(archives, func(i, j int) bool {
		if archives[i].modTime.Equal(archives[j].modTime) {
			return archives[i].name < archives[j].name
		}
		return archives[i].modTime.Before(archives[j].modTime)
	})
	return archives, nil
}

func withMaintenanceLock(directory string, wait time.Duration, action func() error) error {
	lock, acquired, err := acquireFileLockWithin(maintenanceLockPath(directory), wait)
	if err != nil {
		return err
	}
	if !acquired {
		return errMaintenanceBusy
	}
	defer lock.Close()
	return action()
}

func acquireFileLockWithin(path string, wait time.Duration) (*fileLock, bool, error) {
	deadline := time.Now().Add(wait)
	for {
		lock, acquired, err := tryAcquireFileLock(path)
		if err != nil || acquired {
			return lock, acquired, err
		}
		if wait <= 0 || !time.Now().Before(deadline) {
			return nil, false, nil
		}
		time.Sleep(lockRetryInterval)
	}
}

func slotLockPath(directory string, slot int) string {
	return filepath.Join(directory, fmt.Sprintf("scripthold-writer-%02d.lock", slot))
}

func activeLogPath(directory string, slot int) string {
	return filepath.Join(directory, fmt.Sprintf("scripthold-writer-%02d.active.log", slot))
}

func maintenanceLockPath(directory string) string {
	return filepath.Join(directory, "scripthold-maintenance.lock")
}

func warnFixed(stderr io.Writer, message string) {
	if stderr == nil {
		return
	}
	_, _ = fmt.Fprintln(stderr, "WARN", message)
}

func compressArchive(source, destination string, maximum int64) (err error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	inputClosed := false
	defer func() {
		if !inputClosed {
			_ = input.Close()
		}
	}()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > maximum {
		return errors.New("diagnostics archive exceeds compression bound")
	}

	temporary := destination + ".tmp"
	output, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	published := false
	defer func() {
		_ = output.Close()
		if !published {
			_ = os.Remove(temporary)
		}
	}()
	if err := output.Chmod(0o600); err != nil {
		return err
	}
	limited := &boundedWriter{writer: output, remaining: maximum}
	zipper := gzip.NewWriter(limited)
	if _, err := io.Copy(zipper, input); err != nil {
		_ = zipper.Close()
		return err
	}
	if err := zipper.Close(); err != nil {
		return err
	}
	if err := input.Close(); err != nil {
		return err
	}
	inputClosed = true
	if err := output.Sync(); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := filesystem.MoveNoReplace(temporary, destination); err != nil {
		return err
	}
	published = true
	if err := os.Remove(source); err != nil {
		if cleanupErr := os.Remove(destination); cleanupErr != nil {
			return errors.Join(err, cleanupErr)
		}
		return err
	}
	return nil
}

type boundedWriter struct {
	writer    io.Writer
	remaining int64
}

func (writer *boundedWriter) Write(payload []byte) (int, error) {
	if int64(len(payload)) > writer.remaining {
		return 0, errors.New("diagnostics compressed archive exceeds bound")
	}
	written, err := writer.writer.Write(payload)
	writer.remaining -= int64(written)
	return written, err
}
