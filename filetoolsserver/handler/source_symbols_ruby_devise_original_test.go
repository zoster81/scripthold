package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The independently annotated originals remain outside the repository.
func TestSourceSymbolsRealRubyDeviseSelectedFiles(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_RUBY_DEVISE_PROJECT")
	if root == "" {
		t.Skip("original Devise Ruby project not provided")
	}
	type expected struct {
		kind       string
		line       int
		visibility string
	}
	want := map[string]map[string]expected{
		"encryptor.rb": {
			"Devise":                   {"module", 5, "public"},
			"Devise.Encryptor":         {"module", 6, "public"},
			"Devise.Encryptor.digest":  {"method", 7, "public"},
			"Devise.Encryptor.compare": {"method", 14, "public"},
		},
		"parameter_filter.rb": {
			"Devise":                                 {"module", 3, "public"},
			"Devise.ParameterFilter":                 {"class", 4, "public"},
			"Devise.ParameterFilter.ParameterFilter": {"constructor", 5, "public"},
			"Devise.ParameterFilter.filter":          {"method", 10, "public"},
			"Devise.ParameterFilter.filtered_hash_by_method_for_given_keys": {"method", 19, "public"},
			"Devise.ParameterFilter.stringify_params":                       {"method", 31, "public"},
			"Devise.ParameterFilter.param_requires_string_conversion?":      {"method", 40, "private"},
		},
		"token_generator.rb": {
			"Devise":                               {"module", 5, "public"},
			"Devise.TokenGenerator":                {"class", 6, "public"},
			"Devise.TokenGenerator.TokenGenerator": {"constructor", 7, "public"},
			"Devise.TokenGenerator.digest":         {"method", 12, "public"},
			"Devise.TokenGenerator.generate":       {"method", 16, "public"},
			"Devise.TokenGenerator.key_for":        {"method", 28, "private"},
		},
		"delegator.rb": {
			"Devise":                       {"module", 3, "public"},
			"Devise.Delegator":             {"class", 5, "public"},
			"Devise.Delegator.call":        {"method", 6, "public"},
			"Devise.Delegator.failure_app": {"method", 10, "public"},
		},
		"time_inflector.rb": {
			"Devise":               {"module", 5, "public"},
			"Devise.TimeInflector": {"class", 6, "public"},
		},
		"version.rb": {
			"Devise":         {"module", 3, "public"},
			"Devise.VERSION": {"constant", 4, "public"},
		},
	}
	paths := make([]string, 0, 6)
	for _, name := range []string{"encryptor.rb", "parameter_filter.rb", "token_generator.rb", "delegator.rb", "time_inflector.rb", "version.rb"} {
		paths = append(paths, filepath.Join(root, "lib", "devise", name))
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Presentation: "detailed", MaxFiles: 6, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !outline.CoverageComplete || outline.SymbolCount != 25 ||
		outline.FilesParsed != 6 || len(outline.Files) != 6 || len(outline.Symbols) != 25 {
		t.Fatalf("original Devise outline: err=%v failed=%+v count=%d parsed=%d complete=%v",
			err, failed, outline.SymbolCount, outline.FilesParsed, outline.CoverageComplete)
	}
	fingerprints := make(map[string]string)
	for _, f := range outline.Files {
		name := filepath.Base(f.Path)
		if !f.CoverageComplete || f.Status != "parsed" || f.Language != "ruby" ||
			f.SourceFingerprint == "" || len(want[name]) == 0 {
			t.Errorf("unexpected original Devise file %+v", f)
		}
		fingerprints[name] = f.SourceFingerprint
	}
	byFile := make(map[string]map[string]int)
	byID := make(map[string]map[string]string)
	for _, symbol := range outline.Symbols {
		name := filepath.Base(symbol.Path)
		if byFile[name] == nil {
			byFile[name] = make(map[string]int)
		}
		if byID[name] == nil {
			byID[name] = make(map[string]string)
		}
		byFile[name][symbol.QualifiedName]++
		byID[name][symbol.ID] = symbol.QualifiedName
	}
	for _, symbol := range outline.Symbols {
		name := filepath.Base(symbol.Path)
		expected, ok := want[name][symbol.QualifiedName]
		parent := ""
		if at := strings.LastIndex(symbol.QualifiedName, "."); at >= 0 {
			parent = symbol.QualifiedName[:at]
		}
		if !ok || string(symbol.Kind) != expected.kind || symbol.DeclarationRange.Start.Line != expected.line ||
			string(symbol.Visibility) != expected.visibility || symbol.ParentQualifiedName != parent ||
			(parent != "" && byID[name][symbol.ParentID] != parent) ||
			(parent == "" && symbol.ParentID != "") {
			t.Errorf("incorrect Devise Ruby declaration %+v expected=%+v exists=%v", symbol, expected, ok)
		}
	}
	for file, symbols := range want {
		if len(byFile[file]) != len(symbols) {
			t.Errorf("Devise Ruby file %s returned %d symbols, want %d", file, len(byFile[file]), len(symbols))
		}
		for name := range symbols {
			if byFile[file][name] != 1 {
				t.Errorf("Devise Ruby file %s declaration %s count=%d", file, name, byFile[file][name])
			}
		}
	}
	failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Paths: paths, Presentation: "compact", MaxFiles: 6,
	})
	if err != nil || failed != nil || !digest.CoverageComplete || digest.SymbolCount != 25 ||
		digest.FilesParsed != 6 || len(digest.OverviewFiles) != 6 {
		t.Fatalf("original Devise digest: err=%v failed=%+v output=%+v", err, failed, digest)
	}
	for _, f := range digest.OverviewFiles {
		name := filepath.Base(f.Path)
		if !f.CoverageComplete || f.SourceFingerprint != fingerprints[name] {
			t.Errorf("Devise digest fingerprint mismatch %+v", f)
		}
		total := 0
		for _, entry := range f.DeclarationCounts {
			total += entry.Count
		}
		if total != len(want[name]) {
			t.Errorf("Devise digest %s count=%d want=%d", name, total, len(want[name]))
		}
	}
	// The original private method has no nested Ruby end-blocks: its entire
	// implementation must be available without reading the surrounding file.
	bodyPath := filepath.Join(root, "lib", "devise", "parameter_filter.rb")
	for _, symbol := range outline.Symbols {
		if symbol.QualifiedName != "Devise.ParameterFilter.param_requires_string_conversion?" {
			continue
		}
		if symbol.BodyRange == nil {
			t.Fatal("Devise simple private method lacks its verified body range")
		}
		failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "show", Path: bodyPath, SymbolID: symbol.ID,
			SourceFingerprint: fingerprints["parameter_filter.rb"], Language: "ruby", Encoding: "utf-8", MaxBytes: 1024,
		})
		if err != nil || failed != nil || shown.Show == nil ||
			!strings.HasPrefix(shown.Show.Text, "def param_requires_string_conversion?(value)") ||
			!strings.Contains(shown.Show.Text, "true") ||
			!strings.HasSuffix(strings.TrimSpace(shown.Show.Text), "end") {
			t.Fatalf("Devise simple Ruby source show is not the complete method: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
		}
		failed, contextOutput, err := h.SourceQuery(ctx, nil, SourceQueryInput{
			Operation: "context", Paths: []string{bodyPath}, Language: "ruby", Encoding: "utf-8",
			Targets: []SourceSelectorInput{{Kind: "symbol", Path: bodyPath, SymbolID: symbol.ID,
				SourceFingerprint: fingerprints["parameter_filter.rb"]}},
			BodyPolicy: "prefer", BudgetBytes: 2048, MaxDepth: 2, MaxItems: 4,
		})
		if err != nil || failed != nil || contextOutput.Context == nil || len(contextOutput.Context.Items) == 0 ||
			string(contextOutput.Context.Items[0].Representation) != "body" ||
			!strings.Contains(contextOutput.Context.Items[0].Text, "true") {
			t.Fatalf("Devise simple Ruby context did not select implementation: err=%v failed=%+v context=%+v", err, failed, contextOutput.Context)
		}
		break
	}
	path := filepath.Join(root, "lib", "devise", "time_inflector.rb")
	for _, query := range []string{"Devise.TimeInflector.instance", "Devise.TimeInflector.time_ago_in_words"} {
		failed, result, err := h.SourceQuery(ctx, nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Query: query, Match: "exact",
			Mode: "structural", Language: "ruby", Encoding: "utf-8", MaxResults: 8,
		})
		if err != nil || failed != nil || result.Search == nil || len(result.Search.Matches) != 0 {
			t.Errorf("Devise Ruby dynamic accessor/delegate generated false declaration %s: %+v err=%v", query, result, err)
		}
	}
}
