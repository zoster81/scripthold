package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Screens all eligible original Expat C/header files; a successful index is
// not independently complete project-wide semantic qualification.
func TestSourceSymbolsOriginalExpatWholeProjectScreen(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	project := filepath.Join(root, filepath.FromSlash("real-projects/c-libexpat-libexpat"))
	h := NewHandler([]string{root})
	include := []string{"**/*.c", "**/*.h"}
	exclude := []string{"**/.git/**"}
	const expectedFiles = 81
	failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: include, Excludes: exclude, MaxFiles: 100,
	})
	if err != nil || failed != nil || !digest.CoverageComplete || digest.Truncated ||
		digest.FilesConsidered != expectedFiles || digest.FilesParsed != expectedFiles ||
		digest.FilesSkipped != 0 || len(digest.OverviewFiles) != expectedFiles {
		t.Fatalf("whole Expat C source incomplete: err=%v failed=%+v considered=%d parsed=%d skipped=%d complete=%v truncated=%v",
			err, failed, digest.FilesConsidered, digest.FilesParsed, digest.FilesSkipped, digest.CoverageComplete, digest.Truncated)
	}
	fingerprints := make(map[string]string, expectedFiles)
	for _, f := range digest.OverviewFiles {
		if fingerprints[f.Path] != "" || f.SourceFingerprint == "" ||
			f.Status != "parsed" || f.Language != "c" || !f.CoverageComplete {
			t.Errorf("duplicate or incomplete original Expat source: %+v", f)
		}
		fingerprints[f.Path] = f.SourceFingerprint
	}
	// Source read/grep independently establishes the public declaration at
	// expat.h:240 (preceded by XMLPARSEAPI(...) on line 239) and body at 1071.
	failed, found, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Match: "exact", Query: "XML_ParserCreate",
		Paths: []string{project}, Language: "c", Encoding: "utf-8",
		Includes: include, Excludes: exclude, MaxFiles: 100, MaxResults: 10,
	})
	if err != nil || failed != nil || found.Search == nil || !found.Coverage.CoverageComplete ||
		found.Coverage.FilesParsed != expectedFiles || len(found.Search.Matches) != 2 {
		t.Fatalf("whole Expat public API declaration/definition missing: err=%v failed=%+v coverage=%+v search=%+v",
			err, failed, found.Coverage, found.Search)
	}
	expected := map[string]int{
		filepath.Join(project, "expat", "lib", "expat.h"):    240,
		filepath.Join(project, "expat", "lib", "xmlparse.c"): 1071,
	}
	for _, m := range found.Search.Matches {
		line, ok := expected[m.Path]
		if !ok || m.Range.Start.Line != line || m.QualifiedName != "XML_ParserCreate" ||
			string(m.Kind) != "function" || m.SourceFingerprint != fingerprints[m.Path] || m.SymbolID == "" {
			t.Errorf("unexpected Expat API symbol: %+v", m)
		}
	}
	source := filepath.Join(project, "expat", "lib", "xmlparse.c")
	failed, relations, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "relations", Relation: "dependencies", Paths: []string{project},
		Language: "c", Encoding: "utf-8", Includes: include, Excludes: exclude,
		MaxFiles: 100, MaxResults: 40, MaxNodes: 70, MaxEdges: 100, MaxDepth: 1,
		Subject: &SourceSelectorInput{Kind: "path", Path: source, SourceFingerprint: fingerprints[source]},
	})
	if err != nil || failed != nil || !relations.Coverage.CoverageComplete ||
		relations.Coverage.FilesParsed != expectedFiles || relations.Relations == nil ||
		relations.Relations.EvidenceScope != "indexed-relations-only" {
		t.Fatalf("whole Expat dependencies incomplete: err=%v failed=%+v coverage=%+v relations=%+v",
			err, failed, relations.Coverage, relations.Relations)
	}
	resolved := false
	for _, e := range relations.Relations.Relations {
		if e.Source.Path == source && e.Source.Range != nil && e.Source.Range.Start.Line == 122 &&
			e.Target.Path == filepath.Join(project, "expat", "lib", "expat.h") &&
			e.Target.SourceFingerprint == fingerprints[e.Target.Path] &&
			string(e.Resolution) == "resolved" && string(e.Evidence) == "project-resolved" {
			resolved = true
		}
	}
	if !resolved {
		t.Error("original xmlparse.c import of expat.h not resolved")
	}
	t.Log("whole Expat source screen: 81 files, public API declaration and definition, local dependency")
}
