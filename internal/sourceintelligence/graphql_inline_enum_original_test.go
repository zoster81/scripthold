package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type graphqlInlineEnumWant struct {
	enum, member string
	line         int
}

// These literal GraphQL SDL excerpts are verbatim from independent full
// upstream graphql-js and graphql-java parser/schema test files.
func TestGraphQLInlineEnumOriginalSources(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		wants        []graphqlInlineEnumWant
	}{
		{
			name: "LF",
			source: "enum Hello { WO, RLD }\n" +
				"enum Purpose { SECURITY EXECUTION }\n" +
				"# enum Phantom { UNREAL }\n" +
				"type Query { title: String }\n",
			wants: []graphqlInlineEnumWant{
				{"Hello", "WO", 1}, {"Hello", "RLD", 1},
				{"Purpose", "SECURITY", 2}, {"Purpose", "EXECUTION", 2},
			},
		},
		{
			name: "CRLF",
			source: strings.ReplaceAll("enum Hello { WO, RLD }\n"+
				"enum Purpose { SECURITY EXECUTION }\n"+
				"# enum Phantom { UNREAL }\n"+
				"type Query { title: String }\n", "\n", "\r\n"),
			wants: []graphqlInlineEnumWant{
				{"Hello", "WO", 1}, {"Hello", "RLD", 1},
				{"Purpose", "SECURITY", 2}, {"Purpose", "EXECUTION", 2},
			},
		},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			assertGraphQLInlineEnums(t, tc.source, tc.wants)
		})
	}
	t.Run("unsupported-inline-fails-closed", func(t *testing.T) {
		for _, body := range []string{
			"READY @deprecated(reason: \"not a name\") DONE",
			"ACTIVE: String",
			"READY / DONE",
			"true false null",
			"READY (HIDDEN) DONE",
			strings.Repeat("LONG ", 900),
			strings.Repeat("X ", 129),
		} {
			source := "enum Unsupported { " + body + " }\nenum Valid { YES NO }\n"
			res, err := (GraphQLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source),
				testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(res.Analysis.Symbols)
			for _, name := range []string{"Valid.YES", "Valid.NO"} {
				if _, ok := byName[name]; !ok {
					t.Errorf("sibling valid inline enum member lost: %s", name)
				}
			}
			for name := range byName {
				if strings.HasPrefix(name, "Unsupported.") {
					t.Errorf("unsupported inline GraphQL invented member %q from %q", name, body)
				}
			}
			diagnosed := false
			for _, d := range res.Analysis.Diagnostics {
				if d.Code == "graphql-unsupported-inline-enum" && d.Range != nil {
					diagnosed = true
				}
			}
			if res.Analysis.CoverageComplete || !diagnosed {
				t.Errorf("unsupported inline GraphQL not source-ranged incomplete: %+v", res.Analysis)
			}
		}
	})
	t.Run("masked-description-and-comment", func(t *testing.T) {
		const source = "enum Choice { ALPHA \"a description not a name\" BETA } # FALSE_NAME\n"
		res, err := (GraphQLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source),
			testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		byName := symbolsByQualifiedName(res.Analysis.Symbols)
		for _, want := range []string{"Choice.ALPHA", "Choice.BETA"} {
			if _, ok := byName[want]; !ok {
				t.Errorf("masked inline enum member %s omitted", want)
			}
		}
		for _, forbidden := range []string{"Choice.description", "Choice.FALSE_NAME"} {
			if _, ok := byName[forbidden]; ok {
				t.Errorf("masked inline GraphQL invented member %s", forbidden)
			}
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		wants             []graphqlInlineEnumWant
	}{
		{
			name:   "graphql-js",
			env:    "SCRIPTHOLD_REAL_GRAPHQL_INLINE_JS",
			marker: "enum Hello { WO, RLD }",
			wants: []graphqlInlineEnumWant{
				{"Hello", "WO", 1}, {"Hello", "RLD", 1},
			},
		},
		{
			name:   "graphql-java",
			env:    "SCRIPTHOLD_REAL_GRAPHQL_INLINE_JAVA",
			marker: "enum Purpose { SECURITY EXECUTION }",
			wants: []graphqlInlineEnumWant{
				{"Purpose", "SECURITY", 4}, {"Purpose", "EXECUTION", 4},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original GraphQL SDL snippet not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 20 || info.Size() > 1500 {
				t.Fatalf("original SDL unavailable, non-regular or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("verbatim original GraphQL SDL marker drift")
			}
			assertGraphQLInlineEnums(t, source, tc.wants)
		})
	}
}

func assertGraphQLInlineEnums(t *testing.T, source string, wants []graphqlInlineEnumWant) {
	t.Helper()
	res, err := (GraphQLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source),
		testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(res.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, w := range wants {
		if w.line > len(lines) {
			t.Fatalf("invalid original GraphQL enum line %+v", w)
		}
		col := strings.Index(lines[w.line-1], w.member) + 1
		if col < 1 {
			t.Fatalf("original SDL enum member missing from line: %+v", w)
		}
		name := w.enum + "." + w.member
		symbol, ok := byName[name]
		if !ok || symbol.Kind != SymbolKindConstant ||
			symbol.NativeKind != "enum-value" || symbol.ParentQualifiedName != w.enum ||
			symbol.NameRange.Start.Line != w.line || symbol.NameRange.Start.Column != col ||
			symbol.DeclarationRange.Start.Line != w.line {
			t.Errorf("original inline GraphQL enum %s at %d:%d missing/misclassified: %+v", name, w.line, col, symbol)
		}
	}
	for _, forbidden := range []string{"Phantom.UNREAL", "Hello.HELLO", "Purpose.Query"} {
		if _, ok := byName[forbidden]; ok {
			t.Errorf("invented GraphQL enum member %q", forbidden)
		}
	}
}
