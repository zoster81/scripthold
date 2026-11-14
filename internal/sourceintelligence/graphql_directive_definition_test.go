package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Directives are named schema definitions; a directive annotation such as
// @hasRole on a field is not a second definition.
func TestGraphQLDirectiveDefinitionNavigation(t *testing.T) {
	const source = "# directive @commented on FIELD\\n" +
		"\"\"\"directive @quoted on FIELD\"\"\"\\n" +
		"directive @goField(\\n" +
		"  forceResolver: Boolean\\n" +
		"  name: String\\n" +
		") on INPUT_FIELD_DEFINITION | FIELD_DEFINITION\\n" +
		"type Query {\\n" +
		"  todo: ID @hasRole(role: OWNER)\\n" +
		"  directive @nested on FIELD\\n" +
		"}\\n" +
		"directive @hasRole(role: Role!) on FIELD_DEFINITION\\n" +
		"directive @include(if: Boolean!)\\n" +
		"  on FIELD\\n" +
		"  | FRAGMENT_SPREAD\\n" +
		"directive @cached repeatable on FIELD | QUERY\\n"
	// Each literal backslash above is converted to source newlines for the
	// fixture before applying the physical line-ending variation.
	normalized := strings.ReplaceAll(source, "\\n", "\n")
	for _, tc := range []struct{ name, text string }{
		{"LF", normalized}, {"CRLF", strings.ReplaceAll(normalized, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name       string
				start, end int
			}{
				{"goField", 3, 6},
				{"hasRole", 11, 11},
				{"include", 12, 14},
				{"cached", 15, 15},
			} {
				s, ok := byName[want.name]
				if !ok || s.Kind != SymbolKindSection || s.NativeKind != "directive" ||
					s.ParentQualifiedName != "" || s.NameRange.Start.Line != want.start ||
					s.DeclarationRange.End.Line != want.end {
					t.Errorf("directive %q missing/wrong kind/owner/range: %+v", want.name, s)
				}
			}
			for _, forbidden := range []string{"commented", "quoted", "nested"} {
				if s, ok := byName[forbidden]; ok {
					t.Errorf("invented directive from comment, quote or nested field: %+v", s)
				}
			}
			if s, ok := byName["Query"]; !ok || s.Kind != SymbolKindType {
				t.Errorf("real GraphQL type lost: %+v", s)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("well-formed GraphQL schema wrongly partial/truncated: %+v", result.Analysis)
			}
		})
	}
}
