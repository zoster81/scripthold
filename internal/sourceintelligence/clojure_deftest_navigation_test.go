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
