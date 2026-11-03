package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// The original pinned CLI11 repository includes tests, examples and library
// headers. Reconcile the full C++ source set in bounded MCP batches.
// CLI/Error.hpp uses macros that generate constructors: report it as partial,
// rather than claiming semantic completeness for those generated declarations.
func TestSourceSymbolsOriginalCppCLI11WholeProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-cliutils-cli11")
	var originals []string
	err := filepath.WalkDir(project, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		switch filepath.Ext(path) {
		case ".cpp", ".cc", ".cxx", ".hpp", ".h", ".hxx", ".ipp", ".inl", ".tpp":
			if e.Type().IsRegular() {
				originals = append(originals, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(originals)
	const eligible = 111
	if len(originals) != eligible {
		t.Fatalf("original C++ CLI11 source inventory drifted: %d instead of %d", len(originals), eligible)
	}
	h := NewHandler([]string{root})
	const batchSize = 32
	seen := make(map[string]bool, eligible)
	fingerprints := make(map[string]string, eligible)
	expectedPartial := map[string]bool{
		filepath.Join(project, "include", "CLI", "Error.hpp"): true,
	}
	observedPartial := make(map[string]bool)
	for start := 0; start < eligible; start += batchSize {
		end := min(start+batchSize, eligible)
		batchComplete := true
		for _, file := range originals[start:end] {
			if expectedPartial[file] {
				batchComplete = false
			}
		}
		fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: originals[start:end],
			Language: "cpp", Encoding: "utf-8", MaxFiles: batchSize,
		})
		if err != nil || fail != nil || digest.Truncated || digest.CoverageComplete != batchComplete ||
			digest.FilesConsidered != end-start || digest.FilesParsed != end-start ||
			digest.FilesSkipped != 0 || len(digest.OverviewFiles) != end-start {
			var partial []string
			for _, f := range digest.OverviewFiles {
				if !f.CoverageComplete {
					partial = append(partial, f.Path)
				}
			}
			t.Fatalf("C++ CLI11 structural batch %d:%d incomplete: err=%v fail=%+v files=%d parsed=%d skipped=%d complete=%v partial=%v",
				start, end, err, fail, digest.FilesConsidered, digest.FilesParsed,
				digest.FilesSkipped, digest.CoverageComplete, partial)
		}
		required := make(map[string]bool, end-start)
		for _, file := range originals[start:end] {
			required[file] = true
		}
		for _, file := range digest.OverviewFiles {
			if !required[file.Path] || seen[file.Path] || file.Language != "cpp" ||
				file.Status != "parsed" || file.CoverageComplete == expectedPartial[file.Path] || file.SourceFingerprint == "" {
				t.Errorf("unexpected/incorrect C++ source coverage: %+v", file)
			}
			if !file.CoverageComplete {
				observedPartial[file.Path] = true
			}
			seen[file.Path] = true
			fingerprints[file.Path] = file.SourceFingerprint
			delete(required, file.Path)
		}
		for path := range required {
			t.Errorf("original C++ file missing from batch: %s", path)
		}
	}
	if len(seen) != eligible {
		t.Errorf("only %d/%d C++ project source files reconciled", len(seen), eligible)
	}
	if len(observedPartial) != len(expectedPartial) {
		t.Errorf("CLI11 partial-source inventory changed: got %d, expected %d", len(observedPartial), len(expectedPartial))
	}
	for file := range expectedPartial {
		if !observedPartial[file] {
			t.Errorf("original CLI11 macro-generated declarations not marked partial: %s", file)
		}
	}
	// Independent full-tree grep finds exactly one written Simple() declaration
	// in the original 111 files; structured search must preserve its class,
	// kind, location and source identity, without mistaking calls for symbols.
	found := 0
	for start := 0; start < eligible; start += batchSize {
		end := min(start+batchSize, eligible)
		fail, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact", Query: "Simple",
			Paths: originals[start:end], Language: "cpp", Encoding: "utf-8",
			MaxFiles: batchSize, MaxResults: 10,
		})
		queryBatchComplete := true
		for _, file := range originals[start:end] {
			if expectedPartial[file] {
				queryBatchComplete = false
			}
		}
		if err != nil || fail != nil || query.Coverage.CoverageComplete != queryBatchComplete ||
			query.Coverage.FilesParsed != end-start || query.Search == nil {
			t.Fatalf("original CLI11 project-wide structural search %d:%d incomplete: %v %+v %+v", start, end, err, fail, query.Coverage)
		}
		for _, m := range query.Search.Matches {
			found++
			if m.QualifiedName != "CLI.Timer.Simple" || string(m.Kind) != "method" ||
				m.Range.Start.Line != 49 || m.Path != filepath.Join(project, "include", "CLI", "Timer.hpp") ||
				m.SymbolID == "" || m.SourceFingerprint != fingerprints[m.Path] {
				t.Errorf("original C++ project false or unbound Simple() declaration: %+v", m)
			}
		}
	}
	if found != 1 {
		t.Errorf("original CLI11 has one Simple() declaration, source query found %d", found)
	}
	t.Logf("original CLI11 C++ full-file inventory: %d files, %d fully covered, %d explicit macro-generated partial; %d bounded batches", eligible, eligible-len(observedPartial), len(observedPartial), (eligible+batchSize-1)/batchSize)
}

