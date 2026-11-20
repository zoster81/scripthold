package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestJScriptNetReturnedObjectMethodsDoNotInferRuntimeAliases(t *testing.T) {
	source := strings.Join([]string{
		"var Package = (function() {",
		"  // return { Comment: { Ghost: function() {} } };",
		"  var prose = \"return { String: { Ghost: function() {} } }\";",
		"  var notReturned = { Local: { Fake: function() {} } };",
		"  return {",
		"    IconLink: {",
		"      Path: '/tmp/example',",
		"      Create: function(value) { return value; },",
		"      Delete: function() { return null; },",
		"      Arrow: (x) => x,",
		"      Shorthand() { return 0; },",
		"    },",
		"    Other: {",
		"      Create: function() { return null; },",
		"    },",
		"    'Quoted': { Ghost: function() {} },",
		"  };",
		"})();",
		"",
	}, "\n")
	for _, variant := range []struct{ name, source string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(variant.name, func(t *testing.T) {
			document := sourceDocumentForScanner(variant.source)
			result, err := (JScriptNetAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated {
				t.Fatalf("bounded JScript.NET object source unexpectedly truncated: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, target := range []struct {
				name, native, parent string
				kind                 SymbolKind
				line                 int
			}{
				{"IconLink", "returned-object", "", SymbolKindNamespace, 6},
				{"IconLink.Create", "object-function", "IconLink", SymbolKindMethod, 8},
				{"IconLink.Delete", "object-function", "IconLink", SymbolKindMethod, 9},
				{"Other", "returned-object", "", SymbolKindNamespace, 13},
				{"Other.Create", "object-function", "Other", SymbolKindMethod, 14},
			} {
				got, exists := symbols[target.name]
				if !exists || got.NativeKind != target.native || got.Kind != target.kind ||
					got.ParentQualifiedName != target.parent || got.NameRange.Start.Line != target.line ||
					(target.parent != "" && got.ParentID == "") {
					t.Errorf("authored JScript.NET returned-object member missing/misowned %s: %+v", target.name, got)
				}
			}
			for _, falseName := range []string{
				"Package.IconLink.Create", "Package.IconLink.Delete",
				"Local", "Local.Fake", "Comment", "Comment.Ghost",
				"String", "String.Ghost", "IconLink.Path", "IconLink.Arrow",
				"IconLink.Shorthand", "Quoted", "Quoted.Ghost",
			} {
				if got, ok := symbols[falseName]; ok {
					t.Errorf("JScript.NET runtime alias, example or unsupported member fabricated: %s %+v", falseName, got)
				}
			}
			for _, analyzer := range []interface {
				Analyze(context.Context, *SourceDocument, AnalyzeOptions) (AnalyzerResult, error)
				Language() string
			}{JavaScriptAnalyzer{}, TypeScriptAnalyzer{}} {
				ordinary, err := analyzer.Analyze(context.Background(), document, testAnalyzeOptions(true, 96))
				if err != nil {
					t.Fatal(err)
				}
				for _, symbol := range ordinary.Analysis.Symbols {
					if symbol.NativeKind == "object-function" || symbol.NativeKind == "returned-object" {
						t.Errorf("JScript.NET-only object projected into %s: %+v", analyzer.Language(), symbol)
					}
				}
			}
		})
	}
}
