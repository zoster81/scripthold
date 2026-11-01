package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The source checkout is read-only and supplied only to opt-in qualification.
func TestSourceSymbolsRealRubySidekiqSelectedScopes(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_RUBY_SIDEKIQ_PROJECT")
	if root == "" {
		t.Skip("original qualified Ruby project not provided")
	}
	paths := []string{
		filepath.Join(root, "lib", "sidekiq", "paginator.rb"),
		filepath.Join(root, "lib", "sidekiq", "ring_buffer.rb"),
		filepath.Join(root, "lib", "sidekiq", "worker_compatibility_alias.rb"),
	}
	type expected struct {
		kind   string
		line   int
		parent string
	}
	want := map[string]map[string]expected{
		"paginator.rb": {
			"Sidekiq":                      {"module", 3, ""},
			"Sidekiq.Paginator":            {"module", 4, "Sidekiq"},
			"Sidekiq.Paginator.TYPE_CACHE": {"constant", 5, "Sidekiq.Paginator"},
			"Sidekiq.Paginator.page":       {"method", 11, "Sidekiq.Paginator"},
			"Sidekiq.Paginator.page_items": {"method", 61, "Sidekiq.Paginator"},
		},
		"ring_buffer.rb": {
			"Sidekiq":                       {"module", 5, ""},
			"Sidekiq.RingBuffer":            {"class", 6, "Sidekiq"},
			"Sidekiq.RingBuffer.RingBuffer": {"constructor", 12, "Sidekiq.RingBuffer"},
			"Sidekiq.RingBuffer.<<":         {"method", 18, "Sidekiq.RingBuffer"},
			"Sidekiq.RingBuffer.buffer":     {"method", 24, "Sidekiq.RingBuffer"},
			"Sidekiq.RingBuffer.reset":      {"method", 28, "Sidekiq.RingBuffer"},
		},
		"worker_compatibility_alias.rb": {
			"Sidekiq":        {"module", 3, ""},
			"Sidekiq.Worker": {"constant", 12, "Sidekiq"},
		},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Presentation: "detailed", MaxFiles: 3, MaxSymbols: 40,
	})
	if err != nil || failed != nil || !outline.CoverageComplete || outline.FilesParsed != 3 ||
		outline.SymbolCount != 13 || len(outline.Files) != 3 {
		t.Fatalf("Ruby original outline: err=%v failed=%+v result=%+v", err, failed, outline)
	}
	byFile := make(map[string]map[string]int)
	fingerprints := make(map[string]string)
	for _, f := range outline.Files {
		name := filepath.Base(f.Path)
		if !f.CoverageComplete || f.Status != "parsed" || f.Language != "ruby" || f.SourceFingerprint == "" {
			t.Errorf("wrong original Ruby file status %+v", f)
		}
		fingerprints[name] = f.SourceFingerprint
	}
	for _, s := range outline.Symbols {
		name := filepath.Base(s.Path)
		expected, ok := want[name][s.QualifiedName]
		if !ok || string(s.Kind) != expected.kind || s.DeclarationRange.Start.Line != expected.line ||
			s.ParentQualifiedName != expected.parent {
			t.Errorf("wrong original Ruby declaration: %+v (expected=%+v exists=%v)", s, expected, ok)
		}
		if byFile[name] == nil {
			byFile[name] = make(map[string]int)
		}
		byFile[name][s.QualifiedName]++
	}
	for file, values := range want {
		if len(byFile[file]) != len(values) {
			t.Errorf("Ruby file %s: %d symbols expected %d", file, len(byFile[file]), len(values))
		}
		for name := range values {
			if byFile[file][name] != 1 {
				t.Errorf("Ruby %s declaration %s count %d", file, name, byFile[file][name])
			}
		}
	}

	failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Paths: paths, Presentation: "compact", MaxFiles: 3,
	})
	if err != nil || failed != nil || !digest.CoverageComplete || digest.FilesParsed != 3 ||
		digest.SymbolCount != 13 || len(digest.OverviewFiles) != 3 {
		t.Fatalf("Ruby original digest: err=%v failed=%+v result=%+v", err, failed, digest)
	}
	for _, f := range digest.OverviewFiles {
		if !f.CoverageComplete || f.SourceFingerprint != fingerprints[filepath.Base(f.Path)] {
			t.Errorf("Ruby digest source mismatch %+v", f)
		}
		count := 0
		for _, declaration := range f.DeclarationCounts {
			count += declaration.Count
		}
		if count != len(want[filepath.Base(f.Path)]) {
			t.Errorf("Ruby digest count %+v", f)
		}
	}

	for _, s := range outline.Symbols {
		if s.QualifiedName != "Sidekiq.RingBuffer.<<" {
			continue
		}
		path := filepath.Join(root, "lib", "sidekiq", "ring_buffer.rb")
		failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "show", Path: path, Encoding: "utf-8", Language: "ruby",
			SymbolID: s.ID, SourceFingerprint: fingerprints["ring_buffer.rb"], MaxBytes: 4096,
		})
		if err != nil || failed != nil || shown.Show == nil ||
			!strings.HasPrefix(shown.Show.Text, "def <<(element)") {
			t.Fatalf("Ruby operator method show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
		}
		return
	}
	t.Fatal("real Ruby left shift method not found")
}

