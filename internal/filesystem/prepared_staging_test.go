package filesystem

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
)

func TestCreateDirectoryExactNoReplaceDoesNotCreateParents(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "one")
	if err := CreateDirectoryExactNoReplace(path, 0o751); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Fatalf("created directory = %#v / %v", info, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o751 {
		t.Fatalf("created directory mode = %o, want 751", info.Mode().Perm())
	}
	if err := CreateDirectoryExactNoReplace(path, 0o755); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("existing directory error = %v, want ErrDestinationExists", err)
	}
	if err := CreateDirectoryExactNoReplace(filepath.Join(root, "missing", "child"), 0o755); err == nil {
		t.Fatal("mkdir unexpectedly created a missing parent")
	}
}

func TestStagedFilePublishesExactRawBytesNoReplace(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "payload.bin")
	payload := []byte{0x00, 0xff, 0x0a, 0x0d, 0x00, 0x7f}
	staged, err := StageRawFile(context.Background(), root, bytes.NewReader(payload), 0o640, nil, int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Cleanup()
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists before publish: %v", err)
	}
	if err := staged.PublishNoReplace(destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("published bytes = %x, want %x", got, payload)
	}
}

func TestStagedFileDestinationRaceDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "target.txt")
	staged, err := StageRawFile(context.Background(), root, bytes.NewReader([]byte("prepared")), 0o600, nil, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Cleanup()
	if err := os.WriteFile(destination, []byte("racer"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := staged.PublishNoReplace(destination); !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("publish race error = %v, want ErrDestinationExists", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "racer" {
		t.Fatalf("destination overwritten: %q", got)
	}
}

func TestStageRegularFileVerifiesPreparedSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.txt")
	if err := os.WriteFile(source, []byte("alpha"), 0o640); err != nil {
		t.Fatal(err)
	}
	expected, err := CaptureRegularFileSnapshotBounded(context.Background(), source, 1024)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := StagePreparedRegularFile(context.Background(), source, root, expected)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Cleanup()
	if err := os.WriteFile(source, []byte("changed"), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := StagePreparedRegularFile(context.Background(), source, root, expected); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("changed source staging error = %v, want CONFLICT", err)
	}
}

func TestStageExactDirectoryCopyAndPublishPreservesPreparedTree(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(source, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".hidden"), []byte("hidden"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, ".git", "config"), []byte("git"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := ExactTreeOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}), MaxEntries: 64,
		MaxDepth: 16, MaxFileBytes: 1024, MaxAggregateBytes: 4096,
	}
	expected, err := EnumerateExactTree(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := StageExactDirectoryCopy(context.Background(), expected, root, options)
	if err != nil {
		t.Fatal(err)
	}
	defer staged.Cleanup()
	destination := filepath.Join(root, "copy")
	if err := staged.PublishNoReplace(destination); err != nil {
		t.Fatal(err)
	}
	actual, err := EnumerateExactTree(context.Background(), destination, options)
	if err != nil {
		t.Fatal(err)
	}
	if !ExactTreeContentEqual(expected, actual) {
		t.Fatalf("published tree differs: expected=%s actual=%s", expected.ContentFingerprint, actual.ContentFingerprint)
	}
}

func TestStageRawFileCancellationCleansPreparedTemp(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterFirstRead{cancel: cancel, data: []byte("payload")}
	_, err := StageRawFile(ctx, root, reader, 0o600, nil, int64(len(reader.data)))
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("staging cancellation error = %v, want CANCELLED", err)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".scripthold-r24-file-") {
			t.Fatalf("cancelled staging residue remains: %s", entry.Name())
		}
	}
}

type cancelAfterFirstRead struct {
	cancel context.CancelFunc
	data   []byte
	read   bool
}

func (reader *cancelAfterFirstRead) Read(buffer []byte) (int, error) {
	if reader.read {
		return 0, io.EOF
	}
	reader.read = true
	n := copy(buffer, reader.data)
	reader.cancel()
	return n, nil
}

func TestStagedDirectoryCleanupFailsClosedAfterTamper(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := ExactTreeOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}), MaxEntries: 64,
		MaxDepth: 16, MaxFileBytes: 1024, MaxAggregateBytes: 4096,
	}
	expected, err := EnumerateExactTree(context.Background(), source, options)
	if err != nil {
		t.Fatal(err)
	}
	staged, err := StageExactDirectoryCopy(context.Background(), expected, root, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged.path, "intruder.txt"), []byte("intruder"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := staged.Cleanup(); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("tampered cleanup error = %v, want CONFLICT", err)
	}
	if _, err := os.Stat(filepath.Join(staged.path, "intruder.txt")); err != nil {
		t.Fatalf("tampered staging scope was broadened/deleted: %v", err)
	}
}
