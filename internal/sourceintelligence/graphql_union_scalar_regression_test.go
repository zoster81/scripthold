package sourceintelligence

import (
	"context"
	"os"
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
