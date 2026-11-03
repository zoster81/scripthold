package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Cover the full original spdlog project: headers, implementation, examples,
// benchmarks, tests and bundled fmt sources. This is an exhaustive structural
// source screening, not a claim about complete project semantic correctness.
func TestSourceSymbolsOriginalCppSpdlogWholeProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original source corpus is not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-gabime-spdlog")
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
		case ".cpp", ".cc", ".cxx", ".hpp", ".h", ".hxx", ".ipp", ".inl", ".tpp":
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	const eligible = 151
	if len(files) != eligible {
		t.Fatalf("original spdlog source count changed: %d != %d", len(files), eligible)
	}
	h := NewHandler([]string{root})
	const batchSize = 32
	fingerprints := make(map[string]string, eligible)
	for start := 0; start < eligible; start += batchSize {
		end := min(start+batchSize, eligible)
		inputs := files[start:end]
		fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: inputs,
			Language: "cpp", Encoding: "utf-8", MaxFiles: batchSize,
		})
		if err != nil || fail != nil || !digest.CoverageComplete ||
			digest.Truncated || digest.FilesConsidered != len(inputs) ||
			digest.FilesParsed != len(inputs) || digest.FilesSkipped != 0 ||
			len(digest.OverviewFiles) != len(inputs) {
			t.Fatalf("original spdlog C++ batch %d:%d incomplete err=%v failure=%+v considered=%d parsed=%d skipped=%d complete=%v",
				start, end, err, fail, digest.FilesConsidered, digest.FilesParsed, digest.FilesSkipped, digest.CoverageComplete)
		}
		expected := make(map[string]bool, len(inputs))
		for _, p := range inputs {
			expected[p] = true
		}
		for _, file := range digest.OverviewFiles {
			if !expected[file.Path] || fingerprints[file.Path] != "" || !file.CoverageComplete ||
				file.Status != "parsed" || file.Language != "cpp" || file.SourceFingerprint == "" {
				t.Errorf("missing/partial/duplicated original spdlog file: %+v", file)
			}
			fingerprints[file.Path] = file.SourceFingerprint
			delete(expected, file.Path)
		}
		for p := range expected {
			t.Errorf("original file missing from batch: %s", p)
		}
	}
	if len(fingerprints) != eligible {
		t.Errorf("only %d/%d spdlog sources reconciled", len(fingerprints), eligible)
	}

	// Whole-project original grep has sixteen text occurrences of
	// console_mutex, but only one struct declaration. References are not
	// fabricated declaration symbols.
	declarations := 0
	for start := 0; start < eligible; start += batchSize {
		end := min(start+batchSize, eligible)
		fail, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact",
			Query: "console_mutex", Paths: files[start:end], Language: "cpp",
			Encoding: "utf-8", MaxFiles: batchSize, MaxResults: 16,
		})
		if err != nil || fail != nil || query.Search == nil ||
			!query.Coverage.CoverageComplete || query.Coverage.FilesParsed != end-start {
			t.Fatalf("original spdlog full-tree structural search incomplete at %d:%d: err=%v fail=%+v", start, end, err, fail)
		}
		for _, m := range query.Search.Matches {
			declarations++
			if m.Path != filepath.Join(project, "include", "spdlog", "details", "console_globals.h") ||
				m.QualifiedName != "details.console_mutex" || string(m.Kind) != "struct" ||
				m.Range.Start.Line != 12 || m.SourceFingerprint != fingerprints[m.Path] ||
				m.SymbolID == "" {
				t.Errorf("fabricated or unbound spdlog C++ structural declaration: %+v", m)
			}
		}
	}
	if declarations != 1 {
		t.Errorf("original spdlog has one console_mutex declaration, got %d", declarations)
	}
	t.Logf("original spdlog C++ project: %d fully selected source files, %d batches, 1 structural definition vs 16 text grep occurrences", eligible, (eligible+batchSize-1)/batchSize)
}

// This is an independently complete source-derived inventory for the small
// original console_globals.h header, including two structs and their members.
func TestSourceSymbolsOriginalCppSpdlogConsoleGlobalsFullOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original source corpus is not configured")
	}
	path := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-gabime-spdlog", "include", "spdlog", "details", "console_globals.h")
	h := NewHandler([]string{root})
	fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 30,
	})
	if err != nil || fail != nil || !outline.CoverageComplete ||
		outline.Truncated || len(outline.Files) != 1 || outline.Files[0].SourceFingerprint == "" {
		t.Fatalf("original spdlog console header could not be fully analyzed: %v %+v", err, fail)
	}
	type expected struct {
		kind, owner string
		line        int
	}
	oracle := []struct {
		name string
		e    expected
	}{
		{"details", expected{"namespace", "", 10}},
		{"details.console_mutex", expected{"struct", "details", 12}},
		{"details.console_mutex.mutex_t", expected{"alias", "details.console_mutex", 13}},
		{"details.console_mutex.mutex", expected{"method", "details.console_mutex", 14}},
		{"details.console_nullmutex", expected{"struct", "details", 20}},
		{"details.console_nullmutex.mutex_t", expected{"alias", "details.console_nullmutex", 21}},
		{"details.console_nullmutex.mutex", expected{"method", "details.console_nullmutex", 22}},
	}
	want := make(map[string]expected)
	for _, item := range oracle {
		want[item.name+"@"+strconv.Itoa(item.e.line)] = item.e
	}
	if len(outline.Symbols) != len(oracle) {
		t.Errorf("original spdlog console header: got %d symbols; independent oracle has %d", len(outline.Symbols), len(oracle))
	}
	for _, s := range outline.Symbols {
		key := s.QualifiedName + "@" + strconv.Itoa(s.NameRange.Start.Line)
		e, exists := want[key]
		if !exists || string(s.Kind) != e.kind || s.ParentQualifiedName != e.owner ||
			s.ID == "" || e.owner != "" && s.ParentID == "" {
			t.Errorf("original spdlog header misclassified/extra symbol: %+v, expected=%+v present=%v", s, e, exists)
		}
		delete(want, key)
	}
	for name := range want {
		t.Errorf("missing independent original spdlog header declaration: %s", name)
	}

	var chosen string
	for _, s := range outline.Symbols {
		if s.QualifiedName == "details.console_mutex.mutex" {
			chosen = s.ID
			break
		}
	}
	if chosen == "" {
		t.Fatal("original console_mutex.mutex missing from source-bound outline")
	}
	fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: chosen,
		SourceFingerprint: outline.Files[0].SourceFingerprint,
		Language:          "cpp", Encoding: "utf-8", MaxBytes: 2048,
	})
	if err != nil || fail != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "static mutex_t s_mutex;") ||
		!strings.Contains(shown.Show.Text, "return s_mutex;") {
		t.Fatalf("original spdlog member body show cannot match fresh source: err=%v fail=%+v", err, fail)
	}
}
