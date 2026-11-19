package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Whole independent GraphQL.js kitchen sink, inventoried before consulting
// development output: three named operations, one named fragment and two
// anonymous selection blocks that must not become declarations.
func TestGraphQLNamedDirectiveHeaderBoundariesAndNegativeControls(t *testing.T) {
	cases := []struct {
		name, source string
		end          int
	}{
		{"one-directive", "query Target @tag {\n  user { id }\n}\n", 3},
		{"multiline-vars-directives", "subscription Target(\n $input: ID! @argumentTag\n) @one @two {\n  changed { id }\n}\n", 5},
		{"directive-arguments", "query Target @skip(if: true) {\n  id\n}\n", 1},
		{"variable-directive-arguments", "query Target($id: ID @flag(if: true)) {\n  id\n}\n", 1},
		{"malformed-name", "query Target @ {\n  id\n}\n", 1},
		{"too-many-directives", "query Target @a @b @c @d @e {\n  id\n}\n", 1},
		{"nested-default", "query Target($x: Input = {a: 1}) @top {\n  id\n}\n", 1},
		{"quoted-default", "query Target($x: String = \"a } b\") @top {\n  id\n}\n", 1},
		{"unclosed-block", "query Target @good {\n  user { id }\n", 1},
		{"oversized-directive-gap", "query Target @d" + strings.Repeat(" ", 270) + "{\n id\n}\n", 1},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, sep string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				contents := strings.ReplaceAll(tc.source, "\n", eol.sep)
				result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
					testSourceDocument("directives.graphql", contents), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if len(result.Analysis.Symbols) != 1 {
					t.Fatalf("invented/lost named operation: %+v", result.Analysis)
				}
				s := result.Analysis.Symbols[0]
				if s.Name != "Target" || s.NativeKind != "query" && s.NativeKind != "subscription" ||
					s.DeclarationRange.End.Line != tc.end || s.SignatureRange.End.Line != tc.end {
					t.Errorf("unsafe or incomplete directive navigation: %+v; want end line %d", s, tc.end)
				}
			})
		}
	}
}

func TestGraphQLPinnedWholeDirectiveOperationSourceSpans(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_FRAGMENT_JS_ROOT")
	if root == "" {
		t.Skip("independent GraphQL.js original not configured")
	}
	raw, err := os.ReadFile(filepath.Join(root, "files", "graphql", "kitchen-sink.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1024 {
		t.Fatalf("pinned GraphQL.js original changed: %d bytes", len(raw))
	}
	expected := map[string]struct {
		kind       string
		start, end int
	}{
		"queryName":             {"query", 1, 20},
		"likeStory":             {"mutation", 22, 28},
		"StoryLikeSubscription": {"subscription", 30, 43},
		"frag":                  {"fragment", 45, 56},
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
			if result.Analysis.Truncated || len(result.Analysis.Symbols) != len(expected) {
				t.Fatalf("whole authored source inventory mismatch: %+v", result.Analysis)
			}
			seen := map[string]bool{}
			for _, s := range result.Analysis.Symbols {
				w, ok := expected[s.Name]
				if !ok || seen[s.Name] || s.NativeKind != w.kind ||
					s.DeclarationRange.Start.Line != w.start ||
					s.NameRange.Start.Line != w.start ||
					s.DeclarationRange.End.Line != w.end ||
					s.SignatureRange.End.Line != w.end {
					t.Errorf("whole original expected %v but got %+v", w, s)
				}
				seen[s.Name] = true
			}
			if len(seen) != len(expected) {
				t.Fatalf("missing authored declarations: %v", seen)
			}
		})
	}
}
