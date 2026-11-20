package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Named GraphQL fragment definitions are reusable selections, not executable
// query/mutation operations or spread references.
func TestGraphQLOriginalJSFragmentMaskAndScope(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GRAPHQL_FRAGMENT_JS_ROOT")
	if root == "" {
		t.Skip("original GraphQL.js source not configured")
	}
	raw, err := os.ReadFile(filepath.Join(root, "files", "graphql", "kitchen-sink.graphql"))
	if err != nil {
		t.Fatal(err)
	}
	masked := maskGraphQLSource(string(raw))
	matches := graphqlFragment.FindAllStringSubmatchIndex(masked, -1)
	if len(matches) != 1 {
		header := ""
		if at := strings.Index(masked, "fragment frag"); at >= 0 {
			header = masked[at:min(len(masked), at+60)]
		}
		t.Fatalf("expected one lexical fragment header, got %d; masked header=%q", len(matches), header)
	}
	depth := 0
	for _, b := range []byte(masked[:matches[0][0]]) {
		if b == '{' {
			depth++
		} else if b == '}' {
			depth--
		}
	}
	if depth != 0 {
		t.Errorf("fragment is unexpectedly nested at lexical depth %d", depth)
	}
	open := matches[0][1] - 1
	close := matchingBraceOffset(masked, open)
	if close < 0 {
		t.Fatal("fragment with escaped triple quote has no masked closing brace")
	}
	if strings.Count(string(raw[open:close+1]), "\n") != 11 {
		t.Errorf("unexpected original fragment body span; closing byte offset %d", close)
	}
}

func TestGraphQLBlockStringsWithEscapedTripleQuotes(t *testing.T) {
	const source = "fragment actual on User {\n" +
		"  value(arg: {block: \"\"\"\n" +
		"    escaped delimiter \\\"\"\" with literal } and { inside\n" +
		"    \"\"\"})\n" +
		"}\n" +
		"query After { id }\n"
	for _, eol := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			content := strings.ReplaceAll(source, "\n", eol.value)
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(content), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			syms := symbolsByQualifiedName(result.Analysis.Symbols)
			if got, ok := syms["actual"]; !ok || got.Kind != SymbolKindSection ||
				got.NativeKind != "fragment" || got.DeclarationRange.End.Line != 5 {
				t.Errorf("escaped triple-quoted GraphQL fragment missing or truncated: %+v", got)
			}
			if got, ok := syms["After"]; !ok || got.Kind != SymbolKindOperation {
				t.Errorf("neighboring query incorrectly suppressed: %+v", got)
			}
			if !result.Analysis.CoverageComplete {
				t.Error("closed fragment with escaped triple quote marked incomplete")
			}
		})
	}
}

func TestGraphQLNamedFragmentDefinitions(t *testing.T) {
	const source = "# fragment commented on Phantom { id }\n" +
		"\"\"\"fragment described on Phantom { id }\"\"\"\n" +
		"query Fetch {\n" +
		"  user {\n" +
		"    ...real\n" +
		"    fragment nested on Phantom { id }\n" +
		"  }\n" +
		"}\n" +
		"fragment real on Friend @onFragmentDefinition {\n" +
		"  foo(arg: {nested: {id: 1}})\n" +
		"}\n" +
		"fragment second on User {\n" +
		"  id\n" +
		"}\n" +
		"fragment missing on Broken {\n"
	for _, lineEndings := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(lineEndings.name, func(t *testing.T) {
			result, err := (GraphQLAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(lineEndings.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name          string
				line, endLine int
			}{
				{"real", 9, 11},
				{"second", 12, 14},
			} {
				s, ok := byName[want.name]
				if !ok || s.Kind != SymbolKindSection || s.NativeKind != "fragment" ||
					s.NameRange.Start.Line != want.line ||
					s.DeclarationRange.End.Line != want.endLine ||
					s.ParentQualifiedName != "" {
					t.Errorf("named fragment %s not navigable: %+v", want.name, s)
				}
			}
			for _, absent := range []string{"commented", "described", "nested", "missing", "Phantom"} {
				if s, ok := byName[absent]; ok {
					t.Errorf("invalid or non-declaration fragment %s invented: %+v", absent, s)
				}
			}
			if s, ok := byName["Fetch"]; !ok || s.Kind != SymbolKindOperation || s.NativeKind != "query" {
				t.Errorf("neighboring real query lost: %+v", s)
			}
			if result.Analysis.CoverageComplete {
				t.Error("unclosed fragment body should report incomplete coverage")
			}
		})
	}
}
