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

// Inventory every original C++ source, including Windows-specific code,
// tests and headers. Unknown macro-generated semantics must stay partial.
func TestSourceSymbolsOriginalCppTranslucentTBWholeProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	project := filepath.Join(root, "qualification", "final-candidate-corpus", "project-cpp-translucenttb-translucenttb")
	var sources []string
	err := filepath.WalkDir(project, func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && e.Name() == ".git" {
			return filepath.SkipDir
		}
		if !e.Type().IsRegular() {
			return nil
		}
		switch filepath.Ext(p) {
		case ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hxx", ".ipp", ".inl", ".tpp":
			sources = append(sources, p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(sources)
	const n = 194
	if len(sources) != n {
		t.Fatalf("original project file count changed: %d/%d", len(sources), n)
	}
	h := NewHandler([]string{root})
	fingerprints := map[string]string{}
	// Preserve the exact original-file incomplete baseline. No file is omitted.
	expectedPartial := map[string]bool{
		"Common/undoc/explorer.hpp":   true,
		"Common/util/concepts.hpp":    true,
		"Xaml/Controls/ColorPicker.h": true,
	}
	observedPartial := map[string]bool{}
	partial := 0
	for start := 0; start < n; start += 32 {
		end := min(start+32, n)
		want := map[string]bool{}
		for _, p := range sources[start:end] {
			want[p] = true
		}
		fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: sources[start:end],
			Language: "cpp", Encoding: "utf-8", MaxFiles: 32,
		})
		if err != nil || fail != nil || out.Truncated || out.FilesConsidered != end-start ||
			out.FilesParsed != end-start || out.FilesSkipped != 0 || len(out.OverviewFiles) != end-start {
			t.Fatalf("original TranslucentTB batch %d:%d incomplete: err=%v fail=%+v", start, end, err, fail)
		}
		batchComplete := true
		for _, f := range out.OverviewFiles {
			if !want[f.Path] || fingerprints[f.Path] != "" || f.Status != "parsed" ||
				f.Language != "cpp" || f.SourceFingerprint == "" {
				t.Errorf("missing/duplicated/unbound original source: %+v", f)
			}
			relative := filepath.ToSlash(strings.TrimPrefix(f.Path, project+string(filepath.Separator)))
			if f.CoverageComplete == expectedPartial[relative] {
				t.Errorf("original TranslucentTB source coverage changed: %s complete=%v", relative, f.CoverageComplete)
			}
			if !f.CoverageComplete {
				batchComplete = false
				partial++
				observedPartial[relative] = true
				t.Logf("PARTIAL %s", relative)
			}
			fingerprints[f.Path] = f.SourceFingerprint
			delete(want, f.Path)
		}
		if out.CoverageComplete != batchComplete {
			t.Errorf("batch coverage disagrees with individual source: %d:%d", start, end)
		}
		for p := range want {
			t.Errorf("original C++ file excluded: %s", p)
		}
	}
	if len(fingerprints) != n {
		t.Fatalf("only %d/%d source files reconciled", len(fingerprints), n)
	}
	if len(observedPartial) != len(expectedPartial) {
		t.Errorf("original TranslucentTB partial-source count changed: %d instead of %d", len(observedPartial), len(expectedPartial))
	}
	for relative := range expectedPartial {
		if !observedPartial[relative] {
			t.Errorf("expected incomplete original source was not reported: %s", relative)
		}
	}
	t.Logf("original TranslucentTB: 194 original files, %d structurally complete, %d partial", n-partial, partial)

	// Original project grep identifies four written occurrences of maybe_delete:
	// only its struct and constructor are authored declarations; the include
	// and unique_ptr usage are not symbols named maybe_delete.
	seen := map[string]bool{}
	header := filepath.Join(project, "Common", "util", "maybe_delete.hpp")
	for start := 0; start < n; start += 32 {
		end := min(start+32, n)
		fail, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact", Query: "maybe_delete",
			Paths: sources[start:end], Language: "cpp", Encoding: "utf-8",
			MaxFiles: 32, MaxResults: 16,
		})
		if err != nil || fail != nil || out.Search == nil || out.Coverage.FilesParsed != end-start {
			t.Fatalf("original project source query failed in %d:%d: err=%v fail=%+v", start, end, err, fail)
		}
		for _, m := range out.Search.Matches {
			key := m.QualifiedName + "@" + strconv.Itoa(m.Range.Start.Line)
			valid := (key == "Util.maybe_delete@4" && string(m.Kind) == "struct") ||
				(key == "Util.maybe_delete.maybe_delete@18" && string(m.Kind) == "constructor")
			if !valid || m.Path != header || m.SymbolID == "" ||
				m.SourceFingerprint != fingerprints[m.Path] || seen[key] {
				t.Errorf("false/unbound/duplicated original C++ declaration: %+v", m)
			}
			seen[key] = true
		}
	}
	if !seen["Util.maybe_delete@4"] || !seen["Util.maybe_delete.maybe_delete@18"] || len(seen) != 2 {
		t.Errorf("expected two genuine original declarations, got %+v", seen)
	}
}

