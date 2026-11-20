package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// A macro-defined namespace cannot be reconstructed from the header alone.
// Preserve the real alias, report unknown ownership, and never invent a
// function for either of the standalone namespace wrapper invocations.
func TestSourceSymbolsOriginalCppPybind11MacroNamespace(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	file := filepath.Join(root, "qualification", "final-candidate-corpus",
		"project-cpp-pybind-pybind11", "include", "pybind11", "detail", "using_smart_holder.h")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed",
		Paths: []string{file}, Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 40,
	})
	if err != nil || fail != nil || len(out.Files) != 1 || out.Truncated ||
		out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original pybind11 header unavailable: err=%v fail=%+v", err, fail)
	}
	if out.CoverageComplete || out.Files[0].CoverageComplete {
		t.Fatalf("unexpanded source-defined namespace macros must not claim complete scope: %+v", out.Files[0])
	}
	foundAlias := 0
	for _, s := range out.Symbols {
		if s.Name == "PYBIND11_NAMESPACE_BEGIN" || s.Name == "PYBIND11_NAMESPACE_END" {
			t.Errorf("macro wrapper misclassified as C++ function: %+v", s)
		}
		if s.Name == "is_smart_holder" {
			if string(s.Kind) != "alias" || s.NameRange.Start.Line != 19 || s.ParentQualifiedName != "" || s.ID == "" {
				t.Errorf("source-derived alias must retain its true name without guessed namespace ownership: %+v", s)
			}
			foundAlias++
		}
	}
	if foundAlias != 1 {
		t.Errorf("original pybind11 source has one explicitly authored alias at line19; returned %d", foundAlias)
	}
}

// A project-wide scan includes every original C++ source/header, including
// .inl sources that would be silently lost by a .h/.cpp-only glob. Partial
// files remain explicit failures of semantic qualification, never exclusions.
func TestSourceSymbolsOriginalCppPybind11AllSourceInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-pybind-pybind11")
	var files []string
	err := filepath.WalkDir(project, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		if !e.Type().IsRegular() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hxx", ".ipp", ".inl", ".tpp":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	const eligible = 164
	if len(files) != eligible {
		t.Fatalf("original pybind11 whole-source inventory changed: %d vs %d", len(files), eligible)
	}
	h := NewHandler([]string{root})
	const batchSize = 32
	seen := make(map[string]bool, eligible)
	partial := 0
	for first := 0; first < eligible; first += batchSize {
		last := min(first+batchSize, eligible)
		expected := make(map[string]bool, last-first)
		for _, p := range files[first:last] {
			expected[p] = true
		}
		fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: files[first:last],
			Language: "cpp", Encoding: "utf-8", MaxFiles: batchSize,
		})
		if err != nil || fail != nil || out.Truncated || out.FilesConsidered != last-first ||
			out.FilesSkipped != 0 || out.FilesParsed != last-first ||
			len(out.OverviewFiles) != last-first {
			t.Fatalf("original pybind11 batch %d:%d incomplete inventory err=%v fail=%+v considered=%d parsed=%d skipped=%d files=%d",
				first, last, err, fail, out.FilesConsidered, out.FilesParsed, out.FilesSkipped, len(out.OverviewFiles))
		}
		for _, f := range out.OverviewFiles {
			if !expected[f.Path] || seen[f.Path] || f.Language != "cpp" ||
				f.Status != "parsed" || f.SourceFingerprint == "" {
				t.Errorf("wrong or missing pybind11 original source %+v", f)
			}
			if !f.CoverageComplete {
				partial++
			}
			seen[f.Path] = true
			delete(expected, f.Path)
		}
		for missing := range expected {
			t.Errorf("original source excluded from pybind11 batch: %s", missing)
		}
	}
	if len(seen) != eligible {
		t.Errorf("only %d/%d original pybind11 C++ files reconciled", len(seen), eligible)
	}
	if !seen[filepath.Join(project, "tests", "test_eigen_tensor.inl")] {
		t.Error("the original .inl test source must not be dropped")
	}
	if partial == 0 {
		t.Error("original pybind11 unresolved macro regions cannot all claim complete semantic coverage")
	}
	t.Logf("all %d original pybind11 C++ files selected; %d structurally partial files explicitly retained; not a project semantic signoff", eligible, partial)
}
