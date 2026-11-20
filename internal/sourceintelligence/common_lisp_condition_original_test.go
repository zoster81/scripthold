package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCommonLispQualifiedOriginalHunchentootCondition(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_COMMON_LISP_HUNCHENTOOT")
	if path == "" {
		t.Skip("qualified original Common Lisp source not configured")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		t.Fatal("original must be a regular file at most 1MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(data))
	doc.Path = path
	result, err := (CommonLispAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
		t.Fatalf("unexpected partial analysis: %+v", result.Analysis)
	}
	for _, s := range result.Analysis.Symbols {
		if s.Name == "hunchentoot-condition" && s.Kind == SymbolKindClass && s.NativeKind == "define-condition" && s.NameRange.Start.Line == 31 {
			return
		}
	}
	t.Errorf("original Common Lisp condition line 31 missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
}

func TestCommonLispConditionDefinitionLFCRLFAndNegatives(t *testing.T) {
	const source = "(defpackage :demo)\n(in-package :demo)\n(define-condition demo-error (error) ())\n(defun report-it () \"(define-condition phantom (error) ())\")\n; (define-condition comment-phantom (error) ())\n(defclass actual () ())\n"
	for _, tc := range []struct{ name, text string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CommonLispAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("partial: %+v", result.Analysis)
			}
			found := map[string]NormalizedSymbol{}
			for _, s := range result.Analysis.Symbols {
				found[s.Name] = s
			}
			for _, tc := range []struct {
				name, native string
				line         int
			}{{"demo-error", "define-condition", 3}, {"actual", "defclass", 6}} {
				got, ok := found[tc.name]
				if !ok || got.Kind != SymbolKindClass || got.NativeKind != tc.native || got.NameRange.Start.Line != tc.line {
					t.Errorf("missing valid class or condition %q: %+v present=%v", tc.name, got, ok)
				}
			}
			for _, name := range []string{"phantom", "comment-phantom"} {
				if _, ok := found[name]; ok {
					t.Errorf("phantom definition %s incorrectly indexed", name)
				}
			}
		})
	}
}
