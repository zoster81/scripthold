package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestR15ReadTextFileLineNumbers(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	if err := os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	offset, limit := 2, 2
	_, out, err := h.HandleReadTextFile(context.Background(), nil, ReadTextFileInput{
		Path: path, Offset: &offset, Limit: &limit, LineNumbers: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Content != "2\tbeta\n3\tgamma" {
		t.Fatalf("content = %q", out.Content)
	}
}

func TestR15DirectoryAndSearchSorting(t *testing.T) {
	root := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("small.txt", "x")
	write("large.txt", "123456789")
	write("middle.txt", "1234")

	h := NewHandler([]string{root})
	_, listed, _ := h.HandleListDirectory(context.Background(), nil, ListDirectoryInput{Path: root, SortBy: "size", Reverse: true})
	if want := []string{"large.txt", "middle.txt", "small.txt"}; !reflect.DeepEqual(listed.Files, want) {
		t.Fatalf("list = %#v, want %#v", listed.Files, want)
	}
	_, searched, _ := h.HandleSearchFiles(context.Background(), nil, SearchFilesInput{Path: root, Pattern: "*.txt", SortBy: "size", MaxResults: 2})
	if len(searched.Files) != 2 || filepath.Base(searched.Files[0]) != "small.txt" || filepath.Base(searched.Files[1]) != "middle.txt" || !searched.Truncated {
		t.Fatalf("search = %#v, truncated=%v", searched.Files, searched.Truncated)
	}
}

func TestR15GitignoreTraversal(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		".gitignore":         "ignored/\n*.tmp\n!important.tmp\n",
		"keep.txt":           "needle",
		"drop.tmp":           "needle",
		"important.tmp":      "needle",
		"ignored/hidden.txt": "needle",
	} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	_, out, _ := h.HandleSearchFiles(context.Background(), nil, SearchFilesInput{Path: root, Pattern: "*"})
	bases := make([]string, 0, len(out.Files))
	for _, path := range out.Files {
		bases = append(bases, filepath.Base(path))
	}
	joined := strings.Join(bases, ",")
	if strings.Contains(joined, "hidden.txt") || strings.Contains(joined, "drop.tmp") {
		t.Fatalf("ignored files leaked: %#v", out.Files)
	}
	if !strings.Contains(joined, "important.tmp") {
		t.Fatalf("negated file missing: %#v", out.Files)
	}
	respect := false
	_, unfiltered, _ := h.HandleSearchFiles(context.Background(), nil, SearchFilesInput{Path: root, Pattern: "*", RespectGitignore: &respect})
	if len(unfiltered.Files) <= len(out.Files) {
		t.Fatalf("respectGitignore=false did not restore entries: %d <= %d", len(unfiltered.Files), len(out.Files))
	}
}

func TestR15GrepModesPatternsAndPaging(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"a.txt": "foo one\nbar two\nnone\n",
		"b.txt": "foo three\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	_, counts, _ := h.HandleGrep(context.Background(), nil, GrepInput{
		Patterns: []string{"foo", "bar"}, Paths: []string{root}, OutputMode: "count", MaxMatches: 10,
	})
	if len(counts.Counts) != 2 || counts.Counts[0].Count != 2 || counts.Counts[1].Count != 1 {
		t.Fatalf("counts = %#v", counts.Counts)
	}
	_, page, _ := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "foo|bar", Paths: []string{root}, Offset: 1, MaxMatches: 1, MatchesOnly: true,
	})
	if len(page.Matches) != 1 || page.Matches[0].Text != "bar" || page.NextOffset != 2 {
		t.Fatalf("page = %#v next=%d", page.Matches, page.NextOffset)
	}
	_, files, _ := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "foo", Paths: []string{root}, OutputMode: "files_with_matches", MaxMatches: 10,
	})
	if len(files.Files) != 2 {
		t.Fatalf("files = %#v", files.Files)
	}
}

func TestR15ConvertEncodingBatchDryRun(t *testing.T) {
	root := t.TempDir()
	okPath := filepath.Join(root, "ok.txt")
	badPath := filepath.Join(root, "bad.txt")
	if err := os.WriteFile(okPath, []byte("plain text\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte("emoji 😀\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, out, _ := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Paths: []string{okPath, badPath}, To: "windows-1251", DryRun: true,
	})
	if len(out.Results) != 2 || out.SuccessCount != 1 || out.ErrorCount != 1 {
		t.Fatalf("batch = %#v successes=%d errors=%d", out.Results, out.SuccessCount, out.ErrorCount)
	}
	if out.Results[0].Changed {
		t.Fatalf("ASCII dry run should be byte-identical: %#v", out.Results[0])
	}
	if out.Results[1].UnsupportedCount == 0 || len(out.Results[1].Unsupported) == 0 || out.Results[1].Unsupported[0].Line != 1 {
		t.Fatalf("unsupported = %#v", out.Results[1].Unsupported)
	}
	data, err := os.ReadFile(badPath)
	if err != nil || string(data) != "emoji 😀\n" {
		t.Fatalf("dry run modified source: %q, %v", data, err)
	}
}