// This complete-file oracle is derived from the original Sidekiq source rather
// than from the analyzer's output. DSL-generated accessors and aliases are excluded.
func TestSourceSymbolsRealRubySidekiqTestAPI(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_RUBY_SIDEKIQ_PROJECT")
	if root == "" {
		t.Skip("original qualified Ruby project not provided")
	}
	path := filepath.Join(root, "lib", "sidekiq", "test_api.rb")
	want := map[string]struct {
		kind string
		line int
	}{
		"Sidekiq":         {"module", 6},
		"Sidekiq.Testing": {"class", 7},
		"Sidekiq.Testing.TestModeAlreadySetError": {"class", 8},
		"Sidekiq.Testing.__set_test_mode":         {"method", 14},
		"Sidekiq.Testing.__test_mode":             {"method", 32},
		"Sidekiq.Testing.__local_test_mode":       {"method", 36},
		"Sidekiq.Testing.__local_test_mode=":      {"method", 40},
		"Sidekiq.Testing.disable!":                {"method", 44},
		"Sidekiq.Testing.fake!":                   {"method", 48},
		"Sidekiq.Testing.inline!":                 {"method", 52},
		"Sidekiq.Testing.enabled?":                {"method", 56},
		"Sidekiq.Testing.disabled?":               {"method", 60},
		"Sidekiq.Testing.fake?":                   {"method", 64},
		"Sidekiq.Testing.inline?":                 {"method", 68},
		"Sidekiq.Testing.server_middleware":       {"method", 72},
		"Sidekiq.EmptyQueueError":                 {"class", 80},
		"Sidekiq.TestingClient":                   {"module", 82},
		"Sidekiq.TestingClient.atomic_push":       {"method", 83},
		"Sidekiq.Queues":                          {"module", 107},
		"Sidekiq.Queues.[]":                       {"method", 174},
		"Sidekiq.Queues.push":                     {"method", 178},
		"Sidekiq.Queues.jobs_by_queue":            {"method", 183},
		"Sidekiq.Queues.jobs_by_class":            {"method", 187},
		"Sidekiq.Queues.delete_for":               {"method", 192},
		"Sidekiq.Queues.clear_for":                {"method", 197},
		"Sidekiq.Queues.clear_all":                {"method", 202},
		"Sidekiq.Job":                             {"module", 209},
		"Sidekiq.Job.ClassMethods":                {"module", 249},
		"Sidekiq.Job.ClassMethods.queue":          {"method", 251},
		"Sidekiq.Job.ClassMethods.jobs":           {"method", 256},
		"Sidekiq.Job.ClassMethods.clear":          {"method", 261},
		"Sidekiq.Job.ClassMethods.drain":          {"method", 266},
		"Sidekiq.Job.ClassMethods.perform_one":    {"method", 275},
		"Sidekiq.Job.ClassMethods.process_job":    {"method", 282},
		"Sidekiq.Job.ClassMethods.execute_job":    {"method", 291},
		"Sidekiq.Job.jobs":                        {"method", 297},
		"Sidekiq.Job.clear_all":                   {"method", 302},
		"Sidekiq.Job.drain_all":                   {"method", 307},
		"Sidekiq.TestingExtensions":               {"module", 319},
		"Sidekiq.TestingExtensions.jobs_for":      {"method", 320},
	}
	const expectedCount = 40
	if len(want) != expectedCount {
		t.Fatalf("source oracle cardinality %d, want %d", len(want), expectedCount)
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		MaxFiles: 1, MaxSymbols: 80,
	})
	if err != nil || failed != nil || !outline.CoverageComplete || outline.FilesParsed != 1 ||
		outline.SymbolCount != expectedCount || len(outline.Files) != 1 ||
		!outline.Files[0].CoverageComplete || len(outline.Symbols) != expectedCount {
		t.Fatalf("real Ruby test_api outline count/coverage: err=%v failed=%+v count=%d parsed=%d complete=%v symbols=%d",
			err, failed, outline.SymbolCount, outline.FilesParsed, outline.CoverageComplete, len(outline.Symbols))
	}
	byName := make(map[string]int, expectedCount)
	byID := make(map[string]string, expectedCount)
	for _, sym := range outline.Symbols {
		byName[sym.QualifiedName]++
		byID[sym.ID] = sym.QualifiedName
	}
	for _, sym := range outline.Symbols {
		expected, ok := want[sym.QualifiedName]
		parent := ""
		if at := strings.LastIndex(sym.QualifiedName, "."); at >= 0 {
			parent = sym.QualifiedName[:at]
		}
		if !ok || string(sym.Kind) != expected.kind || sym.DeclarationRange.Start.Line != expected.line ||
			sym.ParentQualifiedName != parent || (parent != "" && byID[sym.ParentID] != parent) ||
			(parent == "" && sym.ParentID != "") {
			t.Errorf("real Ruby incorrect symbol %+v expected=%+v exists=%v, parent=%q", sym, expected, ok, parent)
		}
	}
	for qualified := range want {
		if byName[qualified] != 1 {
			t.Errorf("real Ruby missing/duplicate %s count=%d", qualified, byName[qualified])
		}
	}
	failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Paths: []string{path}, Presentation: "compact", MaxFiles: 1,
	})
	if err != nil || failed != nil || !digest.CoverageComplete || digest.SymbolCount != expectedCount ||
		len(digest.OverviewFiles) != 1 || !digest.OverviewFiles[0].CoverageComplete ||
		digest.OverviewFiles[0].SourceFingerprint != outline.Files[0].SourceFingerprint {
		t.Fatalf("real Ruby test_api digest inconsistent: err=%v failed=%+v digest=%+v", err, failed, digest)
	}
	count := make(map[string]int)
	for _, item := range digest.OverviewFiles[0].DeclarationCounts {
		count[string(item.Kind)] += item.Count
	}
	if count["module"] != 6 || count["class"] != 3 || count["method"] != 31 {
		t.Errorf("real Ruby test_api digest counts: %v", count)
	}
	failed, tree, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "tree", MaxFiles: 1, MaxSymbols: 80,
	})
	if err != nil || failed != nil || !tree.CoverageComplete || tree.SymbolCount != expectedCount ||
		len(tree.OutlineFiles) != 1 || !tree.OutlineFiles[0].CoverageComplete {
		t.Fatalf("real Ruby tree incomplete: err=%v failed=%+v tree=%+v", err, failed, tree)
	}
	treeSeen := make(map[string]int, expectedCount)
	var visit func([]*SourceOutlineNode, string)
	visit = func(nodes []*SourceOutlineNode, parent string) {
		for _, node := range nodes {
			wantNode, exists := want[node.QualifiedName]
			if !exists || string(node.Kind) != wantNode.kind || node.Line != wantNode.line {
				t.Errorf("incorrect real Ruby tree node %+v", node)
			}
			if parent != "" && !strings.HasPrefix(node.QualifiedName, parent+".") {
				t.Errorf("wrong Ruby tree child %s of %s", node.QualifiedName, parent)
			}
			treeSeen[node.QualifiedName]++
			visit(node.Children, node.QualifiedName)
		}
	}
	visit(tree.OutlineFiles[0].Symbols, "")
	for qualified := range want {
		if treeSeen[qualified] != 1 {
			t.Errorf("real Ruby tree %s count=%d", qualified, treeSeen[qualified])
		}
	}
	for _, question := range []struct {
		query string
		found bool
	}{
		{"Sidekiq.TestingClient.atomic_push", true},
		{"Sidekiq.Queues.[]", true},
		{"Sidekiq.Queues.push", true},
		{"Sidekiq.Testing.__global_test_mode", false}, // attr_accessor is a runtime DSL
		{"Sidekiq.Queues.jobs_by_worker", false},      // alias_method is a runtime DSL
	} {
		failed, found, err := h.SourceQuery(ctx, nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Language: "ruby", Encoding: "utf-8",
			Query: question.query, Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || found.Search == nil {
			t.Fatalf("real Ruby structural search %s: err=%v failed=%+v found=%+v",
				question.query, err, failed, found)
		}
		matches := found.Search.Matches
		if question.found && (len(matches) != 1 || matches[0].QualifiedName != question.query ||
			string(matches[0].Kind) != "method" || matches[0].SourceFingerprint != outline.Files[0].SourceFingerprint) {
			t.Errorf("real Ruby missing/wrong search %s: %+v", question.query, matches)
		}
		if !question.found && len(matches) != 0 {
			t.Errorf("Ruby dynamic DSL was incorrectly promoted to symbol %s: %+v", question.query, matches)
		}
	}
	for _, sym := range outline.Symbols {
		if sym.QualifiedName != "Sidekiq.Queues.[]" {
			continue
		}
		failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "show", Path: path, Language: "ruby", Encoding: "utf-8",
			SymbolID: sym.ID, SourceFingerprint: outline.Files[0].SourceFingerprint, MaxBytes: 1024,
		})
		if err != nil || failed != nil || shown.Show == nil ||
			!strings.HasPrefix(shown.Show.Text, "def [](queue)") {
			t.Fatalf("real Ruby bracket show: err=%v failed=%+v show=%+v", err, failed, shown.Show)
		}
		return
	}
	t.Fatal("real Ruby bracket method missing")
}
