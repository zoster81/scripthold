package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An intact upstream Ctags translation unit defines one tagged struct and a
// distinct typedef alias; neither may become a global object declaration.
func TestCPinnedWholeOriginalTaggedTypedefAlias(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_C_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned C original corpus not configured")
	}
	path := filepath.Join(root, "c-ctags-keyword-8495a124.c")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 6776 {
		t.Fatalf("upstream original changed: bytes=%d", len(data))
	}
	for _, tc := range []struct{ name, text string }{
		{"git-lf", string(data)}, {"worktree-crlf", strings.ReplaceAll(string(data), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := (CAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("keyword.c", tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !out.Analysis.CoverageComplete || len(out.Analysis.Symbols) != 24 {
				t.Fatalf("original 24 source declarations, got %d complete=%v", len(out.Analysis.Symbols), out.Analysis.CoverageComplete)
			}
			symbols := symbolsByQualifiedName(out.Analysis.Symbols)
			// Every supported named declaration was inventoried from the entire
			// immutable upstream original before comparing analyzer output.
			want := []struct {
				name   string
				line   int
				kind   SymbolKind
				native string
			}{
				{"sHashEntry", 27, SymbolKindStruct, "struct"},
				{"sHashEntry.next", 28, SymbolKindField, "field"},
				{"sHashEntry.string", 29, SymbolKindField, "field"},
				{"sHashEntry.language", 30, SymbolKindField, "field"},
				{"sHashEntry.value", 31, SymbolKindField, "field"},
				{"hashEntry", 32, SymbolKindAlias, "typedef"},
				{"TableSize", 37, SymbolKindConstant, "constant"},
				{"HashTable", 38, SymbolKindVariable, "variable"},
				{"MaxEntryLen", 39, SymbolKindVariable, "variable"},
				{"getHashTable", 45, SymbolKindFunction, "function-definition"},
				{"getHashTableEntry", 63, SymbolKindFunction, "function-definition"},
				{"hashValue", 74, SymbolKindFunction, "function-definition"},
				{"newEntry", 101, SymbolKindFunction, "function-definition"},
				{"addKeyword", 119, SymbolKindFunction, "function-definition"},
				{"lookupKeywordFull", 156, SymbolKindFunction, "function-definition"},
				{"lookupKeyword", 182, SymbolKindFunction, "function-definition"},
				{"lookupCaseKeyword", 187, SymbolKindFunction, "function-definition"},
				{"freeKeywordTable", 192, SymbolKindFunction, "function-definition"},
				{"printEntry", 215, SymbolKindFunction, "function-definition"},
				{"printBucket", 220, SymbolKindFunction, "function-definition"},
				{"printKeywordTable", 246, SymbolKindFunction, "function-definition"},
				{"dumpKeywordTable", 267, SymbolKindFunction, "function-definition"},
				{"addKeywordGroup", 282, SymbolKindFunction, "function-definition"},
				{"addDialectalKeywords", 300, SymbolKindFunction, "function-definition"},
			}
			if len(symbols) != len(want) {
				t.Errorf("expected 24 distinct original source identities, got %d", len(symbols))
			}
			for _, w := range want {
				s, ok := symbols[w.name]
				if !ok || s.NameRange.Start.Line != w.line || s.Kind != w.kind || s.NativeKind != w.native {
					t.Errorf("source %s line%d expected kind=%s native=%s: got %+v exists=%v", w.name, w.line, w.kind, w.native, s, ok)
				}
			}
			alias, ok := symbols["hashEntry"]
			if !ok {
				t.Fatal("original typedef alias missing")
			}
			if alias.Kind != SymbolKindAlias || alias.NativeKind != "typedef" ||
				alias.DeclarationRange.Start.Line != 27 || alias.DeclarationRange.Start.Column != 1 ||
				alias.NameRange.Start.Line != 32 || alias.NameRange.Start.Column != 3 ||
				alias.DeclarationRange.End.Line != 32 || alias.DeclarationRange.End.Column != 13 {
				t.Errorf("tagged typedef must retain whole authored typedef range and alias kind: %+v", alias)
			}
			tag, ok := symbols["sHashEntry"]
			if !ok || tag.Kind != SymbolKindStruct || tag.NameRange.Start.Line != 27 ||
				tag.ParentQualifiedName != "" {
				t.Errorf("tagged struct missing/different: %+v exists=%v", tag, ok)
			}
			for _, name := range []string{"next", "string", "language", "value"} {
				field, ok := symbols["sHashEntry."+name]
				if !ok || field.Kind != SymbolKindField || field.ParentQualifiedName != "sHashEntry" {
					t.Errorf("tag field %s missing/wrong: %+v exists=%v", name, field, ok)
				}
			}
			for _, name := range []string{"TableSize", "HashTable", "MaxEntryLen", "getHashTable"} {
				if _, ok := symbols[name]; !ok {
					t.Errorf("following authentic declaration missing: %s", name)
				}
			}
		})
	}
}

func TestCFamilyTaggedAggregateTypedefAndOrdinaryTrailingObject(t *testing.T) {
	for _, lang := range []struct {
		name string
		a    SourceAnalyzer
	}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
		for _, input := range []struct{ name, src string }{
			{"tagged-alias", "typedef struct Entry { int n; } EntryAlias;\nint after(void);\n"},
			{"tagged-object", "struct Entry { int n; } globalEntry;\nint after(void);\n"},
			{"ordinary-typedef", "typedef int Integer;\nint after(void);\n"},
		} {
			t.Run(lang.name+"/"+input.name, func(t *testing.T) {
				out, err := lang.a.Analyze(context.Background(),
					testSourceDocument("control.c", input.src), testAnalyzeOptions(true, 48))
				if err != nil {
					t.Fatal(err)
				}
				byName := symbolsByQualifiedName(out.Analysis.Symbols)
				if _, ok := byName["after"]; !ok {
					t.Errorf("following declaration missing: %+v", out.Analysis.Symbols)
				}
				if input.name == "tagged-alias" {
					alias, ok := byName["EntryAlias"]
					if !ok || alias.Kind != SymbolKindAlias || alias.NativeKind != "typedef" ||
						alias.DeclarationRange.Start.Line != 1 || alias.DeclarationRange.Start.Column != 1 {
						t.Errorf("tagged alias wrong: %+v present=%v", alias, ok)
					}
				}
				if input.name == "tagged-object" {
					if _, ok := byName["EntryAlias"]; ok {
						t.Error("invented alias for ordinary object")
					}
					if object, ok := byName["globalEntry"]; ok && object.Kind == SymbolKindAlias {
						t.Errorf("ordinary object misclassified as alias: %+v", object)
					}
				}
			})
		}
	}
}
