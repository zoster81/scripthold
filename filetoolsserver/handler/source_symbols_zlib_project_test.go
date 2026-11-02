package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// This screens every eligible C/header source in the pinned original checkout.
// A complete index is not an independent semantic oracle for every declaration.
func TestSourceSymbolsOriginalZlibWholeProjectScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, filepath.FromSlash("real-projects/c-madler-zlib"))
	h := NewHandler([]string{root})
	includes := []string{"**/*.c", "**/*.h"}
	excludes := []string{"**/.git/**"}
	failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: includes, Excludes: excludes, MaxFiles: 100,
	})
	const sourceFiles = 73
	if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete ||
		digest.FilesConsidered != sourceFiles || digest.FilesParsed != sourceFiles ||
		digest.FilesSkipped != 0 || len(digest.OverviewFiles) != sourceFiles {
		t.Fatalf("whole zlib C project incomplete: err=%v failed=%+v considered=%d parsed=%d skipped=%d complete=%v truncated=%v",
			err, failed, digest.FilesConsidered, digest.FilesParsed, digest.FilesSkipped,
			digest.CoverageComplete, digest.Truncated)
	}
	seen := make(map[string]string, sourceFiles)
	for _, f := range digest.OverviewFiles {
		if _, duplicated := seen[f.Path]; duplicated || f.Status != "parsed" ||
			!f.CoverageComplete || f.SourceFingerprint == "" || f.Language != "c" {
			t.Errorf("wrong, duplicate, or incomplete original file: %+v", f)
		}
		seen[f.Path] = f.SourceFingerprint
	}
	// Two independent source-read oracles cover different implementation files;
	// macro invocations elsewhere in zlib.h are not function declarations.
	for _, tc := range []struct {
		function string
		expected map[string]int
	}{
		{"deflateInit2_", map[string]int{
			"deflate.c": 387, "zlib.h": 1907,
		}},
		{"adler32_z", map[string]int{
			"adler32.c": 61, "zlib.h": 1829,
		}},
	} {
		failed, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Mode: "structural", Query: tc.function, Match: "exact",
			Paths: []string{project}, Language: "c", Encoding: "utf-8",
			Includes: includes, Excludes: excludes, MaxFiles: 100, MaxResults: 12,
		})
		if err != nil || failed != nil || !result.Coverage.CoverageComplete ||
			result.Coverage.FilesParsed != sourceFiles || result.Search == nil ||
			len(result.Search.Matches) != len(tc.expected) {
			t.Fatalf("%s project-wide declarations: err=%v failed=%+v coverage=%+v search=%+v",
				tc.function, err, failed, result.Coverage, result.Search)
		}
		for _, m := range result.Search.Matches {
			name := filepath.Base(m.Path)
			want, exists := tc.expected[name]
			if !exists || m.Range.Start.Line != want || m.QualifiedName != tc.function ||
				string(m.Kind) != "function" || m.SourceFingerprint != seen[m.Path] || m.SymbolID == "" {
				t.Errorf("incorrect %s cross-file declaration: %+v", tc.function, m)
			}
		}
	}
	source := filepath.Join(project, "deflate.c")
	if seen[source] == "" {
		t.Fatal("deflate.c absent from complete project digest")
	}
	failed, relations, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Relation: "dependencies", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: includes, Excludes: excludes,
		MaxFiles: 100, MaxResults: 20, MaxNodes: 64, MaxEdges: 64, MaxDepth: 1,
		Subject: &SourceSelectorInput{Kind: "path", Path: source, SourceFingerprint: seen[source]},
	})
	if err != nil || failed != nil || !relations.Coverage.CoverageComplete ||
		relations.Coverage.FilesParsed != sourceFiles || relations.Relations == nil ||
		relations.Relations.EvidenceScope != "indexed-relations-only" ||
		len(relations.Relations.Relations) != 1 {
		t.Fatalf("whole zlib project dependency: err=%v failed=%+v coverage=%+v relations=%+v",
			err, failed, relations.Coverage, relations.Relations)
	}
	e := relations.Relations.Relations[0]
	if e.Source.Path != source || e.Source.Range == nil || e.Source.Range.Start.Line != 52 ||
		e.Target.Path != filepath.Join(project, "deflate.h") ||
		e.Target.SourceFingerprint != seen[e.Target.Path] ||
		string(e.Resolution) != "resolved" || string(e.Evidence) != "project-resolved" {
		t.Errorf("wrong project-local dependency: %+v", e)
	}
	t.Logf("whole zlib project screening passed: %d source files, 2 declaration/definition pairs, 1 resolved include", sourceFiles)
}
