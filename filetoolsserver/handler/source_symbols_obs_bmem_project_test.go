package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The full original OBS Studio bmem.h declaration set (not only individual
// positive matches) is independently annotated from the unmodified header.
func TestSourceSymbolsOriginalOBSBmemFullHeaderOracle(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, "real-projects", "c-obsproject-obs-studio", "libobs", "util")
	header := filepath.Join(project, "bmem.h")
	source := filepath.Join(project, "bmem.c")
	h := NewHandler([]string{root})
	fail, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{header},
		Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 80,
	})
	if err != nil || fail != nil || !out.CoverageComplete || out.Truncated || len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
		t.Fatalf("original OBS bmem.h unavailable: err=%v failure=%+v result=%+v", err, fail, out)
	}
	type wantSymbol struct {
		kind, owner string
		line        int
	}
	want := map[string]wantSymbol{
		"base_allocator":         {"struct", "", 28},
		"base_allocator.malloc":  {"field", "base_allocator", 29},
		"base_allocator.realloc": {"field", "base_allocator", 30},
		"base_allocator.free":    {"field", "base_allocator", 31},
		"bmalloc":                {"function", "", 34},
		"brealloc":               {"function", "", 35},
		"bfree":                  {"function", "", 36},
		"base_get_alignment":     {"function", "", 38},
		"bnum_allocs":            {"function", "", 40},
		"bmemdup":                {"function", "", 42},
		"bzalloc":                {"function", "", 44},
		"bstrdup_n":              {"function", "", 52},
		"bwstrdup_n":             {"function", "", 64},
		"bstrdup":                {"function", "", 76},
		"bwstrdup":               {"function", "", 84},
	}
	if len(out.Symbols) != len(want) {
		t.Errorf("original OBS bmem.h has %d real symbols, got %d", len(want), len(out.Symbols))
	}
	for _, s := range out.Symbols {
		expected, exists := want[s.QualifiedName]
		if !exists || string(s.Kind) != expected.kind || s.ParentQualifiedName != expected.owner ||
			s.NameRange.Start.Line != expected.line || s.ID == "" ||
			(expected.owner != "" && s.ParentID == "") {
			t.Errorf("invalid/extra OBS header declaration %+v expected=%+v present=%v", s, expected, exists)
		}
		delete(want, s.QualifiedName)
	}
	for missing := range want {
		t.Errorf("original OBS bmem.h missing %s", missing)
	}
	fail, q, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Match: "exact", Query: "bmalloc",
		Paths: []string{header, source}, Language: "c", Encoding: "utf-8",
		MaxFiles: 2, MaxResults: 10,
	})
	if err != nil || fail != nil || !q.Coverage.CoverageComplete || q.Search == nil || len(q.Search.Matches) != 2 {
		t.Fatalf("OBS original allocator cross-file search failed: err=%v failure=%+v result=%+v", err, fail, q)
	}
	expected := map[string]int{header: 34, source: 101}
	for _, m := range q.Search.Matches {
		line, exists := expected[m.Path]
		if !exists || string(m.Kind) != "function" || m.QualifiedName != "bmalloc" || m.Range.Start.Line != line || m.SymbolID == "" || m.SourceFingerprint == "" {
			t.Errorf("original OBS allocator declaration/definition mismatch: %+v", m)
		}
		delete(expected, m.Path)
	}
	for missing := range expected {
		t.Errorf("missing OBS allocator declaration/definition in %s", missing)
	}
	var inlineID string
	for _, s := range out.Symbols {
		if s.QualifiedName == "bzalloc" {
			inlineID = s.ID
			break
		}
	}
	if inlineID == "" {
		t.Fatal("no original bzalloc body to retrieve")
	}
	fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: header, SymbolID: inlineID,
		SourceFingerprint: out.Files[0].SourceFingerprint, Language: "c", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "bmalloc(size)") ||
		!strings.Contains(shown.Show.Text, "memset(mem, 0, size)") {
		t.Fatalf("original OBS selected function body not retrievable by exact source fingerprint: %v %+v", err, fail)
	}
}