func TestR15EditFuzzyAndPatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "edit.txt")
	if err := os.WriteFile(path, []byte("header\nvalue = original\nfooter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	similarity := 0.80
	result, _, _ := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Path:  path,
		Edits: []EditOperation{{OldText: "value = origina1", NewText: "value = changed", Similarity: &similarity}},
	})
	if result.IsError {
		t.Fatalf("fuzzy edit failed: %#v", result.Content)
	}
	patch := "--- a/edit.txt\n+++ b/edit.txt\n@@ -1,3 +1,3 @@\n header\n-value = changed\n+value = patched\n footer\n"
	result, _, _ = h.HandleEditFile(context.Background(), nil, EditFileInput{Path: path, Patch: patch})
	if result.IsError {
		t.Fatalf("patch failed: %#v", result.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "header\nvalue = patched\nfooter\n" {
		t.Fatalf("patched content = %q", data)
	}
}

func TestR15FuzzyEditRejectsAmbiguity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "ambiguous.txt")
	original := "value = alpha\nseparator\nvalue = alpha\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	similarity := 0.8
	result, _, _ := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Path: path, Edits: []EditOperation{{OldText: "value = alphx", NewText: "changed", Similarity: &similarity}},
	})
	if !result.IsError {
		t.Fatal("ambiguous fuzzy edit unexpectedly succeeded")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatalf("ambiguous edit modified file: %q", data)
	}
}

func TestR15FuzzyEditRejectsEqualScoreCandidates(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "equal-score.txt")
	original := "ac\naxyz\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	similarity := 0.5
	result, _, _ := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Path: path, Edits: []EditOperation{{OldText: "az", NewText: "changed", Similarity: &similarity}},
	})
	if !result.IsError {
		t.Fatal("equal-score fuzzy candidates unexpectedly succeeded")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatalf("ambiguous fuzzy edit modified file: %q", data)
	}
}

func TestR15PatchRejectsInconsistentNewStart(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "new-start.txt")
	original := "alpha\nbeta\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "--- a/new-start.txt\n+++ b/new-start.txt\n@@ -1,1 +2,1 @@\n-alpha\n+changed\n"
	h := NewHandler([]string{root})
	result, _, _ := h.HandleEditFile(context.Background(), nil, EditFileInput{Path: path, Patch: patch})
	if !result.IsError {
		t.Fatal("patch with inconsistent new start unexpectedly succeeded")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatalf("invalid patch modified file: %q", data)
	}
}

func TestR15FuzzyDeleteDoesNotLeaveIndentation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "delete.txt")
	if err := os.WriteFile(path, []byte("header\n    value = original\nfooter\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	similarity := 0.8
	result, _, _ := h.HandleEditFile(context.Background(), nil, EditFileInput{
		Path: path, Edits: []EditOperation{{OldText: "value = origina1", NewText: "", Similarity: &similarity}},
	})
	if result.IsError {
		t.Fatalf("fuzzy deletion failed: %#v", result.Content)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "header\n\nfooter\n" {
		t.Fatalf("fuzzy deletion left content %q", data)
	}
}

func TestR15GrepFileModeReducesRetainedOutput(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 3; index++ {
		name := filepath.Join(root, fmt.Sprintf("file-%d.txt", index))
		content := "needle " + strings.Repeat("payload-", 100) + "\n"
		if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	_, contentOutput, _ := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "needle", Paths: []string{root}, OutputMode: "content", MaxMatches: 10,
	})
	_, filesOutput, _ := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "needle", Paths: []string{root}, OutputMode: "files_with_matches", MaxMatches: 10,
	})
	contentJSON, err := json.Marshal(contentOutput)
	if err != nil {
		t.Fatal(err)
	}
	filesJSON, err := json.Marshal(filesOutput)
	if err != nil {
		t.Fatal(err)
	}
	if len(filesJSON)*4 >= len(contentJSON) {
		t.Fatalf("files mode retained %d bytes, content mode %d bytes", len(filesJSON), len(contentJSON))
	}
	t.Logf("files mode retained %d JSON bytes versus %d for content mode", len(filesJSON), len(contentJSON))
}

func TestR15GrepCountDoesNotStarveLaterFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte(strings.Repeat("hit\n", 50)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("hit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, output, _ := h.HandleGrep(context.Background(), nil, GrepInput{
		Pattern: "hit", Paths: []string{root}, OutputMode: "count", MaxMatches: 2,
	})
	if len(output.Counts) != 2 || output.Counts[0].Count != 50 || output.Counts[1].Count != 1 {
		t.Fatalf("counts = %#v", output.Counts)
	}
}

func TestR15ListDirectoryMetadataSortSkipsExternalSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked.txt")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, defaultOutput, _ := h.HandleListDirectory(context.Background(), nil, ListDirectoryInput{Path: root})
	if !reflect.DeepEqual(defaultOutput.Files, []string{"linked.txt", "safe.txt"}) {
		t.Fatalf("default name listing = %#v", defaultOutput.Files)
	}
	_, output, _ := h.HandleListDirectory(context.Background(), nil, ListDirectoryInput{Path: root, SortBy: "size"})
	if !reflect.DeepEqual(output.Files, []string{"safe.txt"}) {
		t.Fatalf("metadata-sorted entries = %#v", output.Files)
	}
}

