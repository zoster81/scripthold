package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestCFamilyInlineBlockCommentApostropheDoesNotTriggerStringError(t *testing.T) {
	source := "int compare(void) {\n if (a ||/* previous line doesn't change */\n b) return 1;\n return 0;\n}\n"
	for _, ending := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		for _, analyzer := range []struct {
			name   string
			source SourceAnalyzer
		}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
			t.Run(analyzer.name+"/"+ending.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", ending.value))
				got, err := analyzer.source.Analyze(context.Background(), doc, testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if !got.Analysis.CoverageComplete || got.Analysis.Truncated {
					t.Fatalf("valid C block comment with apostrophe must not report unterminated string: diagnostics=%+v", got.Analysis.Diagnostics)
				}
			})
		}
	}
}
