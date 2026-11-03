package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// A complete inventory scans every original Typesense C++ source, including
// tests and vendored headers. It does not imply complete macro semantics.
func TestSourceSymbolsOriginalCppTypesenseWholeProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original source corpus not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-typesense-typesense")
	var originals []string
	err := filepath.WalkDir(project, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		switch filepath.Ext(path) {
		case ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hxx", ".ipp", ".inl", ".tpp":
			originals = append(originals, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(originals)
	const expectedSources = 250
	if len(originals) != expectedSources {
		t.Fatalf("original Typesense source inventory changed: %d instead of %d", len(originals), expectedSources)
	}
	h := NewHandler([]string{root})
	fingerprints := make(map[string]string, expectedSources)
	// Exact original-source incomplete inventory, not an exclusion list.
	expectedPartial := map[string]bool{
		"include/filter.h":                 true,
		"include/filter_result_iterator.h": true,
		"include/json.hpp":                 true,
		"src/filter_result_iterator.cpp":   true,
		"src/http_server.cpp":              true,
	}
	observedPartial := make(map[string]bool, len(expectedPartial))
	partial := make([]string, 0)
	for start := 0; start < len(originals); start += 32 {
		end := min(start+32, len(originals))
		paths := originals[start:end]
		expected := make(map[string]bool, len(paths))
		for _, path := range paths {
			expected[path] = true
		}
		fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: paths,
			Language: "cpp", MaxFiles: 32,
		})
		if err != nil || fail != nil || out.Truncated ||
			out.FilesConsidered != len(paths) || out.FilesParsed != len(paths) ||
			out.FilesSkipped != 0 || len(out.OverviewFiles) != len(paths) {
			t.Fatalf("original Typesense batch %d:%d incomplete: err=%v fail=%+v considered=%d parsed=%d skipped=%d",
				start, end, err, fail, out.FilesConsidered, out.FilesParsed, out.FilesSkipped)
		}
		batchComplete := true
		for _, file := range out.OverviewFiles {
			if !expected[file.Path] || fingerprints[file.Path] != "" || file.Status != "parsed" ||
				file.Language != "cpp" || file.SourceFingerprint == "" {
				t.Errorf("missing, duplicate or unbound original file: %+v", file)
			}
			relative := filepath.ToSlash(strings.TrimPrefix(file.Path, project+string(filepath.Separator)))
			if file.CoverageComplete == expectedPartial[relative] {
				t.Errorf("original Typesense file coverage changed: %s complete=%v", relative, file.CoverageComplete)
			}
			if !file.CoverageComplete {
				batchComplete = false
				partial = append(partial, relative)
				observedPartial[relative] = true
			}
			fingerprints[file.Path] = file.SourceFingerprint
			delete(expected, file.Path)
		}
		if out.CoverageComplete != batchComplete {
			t.Errorf("original batch coverage disagrees with individual source coverage at %d:%d", start, end)
		}
		for path := range expected {
			t.Errorf("original source excluded: %s", path)
		}
	}
	if len(fingerprints) != expectedSources {
		t.Fatalf("only %d/%d original source fingerprints found", len(fingerprints), expectedSources)
	}
	if len(observedPartial) != len(expectedPartial) {
		t.Errorf("original Typesense incomplete-source inventory changed: got %d expected %d", len(observedPartial), len(expectedPartial))
	}
	for path := range expectedPartial {
		if !observedPartial[path] {
			t.Errorf("original Typesense expected partial source not observed: %s", path)
		}
	}
	sort.Strings(partial)
	t.Logf("original Typesense %d files: %d structurally complete, %d partial across 8 bounded batches", expectedSources, expectedSources-len(partial), len(partial))
	for _, path := range partial {
		t.Logf("PARTIAL %s", path)
	}

	// Independent whole-project original grep reports one text occurrence of
	// ExecTime, the authored class in include/exectime.h:6. Its members are
	// not separate declarations named ExecTime.
	found := false
	header := filepath.Join(project, "include", "exectime.h")
	for start := 0; start < len(originals); start += 32 {
		end := min(start+32, len(originals))
		paths := originals[start:end]
		fail, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact",
			Query: "ExecTime", Paths: paths, Language: "cpp",
			MaxFiles: 32, MaxResults: 12,
		})
		if err != nil || fail != nil || out.Search == nil ||
			out.Coverage.FilesParsed != len(paths) || out.Coverage.FilesSkipped != 0 {
			t.Fatalf("original Typesense structural search %d:%d incomplete: err=%v fail=%+v coverage=%+v", start, end, err, fail, out.Coverage)
		}
		for _, match := range out.Search.Matches {
			if found || match.Path != header || match.QualifiedName != "ExecTime" ||
				string(match.Kind) != "class" || match.Range.Start.Line != 6 ||
				match.SymbolID == "" || match.SourceFingerprint != fingerprints[header] {
				t.Errorf("invented, duplicated or misplaced original ExecTime declaration: %+v", match)
			}
			found = true
		}
	}
	if !found {
		t.Error("original Typesense ExecTime class omitted from project-wide search")
	}
}

// All four original declarations in a short CRLF-encoded Typesense header
// are independently annotated by name, kind, source line and ownership.
func TestSourceSymbolsOriginalCppTypesenseExecTimeFullHeaderOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original source corpus not configured")
	}
	file := filepath.Join(root, "qualification", "final-candidate-corpus",
		"project-cpp-typesense-typesense", "include", "exectime.h")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{file},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || fail != nil || out.Truncated || !out.CoverageComplete ||
		len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("complete original Typesense header oracle unavailable: err=%v fail=%+v", err, fail)
	}
	type expected struct {
		kind, parent string
		line         int
	}
	want := map[string]expected{
		"ExecTime":       {"class", "", 6},
		"ExecTime.begin": {"field", "ExecTime", 7},
		"ExecTime.start": {"method", "ExecTime", 9},
		"ExecTime.log":   {"method", "ExecTime", 13},
	}
	if len(out.Symbols) != len(want) {
		t.Errorf("original header has four authored symbols, returned %d: %+v", len(out.Symbols), out.Symbols)
	}
	methodID := ""
	for _, symbol := range out.Symbols {
		expected, ok := want[symbol.QualifiedName]
		if !ok || string(symbol.Kind) != expected.kind ||
			symbol.NameRange.Start.Line != expected.line ||
			symbol.ParentQualifiedName != expected.parent || symbol.ID == "" ||
			(expected.parent != "" && symbol.ParentID == "") {
			t.Errorf("fabricated/missing/misowned original declaration: %+v expected=%+v present=%v", symbol, expected, ok)
		}
		if symbol.QualifiedName == "ExecTime.start" {
			methodID = symbol.ID
		}
		delete(want, symbol.QualifiedName)
	}
	for name := range want {
		t.Errorf("original source declaration missing: %s", name)
	}
	if methodID == "" {
		t.Fatal("original ExecTime.start method missing")
	}
	fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: file, SymbolID: methodID,
		SourceFingerprint: out.Files[0].SourceFingerprint,
		Language:          "cpp", Encoding: "utf-8", MaxBytes: 1000,
	})
	if err != nil || fail != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "high_resolution_clock::now()") {
		t.Errorf("original method body retrieval missing, stale or incorrect: err=%v fail=%+v", err, fail)
	}
}
