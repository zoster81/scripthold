package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Indented Sass selectors own the direct and nested source needed to maintain
// their layout; siblings and unrelated top-level blocks must remain separate.
func TestSassIndentedSelectorDeclarationIncludesBody(t *testing.T) {
	const source = ".hbox\n" +
		"  +display-box\n" +
		"  +box-orient\n" +
		"  & > *\n" +
		"    +box-flex\n" +
		"  // a nested source comment\n" +
		"\n" +
		".vbox\n" +
		"  +box-orient(vertical)\n" +
		"\n" +
		"#app\n" +
		"  color: $text\n" +
		"  @media (max-width: $break-small)\n" +
		"    width: 250px\n" +
		"  box-shadow: 0px 10px $shadow\n" +
		".secondary\n" +
		"  color: black\n"
	for _, variant := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(variant.name, func(t *testing.T) {
			result, err := (SassAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", variant.newline)),
				testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("valid Sass must remain covered: %+v", result.Analysis)
			}
			want := map[string]struct{ start, end int }{
				".hbox":      {1, 5},
				"& > *":      {4, 5},
				".vbox":      {8, 9},
				"#app":       {11, 15},
				".secondary": {16, 17},
			}
			seen := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				expected, ok := want[symbol.Name]
				if !ok {
					continue
				}
				seen[symbol.Name] = true
				if symbol.Kind != SymbolKindSelector ||
					symbol.DeclarationRange.Start.Line != expected.start ||
					symbol.DeclarationRange.End.Line != expected.end {
					t.Errorf("Sass selector %q must own its exact indented block: %+v", symbol.Name, symbol)
				}
				if strings.Contains(symbol.Signature, "\n") {
					t.Errorf("outline signature for %q must stay header-only: %q", symbol.Name, symbol.Signature)
				}
			}
			for name := range want {
				if !seen[name] {
					t.Errorf("expected selector %q absent: %+v", name, result.Analysis.Symbols)
				}
			}
		})
	}
}

