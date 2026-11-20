package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDOriginalDeclaredTypesAndFalseSymbols(t *testing.T) {
	cases := []struct {
		env    string
		name   string
		line   int
		kind   SymbolKind
		absent []string
	}{
		{"SCRIPTHOLD_REAL_D_SIDELOADER", "AppIdCommand", 21, SymbolKindStruct, nil},
		{"SCRIPTHOLD_REAL_D_CREATOR", "ParameterBindingAddRemoveAction", 39, SymbolKindClass, nil},
		{"SCRIPTHOLD_REAL_D_PUPPET", "Puppet2D", 20, SymbolKindClass, nil},
		{"SCRIPTHOLD_REAL_D_VECTORFLOW", "DataFileReader", 29, SymbolKindClass, nil},
		{"SCRIPTHOLD_REAL_D_EBAY", "tsv_utils.common.fieldlist", 182, SymbolKindModule, []string{"tsv_utils.common.fieldlist.enforce", "tsv_utils.common.fieldlist.format", "tsv_utils.common.fieldlist.stdio"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("qualified original fixture not configured")
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Size() > 1<<20 {
				t.Fatal("original exceeds 1MiB test bound")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = path
			result, err := (DAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 20000))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == tc.name && sym.Kind == tc.kind && sym.NameRange.Start.Line == tc.line {
					found = true
				}
				for _, name := range tc.absent {
					if sym.QualifiedName == name {
						t.Errorf("import incorrectly projected as declaration: %s", name)
					}
				}
			}
			if !found {
				t.Fatalf("%s type %s at line %d not found; symbols=%v", tc.kind, tc.name, tc.line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestDAnnotatedAndTemplatedTypesDoNotBecomeCallables(t *testing.T) {
	const source = `module demo;
import std.exception : enforce;
@(Command("run").Description("docs"))
struct CommandHandler { int opCall() { return 1; } }
class TemplateType(T) : Base { T item; }
@class_name("Puppet")
class Puppet2D : Base {}
mixin GodotClass!Puppet2D;
private {
 class Hidden(T) { T field; }
}
void sibling() {}
`
	for _, tc := range []struct{ name, text string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (DAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name string
				kind SymbolKind
				line int
			}{
				{"demo.CommandHandler", SymbolKindStruct, 4},
				{"demo.TemplateType", SymbolKindClass, 5},
				{"demo.Puppet2D", SymbolKindClass, 7},
				{"demo.Hidden", SymbolKindClass, 10},
				{"demo.sibling", SymbolKindFunction, 12},
			} {
				sym, ok := byName[want.name]
				if !ok || sym.Kind != want.kind || sym.NameRange.Start.Line != want.line {
					t.Errorf("%s: got=%+v present=%v, want kind=%s line=%d; names=%v", want.name, sym, ok, want.kind, want.line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, name := range []string{"demo.enforce", "demo.GodotClass", "demo.Puppet2D.Puppet2D"} {
				if sym, ok := byName[name]; ok {
					t.Errorf("false symbol %s: %+v", name, sym)
				}
			}
		})
	}
}
