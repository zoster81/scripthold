package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestClojureTestDeclarationsAreNavigableWithoutFabricatedReaderForms(t *testing.T) {
	lines := []string{
		"(ns demo.tests (:require [clojure.test :refer [deftest]]))",
		"(deftest real-test (is true))",
		"(clojure.test/deftest qualified-test (is true))",
		"(defn helper [] true)",
		"; (deftest comment-fake (is true))",
		"#_(deftest discarded-fake (is true))",
		"'(deftest quoted-fake (is true))",
		"(other.lib/deftest unrelated-macro (is true))",
		"(defn wrapper [] (deftest nested-fake (is true)))",
	}
	for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			src := strings.Join(lines, eol.value) + eol.value
			result, err := (ClojureAnalyzer{}).Analyze(context.Background(), testSourceDocument("demo.clj", src), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("complete Clojure source reported partial: %+v", result.Analysis.Diagnostics)
			}
			found := map[string]NormalizedSymbol{}
			for _, symbol := range result.Analysis.Symbols {
				found[symbol.Name] = symbol
			}
			for name, line := range map[string]int{"real-test": 2, "qualified-test": 3} {
				sym, ok := found[name]
				if !ok || sym.Kind != SymbolKindFunction || sym.NativeKind != "deftest" || sym.NameRange.Start.Line != line {
					t.Fatalf("test declaration %s:%d not indexed as source-confirmed test: %+v", name, line, sym)
				}
			}
			if _, ok := found["helper"]; !ok {
				t.Fatal("adjacent ordinary function missing")
			}
			for _, fake := range []string{"comment-fake", "discarded-fake", "quoted-fake", "unrelated-macro", "nested-fake"} {
				if _, ok := found[fake]; ok {
					t.Fatalf("invented Clojure declaration %s: %+v", fake, found[fake])
				}
			}
		})
	}
}

// A reader quote before a form is not the apostrophe inside an authored name.
// After metadata, only the next immediate atom can be a declaration name.
func TestClojureApostropheAndMetadataDeclarationNames(t *testing.T) {
	lines := []string{
		"(ns highlighting)",
		"(def fizz 1)",
		"(def fizz' 2)",
		"(def ^{:private true}",
		"  (= 1 1))",
		"(def = 3)",
		"(defn keep [] 'quoted)",
		"; (def ghost 4)",
	}
	for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			text := strings.Join(lines, eol.value) + eol.value
			result, err := (ClojureAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("names.clj", text), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("bounded original-like Clojure source unexpectedly incomplete: %+v", result.Analysis.Diagnostics)
			}
			if got := len(result.Analysis.Symbols); got != 5 {
				t.Fatalf("expected ns, fizz, fizz', =, keep; got %d symbols: %+v", got, result.Analysis.Symbols)
			}
			seen := map[string]int{}
			for _, sym := range result.Analysis.Symbols {
				seen[sym.Name]++
				if sym.Name == "fizz'" {
					if sym.QualifiedName != "highlighting.fizz'" ||
						sym.NameRange.Start.Line != 3 || sym.NameRange.Start.Column != 6 ||
						sym.NameRange.End.Line != 3 || sym.NameRange.End.Column != 11 ||
						sym.Signature != "(def fizz'" {
						t.Errorf("valid apostrophe-containing name lost authored spelling/ranges: %+v", sym)
					}
				}
			}
			for _, name := range []string{"highlighting", "fizz", "fizz'", "=", "keep"} {
				if seen[name] != 1 {
					t.Errorf("expected one authored %q, got %d", name, seen[name])
				}
			}
			if seen["ghost"] != 0 {
				t.Errorf("comment fabricated declaration ghost")
			}
		})
	}
}
