package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Full immutable upstream originals are opt-in private test fixtures, not Git inputs.
func TestNimOriginalVariantBranchesAndSymbolicProcDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_NIM_OUTLINE_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_NIM_OUTLINE_ROOT for two complete pinned Nim originals")
	}
	type expected struct {
		name, native, owner string
		kind                SymbolKind
		line, column        int
	}
	for _, tc := range []struct {
		file string
		want []expected
	}{
		{"nim-new-dmodule.nim", []expected{
			{"MinKind", "enum", "", SymbolKindEnum, 2, 3},
			{"MinValue", "object", "", SymbolKindClass, 5, 3},
			{"symbols", "field", "MinValue", SymbolKindField, 8, 7},
			{"MinOperator", "object", "", SymbolKindClass, 10, 3},
			{"parseMinValue", "proc", "", SymbolKindFunction, 21, 6},
		}},
		{"ws-echo.nim", []expected{
			{"cb", "proc", "", SymbolKindFunction, 3, 6},
			{"ws", "var", "cb", SymbolKindVariable, 6, 11},
			{"packet", "let", "cb", SymbolKindVariable, 11, 13},
			{"server", "var", "", SymbolKindVariable, 17, 5},
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil || len(data) == 0 {
				t.Fatalf("configured full original missing or empty: %v", err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = tc.file
			result, err := (NimAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			got := result.Analysis.Symbols
			if len(got) != len(tc.want) {
				t.Errorf("authored supported declarations=%d, actual=%d: %v",
					len(tc.want), len(got), sortedSymbolQualifiedNames(got))
			}
			for _, w := range tc.want {
				matches := 0
				for _, sym := range got {
					if sym.Name != w.name || sym.NameRange.Start.Line != w.line {
						continue
					}
					matches++
					qualified := w.name
					if w.owner != "" {
						qualified = w.owner + "." + w.name
					}
					if sym.NativeKind != w.native || sym.Kind != w.kind ||
						sym.NameRange.Start.Column != w.column ||
						sym.ParentQualifiedName != w.owner || sym.QualifiedName != qualified ||
						(sym.ParentID == "") != (w.owner == "") {
						t.Errorf("incorrect original authored identity %s at line %d: %+v", w.name, w.line, sym)
					}
				}
				if matches != 1 {
					t.Errorf("missing/duplicate original %s line %d (%d matches)",
						w.name, w.line, matches)
				}
			}
			for _, symbol := range got {
				if symbol.Name == "else" || symbol.Name == "a" {
					t.Errorf("phantom from variant branch or operator parameter: %+v", symbol)
				}
			}
			if result.Analysis.Truncated {
				t.Error("unexpected truncation of tiny original source")
			}
		})
	}
}

func TestNimSymbolicCallableAndVariantKeywordBoundaries(t *testing.T) {
	src := "type\n  Choice = object\n    case kind: int\n    of 1:\n      real: int\n    else: discard\n" +
		"proc `$`*(a: Choice): string = discard\n" +
		"proc normal*(x: Choice): string = discard\n"
	for _, tc := range []struct{ name, text string }{
		{"lf", src},
		{"crlf", strings.ReplaceAll(src, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (NimAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			for _, sym := range got.Analysis.Symbols {
				if sym.Name == "else" || sym.Name == "a" || sym.Name == "of" || sym.Name == "case" {
					t.Errorf("Nim branch keyword/parameter misclassified: %+v", sym)
				}
			}
			found := map[string]bool{}
			for _, sym := range got.Analysis.Symbols {
				found[sym.QualifiedName] = true
			}
			for _, name := range []string{"Choice", "Choice.real", "normal"} {
				if !found[name] {
					t.Errorf("missing authored source %s, actual %v", name, sortedSymbolQualifiedNames(got.Analysis.Symbols))
				}
			}
			if len(got.Analysis.Symbols) != 3 {
				t.Errorf("invented/omitted Nim identities: %+v", got.Analysis.Symbols)
			}
		})
	}
}
