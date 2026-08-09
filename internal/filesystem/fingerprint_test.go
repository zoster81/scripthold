package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/security"
	"golang.org/x/text/unicode/norm"
)

func TestCaptureRegularFileSnapshotBounded(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.bin")
	data := []byte("alpha\x00beta\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := CaptureRegularFileSnapshotBounded(context.Background(), path, int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := FingerprintRegularFileSnapshot(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != FingerprintRegularFileData(data) {
		t.Fatalf("bounded fingerprint = %s, want %s", fingerprint, FingerprintRegularFileData(data))
	}
	if _, err := CaptureRegularFileSnapshotBounded(context.Background(), path, int64(len(data)-1)); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("oversized error=%v kind=%s", err, operation.KindOf(err))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := CaptureRegularFileSnapshotBounded(ctx, path, int64(len(data))); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancelled error=%v kind=%s", err, operation.KindOf(err))
	}
}

func TestFingerprintRegularFileContentDigestMatchesDataFingerprint(t *testing.T) {
	data := []byte("alpha\x00beta\n")
	digest := sha256.Sum256(data)
	fromDigest, err := FingerprintRegularFileContentDigest(int64(len(data)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	if fromDigest != FingerprintRegularFileData(data) {
		t.Fatalf("digest-derived fingerprint = %s, data fingerprint = %s", fromDigest, FingerprintRegularFileData(data))
	}
	for _, test := range []struct {
		size   int64
		digest string
	}{
		{size: -1, digest: hex.EncodeToString(digest[:])},
		{size: int64(len(data)), digest: "invalid"},
	} {
		if _, err := FingerprintRegularFileContentDigest(test.size, test.digest); operation.KindOf(err) != operation.KindInvalidInput {
			t.Fatalf("FingerprintRegularFileContentDigest(%d, %q) error=%v kind=%s", test.size, test.digest, err, operation.KindOf(err))
		}
	}
}

func TestFingerprintPathsStableAcrossRootsAndMetadata(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	second := filepath.Join(parent, "second")
	for _, root := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(root, "nested"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "alpha.txt"), []byte("alpha\n"), 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "nested", "beta.bin"), []byte{0, 1, 2, 3}, 0644); err != nil {
			t.Fatal(err)
		}
	}

	options := FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{parent}),
		RespectGitignore:    true,
		IncludeEntries:      true,
		MaxEntries:          100,
		MaxEntryDetails:     100,
	}
	firstResult, err := FingerprintPaths(context.Background(), []string{first}, options)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := FingerprintPaths(context.Background(), []string{second}, options)
	if err != nil {
		t.Fatal(err)
	}
	if firstResult.Fingerprint != secondResult.Fingerprint {
		t.Fatalf("identical trees under different roots diverged: %s != %s", firstResult.Fingerprint, secondResult.Fingerprint)
	}
	if firstResult.FileCount != 2 || firstResult.DirectoryCount != 2 || firstResult.TotalBytes != 10 {
		t.Fatalf("unexpected counts: %+v", firstResult)
	}
	if got, want := entryPaths(firstResult.Entries), []string{".", "alpha.txt", "nested", "nested/beta.bin"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("entry paths = %v, want %v", got, want)
	}

	changedTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(filepath.Join(first, "alpha.txt"), changedTime, changedTime); err != nil {
		t.Fatal(err)
	}
	metadataOnly, err := FingerprintPaths(context.Background(), []string{first}, options)
	if err != nil {
		t.Fatal(err)
	}
	if metadataOnly.Fingerprint != firstResult.Fingerprint {
		t.Fatalf("metadata-only change altered content fingerprint: %s != %s", metadataOnly.Fingerprint, firstResult.Fingerprint)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(filepath.Join(first, "alpha.txt"), 0600); err != nil {
			t.Fatal(err)
		}
		permissionOnly, err := FingerprintPaths(context.Background(), []string{first}, options)
		if err != nil {
			t.Fatal(err)
		}
		if permissionOnly.Fingerprint != firstResult.Fingerprint {
			t.Fatalf("permission-only change altered content fingerprint: %s != %s", permissionOnly.Fingerprint, firstResult.Fingerprint)
		}
	}

	if err := os.WriteFile(filepath.Join(first, "alpha.txt"), []byte("ALPHA\n"), 0644); err != nil {
		t.Fatal(err)
	}
	contentChanged, err := FingerprintPaths(context.Background(), []string{first}, options)
	if err != nil {
		t.Fatal(err)
	}
	if contentChanged.Fingerprint == firstResult.Fingerprint {
		t.Fatal("content change did not alter fingerprint")
	}
}

