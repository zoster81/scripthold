package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestSourceSymbolsOriginalScrcpyWholeProjectScreen checks every eligible C source
// file and independently annotated complete declarations in two original headers.
// The remaining project files are structurally screened, not semantically certified.
func TestSourceSymbolsOriginalScrcpyWholeProjectScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, "real-projects", "c-genymobile-scrcpy")
	h := NewHandler([]string{root})
	includes, excludes := []string{"**/*.c", "**/*.h"}, []string{"**/.git/**"}
	files := map[string]bool{}
	err := filepath.WalkDir(project, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() && (filepath.Ext(path) == ".c" || filepath.Ext(path) == ".h") {
			files[path] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const eligible = 180 // Pinned original checkout: 91 .c files and 89 .h files.
	if len(files) != eligible {
		t.Fatalf("original C/header inventory drift: %d want %d", len(files), eligible)
	}
	fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: includes, Excludes: excludes, MaxFiles: 200,
	})
	if err != nil || fail != nil || digest.Truncated || !digest.CoverageComplete ||
		digest.FilesConsidered != eligible || digest.FilesParsed != eligible ||
		digest.FilesSkipped != 0 || len(digest.OverviewFiles) != eligible {
		t.Fatalf("scrcpy source index incomplete: err=%v fail=%+v considered=%d parsed=%d skipped=%d complete=%v",
			err, fail, digest.FilesConsidered, digest.FilesParsed, digest.FilesSkipped, digest.CoverageComplete)
	}
	fingerprints := make(map[string]string, eligible)
	for _, item := range digest.OverviewFiles {
		if !files[item.Path] || fingerprints[item.Path] != "" ||
			item.Status != "parsed" || !item.CoverageComplete || item.SourceFingerprint == "" || item.Language != "c" {
			t.Errorf("duplicate/unexpected/unparsed scrcpy source: %+v", item)
		}
		fingerprints[item.Path] = item.SourceFingerprint
	}
	for path := range files {
		if fingerprints[path] == "" {
			t.Errorf("eligible original source missing: %s", path)
		}
	}

	// Complete source-derived inventory for two headers, including nested fields,
	// declaration name lines and parent ownership; macros/comments are not symbols.
	for _, tc := range []struct{ relative, expected string }{
		{filepath.Join("app", "src", "fps_counter.h"), `
sc_fps_counter struct 12 -
sc_fps_counter.thread field 13 sc_fps_counter
sc_fps_counter.mutex field 14 sc_fps_counter
sc_fps_counter.state_cond field 15 sc_fps_counter
sc_fps_counter.thread_started field 17 sc_fps_counter
sc_fps_counter.started field 21 sc_fps_counter
sc_fps_counter.interrupted field 24 sc_fps_counter
sc_fps_counter.nr_rendered field 25 sc_fps_counter
sc_fps_counter.nr_skipped field 26 sc_fps_counter
sc_fps_counter.next_timestamp field 27 sc_fps_counter
sc_fps_counter_init function 31 -
sc_fps_counter_destroy function 34 -
sc_fps_counter_start function 37 -
sc_fps_counter_stop function 40 -
sc_fps_counter_is_started function 43 -
sc_fps_counter_interrupt function 48 -
sc_fps_counter_join function 51 -
sc_fps_counter_add_rendered_frame function 54 -
sc_fps_counter_add_skipped_frame function 57 -
`},
		{filepath.Join("app", "src", "adb", "adb.h"), `
sc_adb_init function 19 -
sc_adb_destroy function 22 -
sc_adb_get_executable function 25 -
sc_adb_device_selector_type enum 27 -
sc_adb_device_selector struct 34 -
sc_adb_device_selector.type field 35 sc_adb_device_selector
sc_adb_device_selector.serial field 36 sc_adb_device_selector
sc_adb_execute function 40 -
sc_adb_start_server function 43 -
sc_adb_kill_server function 46 -
sc_adb_forward function 49 -
sc_adb_forward_remove function 53 -
sc_adb_reverse function 57 -
sc_adb_reverse_remove function 62 -
sc_adb_push function 66 -
sc_adb_install function 70 -
sc_adb_tcpip function 77 -
sc_adb_connect function 86 -
sc_adb_disconnect function 95 -
sc_adb_select_device function 103 -
sc_adb_getprop function 111 -
sc_adb_get_device_ip function 121 -
sc_adb_get_device_sdk_version function 127 -
`},
	} {
		path := filepath.Join(project, tc.relative)
		fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: "detailed", Paths: []string{path},
			Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 100,
		})
		if err != nil || fail != nil || !outline.CoverageComplete || len(outline.Files) != 1 {
			t.Fatalf("%s detailed outline failed: %v %+v %+v", tc.relative, err, fail, outline)
		}
		expected := map[string]struct {
			kind, parent string
			line         int
		}{}
		for _, line := range strings.Split(strings.TrimSpace(tc.expected), "\n") {
			parts := strings.Fields(line)
			if len(parts) != 4 {
				t.Fatalf("invalid independent oracle line %q", line)
			}
			lineNo, err := strconv.Atoi(parts[2])
			if err != nil {
				t.Fatal(err)
			}
			parent := parts[3]
			if parent == "-" {
				parent = ""
			}
			expected[parts[0]] = struct {
				kind, parent string
				line         int
			}{parts[1], parent, lineNo}
		}
		if len(outline.Symbols) != len(expected) {
			t.Errorf("%s symbols=%d expected=%d", tc.relative, len(outline.Symbols), len(expected))
		}
		for _, symbol := range outline.Symbols {
			want, ok := expected[symbol.QualifiedName]
			if !ok || string(symbol.Kind) != want.kind || symbol.ParentQualifiedName != want.parent ||
				symbol.NameRange.Start.Line != want.line || symbol.ID == "" {
				t.Errorf("%s unexpected/misplaced/incorrect symbol: %+v oracle=%+v present=%v", tc.relative, symbol, want, ok)
			}
			delete(expected, symbol.QualifiedName)
		}
		for name := range expected {
			t.Errorf("%s missing required declaration %q", tc.relative, name)
		}
	}

	// All 180 files are the search scope. These line pairs were checked in
	// original headers and implementations, not derived from the analyzer.
	for _, tc := range []struct {
		name, header, body   string
		headerLine, bodyLine int
	}{
		{"sc_fps_counter_init", filepath.Join("app", "src", "fps_counter.h"), filepath.Join("app", "src", "fps_counter.c"), 31, 11},
		{"sc_fps_counter_destroy", filepath.Join("app", "src", "fps_counter.h"), filepath.Join("app", "src", "fps_counter.c"), 34, 31},
		{"sc_adb_init", filepath.Join("app", "src", "adb", "adb.h"), filepath.Join("app", "src", "adb", "adb.c"), 19, 32},
		{"sc_adb_get_executable", filepath.Join("app", "src", "adb", "adb.h"), filepath.Join("app", "src", "adb", "adb.c"), 25, 67},
	} {
		fail, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact", Query: tc.name,
			Paths: []string{project}, Language: "c", Encoding: "utf-8",
			Includes: includes, Excludes: excludes, MaxFiles: 200, MaxResults: 10,
		})
		if err != nil || fail != nil || !result.Coverage.CoverageComplete ||
			result.Coverage.FilesParsed != eligible || result.Search == nil || len(result.Search.Matches) != 2 {
			t.Fatalf("%s project structural search failed: err=%v fail=%+v coverage=%+v search=%+v",
				tc.name, err, fail, result.Coverage, result.Search)
		}
		for _, match := range result.Search.Matches {
			rel := relativeSourcePath(project, match.Path)
			want := 0
			switch rel {
			case tc.header:
				want = tc.headerLine
			case tc.body:
				want = tc.bodyLine
			}
			if want == 0 || match.Range.Start.Line != want || string(match.Kind) != "function" ||
				match.QualifiedName != tc.name || match.SourceFingerprint != fingerprints[match.Path] ||
				match.SymbolID == "" {
				t.Errorf("%s unexpected match: %+v", tc.name, match)
			}
		}
	}

	// A source-verified local dependency and a fingerprint-bound body lookup.
	source := filepath.Join(project, "app", "src", "fps_counter.c")
	fail, rel, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Relation: "dependencies", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: includes, Excludes: excludes,
		MaxFiles: 200, MaxResults: 32, MaxNodes: 80, MaxEdges: 80, MaxDepth: 1,
		Subject: &SourceSelectorInput{Kind: "path", Path: source, SourceFingerprint: fingerprints[source]},
	})
	if err != nil || fail != nil || !rel.Coverage.CoverageComplete || rel.Relations == nil ||
		rel.Relations.EvidenceScope != "indexed-relations-only" {
		t.Fatalf("scrcpy dependencies failed: %v %+v %+v", err, fail, rel)
	}
	found := false
	for _, edge := range rel.Relations.Relations {
		if edge.Source.Path == source && edge.Source.Range != nil && edge.Source.Range.Start.Line == 1 &&
			edge.Target.Path == filepath.Join(project, "app", "src", "fps_counter.h") &&
			edge.Target.SourceFingerprint == fingerprints[edge.Target.Path] &&
			string(edge.Resolution) == "resolved" && string(edge.Evidence) == "project-resolved" {
			found = true
		}
	}
	if !found {
		t.Fatal("fps_counter.c include of project-local header missing")
	}
	fail, lookup, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{source}, Query: "sc_fps_counter_init",
		Match: "exact", Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || fail != nil || !lookup.CoverageComplete || len(lookup.Symbols) != 1 {
		t.Fatalf("scrcpy definition find failed: %v %+v %+v", err, fail, lookup)
	}
	fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: source, SymbolID: lookup.Symbols[0].ID,
		SourceFingerprint: fingerprints[source], Language: "c", Encoding: "utf-8", MaxBytes: 8192,
	})
	if err != nil || fail != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "sc_mutex_init(&counter->mutex)") {
		t.Fatalf("fingerprint-bound scrcpy original body unavailable: %v %+v %+v", err, fail, shown)
	}
	t.Logf("scrcpy C screen: %d original files, two independently annotated complete headers, 4 declaration/definition pairs, local dependency and verified body", eligible)
}
