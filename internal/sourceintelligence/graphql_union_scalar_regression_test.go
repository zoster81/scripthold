package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Original Git source contains top-level unions and scalars needed to
// understand an unfamiliar GraphQL schema. Basic type navigation must not
// silently omit them while reporting the file complete.
func TestGraphQLUnionAndScalarDeclarations(t *testing.T) {
	check := func(t *testing.T, source string, want map[string]int) {
		t.Helper()
		out, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		seen := make(map[string]int)
		for _, sym := range out.Analysis.Symbols {
			if sym.NativeKind == "union" || sym.NativeKind == "scalar" {
				seen[sym.Name] = sym.NameRange.Start.Line
			}
		}
		if len(seen) != len(want) {
			t.Errorf("top-level union/scalar inventory mismatch: got=%v want=%v", seen, want)
		}
		for name, line := range want {
			if seen[name] != line {
				t.Errorf("missing or misplaced union/scalar %s:%d, got %v", name, line, seen)
			}
		}
	}
	t.Run("synthetic-negative-comments-and-block-strings", func(t *testing.T) {
		check(t, `# scalar CommentGhost
"""union DescriptionGhost
scalar StringGhost
"""
type Real { value: String }
union Feed = Story | Article
scalar CustomScalar @onScalar
`, map[string]int{"Feed": 6, "CustomScalar": 7})
	})
	if original := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_SCHEMA"); original != "" {
		t.Run("independent-original-schema", func(t *testing.T) {
			data, err := os.ReadFile(original)
			if err != nil {
				t.Fatal(err)
			}
			check(t, string(data), map[string]int{
				"Feed": 30, "AnnotatedUnion": 32,
				"CustomScalar": 34, "AnnotatedScalar": 36,
			})
		})
	}
}

// A field selector needs the authored type, not only the colon.
func TestGraphQLFieldSourceRangeRetainsTypeReference(t *testing.T) {
	const fixture = "type Mutation {\n" +
		"  actionName(\n" +
		"    arg1: SampleInput!\n" +
		"  ): SampleOutput\n" +
		"}\n" +
		"input SampleInput {\n" +
		"  username: String!\n" +
		"}\n" +
		"type SampleOutput {\n" +
		"  accessToken: String!\n" +
		"  statuses: [String!]!\n" +
		"  next: ID\n" +
		"}\n"
	for _, ending := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[ending], func(t *testing.T) {
			text := strings.ReplaceAll(fixture, "\n", ending)
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(text), testAnalyzeOptions(true, 40))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("supported GraphQL fields unexpectedly partial: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, want := range map[string]string{
				"Mutation.actionName":      "actionName(\n    arg1: SampleInput!\n  ): SampleOutput",
				"SampleInput.username":     "username: String!",
				"SampleOutput.accessToken": "accessToken: String!",
				"SampleOutput.statuses":    "statuses: [String!]!",
				"SampleOutput.next":        "next: ID",
			} {
				symbol, ok := symbols[name]
				if !ok || symbol.Kind != SymbolKindField {
					t.Errorf("missing qualified GraphQL field %s: %+v", name, symbol)
					continue
				}
				got := text[symbol.declarationOffsets.Start:symbol.declarationOffsets.End]
				want = strings.ReplaceAll(want, "\n", ending)
				if got != want {
					t.Errorf("%s declaration = %q, want %q", name, got, want)
				}
			}
			if _, falseField := symbols["Mutation.arg1"]; falseField {
				t.Error("argument falsely promoted to a Mutation field")
			}
		})
	}
}

// Based on gqlgen's original Todo / TodoInput schema: GraphQL permits
// ignored line endings between a directive name and its argument list.
// Those argument keys are not fields of the surrounding type.
func TestGraphQLDirectiveArgumentsFollowingLineBreakStayOutOfFields(t *testing.T) {
	const fixture = "type Todo {\n" +
		"  done: Boolean! @hasRole\n" +
		"    (role: OWNER)\n" +
		"  next: String!\n" +
		"}\n" +
		"input TodoInput {\n" +
		"  number: Int! @goField\n" +
		"    (type: \"github.com/99designs/gqlgen/_examples/todo.Number(int)\")\n" +
		"  enabled: Boolean!\n" +
		"}\n"
	for _, ending := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[ending], func(t *testing.T) {
			text := strings.ReplaceAll(fixture, "\n", ending)
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(text), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid directive argument formatting must not make source partial: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, want := range map[string]string{
				"Todo.done":         "done: Boolean!",
				"Todo.next":         "next: String!",
				"TodoInput.number":  "number: Int!",
				"TodoInput.enabled": "enabled: Boolean!",
			} {
				field, ok := symbols[name]
				if !ok || field.Kind != SymbolKindField {
					t.Errorf("missing authored field %s: %+v", name, field)
					continue
				}
				got := text[field.declarationOffsets.Start:field.declarationOffsets.End]
				if got != want {
					t.Errorf("%s source range = %q, want %q", name, got, want)
				}
			}
			for _, ghost := range []string{"Todo.role", "TodoInput.type"} {
				if s, exists := symbols[ghost]; exists {
					t.Errorf("directive argument was invented as a field: %+v", s)
				}
			}
		})
	}
}
