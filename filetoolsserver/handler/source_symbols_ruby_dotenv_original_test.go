package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original declarations were annotated from the complete pinned Dotenv source.
func TestSourceSymbolsRealRubyDotenvSelectedFile(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_RUBY_DOTENV_PROJECT")
	if root == "" {
		t.Skip("original qualified Dotenv project not provided")
	}
	path := filepath.Join(root, "lib", "dotenv.rb")
	type item struct {
		kind   string
		line   int
		vis    string
		parent string
	}
	want := map[string]item{
		"Dotenv":              {"module", 8, "public", ""},
		"Dotenv.SEMAPHORE":    {"constant", 12, "public", "Dotenv"},
		"Dotenv.load":         {"method", 18, "public", "Dotenv"},
		"Dotenv.load!":        {"method", 27, "public", "Dotenv"},
		"Dotenv.overwrite":    {"method", 32, "public", "Dotenv"},
		"Dotenv.overwrite!":   {"method", 38, "public", "Dotenv"},
		"Dotenv.parse":        {"method", 50, "public", "Dotenv"},
		"Dotenv.save":         {"method", 67, "public", "Dotenv"},
		"Dotenv.restore":      {"method", 77, "public", "Dotenv"},
		"Dotenv.update":       {"method", 98, "public", "Dotenv"},
		"Dotenv.modify":       {"method", 124, "public", "Dotenv"},
		"Dotenv.require_keys": {"method", 134, "public", "Dotenv"},
		"Dotenv.instrument":   {"method", 142, "private", "Dotenv"},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	failed, detail, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Presentation: "detailed", Language: "ruby", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 32})
	if err != nil || failed != nil || !detail.CoverageComplete || len(detail.Files) != 1 || !detail.Files[0].CoverageComplete || detail.SymbolCount != len(want) || len(detail.Symbols) != len(want) {
		t.Fatalf("Dotenv outline mismatch err=%v failed=%+v output=%+v", err, failed, detail)
	}
	fingerprint := detail.Files[0].SourceFingerprint
	seen := map[string]int{}
	ids := map[string]string{}
	for _, s := range detail.Symbols {
		v, ok := want[s.QualifiedName]
		seen[s.QualifiedName]++
		ids[s.QualifiedName] = s.ID
		if !ok || string(s.Kind) != v.kind || s.DeclarationRange.Start.Line != v.line || string(s.Visibility) != v.vis || s.ParentQualifiedName != v.parent || s.Path != path {
			t.Errorf("unexpected Dotenv symbol %+v expected=%+v exists=%v", s, v, ok)
		}
	}
	for k := range want {
		if seen[k] != 1 {
			t.Errorf("Dotenv missing/duplicate %s: %d", k, seen[k])
		}
	}
	for _, s := range detail.Symbols {
		if s.ParentQualifiedName != "" && s.ParentID != ids[s.ParentQualifiedName] {
			t.Errorf("Dotenv parent ID wrong %+v", s)
		}
	}
	failed, tree, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Presentation: "tree", Language: "ruby", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 32})
	if err != nil || failed != nil || !tree.CoverageComplete || tree.SymbolCount != len(want) || len(tree.OutlineFiles) != 1 {
		t.Fatalf("Dotenv tree mismatch err=%v failed=%+v tree=%+v", err, failed, tree)
	}
	nodes := map[string]int{}
	var visit func([]*SourceOutlineNode, string)
	visit = func(items []*SourceOutlineNode, parent string) {
		for _, n := range items {
			v, ok := want[n.QualifiedName]
			nodes[n.QualifiedName]++
			if !ok || v.kind != string(n.Kind) || v.line != n.Line || v.parent != parent {
				t.Errorf("Dotenv tree node %+v parent=%s", n, parent)
			}
			visit(n.Children, n.QualifiedName)
		}
	}
	visit(tree.OutlineFiles[0].Symbols, "")
	for k := range want {
		if nodes[k] != 1 {
			t.Errorf("Dotenv tree missing/duplicate %s: %d", k, nodes[k])
		}
	}
	failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "digest", Paths: []string{path}, Presentation: "compact", Language: "ruby", Encoding: "utf-8", MaxFiles: 1})
	if err != nil || failed != nil || !digest.CoverageComplete || digest.SymbolCount != 13 || len(digest.OverviewFiles) != 1 || digest.OverviewFiles[0].SourceFingerprint != fingerprint {
		t.Fatalf("Dotenv digest err=%v failed=%+v output=%+v", err, failed, digest)
	}
	counts := map[string]int{}
	for _, c := range digest.OverviewFiles[0].DeclarationCounts {
		counts[string(c.Kind)] += c.Count
	}
	if counts["module"] != 1 || counts["constant"] != 1 || counts["method"] != 11 {
		t.Errorf("Dotenv digest kinds=%v", counts)
	}
	failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Query: "instrument", Match: "exact", Language: "ruby", Encoding: "utf-8", MaxSymbols: 8})
	if err != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].QualifiedName != "Dotenv.instrument" {
		t.Fatalf("Dotenv find err=%v failed=%+v output=%+v", err, failed, found)
	}
	for _, q := range []struct {
		name    string
		present bool
	}{{"Dotenv.load!", true}, {"Dotenv.instrument", true}, {"Dotenv.instrumenter", false}, {"Dotenv.overload", false}, {"Dotenv.overload!", false}} {
		failed, out, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Paths: []string{path}, Query: q.name, Match: "exact", Mode: "structural", Language: "ruby", Encoding: "utf-8", MaxResults: 8})
		if err != nil || failed != nil || out.Search == nil {
			t.Fatalf("Dotenv search %s err=%v failed=%+v", q.name, err, failed)
		}
		got := len(out.Search.Matches)
		if q.present && got != 1 || !q.present && got != 0 {
			t.Errorf("Dotenv search %s got=%+v", q.name, out.Search.Matches)
		}
	}
	failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, SymbolID: ids["Dotenv.load!"], SourceFingerprint: fingerprint, Language: "ruby", Encoding: "utf-8", MaxBytes: 1024})
	if err != nil || failed != nil || shown.Show == nil || !strings.HasPrefix(shown.Show.Text, "def load!(*filenames)") || !strings.Contains(shown.Show.Text, "load(*filenames, ignore: false)") || !strings.HasSuffix(strings.TrimSpace(shown.Show.Text), "end") {
		t.Fatalf("Dotenv show err=%v failed=%+v show=%+v", err, failed, shown.Show)
	}
	failed, c, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "context", Paths: []string{path}, Language: "ruby", Encoding: "utf-8", Targets: []SourceSelectorInput{{Kind: "symbol", Path: path, SymbolID: ids["Dotenv.load!"], SourceFingerprint: fingerprint}}, BodyPolicy: "prefer", BudgetBytes: 2048, MaxItems: 4, MaxDepth: 2})
	if err != nil || failed != nil || c.Context == nil || len(c.Context.Items) == 0 || string(c.Context.Items[0].Representation) != "body" || !strings.Contains(c.Context.Items[0].Text, "load(*filenames, ignore: false)") {
		t.Fatalf("Dotenv context err=%v failed=%+v context=%+v", err, failed, c.Context)
	}
}
