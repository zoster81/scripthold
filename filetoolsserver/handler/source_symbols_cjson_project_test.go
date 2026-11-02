package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Project-wide screening: all C/header sources, including tests and embedded
// dependencies; completeness flags do not prove semantic correctness.
func TestSourceSymbolsOriginalCJSONWholeProjectScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, filepath.FromSlash("real-projects/c-davegamble-cjson"))
	h := NewHandler([]string{root})
	includes := []string{"**/*.c", "**/*.h"}
	excludes := []string{"**/.git/**"}
	failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: includes, Excludes: excludes, MaxFiles: 100,
	})
	if err != nil || failed != nil || digest.Truncated || !digest.CoverageComplete ||
		digest.FilesConsidered != 53 || digest.FilesParsed != 53 || digest.FilesSkipped != 0 ||
		len(digest.OverviewFiles) != 53 {
		t.Fatalf("whole project C digest incomplete: err=%v failed=%+v considered=%d parsed=%d skipped=%d complete=%v truncated=%v",
			err, failed, digest.FilesConsidered, digest.FilesParsed, digest.FilesSkipped,
			digest.CoverageComplete, digest.Truncated)
	}
	seen := make(map[string]bool, 53)
	for _, file := range digest.OverviewFiles {
		if seen[file.Path] || file.Status != "parsed" || !file.CoverageComplete || file.SourceFingerprint == "" || file.Language != "c" {
			t.Errorf("duplicate or incomplete source file: %+v", file)
		}
		seen[file.Path] = true
	}
	failed, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Match: "exact", Query: "cJSONUtils_GetPointer",
		Paths: []string{project}, Language: "c", Encoding: "utf-8",
		Includes: includes, Excludes: excludes, MaxFiles: 100, MaxResults: 8,
	})
	if err != nil || failed != nil || !query.Coverage.CoverageComplete ||
		query.Coverage.FilesParsed != 53 || query.Search == nil || len(query.Search.Matches) != 2 {
		t.Fatalf("whole project structural query incomplete: err=%v failed=%+v coverage=%+v search=%+v",
			err, failed, query.Coverage, query.Search)
	}
	want := map[string]int{
		filepath.Join(project, "cJSON_Utils.h"): 34,
		filepath.Join(project, "cJSON_Utils.c"): 348,
	}
	for _, match := range query.Search.Matches {
		if line, ok := want[match.Path]; !ok || match.Range.Start.Line != line ||
			match.QualifiedName != "cJSONUtils_GetPointer" || match.SymbolID == "" {
			t.Errorf("wrong cross-file function declaration/definition: %+v", match)
		}
	}
	// The 53-file index must also resolve a real project-local header import.
	ownHeader := filepath.Join(project, "cJSON_Utils.h")
	ownFingerprint := ""
	for _, file := range digest.OverviewFiles {
		if file.Path == ownHeader {
			ownFingerprint = file.SourceFingerprint
		}
	}
	if ownFingerprint == "" {
		t.Fatal("project header fingerprint absent")
	}
	failed, relations, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Relation: "dependencies",
		Paths: []string{project}, Includes: includes, Excludes: excludes,
		Language: "c", Encoding: "utf-8", MaxFiles: 100, MaxResults: 32,
		MaxNodes: 64, MaxEdges: 128, MaxDepth: 2,
		Subject: &SourceSelectorInput{Kind: "path", Path: ownHeader, SourceFingerprint: ownFingerprint},
	})
	if err != nil || failed != nil || !relations.Coverage.CoverageComplete ||
		relations.Coverage.FilesParsed != 53 || relations.Relations == nil ||
		relations.Relations.EvidenceScope != "indexed-relations-only" ||
		len(relations.Relations.Relations) != 1 {
		t.Fatalf("whole C project dependency index mismatch: err=%v failed=%+v coverage=%+v relations=%+v",
			err, failed, relations.Coverage, relations.Relations)
	}
	edge := relations.Relations.Relations[0]
	if edge.Source.Path != ownHeader || edge.Source.Range == nil ||
		edge.Source.Range.Start.Line != 31 || edge.Target.Path != filepath.Join(project, "cJSON.h") ||
		string(edge.Resolution) != "resolved" || string(edge.Evidence) != "project-resolved" {
		t.Errorf("wrong project-local dependency: %+v", edge)
	}
	t.Logf("whole cJSON project screen: %d files, declaration/definition and one resolved local include", len(seen))
}
