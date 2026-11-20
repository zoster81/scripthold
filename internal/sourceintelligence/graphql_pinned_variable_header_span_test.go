package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A full independent GraphQL-Go upstream source contains named operations with
// authored variable headers. Source excerpts must not end at only the name.
func TestGraphQLPinnedWholeVariableHeaderSourceSpans(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_GO_AUDIT_ROOT")
	if root == "" {
		t.Skip("pinned GraphQL-Go source not configured")
	}
	raw, err := os.ReadFile(filepath.Join(root, "kitchen-sink.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 823 {
		t.Fatalf("pinned original changed: %d bytes", len(raw))
	}
	for _, tc := range []struct{ name, source string }{
		{"LF", string(raw)}, {"CRLF", strings.ReplaceAll(string(raw), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("kitchen-sink.graphql", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			expected := map[string]struct {
				start, end int
				native     string
			}{
				"namedQuery":          {3, 22, "query"},
				"favPost":             {24, 30, "mutation"},
				"PostFavSubscription": {32, 43, "subscription"},
				"frag":                {45, 47, "fragment"},
			}
			actual := make(map[string]bool)
			for _, symbol := range result.Analysis.Symbols {
				w, ok := expected[symbol.Name]
				if !ok {
					t.Errorf("invented or unrequested source symbol: %+v", symbol)
					continue
				}
				if actual[symbol.Name] {
					t.Errorf("duplicate source name: %+v", symbol)
				}
				actual[symbol.Name] = true
				if symbol.NativeKind != w.native ||
					symbol.NameRange.Start.Line != w.start ||
					symbol.DeclarationRange.Start.Line != w.start ||
					symbol.DeclarationRange.End.Line != w.end ||
					symbol.SignatureRange.End.Line != w.end {
					t.Errorf("complete original mismatch for %s: %+v, want end %d", symbol.Name, symbol, w.end)
				}
			}
			if len(actual) != len(expected) || result.Analysis.Truncated {
				t.Fatalf("whole original source census mismatch: %v %+v", actual, result.Analysis)
			}
		})
	}
}

func TestGraphQLVariableHeaderBoundedSyntaxAndNegativeControls(t *testing.T) {
	cases := []struct {
		name, source string
		endLine      int
	}{
		{"list-type-numeric-default", "query Target($ids: [ID!]!, $n: Int = -1) {\n  user { id }\n}\n", 3},
		{"nested-object-default", "query Target($x: Input = {key: 1}) {\n  id\n}\n", 1},
		{"list-default", "query Target($x: [ID] = [1,2]) {\n  id\n}\n", 1},
		{"quoted-string-default", "query Target($x: String = \"brace } value\") {\n  id\n}\n", 1},
		{"directive-arguments", "query Target($x: ID) @skip(if: true) {\n  id\n}\n", 1},
		{"malformed-definition", "query Target(notAnAuthoredVariable) {\n  id\n}\n", 1},
		{"unclosed-definition", "query Target($x: ID {\n  id\n}\n", 1},
		{"too-many-variables", "query Target(" + strings.Repeat("$x: ID,", 16) + "$y: ID) {\n  id\n}\n", 1},
		{"oversized-variable-header", "query Target($x: ID" + strings.Repeat(" ", 260) + ") {\n  id\n}\n", 1},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, sep string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				content := strings.ReplaceAll(tc.source, "\n", eol.sep)
				result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
					testSourceDocument("bounded.graphql", content), testAnalyzeOptions(true, 32))
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Analysis.Symbols) != 1 {
					t.Fatalf("false declaration from query body %+v", result.Analysis)
				}
				s := result.Analysis.Symbols[0]
				if s.Name != "Target" || s.NativeKind != "query" ||
					s.DeclarationRange.End.Line != tc.endLine || s.SignatureRange.End.Line != tc.endLine {
					t.Errorf("unsupported variable header promoted or supported header truncated: %+v; want end %d", s, tc.endLine)
				}
			})
		}
	}
}

func TestGraphQLNamedOperationVariableHeaderConservativeBounds(t *testing.T) {
	const source = "query First($x: ID!) {\n user(id: $x) { id }\n}\n" +
		"subscription Next($input: String) { changed { id } }\n" +
		"query NoBody($x: ID!) { broken\n" +
		"query Invalid(foo) { id }\n"
	for _, tc := range []struct{ name, src string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("variables.graphql", tc.src), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			for _, s := range result.Analysis.Symbols {
				if s.NativeKind != "query" && s.NativeKind != "subscription" {
					continue
				}
				switch s.Name {
				case "First":
					if s.DeclarationRange.End.Line != 3 {
						t.Errorf("safe variable query truncated %+v", s)
					}
				case "Next":
					if s.DeclarationRange.End.Line != 4 {
						t.Errorf("safe subscription truncated %+v", s)
					}
				case "NoBody":
					if s.DeclarationRange.End.Line != 5 {
						t.Errorf("unclosed query extended %+v", s)
					}
				case "Invalid":
					if s.DeclarationRange.End.Line != 6 {
						t.Errorf("invalid variable definitions extended %+v", s)
					}
				}
			}
		})
	}
}
