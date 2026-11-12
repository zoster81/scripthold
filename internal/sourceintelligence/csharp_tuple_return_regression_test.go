package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Parentheses in a tuple return type are not method parameter lists.
func TestCSharpTupleReturnMethodsAcrossHosts(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
	}{
		{
			name: "csharp", analyzer: CSharpAnalyzer{},
			source: `class TicketQueries {
    IEnumerable<(string Actual, string Expected)> Cases() {
        yield return ("actual", "expected");
    }
    IEnumerable<(string Actual, string Expected)> Run() => Cases();
    bool IsValid() => true;
}`,
		},
		{
			name: "blazor", analyzer: BlazorAnalyzer{},
			source: `@code {
    IEnumerable<(string Actual, string Expected)> Cases() {
        yield return ("actual", "expected");
    }
    IEnumerable<(string Actual, string Expected)> Run() => Cases();
    bool IsValid() => true;
}`,
		},
		{
			name: "razor", analyzer: RazorAnalyzer{},
			source: `@functions {
    IEnumerable<(string Actual, string Expected)> Cases() {
        yield return ("actual", "expected");
    }
    IEnumerable<(string Actual, string Expected)> Run() => Cases();
    bool IsValid() => true;
}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ending := range []struct {
				name, source string
			}{
				{"LF", tc.source},
				{"CRLF", strings.ReplaceAll(tc.source, "\n", "\r\n")},
			} {
				t.Run(ending.name, func(t *testing.T) {
					result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(ending.source), testAnalyzeOptions(true, 64))
					if err != nil {
						t.Fatal(err)
					}
					if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
						t.Fatalf("source analysis partial: %+v", result.Analysis)
					}
					counts := map[string]int{}
					for _, symbol := range result.Analysis.Symbols {
						if symbol.Kind == SymbolKindMethod {
							counts[symbol.Name]++
						}
					}
					if counts["Cases"] != 1 || counts["Run"] != 1 || counts["IsValid"] != 1 || len(counts) != 3 {
						t.Fatalf("tuple return method declarations mismatch: %v", counts)
					}
				})
			}
		})
	}
}
