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

// This full-file oracle is manually derived from the original 38-line
// null_mutex.h; parameter defaults, macros, and comments are not declarations.
// Exact matches are counted for one bounded scope, not extrapolated to C++.
func TestSourceSymbolsOriginalCppSpdlogNullMutexCompleteOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original source corpus is not configured")
	}
	path := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-gabime-spdlog",
		"include", "spdlog", "details", "null_mutex.h")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"struct null_mutex {", "struct null_atomic_int {",
		"int value{0};", "null_atomic_int() = default;",
		"explicit null_atomic_int(int new_value)",
		"int load(std::memory_order = std::memory_order_relaxed) const",
		"int exchange(int new_value, std::memory_order = std::memory_order_relaxed)",
	} {
		if !strings.Contains(string(raw), marker) {
			t.Fatalf("original source oracle changed: %s", marker)
		}
	}
	h := NewHandler([]string{root})
	failure, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failure != nil || !outline.CoverageComplete ||
		outline.Truncated || len(outline.Files) != 1 || outline.Files[0].SourceFingerprint == "" {
		t.Fatalf("original header not fully analyzed: failure=%+v err=%v", failure, err)
	}
	type expectation struct{ kind, owner string }
	expected := map[string]expectation{
		"details@13":                                 {"namespace", ""},
		"details.null_mutex@14":                      {"struct", "details"},
		"details.null_mutex.lock@15":                 {"method", "details.null_mutex"},
		"details.null_mutex.unlock@16":               {"method", "details.null_mutex"},
		"details.null_atomic_int@19":                 {"struct", "details"},
		"details.null_atomic_int.value@20":           {"field", "details.null_atomic_int"},
		"details.null_atomic_int.null_atomic_int@21": {"constructor", "details.null_atomic_int"},
		"details.null_atomic_int.null_atomic_int@23": {"constructor", "details.null_atomic_int"},
		"details.null_atomic_int.load@26":            {"method", "details.null_atomic_int"},
		"details.null_atomic_int.store@28":           {"method", "details.null_atomic_int"},
		"details.null_atomic_int.exchange@30":        {"method", "details.null_atomic_int"},
	}
	tp, fp := 0, 0
	for _, sym := range outline.Symbols {
		key := sym.QualifiedName + "@" + strconv.Itoa(sym.NameRange.Start.Line)
		want, ok := expected[key]
		if !ok || string(sym.Kind) != want.kind ||
			sym.ParentQualifiedName != want.owner || sym.ID == "" ||
			(want.owner != "" && sym.ParentID == "") {
			fp++
			t.Errorf("false/wrong original symbol: %+v (expected=%+v present=%v)", sym, want, ok)
			continue
		}
		tp++
		delete(expected, key)
	}
	fn := len(expected)
	for missing := range expected {
		t.Errorf("missing original declaration: %s", missing)
	}
	if fp != 0 || fn != 0 || tp != 11 {
		t.Fatalf("original bounded full-file precision/recall: TP=%d FP=%d FN=%d returned=%d; not a project-wide metric",
			tp, fp, fn, len(outline.Symbols))
	}
	t.Logf("original spdlog null_mutex.h complete oracle: TP=%d FP=%d FN=%d (scope: one 38-line file)", tp, fp, fn)
}