func TestFingerprintPathsDistinguishesOrderedRootsWithSharedRelativePaths(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	second := filepath.Join(parent, "second")
	writeFingerprintFixture(t, filepath.Join(first, "same.txt"), "first")
	writeFingerprintFixture(t, filepath.Join(second, "same.txt"), "second")
	options := FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{parent}),
		RespectGitignore:    true,
		IncludeEntries:      true,
		MaxEntries:          100,
		MaxEntryDetails:     100,
	}

	ordered, err := FingerprintPaths(context.Background(), []string{first, second}, options)
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := FingerprintPaths(context.Background(), []string{second, first}, options)
	if err != nil {
		t.Fatal(err)
	}
	if ordered.Fingerprint == reversed.Fingerprint {
		t.Fatal("root order and root index association did not affect aggregate fingerprint")
	}
	if got, want := []int{ordered.Entries[0].RootIndex, ordered.Entries[1].RootIndex, ordered.Entries[2].RootIndex, ordered.Entries[3].RootIndex}, []int{0, 0, 1, 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("root indexes = %v, want %v", got, want)
	}
}

func TestFingerprintPathsNormalizesUnicodePathsAndRejectsCanonicalCollisions(t *testing.T) {
	parent := t.TempDir()
	first := filepath.Join(parent, "first")
	second := filepath.Join(parent, "second")
	nfcName := "café.txt"
	nfdName := norm.NFD.String(nfcName)
	writeFingerprintFixture(t, filepath.Join(first, nfcName), "content")
	writeFingerprintFixture(t, filepath.Join(second, nfdName), "content")
	options := FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{parent}),
		RespectGitignore:    true,
		IncludeEntries:      true,
		MaxEntries:          100,
		MaxEntryDetails:     100,
	}

	firstResult, err := FingerprintPaths(context.Background(), []string{first}, options)
	if err != nil {
		t.Fatal(err)
	}
	secondResult, err := FingerprintPaths(context.Background(), []string{second}, options)
	if err != nil {
		t.Fatal(err)
	}
	if firstResult.Fingerprint != secondResult.Fingerprint {
		t.Fatalf("Unicode-equivalent paths diverged: %s != %s", firstResult.Fingerprint, secondResult.Fingerprint)
	}
	if got := firstResult.Entries[1].Path; got != nfcName {
		t.Fatalf("canonical path = %q, want NFC %q", got, nfcName)
	}
	if got := secondResult.Entries[1].Path; got != nfcName {
		t.Fatalf("canonical path = %q, want NFC %q", got, nfcName)
	}

	collisionRoot := filepath.Join(parent, "collision")
	writeFingerprintFixture(t, filepath.Join(collisionRoot, nfcName), "first")
	if err := os.WriteFile(filepath.Join(collisionRoot, nfdName), []byte("second"), 0644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(collisionRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Skip("filesystem normalizes canonically equivalent names")
	}
	_, err = FingerprintPaths(context.Background(), []string{collisionRoot}, options)
	if operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("canonical collision error = %v, kind=%s", err, operation.KindOf(err))
	}
}

func TestFingerprintPathsExcludesSafeLinksWithoutFollowingThem(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	writeFingerprintFixture(t, target, "content")
	options := FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
		IncludeEntries:      true,
		MaxEntries:          100,
		MaxEntryDetails:     100,
	}
	before, err := FingerprintPaths(context.Background(), []string{root}, options)
	if err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(root, "alias.txt")
	if err := os.Symlink(target, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable on Windows: %v", err)
		}
		t.Fatal(err)
	}
	after, err := FingerprintPaths(context.Background(), []string{root}, options)
	if err != nil {
		t.Fatal(err)
	}
	if after.Fingerprint != before.Fingerprint {
		t.Fatalf("safe link changed content fingerprint: %s != %s", after.Fingerprint, before.Fingerprint)
	}
	if strings.Contains(strings.Join(entryPaths(after.Entries), "\n"), "alias.txt") {
		t.Fatalf("safe link was included in fingerprint entries: %+v", after.Entries)
	}
}

func TestFingerprintPathsRespectsGitignoreAndAlwaysExcludesGitDirectory(t *testing.T) {
	root := t.TempDir()
	writeFingerprintFixture(t, filepath.Join(root, ".gitignore"), "ignored.txt\nignored-dir/\n")
	writeFingerprintFixture(t, filepath.Join(root, "kept.txt"), "kept")
	writeFingerprintFixture(t, filepath.Join(root, "ignored.txt"), "ignored")
	writeFingerprintFixture(t, filepath.Join(root, "ignored-dir", "hidden.txt"), "hidden")
	writeFingerprintFixture(t, filepath.Join(root, ".git", "config"), "secret")

	options := FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
		IncludeEntries:      true,
		MaxEntries:          100,
		MaxEntryDetails:     100,
	}
	respected, err := FingerprintPaths(context.Background(), []string{root}, options)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(entryPaths(respected.Entries), "\n")
	for _, forbidden := range []string{"ignored.txt", "ignored-dir", ".git/config"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("ignored path %q present in %q", forbidden, joined)
		}
	}

	options.RespectGitignore = false
	unignored, err := FingerprintPaths(context.Background(), []string{root}, options)
	if err != nil {
		t.Fatal(err)
	}
	joined = strings.Join(entryPaths(unignored.Entries), "\n")
	for _, required := range []string{"ignored.txt", "ignored-dir", "ignored-dir/hidden.txt"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("expected path %q in %q", required, joined)
		}
	}
	if strings.Contains(joined, ".git/config") {
		t.Fatalf(".git contents must remain excluded: %q", joined)
	}
}

