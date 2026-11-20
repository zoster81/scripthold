package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Only an explicit TclOO class creation body may own method and lifecycle
// declarations. Ordinary Tcl commands and substitutions are not classes.
func TestTclOOLexicalClassMembersAndNondeclarations(t *testing.T) {
	source := strings.Join([]string{
		"namespace eval widgets {",
		"  oo::class create Item {",
		"    constructor {{initial 1}} { set x $initial }",
		"    method update {x} { return $x }",
		"    method render {} { return 2 }",
		"    destructor { set x \"\" }",
		"  }",
		"  proc outside {} { return ok }",
		"}",
		"method globallyUnrelated {} { return 0 }",
		"class create Shadow { method ghost {} {} }",
		"set prose \"oo::class create Text { method fake {} {} }\"",
		"# oo::class create Commented { method fake {} {} }",
		"oo::class create {} { method malformed {} {} }",
		"namespace eval other {",
		"  oo::class create Item { method update {} { return 3 } }",
		"}",
		"OO::CLASS create UpperCase { method ghost {} {} }",
		"oo::class create CaseClass { METHOD invalid {} {} }",
		"oo::class create CallsProc { proc helper {} { return 0 } }",
		"",
	}, "\n")
	for _, tc := range []struct{ name, text string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (TclAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated {
				t.Fatalf("bounded TclOO source unexpectedly truncated: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, item := range []struct {
				name, native, parent string
				kind                 SymbolKind
				line                 int
			}{
				{"widgets", "namespace-eval", "", SymbolKindNamespace, 1},
				{"widgets.Item", "tcloo-class", "widgets", SymbolKindClass, 2},
				{"widgets.Item.constructor", "tcloo-constructor", "widgets.Item", SymbolKindConstructor, 3},
				{"widgets.Item.update", "tcloo-method", "widgets.Item", SymbolKindMethod, 4},
				{"widgets.Item.render", "tcloo-method", "widgets.Item", SymbolKindMethod, 5},
				{"widgets.Item.destructor", "tcloo-destructor", "widgets.Item", SymbolKindMethod, 6},
				{"widgets.outside", "proc", "widgets", SymbolKindFunction, 8},
				{"other.Item", "tcloo-class", "other", SymbolKindClass, 16},
				{"other.Item.update", "tcloo-method", "other.Item", SymbolKindMethod, 16},
			} {
				got, ok := symbols[item.name]
				if !ok || got.Kind != item.kind || got.NativeKind != item.native ||
					got.ParentQualifiedName != item.parent || got.NameRange.Start.Line != item.line ||
					(item.parent != "" && got.ParentID == "") {
					t.Errorf("TclOO lexical declaration missing or misowned %s: %+v", item.name, got)
				}
			}
			for _, invalid := range []string{
				"globallyUnrelated", "Shadow", "Shadow.ghost",
				"Text", "Text.fake", "Commented", "Commented.fake",
				"malformed", "other.Item.globallyUnrelated", "UpperCase", "UpperCase.ghost", "CaseClass.invalid", "CallsProc.helper",
			} {
				if got, ok := symbols[invalid]; ok {
					t.Errorf("TclOO call/comment/string/malformed class incorrectly indexed: %s %+v", invalid, got)
				}
			}
		})
	}
}
