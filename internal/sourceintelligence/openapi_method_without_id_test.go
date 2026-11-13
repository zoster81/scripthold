package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestOpenAPIYAMLMethodsWithoutIDsStayWithinPathAndMethodScopes(t *testing.T) {
	source := strings.Join([]string{
		"openapi: 3.1.0",
		"info:",
		"  title: Nested endpoints",
		"paths:",
		"  /pets:",
		"    get:",
		"      operationId: listPets",
		"      responses:",
		"        '200':",
		"          description: OK",
		"    post:",
		"      summary: Create pet",
		"      responses:",
		"        '201':",
		"          content:",
		"            application/json:",
		"              schema:",
		"                properties:",
		"                  get:",
		"                    type: string",
		"  '/quoted/{id}':",
		"    delete:",
		"      summary: Delete pet",
		"    parameters:",
		"      - name: id",
		"        in: path",
		"components:",
		"  schemas:",
		"    Fake:",
		"      properties:",
		"        post:",
		"          type: string",
		"",
	}, "\n")
	for _, tc := range []struct{ name, text string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (OpenAPIAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("bounded OpenAPI syntax lost coverage: %+v", result.Analysis)
			}
			syms := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, expected := range []struct {
				name, native string
				line         int
			}{
				{"listPets", "operation", 7},
				{"POST /pets", "http-method", 11},
				{"DELETE /quoted/{id}", "http-method", 22},
			} {
				sym, ok := syms[expected.name]
				if !ok || sym.Kind != SymbolKindOperation || sym.NativeKind != expected.native ||
					sym.NameRange.Start.Line != expected.line {
					t.Errorf("OpenAPI expected endpoint missing/wrong %s: %+v", expected.name, sym)
				}
			}
			for _, forbidden := range []string{
				"GET /pets", "GET /quoted/{id}", "GET /pets/get",
				"POST /quoted/{id}", "POST /Fake", "DELETE /pets",
			} {
				if sym, ok := syms[forbidden]; ok {
					t.Errorf("OpenAPI nested/nonexistent endpoint invented %s: %+v", forbidden, sym)
				}
			}
		})
	}
}
