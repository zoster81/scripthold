package handler

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// This screens the entire original Libevent C/header checkout, not its complete semantics.
func TestSourceSymbolsOriginalLibeventWholeProjectScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, filepath.FromSlash("real-projects/c-libevent-libevent"))
	h := NewHandler([]string{root})
	includes := []string{"**/*.c", "**/*.h"}
	excludes := []string{"**/.git/**"}

	// Independently enumerate the original checkout, including tests and platform variants.
	original := make(map[string]bool)
	err := filepath.WalkDir(project, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.Type().IsRegular() || (filepath.Ext(path) != ".c" && filepath.Ext(path) != ".h") {
			return nil
		}
		original[path] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	const sourceFiles = 174 // Independent pinned checkout inventory: 100 .c, 74 .h.
	if len(original) != sourceFiles {
		t.Fatalf("original checkout drift: got %d C/header files, want %d", len(original), sourceFiles)
	}

	failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: includes, Excludes: excludes, MaxFiles: 200,
	})
	if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete ||
		digest.FilesConsidered != sourceFiles || digest.FilesParsed != sourceFiles ||
		digest.FilesSkipped != 0 || len(digest.OverviewFiles) != sourceFiles {
		t.Fatalf("Libevent full C source index incomplete: err=%v failed=%+v considered=%d parsed=%d skipped=%d complete=%v truncated=%v",
			err, failed, digest.FilesConsidered, digest.FilesParsed, digest.FilesSkipped, digest.CoverageComplete, digest.Truncated)
	}
	fingerprints := make(map[string]string, sourceFiles)
	for _, file := range digest.OverviewFiles {
		if !original[file.Path] || fingerprints[file.Path] != "" || file.SourceFingerprint == "" ||
			file.Status != "parsed" || !file.CoverageComplete || file.Language != "c" {
			t.Errorf("unexpected, duplicate, or incomplete C/header file: %+v", file)
		}
		fingerprints[file.Path] = file.SourceFingerprint
	}
	for path := range original {
		if fingerprints[path] == "" {
			t.Errorf("original C/header file omitted: %s", path)
		}
	}

	// Independent source lines establish two different public API declarations/definitions.
	for _, tc := range []struct {
		name  string
		lines map[string]int
	}{
		{"event_base_new", map[string]int{"event.c": 540, filepath.Join("include", "event2", "event.h"): 344}},
		{"event_base_free", map[string]int{"event.c": 993, filepath.Join("include", "event2", "event.h"): 725}},
	} {
		failed, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Match: "exact", Query: tc.name,
			Paths: []string{project}, Language: "c", Encoding: "utf-8",
			Includes: includes, Excludes: excludes, MaxFiles: 200, MaxResults: 10,
		})
		if err != nil || failed != nil || !result.Coverage.CoverageComplete ||
			result.Coverage.FilesParsed != sourceFiles || result.Search == nil ||
			len(result.Search.Matches) != len(tc.lines) {
			t.Fatalf("%s incomplete cross-file search: err=%v failed=%+v coverage=%+v search=%+v",
				tc.name, err, failed, result.Coverage, result.Search)
		}
		for _, match := range result.Search.Matches {
			want, found := tc.lines[relativeSourcePath(project, match.Path)]
			if !found || match.Range.Start.Line != want || string(match.Kind) != "function" ||
				match.QualifiedName != tc.name || match.SymbolID == "" ||
				match.SourceFingerprint != fingerprints[match.Path] {
				t.Errorf("incorrect public API %s: %+v", tc.name, match)
			}
		}
	}

	source := filepath.Join(project, "event.c")
	failed, relations, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Relation: "dependencies", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: includes, Excludes: excludes,
		MaxFiles: 200, MaxResults: 80, MaxNodes: 200, MaxEdges: 200, MaxDepth: 1,
		Subject: &SourceSelectorInput{Kind: "path", Path: source, SourceFingerprint: fingerprints[source]},
	})
	if err != nil || failed != nil || !relations.Coverage.CoverageComplete ||
		relations.Coverage.FilesParsed != sourceFiles || relations.Relations == nil ||
		relations.Relations.EvidenceScope != "indexed-relations-only" {
		t.Fatalf("Libevent project dependency index incomplete: err=%v failed=%+v coverage=%+v relations=%+v",
			err, failed, relations.Coverage, relations.Relations)
	}
	found := false
	for _, edge := range relations.Relations.Relations {
		if edge.Source.Path == source && edge.Source.Range != nil && edge.Source.Range.Start.Line == 63 &&
			edge.Target.Path == filepath.Join(project, "event-internal.h") &&
			edge.Target.SourceFingerprint == fingerprints[edge.Target.Path] &&
			string(edge.Resolution) == "resolved" && string(edge.Evidence) == "project-resolved" {
			found = true
		}
	}
	if !found {
		t.Error("original event.c line 63 local header dependency not resolved")
	}
	t.Logf("Libevent structural screen: %d original C/header files, 2 cross-file API pairs, local dependency", sourceFiles)
}

func relativeSourcePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return ""
	}
	return rel
}