// A second full original spdlog file checks an exported struct, five
// constructors, an assignment operator, and eight fields. Macro tokens and
// parameter names are forbidden from becoming parentless declarations.
func TestSourceSymbolsOriginalCppSpdlogLogMsgCompleteOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original source corpus is not configured")
	}
	path := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-gabime-spdlog",
		"include", "spdlog", "details", "log_msg.h")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{
		"struct SPDLOG_API log_msg {", "log_msg() = default;", "log_msg(const log_msg &other) = default;",
		"log_msg &operator=(const log_msg &other) = default;", "level::level_enum level{level::off};",
		"mutable size_t color_range_start{0};",
	} {
		if !strings.Contains(string(raw), text) {
			t.Fatalf("original oracle changed: %s", text)
		}
	}
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !out.CoverageComplete || out.Truncated ||
		len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original log_msg.h not fully analyzed: err=%v fail=%+v", err, failed)
	}
	type expectation struct{ kind, owner string }
	const owner = "details.log_msg"
	oracle := map[string]expectation{
		"details@10":                           {"namespace", ""},
		"details.log_msg@11":                   {"struct", "details"},
		"details.log_msg.log_msg@12":           {"constructor", owner},
		"details.log_msg.log_msg@13":           {"constructor", owner},
		"details.log_msg.log_msg@18":           {"constructor", owner},
		"details.log_msg.log_msg@19":           {"constructor", owner},
		"details.log_msg.log_msg@20":           {"constructor", owner},
		"details.log_msg.operator=@21":         {"operator", owner},
		"details.log_msg.logger_name@23":       {"field", owner},
		"details.log_msg.level@24":             {"field", owner},
		"details.log_msg.time@25":              {"field", owner},
		"details.log_msg.thread_id@26":         {"field", owner},
		"details.log_msg.color_range_start@29": {"field", owner},
		"details.log_msg.color_range_end@30":   {"field", owner},
		"details.log_msg.source@32":            {"field", owner},
		"details.log_msg.payload@33":           {"field", owner},
	}
	tp, fp := 0, 0
	for _, sym := range out.Symbols {
		key := sym.QualifiedName + "@" + strconv.Itoa(sym.NameRange.Start.Line)
		want, ok := oracle[key]
		if !ok || string(sym.Kind) != want.kind || sym.ParentQualifiedName != want.owner ||
			sym.ID == "" || (want.owner != "" && sym.ParentID == "") {
			fp++
			t.Errorf("false/incorrect original declaration: %+v (wanted %+v exists=%v)", sym, want, ok)
			continue
		}
		tp++
		delete(oracle, key)
	}
	fn := len(oracle)
	for missing := range oracle {
		t.Errorf("missing original declaration: %s", missing)
	}
	if tp != 16 || fp != 0 || fn != 0 {
		t.Fatalf("original log_msg.h bounded whole-file oracle TP=%d FP=%d FN=%d returned=%d",
			tp, fp, fn, len(out.Symbols))
	}
	t.Logf("original spdlog log_msg.h: TP=%d FP=%d FN=%d (41-line scope only)", tp, fp, fn)
}

// Inventory the complete original spdlog project, including bundled fmt.
// Three original fmt headers use unresolved C++ macros; all other source
// files must remain fully covered, with no silently excluded inputs.
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
	expectedPartial := map[string]bool{
		filepath.Join(project, "include", "spdlog", "fmt", "bundled", "base.h"):   true,
		filepath.Join(project, "include", "spdlog", "fmt", "bundled", "format.h"): true,
		filepath.Join(project, "include", "spdlog", "fmt", "bundled", "ranges.h"): true,
	}
	observedPartial := make(map[string]bool)
	for start := 0; start < eligible; start += batchSize {
		end := min(start+batchSize, eligible)
		batchComplete := true
		for _, file := range files[start:end] {
			if expectedPartial[file] {
				batchComplete = false
			}
		}
		inputs := files[start:end]
		fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: inputs,
			Language: "cpp", Encoding: "utf-8", MaxFiles: batchSize,
		})
		if err != nil || fail != nil || digest.CoverageComplete != batchComplete ||
			digest.Truncated || digest.FilesConsidered != len(inputs) ||
			digest.FilesParsed != len(inputs) || digest.FilesSkipped != 0 ||
			len(digest.OverviewFiles) != len(inputs) {
			var partial []string
			for _, f := range digest.OverviewFiles {
				if !f.CoverageComplete {
					partial = append(partial, f.Path)
				}
			}
			t.Fatalf("original spdlog C++ batch %d:%d incomplete err=%v failure=%+v considered=%d parsed=%d skipped=%d complete=%v partial=%v",
				start, end, err, fail, digest.FilesConsidered, digest.FilesParsed, digest.FilesSkipped, digest.CoverageComplete, partial)
		}
		expected := make(map[string]bool, len(inputs))
		for _, p := range inputs {
			expected[p] = true
		}
		for _, file := range digest.OverviewFiles {
			if !expected[file.Path] || fingerprints[file.Path] != "" ||
				file.CoverageComplete == expectedPartial[file.Path] ||
				file.Status != "parsed" || file.Language != "cpp" || file.SourceFingerprint == "" {
				t.Errorf("missing/incorrectly covered/duplicated original spdlog file: %+v", file)
			}
			if !file.CoverageComplete {
				observedPartial[file.Path] = true
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
	if len(observedPartial) != len(expectedPartial) {
		t.Errorf("spdlog partial-source inventory changed: %d instead of %d", len(observedPartial), len(expectedPartial))
	}
	for file := range expectedPartial {
		if !observedPartial[file] {
			t.Errorf("original spdlog unresolved macro source not marked partial: %s", file)
		}
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
		queryBatchComplete := true
		for _, file := range files[start:end] {
			if expectedPartial[file] {
				queryBatchComplete = false
			}
		}
		if err != nil || fail != nil || query.Search == nil ||
			query.Coverage.CoverageComplete != queryBatchComplete || query.Coverage.FilesParsed != end-start {
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
	t.Logf("original spdlog C++ project: %d sources inventoried, %d complete, %d explicitly partial, %d batches; 1 known written structural declaration vs 16 text grep occurrences", eligible, eligible-len(observedPartial), len(observedPartial), (eligible+batchSize-1)/batchSize)
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
