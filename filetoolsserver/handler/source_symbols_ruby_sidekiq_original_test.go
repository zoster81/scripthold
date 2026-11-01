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