// This original-source oracle is COMPLETE for CLI/Timer.hpp, including
// constructor inheritance and operator declarations. Grepping names alone
// cannot distinguish fields, methods, overloads and false positive locals.
func TestSourceSymbolsOriginalCppCLI11TimerHeaderOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	header := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-cliutils-cli11", "include", "CLI", "Timer.hpp")
	h := NewHandler([]string{root})
	fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{header},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 100,
	})
	if err != nil || fail != nil || !outline.CoverageComplete ||
		outline.Truncated || len(outline.Files) != 1 || outline.Files[0].SourceFingerprint == "" {
		t.Fatalf("original CLI11 Timer.hpp unavailable: %v %+v", err, fail)
	}
	type expected struct {
		name, kind, owner string
		line              int
	}
	oracle := []expected{
		{"CLI", "namespace", "", 21},
		{"CLI.Timer", "class", "CLI", 24},
		{"CLI.Timer.clock", "alias", "CLI.Timer", 27},
		{"CLI.Timer.time_point", "alias", "CLI.Timer", 30},
		{"CLI.Timer.time_print_t", "alias", "CLI.Timer", 33},
		{"CLI.Timer.title_", "field", "CLI.Timer", 36},
		{"CLI.Timer.time_print_", "field", "CLI.Timer", 39},
		{"CLI.Timer.start_", "field", "CLI.Timer", 42},
		{"CLI.Timer.cycles", "field", "CLI.Timer", 45},
		{"CLI.Timer.Simple", "method", "CLI.Timer", 49},
		{"CLI.Timer.Big", "method", "CLI.Timer", 52},
		{"CLI.Timer.Timer", "constructor", "CLI.Timer", 59},
		{"CLI.Timer.time_it", "method", "CLI.Timer", 63},
		{"CLI.Timer.make_time_str", "method", "CLI.Timer", 81},
		{"CLI.Timer.make_time_str", "method", "CLI.Timer", 90},
		{"CLI.Timer.to_string", "method", "CLI.Timer", 109},
		{"CLI.Timer.operator/", "operator", "CLI.Timer", 112},
		{"CLI.AutoTimer", "class", "CLI", 119},
		{"CLI.AutoTimer.AutoTimer", "destructor", "CLI.AutoTimer", 124},
		{"operator<<", "operator", "", 130},
	}
	want := make(map[string]expected, len(oracle))
	for _, e := range oracle {
		want[e.name+"@"+stringLine(e.line)] = e
	}
	if len(outline.Symbols) != len(oracle) {
		t.Errorf("original Timer.hpp has %d reviewed C++ declarations; result has %d", len(oracle), len(outline.Symbols))
	}
	for _, actual := range outline.Symbols {
		key := actual.QualifiedName + "@" + stringLine(actual.NameRange.Start.Line)
		e, ok := want[key]
		if !ok || string(actual.Kind) != e.kind || actual.ParentQualifiedName != e.owner ||
			actual.ID == "" || e.owner != "" && actual.ParentID == "" {
			t.Errorf("unexpected/misclassified/misowned original C++ declaration %+v, want=%+v present=%v", actual, e, ok)
		}
		delete(want, key)
	}
	for missing := range want {
		t.Errorf("missing original CLI11 C++ declaration: %s", missing)
	}
}

func stringLine(n int) string {
	return strconv.Itoa(n)
}