// Nested indented Sass selectors have a lexical owner. Their source identity
// must not be presented as a peer of the enclosing selector.
func TestSassNestedSelectorRetainsLexicalParent(t *testing.T) {
	check := func(t *testing.T, result AnalyzerResult) {
		t.Helper()
		var outer, nested *NormalizedSymbol
		for i := range result.Analysis.Symbols {
			symbol := &result.Analysis.Symbols[i]
			switch symbol.Name {
			case ".v-alert--doc":
				outer = symbol
			case ".v-alert__content p:last-child":
				nested = symbol
			}
		}
		if outer == nil || nested == nil {
			t.Fatalf("authored Sass selector missing: %+v", result.Analysis.Symbols)
		}
		if nested.ParentID != outer.ID || nested.ParentQualifiedName != outer.QualifiedName {
			t.Fatalf("nested Sass selector has incorrect lexical owner: outer=%+v nested=%+v", outer, nested)
		}
		if nested.QualifiedName != nested.Name {
			t.Fatalf("nested Sass exact-name queries must remain compatible: %+v", nested)
		}
	}
	const sass = ".v-alert--doc\n  .v-alert__content p:last-child\n    margin-bottom: 0 !important\n"
	for _, ending := range []struct{ name, sep string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(ending.name, func(t *testing.T) {
			result, err := (SassAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("nested.sass", strings.ReplaceAll(sass, "\n", ending.sep)),
				testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			check(t, result)
		})
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_VUE_VUETIFY_TS_ROOT"); root != "" {
		t.Run("source-original-vuetify-Alert-vue-sass", func(t *testing.T) {
			path := filepath.Join(root, "Alert.vue")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Vue source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if !strings.Contains(source, "<style lang=\"sass\">") ||
				!strings.Contains(source, "  .v-alert--doc\n    .v-alert__content p:last-child") {
				t.Fatal("original Vuetify nested Sass selector oracle changed")
			}
			result, err := (VueAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("Alert.vue", source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			check(t, result)
		})
	}
}

// Child-combinator selectors are declarations in indented Sass, not properties.
// Keep the authored header and lexical owner even when the header begins with >.
func TestSassChildCombinatorSelectorsFromOriginalSource(t *testing.T) {
	check := func(t *testing.T, source string) {
		t.Helper()
		result, err := (SassAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("style.sass", source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		parents := make(map[string]NormalizedSymbol)
		var h4 []NormalizedSymbol
		var right []NormalizedSymbol
		for _, symbol := range result.Analysis.Symbols {
			switch symbol.Name {
			case "#title", "#eth-info":
				parents[symbol.Name] = symbol
			case "> h4":
				h4 = append(h4, symbol)
			case "> #right":
				right = append(right, symbol)
			}
		}
		if len(parents) != 2 || len(h4) != 2 || len(right) != 1 {
			t.Fatalf("authored Sass combinator declarations omitted: parents=%v h4=%+v right=%+v symbols=%v",
				parents, h4, right, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		ownedBy := func(symbol NormalizedSymbol, parent NormalizedSymbol) bool {
			return symbol.ParentID == parent.ID && symbol.ParentQualifiedName == parent.QualifiedName &&
				symbol.QualifiedName == symbol.Name
		}
		if !ownedBy(h4[0], parents["#title"]) || !ownedBy(h4[1], parents["#eth-info"]) ||
			!ownedBy(right[0], parents["#eth-info"]) || h4[0].ID == h4[1].ID {
			t.Fatalf("Sass combinator parent/identity incorrect: h4=%+v right=%+v parents=%+v", h4, right, parents)
		}
	}
	const fixture = "#title\n  > h4\n    margin: 0\n#eth-info\n  > h4\n    margin: 0\n  > #right\n    text-align: right\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture}, {"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) { check(t, tc.source) })
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_SASS_DANILO_ROOT"); root != "" {
		t.Run("original-sass-danilo-style", func(t *testing.T) {
			path := filepath.Join(root, "style.sass")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Sass source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, "\n  > h4\n") != 2 ||
				strings.Count(source, "\n  > #right\n") != 1 {
				t.Fatal("original Sass combinator oracle changed")
			}
			check(t, source)
		})
	}
}

func TestSassSelectorBodyStopsBeforePeerAtSameIndentWithTabs(t *testing.T) {
	const source = ".outer\n" +
		"\tcolor: red\n" +
		"\t.nested\n" +
		"\t\tcolor: blue\n" +
		"\t.other\n" +
		"\t\tcolor: green\n" +
		".next\n" +
		"\tcolor: black\n"
	for _, newline := range []string{"\n", "\r\n"} {
		result, err := (SassAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(strings.ReplaceAll(source, "\n", newline)),
			testAnalyzeOptions(true, 128))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]int{".outer": 6, ".nested": 4, ".other": 6, ".next": 8}
		seen := make(map[string]NormalizedSymbol, len(want))
		for _, symbol := range result.Analysis.Symbols {
			if end, ok := want[symbol.Name]; ok {
				seen[symbol.Name] = symbol
				if symbol.DeclarationRange.End.Line != end {
					t.Errorf("tab-indented peer should not leak into %s: %+v", symbol.Name, symbol)
				}
			}
		}
		if len(seen) != len(want) {
			t.Fatalf("Sass tab-indented selectors missing: %+v", result.Analysis.Symbols)
		}
		for _, name := range []string{".nested", ".other"} {
			if seen[name].QualifiedName != name {
				t.Errorf("Sass selector exact-name query changed: %+v", seen[name])
			}
			if seen[name].ParentID != seen[".outer"].ID ||
				seen[name].ParentQualifiedName != seen[".outer"].QualifiedName {
				t.Errorf("%s must be owned by outer Sass selector: %+v", name, seen[name])
			}
		}
		for _, name := range []string{".outer", ".next"} {
			if seen[name].ParentID != "" || seen[name].ParentQualifiedName != "" {
				t.Errorf("top-level Sass selector %s acquired a false parent: %+v", name, seen[name])
			}
		}
	}
}
