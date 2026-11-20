package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Jzon uses percent-prefixed internal function names, including %parse-next.
// Such reader symbols must not disappear while coverage is reported complete.
func TestCommonLispPercentSymbolsLFCRLFAndNegative(t *testing.T) {
	const original = "(defpackage :demo)\n(in-package :demo)\n(defun %parse-next (state) state)\n(defun parse%ready (state) state)\n; (defun %not-real (state) state)\n(defun literal () \"(defun %not-from-string (state) state)\")\n#\\%\n(defmacro %with-parser (body) body)\n"
	for _, tc := range []struct{ name, text string }{{"LF", original}, {"CRLF", strings.ReplaceAll(original, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (CommonLispAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if res.Analysis.Truncated || !res.Analysis.CoverageComplete {
				t.Fatalf("unexpected partial: %+v", res.Analysis)
			}
			found := make(map[string]NormalizedSymbol)
			for _, symbol := range res.Analysis.Symbols {
				found[symbol.Name] = symbol
			}
			for _, want := range []struct {
				name, native string
				line         int
			}{{"%parse-next", "defun", 3}, {"parse%ready", "defun", 4}, {"%with-parser", "defmacro", 8}} {
				symbol, ok := found[want.name]
				if !ok || symbol.NativeKind != want.native || symbol.NameRange.Start.Line != want.line {
					t.Errorf("missing source declaration %s: %+v present=%v", want.name, symbol, ok)
				}
			}
			for _, name := range []string{"%not-real", "%not-from-string"} {
				if _, ok := found[name]; ok {
					t.Errorf("phantom declaration %s indexed", name)
				}
			}
		})
	}
}