func TestR15PatchRejectsMultiFileWithoutMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "target.txt")
	original := "alpha\nbeta\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	patch := "--- a/target.txt\n+++ b/target.txt\n@@ -1,1 +1,1 @@\n-alpha\n+changed\n--- a/other.txt\n+++ b/other.txt\n@@ -1,1 +1,1 @@\n-old\n+new\n"
	h := NewHandler([]string{root})
	result, _, _ := h.HandleEditFile(context.Background(), nil, EditFileInput{Path: path, Patch: patch})
	if !result.IsError {
		t.Fatal("multi-file patch unexpectedly succeeded")
	}
	data, _ := os.ReadFile(path)
	if string(data) != original {
		t.Fatalf("multi-file patch modified target: %q", data)
	}
}

func TestR15RecursiveToolsFailClosedOnInvalidGitignore(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideIgnore := filepath.Join(outside, "outside.ignore")
	if err := os.WriteFile(outsideIgnore, []byte("*.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideIgnore, filepath.Join(root, ".gitignore")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "visible.txt"), []byte("needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := NewHandler([]string{root})
	searchResult, _, _ := h.HandleSearchFiles(context.Background(), nil, SearchFilesInput{Path: root, Pattern: "*"})
	if !searchResult.IsError {
		t.Fatal("search_files accepted an invalid .gitignore")
	}
	treeResult, _, _ := h.HandleTree(context.Background(), nil, TreeInput{Path: root})
	if !treeResult.IsError {
		t.Fatal("tree accepted an invalid .gitignore")
	}
	grepResult, _, _ := h.HandleGrep(context.Background(), nil, GrepInput{Pattern: "needle", Paths: []string{root}})
	if !grepResult.IsError {
		t.Fatal("grep_text_files accepted an invalid .gitignore")
	}
}

func TestR15ConvertEncodingRejectsBackupTargetCollision(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "primary.txt")
	backupTarget := primary + ".bak"
	primaryContent := "Привет\n"
	backupContent := "existing backup target\n"
	if err := os.WriteFile(primary, []byte(primaryContent), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(backupTarget, []byte(backupContent), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	result, _, _ := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Paths: []string{primary, backupTarget}, To: "windows-1251", Backup: true,
	})
	if !result.IsError {
		t.Fatal("backup/target collision unexpectedly succeeded")
	}
	primaryData, _ := os.ReadFile(primary)
	backupData, _ := os.ReadFile(backupTarget)
	if string(primaryData) != primaryContent || string(backupData) != backupContent {
		t.Fatalf("collision modified files: primary=%q backup=%q", primaryData, backupData)
	}
}

func FuzzR15UnifiedPatch(f *testing.F) {
	for _, seed := range []string{
		"--- a/target.txt\n+++ b/target.txt\n@@ -1,1 +1,1 @@\n-alpha\n+changed\n",
		"not a patch",
		"--- a/other.txt\n+++ b/other.txt\n@@ -1,1 +1,1 @@\n-alpha\n+changed\n",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, patch string) {
		if len(patch) > 64*1024 {
			t.Skip()
		}
		_, _ = applyUnifiedPatch("alpha\nbeta\n", patch, "target.txt", 64*1024)
	})
}

func FuzzR15FuzzyMatch(f *testing.F) {
	f.Add("alpha\nbeta\n", "alphx", "changed")
	f.Add("same\nsame\n", "sane", "changed")
	f.Fuzz(func(t *testing.T, content, oldText, newText string) {
		if len(content) > 64*1024 || len(oldText) > 8*1024 || len(newText) > 8*1024 {
			t.Skip()
		}
		_, _, _ = tryFuzzyMatch(content, oldText, newText, 0.8)
	})
}

func TestR15ConvertEncodingBatchPartialCommitIsExplicit(t *testing.T) {
	root := t.TempDir()
	convertible := filepath.Join(root, "convertible.txt")
	unsupported := filepath.Join(root, "unsupported.txt")
	if err := os.WriteFile(convertible, []byte("Привет\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unsupported, []byte("emoji 😀\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	_, output, _ := h.HandleConvertEncoding(context.Background(), nil, ConvertEncodingInput{
		Paths: []string{convertible, unsupported}, To: "windows-1251", Backup: true,
	})
	if output.SuccessCount != 1 || output.ErrorCount != 1 || len(output.Results) != 2 {
		t.Fatalf("batch output = %+v", output)
	}
	if output.Results[0].BackupPath == "" || output.Results[1].ErrorCode != ErrCodeEncoding || output.Results[1].EncodingErrorCode != EncodingErrorUnrepresentable {
		t.Fatalf("batch results = %+v", output.Results)
	}
	if _, err := os.Stat(convertible + ".bak"); err != nil {
		t.Fatalf("successful item backup missing: %v", err)
	}
	data, err := os.ReadFile(unsupported)
	if err != nil || string(data) != "emoji 😀\n" {
		t.Fatalf("unsupported item changed: %q, %v", data, err)
	}
}
