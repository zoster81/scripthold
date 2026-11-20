package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestOpenAPIDialectRejectsArazzoWorkflowReferences(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		for _, tc := range []struct {
			name, source, want string
			complete           bool
		}{
			{"arazzo", "arazzo: 1.0.1\nworkflows:\n  - workflowId: buy\n    steps:\n      - stepId: call\n        operationId: buyMuseumTickets\n", "", false},
			{"arazzo-after-reference", "steps:\n  - operationId: falsePositive\narazzo: 1.0.1\n", "", false},
			{"openapi", "openapi: 3.1.0\npaths:\n  /tickets:\n    get:\n      operationId: buyMuseumTickets\n", "buyMuseumTickets", true},
			{"rootless-component", "paths:\n  /tickets:\n    get:\n      operationId: buyMuseumTickets\n", "buyMuseumTickets", true},
		} {
			t.Run(tc.name+map[string]string{"\n": "-lf", "\r\n": "-crlf"}[eol], func(t *testing.T) {
				src := strings.ReplaceAll(tc.source, "\n", eol)
				result, err := (OpenAPIAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(src), testAnalyzeOptions(true, 100))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.CoverageComplete != tc.complete {
					t.Errorf("coverageComplete=%v want %v", result.Analysis.CoverageComplete, tc.complete)
				}
				found := false
				for _, sym := range result.Analysis.Symbols {
					if sym.Kind == SymbolKindOperation {
						if tc.want == "" {
							t.Errorf("Arazzo reference misclassified as OpenAPI operation: %+v", sym)
						}
						if sym.Name == tc.want {
							found = true
						}
					}
				}
				if tc.want != "" && !found {
					t.Errorf("missing genuine OpenAPI operation %q: %v", tc.want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			})
		}
	}
}

func TestOpenAPIOriginalArazzoIsNotOperation(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_ARAZZO_WORKFLOW_20261101")
	if path == "" {
		t.Skip("qualified read-only Arazzo fixture is not configured")
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (OpenAPIAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(string(buf)), testAnalyzeOptions(true, 100))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Errorf("wrong grammar must not claim complete OpenAPI analysis")
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Kind == SymbolKindOperation {
			t.Errorf("workflow step reference was indexed as an operation: %+v", sym)
		}
	}
}
