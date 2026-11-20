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

// Original curl C project structural screen. A complete file selection and
// independently complete *selected header* inventories do not certify all
// project-wide symbol semantics or relations.
func TestSourceSymbolsOriginalCurlWholeProjectBatchedScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, "real-projects", "c-curl")
	h := NewHandler([]string{root})
	var originals []string
	err := filepath.WalkDir(project, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() && (filepath.Ext(path) == ".c" || filepath.Ext(path) == ".h") {
			originals = append(originals, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(originals)
	const total = 1017 // Original pinned curl checkout: 760 .c and 257 .h files.
	if len(originals) != total {
		t.Fatalf("original curl source set drift: got %d, want %d", len(originals), total)
	}

	// The handler limits explicitly supplied input paths to 32 by default.
	// Reconcile every path once rather than claiming that one truncated directory
	// request represents this whole upstream project.
	const batchSize = 32
	fingerprints := make(map[string]string, total)
	for start := 0; start < total; start += batchSize {
		end := min(start+batchSize, total)
		paths := originals[start:end]
		fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Presentation: "compact", Paths: paths, Language: "c",
			Encoding: "utf-8", MaxFiles: batchSize,
		})
		if err != nil || fail != nil || !digest.CoverageComplete ||
			digest.Truncated || digest.FilesConsidered != len(paths) ||
			digest.FilesParsed != len(paths) || digest.FilesSkipped != 0 ||
			len(digest.OverviewFiles) != len(paths) {
			t.Fatalf("curl source digest batch %d:%d incomplete: err=%v failure=%+v files=%d parsed=%d skipped=%d complete=%v",
				start, end, err, fail, digest.FilesConsidered, digest.FilesParsed,
				digest.FilesSkipped, digest.CoverageComplete)
		}
		batch := make(map[string]bool, len(paths))
		for _, path := range paths {
			batch[path] = true
		}
		for _, file := range digest.OverviewFiles {
			if !batch[file.Path] || fingerprints[file.Path] != "" ||
				!file.CoverageComplete || file.Status != "parsed" ||
				file.Language != "c" || file.SourceFingerprint == "" {
				t.Errorf("invalid, duplicate or missing original curl C source: %+v", file)
			}
			fingerprints[file.Path] = file.SourceFingerprint
			delete(batch, file.Path)
		}
		for path := range batch {
			t.Errorf("original C/header file omitted: %s", path)
		}
	}
	if len(fingerprints) != total {
		t.Fatalf("only %d/%d original curl C/header files checked", len(fingerprints), total)
	}

	// Full independently read declaration inventories of three public headers.
	// Each expected record: qualified name, kind, original name line and parent.
	for _, tc := range []struct{ file, oracle string }{
		{"easy.h", "curl_blob struct 34 -\ncurl_blob.data field 35 curl_blob\ncurl_blob.len field 36 curl_blob\ncurl_blob.flags field 37 curl_blob\ncurl_easy_init function 41 -\ncurl_easy_setopt function 42 -\ncurl_easy_perform function 43 -\ncurl_easy_cleanup function 44 -\ncurl_easy_getinfo function 59 -\ncurl_easy_duphandle function 73 -\ncurl_easy_reset function 86 -\ncurl_easy_recv function 96 -\ncurl_easy_send function 107 -\ncurl_easy_upkeep function 117 -"},
		{"header.h", "curl_header struct 31 -\ncurl_header.name field 32 curl_header\ncurl_header.value field 33 curl_header\ncurl_header.amount field 34 curl_header\ncurl_header.index field 35 curl_header\ncurl_header.origin field 36 curl_header\ncurl_header.anchor field 37 curl_header\nCURLHcode alias 56 -\ncurl_easy_header function 58 -\ncurl_easy_nextheader function 65 -"},
		{"urlapi.h", "CURLUcode alias 69 -\nCURLUPart alias 83 -\nCURLU alias 108 -\ncurl_url function 114 -\ncurl_url_cleanup function 121 -\ncurl_url_dup function 127 -\ncurl_url_get function 134 -\ncurl_url_set function 142 -\ncurl_url_strerror function 150 -"},
	} {
		file := filepath.Join(project, "include", "curl", tc.file)
		fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: "detailed", Paths: []string{file},
			Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 100,
		})
		if err != nil || fail != nil || !outline.CoverageComplete ||
			len(outline.Files) != 1 || outline.Files[0].SourceFingerprint != fingerprints[file] {
			t.Fatalf("curl public header %s incomplete: %v %+v", tc.file, err, fail)
		}
		type expected struct {
			kind, parent string
			line         int
		}
		want := map[string]expected{}
		for _, line := range strings.Split(tc.oracle, "\n") {
			parts := strings.Fields(line)
			if len(parts) != 4 {
				t.Fatalf("incorrect original-source oracle record %q", line)
			}
			number, err := strconv.Atoi(parts[2])
			if err != nil {
				t.Fatal(err)
			}
			parent := parts[3]
			if parent == "-" {
				parent = ""
			}
			want[parts[0]] = expected{kind: parts[1], parent: parent, line: number}
		}
		if len(outline.Symbols) != len(want) {
			t.Errorf("%s declarations: got %d, want %d", tc.file, len(outline.Symbols), len(want))
		}
		for _, symbol := range outline.Symbols {
			e, ok := want[symbol.QualifiedName]
			if !ok || string(symbol.Kind) != e.kind || symbol.ParentQualifiedName != e.parent ||
				symbol.NameRange.Start.Line != e.line || symbol.ID == "" {
				t.Errorf("%s fabricated or incorrect declaration: %+v expected=%+v present=%v",
					tc.file, symbol, e, ok)
			}
			delete(want, symbol.QualifiedName)
		}
		for missing := range want {
			t.Errorf("%s missed original declaration %s", tc.file, missing)
		}
	}

	// The source's public declaration and concrete implementation are in
	// different original files; look for both across the entire source set.
	for _, q := range []struct {
		name, header, implementation string
		headerLine, definitionLine   int
	}{
		{"curl_easy_upkeep", filepath.Join("include", "curl", "easy.h"), filepath.Join("lib", "easy.c"), 117, 1354},
		{"curl_easy_init", filepath.Join("include", "curl", "easy.h"), filepath.Join("lib", "easy.c"), 41, 332},
	} {
		seen := map[string]int{}
		for start := 0; start < total; start += batchSize {
			end := min(start+batchSize, total)
			fail, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Query: q.name, Match: "exact",
				Paths: originals[start:end], Language: "c", Encoding: "utf-8",
				MaxFiles: batchSize, MaxResults: 20,
			})
			if err != nil || fail != nil || !result.Coverage.CoverageComplete ||
				result.Coverage.FilesParsed != end-start || result.Search == nil {
				t.Fatalf("curl %s structural search batch %d:%d incomplete: %v %+v coverage=%+v",
					q.name, start, end, err, fail, result.Coverage)
			}
			for _, m := range result.Search.Matches {
				if string(m.Kind) != "function" || m.QualifiedName != q.name ||
					m.SourceFingerprint != fingerprints[m.Path] || m.SymbolID == "" {
					t.Errorf("incorrect/unbound curl structural search match: %+v", m)
				}
				seen[m.Path] = m.Range.Start.Line
			}
		}
		want := map[string]int{
			filepath.Join(project, q.header):         q.headerLine,
			filepath.Join(project, q.implementation): q.definitionLine,
		}
		if len(seen) != len(want) {
			t.Errorf("curl %s unexpectedly found %d declaration locations: %v (want %v)",
				q.name, len(seen), seen, want)
		}
		for file, line := range want {
			if seen[file] != line {
				t.Errorf("curl %s expected %s:%d, got %d", q.name, file, line, seen[file])
			}
		}
	}

	source := filepath.Join(project, "lib", "easy.c")
	header := filepath.Join(project, "lib", "easyif.h")
	fail, relations, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Relation: "dependencies", Paths: []string{source, header},
		Language: "c", Encoding: "utf-8", MaxFiles: 2,
		MaxResults: 80, MaxNodes: 80, MaxEdges: 80, MaxDepth: 1,
		Subject: &SourceSelectorInput{Kind: "path", Path: source, SourceFingerprint: fingerprints[source]},
	})
	if err != nil || fail != nil || relations.Relations == nil ||
		!relations.Coverage.CoverageComplete || relations.Relations.EvidenceScope != "indexed-relations-only" {
		t.Fatalf("curl local dependency lookup failed: %v %+v", err, fail)
	}
	found := false
	for _, edge := range relations.Relations.Relations {
		if edge.Source.Path == source && edge.Source.Range != nil &&
			edge.Source.Range.Start.Line == 56 && edge.Target.Path == header &&
			edge.Target.SourceFingerprint == fingerprints[header] &&
			string(edge.Resolution) == "resolved" && string(edge.Evidence) == "project-resolved" {
			found = true
		}
	}
	if !found {
		t.Errorf("curl original easy.c line 56 local header dependency not resolved")
	}
	fail, look, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{source}, Query: "curl_easy_upkeep",
		Match: "exact", Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 6,
	})
	if err != nil || fail != nil || !look.CoverageComplete || len(look.Symbols) != 1 {
		t.Fatalf("curl original function find failed: %v %+v", err, fail)
	}
	fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: source, SymbolID: look.Symbols[0].ID,
		SourceFingerprint: fingerprints[source], Language: "c", Encoding: "utf-8",
		MaxBytes: 8192,
	})
	if err != nil || fail != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "curl_easy_upkeep") {
		t.Fatalf("curl exact bound function retrieval failed: %v %+v", err, fail)
	}
	t.Logf("curl original C structural screen: %d complete source files in %d batches, 3 complete independently annotated headers and 2 project-wide APIs", total, (total+batchSize-1)/batchSize)
}
