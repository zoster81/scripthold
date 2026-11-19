package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The original graphql-client file authors three fragments and three named
// query selections; whole declaration excerpts should not stop at the name.
func TestGraphQLPinnedNamedOperationFullSourceExcerpts(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("independent pinned GraphQL outline originals not configured")
	}
	raw, err := os.ReadFile(filepath.Join(root, "rust-fragments.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 394 {
		t.Fatalf("upstream GraphQL original changed: %d bytes", len(raw))
	}
	for _, tc := range []struct{ name, source string }{
		{"LF", string(raw)}, {"CRLF", strings.ReplaceAll(string(raw), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("rust-fragments.graphql", tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 6 {
				t.Fatalf("six source declarations unexpectedly changed: %+v", result.Analysis)
			}
			wanted := map[string]int{"FragmentReference": 18, "SnakeCaseFragment": 22, "RecursiveFragmentQuery": 28}
			fragments := map[int]int{1: 3, 5: 7, 9: 14}
			seenQueries := map[string]bool{}
			for _, s := range result.Analysis.Symbols {
				if s.Kind == SymbolKindSection && s.NativeKind == "fragment" {
					want, ok := fragments[s.NameRange.Start.Line]
					if !ok || s.DeclarationRange.End.Line != want {
						t.Errorf("fragment span changed: %+v", s)
					}
					continue
				}
				if s.Kind != SymbolKindOperation || s.NativeKind != "query" {
					t.Errorf("unexpected declaration %+v", s)
					continue
				}
				want, ok := wanted[s.Name]
				if !ok || s.DeclarationRange.End.Line != want ||
					s.SignatureRange.End.Line != want || s.NameRange.Start.Line != s.DeclarationRange.Start.Line {
					t.Errorf("whole authored query selection missing from excerpt: %+v", s)
				}
				seenQueries[s.Name] = true
			}
			if len(seenQueries) != 3 {
				t.Fatalf("missing original named queries %v", seenQueries)
			}
		})
	}
}

func TestGraphQLDirectMutationAndSubscriptionSelectionRanges(t *testing.T) {
	const originalShape = "mutation Save {\n  write { ok }\n}\n" +
		"subscription Watch {\n  changed { id }\n}\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", originalShape}, {"CRLF", strings.ReplaceAll(originalShape, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("operation.graphql", tc.source), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]struct {
				native string
				end    int
			}{"Save": {"mutation", 3}, "Watch": {"subscription", 6}}
			if len(result.Analysis.Symbols) != len(want) {
				t.Fatalf("two genuine operations expected, got %+v", result.Analysis.Symbols)
			}
			for _, symbol := range result.Analysis.Symbols {
				w, ok := want[symbol.Name]
				if !ok || symbol.NativeKind != w.native ||
					symbol.DeclarationRange.End.Line != w.end ||
					symbol.SignatureRange.End.Line != w.end {
					t.Errorf("direct operation selection not preserved: %+v", symbol)
				}
			}
		})
	}
}

func TestGraphQLDirectOperationSpanBoundsAndNestedRejection(t *testing.T) {
	cases := []struct {
		name, source, target string
		endLine              int
	}{
		{"large-body", "query Huge {\n  thing(arg: \"" + strings.Repeat("x", 17000) + "\")\n}\n", "Huge", 1},
		{"too-many-lines", "query Tall {\n" + strings.Repeat("  item\n", 129) + "}\n", "Tall", 1},
		{"nested-token", "fragment Outer on Type {\n  query Nested { id }\n}\n", "Nested", 2},
		{"unclosed", "query Open {\n  item { id }\n", "Open", 1},
		{"direct-sibling", "query First { id }\nquery Second { id }\n", "Second", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("bounded.graphql", tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name == tc.target && symbol.NativeKind == "query" {
					found = true
					if symbol.DeclarationRange.End.Line != tc.endLine {
						t.Errorf("unproven GraphQL selection extended or bounded block lost: %+v", symbol)
					}
				}
			}
			if !found {
				t.Fatalf("existing named header not navigable: %s", tc.target)
			}
		})
	}
}

func TestGraphQLSimpleNamedOperationLexicalScopeAndBoundaries(t *testing.T) {
	const source = "query First {\n user { id }\n}\n" +
		"query Second { text(arg: \"brace } string\") }\n" +
		"# query Commented { bogus }\n" +
		"\"\"\"query Quoted { bogus }\"\"\"\n" +
		"query Complex($name: String!) { user(name: $name) { id } }\n" +
		"query Missing { user { id }\n"
	for _, tc := range []struct{ name, source string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("operations.graphql", tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, end := range map[string]int{"First": 3, "Second": 4} {
				s, ok := byName[name]
				if !ok || s.NativeKind != "query" || s.DeclarationRange.End.Line != end ||
					s.SignatureRange.End.Line != end {
					t.Errorf("simple named operation %s should include safe selection block: %+v", name, s)
				}
			}
			for _, name := range []string{"Commented", "Quoted"} {
				if _, ok := byName[name]; ok {
					t.Errorf("unproven text created operation: %s", name)
				}
			}
			if s, ok := byName["Complex"]; !ok || s.DeclarationRange.End.Line != 7 {
				t.Errorf("complex original must remain navigable by header: %+v", s)
			}
			if s, ok := byName["Missing"]; !ok || s.DeclarationRange.End.Line != 8 {
				t.Errorf("unterminated operation must not steal unrelated source: %+v", s)
			}
		})
	}
}