// Source-derived exhaustive 5-declaration oracle for the real small header;
// a method call, template parameter and comment must not become declarations.
func TestSourceSymbolsOriginalCppTranslucentTBMaybeDeleteHeaderOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	path := filepath.Join(root, "qualification", "final-candidate-corpus",
		"project-cpp-translucenttb-translucenttb", "Common", "util", "maybe_delete.hpp")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
	})
	if err != nil || fail != nil || !out.CoverageComplete || out.Truncated ||
		len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original complete header unavailable: %v %+v", err, fail)
	}
	type expected struct {
		kind, owner string
		line        int
	}
	want := map[string]expected{
		"Util":                             {"namespace", "", 3},
		"Util.maybe_delete":                {"struct", "Util", 4},
		"Util.maybe_delete.m_ShouldDelete": {"field", "Util.maybe_delete", 6},
		"Util.maybe_delete.operator()":     {"operator", "Util.maybe_delete", 10},
		"Util.maybe_delete.maybe_delete":   {"constructor", "Util.maybe_delete", 18},
	}
	if len(out.Symbols) != len(want) {
		t.Errorf("original header has 5 declarations, got %d", len(out.Symbols))
	}
	operatorID := ""
	for _, s := range out.Symbols {
		e, ok := want[s.QualifiedName]
		if !ok || string(s.Kind) != e.kind || s.ParentQualifiedName != e.owner ||
			s.NameRange.Start.Line != e.line || s.ID == "" || (e.owner != "" && s.ParentID == "") {
			t.Errorf("invented/missing/misowned original C++ declaration: %+v want=%+v", s, e)
		}
		if s.QualifiedName == "Util.maybe_delete.operator()" {
			operatorID = s.ID
		}
		delete(want, s.QualifiedName)
	}
	for name := range want {
		t.Errorf("missing original declaration: %s", name)
	}
	if operatorID == "" {
		t.Fatal("operator() missing")
	}
	fail, show, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: operatorID,
		SourceFingerprint: out.Files[0].SourceFingerprint,
		Language:          "cpp", Encoding: "utf-8", MaxBytes: 1500,
	})
	if err != nil || fail != nil || show.Show == nil ||
		!strings.Contains(show.Show.Text, "delete that;") {
		t.Errorf("original C++ operator body unavailable or stale: %v %+v", err, fail)
	}
}

func TestSourceSymbolsOriginalCppTranslucentTBConceptIsNotAVariable(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus not configured")
	}
	path := filepath.Join(root, "qualification", "final-candidate-corpus",
		"project-cpp-translucenttb-translucenttb", "Common", "util", "concepts.hpp")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || fail != nil || out.Truncated || out.CoverageComplete ||
		len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original C++20 concept must report partial coverage: err=%v fail=%+v out=%+v", err, fail, out)
	}
	if len(out.Symbols) != 1 || out.Symbols[0].QualifiedName != "Util" ||
		string(out.Symbols[0].Kind) != "namespace" {
		t.Fatalf("concept must not become a variable; preserve namespace: %+v", out.Symbols)
	}
}