func TestFingerprintPathsLimitsCancellationAndUnsafeLinks(t *testing.T) {
	root := t.TempDir()
	writeFingerprintFixture(t, filepath.Join(root, "a.txt"), "a")
	writeFingerprintFixture(t, filepath.Join(root, "b.txt"), "b")

	_, err := FingerprintPaths(context.Background(), []string{root}, FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
		MaxEntries:          2,
	})
	if operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("entry limit error = %v, kind=%s", err, operation.KindOf(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = FingerprintPaths(ctx, []string{root}, FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
		MaxEntries:          100,
	})
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancellation error = %v, kind=%s", err, operation.KindOf(err))
	}

	outside := t.TempDir()
	writeFingerprintFixture(t, filepath.Join(outside, "outside.txt"), "outside")
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable on Windows: %v", err)
		}
		t.Fatal(err)
	}
	_, err = FingerprintPaths(context.Background(), []string{root}, FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
		MaxEntries:          100,
	})
	if operation.KindOf(err) != operation.KindSymlinkEscape {
		t.Fatalf("unsafe link error = %v, kind=%s", err, operation.KindOf(err))
	}
}

func TestFingerprintPathsDetectsChangesBetweenPasses(t *testing.T) {
	t.Run("same-size file with restored timestamp", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "data.bin")
		if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}

		_, err = fingerprintPathsWithHook(context.Background(), []string{path}, FingerprintOptions{
			ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
			RespectGitignore:    true,
			MaxEntries:          100,
		}, func() error {
			if err := os.WriteFile(path, []byte("modified"), 0644); err != nil {
				return err
			}
			return os.Chtimes(path, info.ModTime(), info.ModTime())
		})
		if operation.KindOf(err) != operation.KindConflict {
			t.Fatalf("concurrent file change error = %v, kind=%s", err, operation.KindOf(err))
		}
	})

	t.Run("directory entry added", func(t *testing.T) {
		root := t.TempDir()
		writeFingerprintFixture(t, filepath.Join(root, "first.txt"), "first")
		_, err := fingerprintPathsWithHook(context.Background(), []string{root}, FingerprintOptions{
			ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
			RespectGitignore:    true,
			MaxEntries:          100,
		}, func() error {
			return os.WriteFile(filepath.Join(root, "second.txt"), []byte("second"), 0644)
		})
		if operation.KindOf(err) != operation.KindConflict {
			t.Fatalf("directory change error = %v, kind=%s", err, operation.KindOf(err))
		}
	})

	t.Run("required file removed", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "required.txt")
		writeFingerprintFixture(t, path, "required")
		_, err := fingerprintPathsWithHook(context.Background(), []string{path}, FingerprintOptions{
			ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
			RespectGitignore:    true,
			MaxEntries:          100,
		}, func() error {
			return os.Remove(path)
		})
		if operation.KindOf(err) != operation.KindConflict {
			t.Fatalf("removed file error = %v, kind=%s", err, operation.KindOf(err))
		}
	})
}

func TestFingerprintPathsRejectsExplicitEscapingRoot(t *testing.T) {
	allowed := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")
	writeFingerprintFixture(t, outsideFile, "outside")
	link := filepath.Join(allowed, "outside-link.txt")
	if err := os.Symlink(outsideFile, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable on Windows: %v", err)
		}
		t.Fatal(err)
	}

	_, err := FingerprintPaths(context.Background(), []string{link}, FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{allowed}),
		RespectGitignore:    true,
		MaxEntries:          100,
	})
	if operation.KindOf(err) != operation.KindSymlinkEscape {
		t.Fatalf("explicit unsafe root error = %v, kind=%s", err, operation.KindOf(err))
	}
}

func TestFingerprintPathsBoundsEntryDetailsWithoutTruncatingAggregate(t *testing.T) {
	root := t.TempDir()
	writeFingerprintFixture(t, filepath.Join(root, "a.txt"), "a")
	writeFingerprintFixture(t, filepath.Join(root, "b.txt"), "b")

	limited, err := FingerprintPaths(context.Background(), []string{root}, FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
		IncludeEntries:      true,
		MaxEntries:          100,
		MaxEntryDetails:     1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited.Entries) != 1 || !limited.EntriesTruncated || limited.FileCount != 2 || limited.DirectoryCount != 1 {
		t.Fatalf("unexpected bounded details result: %+v", limited)
	}

	full, err := FingerprintPaths(context.Background(), []string{root}, FingerprintOptions{
		ResolvedAllowedDirs: security.ResolveAllowedDirs([]string{root}),
		RespectGitignore:    true,
		IncludeEntries:      false,
		MaxEntries:          100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if limited.Fingerprint != full.Fingerprint {
		t.Fatalf("entry detail retention changed aggregate: %s != %s", limited.Fingerprint, full.Fingerprint)
	}
}

func writeFingerprintFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func entryPaths(entries []FingerprintEntry) []string {
	paths := make([]string, len(entries))
	for index, entry := range entries {
		paths[index] = entry.Path
	}
	return paths
}
