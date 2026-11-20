package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestFSharpQualifiedNamespaceAndModifiers(t *testing.T) {
	source := "namespace Demo.Core\ntype internal Worker() =\n    member val internal Counter = 0 with get, set\n    member private _.Run() = 42\n"
	for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			result, err := (FSharpAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.value)), testAnalyzeOptions(true, 100))
			if err != nil {
				t.Fatal(err)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name string
				kind SymbolKind
				line int
			}{
				{"Demo.Core", SymbolKindNamespace, 1},
				{"Demo.Core.Worker", SymbolKindType, 2},
				{"Demo.Core.Worker.Counter", SymbolKindMethod, 3},
				{"Demo.Core.Worker.Run", SymbolKindMethod, 4},
			} {
				symbol, exists := got[want.name]
				if !exists || symbol.Kind != want.kind || symbol.NameRange.Start.Line != want.line {
					t.Fatalf("missing %s at line %d: %+v exists=%v, all=%v", want.name, want.line, symbol, exists, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, forbidden := range []string{"Demo", "Demo.Core.internal", "Demo.Core.Worker.internal", "Demo.Core.Worker.private"} {
				if _, exists := got[forbidden]; exists {
					t.Errorf("modifier or partial namespace leaked as %s", forbidden)
				}
			}
		})
	}
}

func TestFSharpAdditionalOriginalProjectOwnership(t *testing.T) {
	cases := []struct {
		env  string
		want []struct {
			name string
			kind SymbolKind
			line int
		}
		forbidden []string
	}{
		{"SCRIPTHOLD_REAL_FS_AVALONIA", []struct {
			name string
			kind SymbolKind
			line int
		}{
			{"Avalonia.FuncUI", SymbolKindNamespace, 1},
			{"Avalonia.FuncUI.Component", SymbolKindType, 13},
			{"Avalonia.FuncUI.Component.create", SymbolKindMethod, 21},
		}, []string{"Avalonia", "Avalonia.Component"}},
		{"SCRIPTHOLD_REAL_FS_FSCHECK", []struct {
			name string
			kind SymbolKind
			line int
		}{
			{"FsCheck", SymbolKindNamespace, 1},
			{"FsCheck.IArbMap", SymbolKindType, 10},
			{"FsCheck.ArbMap", SymbolKindType, 20},
			{"FsCheck.ArbMap.ArbFinder", SymbolKindMethod, 35},
		}, []string{"FsCheck.internal", "FsCheck.internal.ArbFinder"}},
		{"SCRIPTHOLD_REAL_FS_BOLERO", []struct {
			name string
			kind SymbolKind
			line int
		}{
			{"Bolero", SymbolKindNamespace, 21},
			{"Bolero.Program", SymbolKindType, 118},
			{"Bolero.Program.InitRouter", SymbolKindMethod, 291},
			{"Bolero.ElmishComponent.OldModel", SymbolKindMethod, 78},
		}, []string{"Bolero.Program.internal", "Bolero.Program.private", "Bolero.ElmishComponent.internal"}},
	}
	for _, tc := range cases {
		t.Run(tc.env, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("qualified original not configured")
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{MaxFileBytes: 1 << 20, MaxDecodedCharacters: 1 << 20})
			if err != nil {
				t.Fatal(err)
			}
			result, err := (FSharpAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 20000))
			if err != nil {
				t.Fatal(err)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range tc.want {
				found := false
				for _, symbol := range result.Analysis.Symbols {
					if symbol.QualifiedName == want.name && symbol.Kind == want.kind && symbol.NameRange.Start.Line == want.line {
						found = true
					}
				}
				if !found {
					t.Errorf("missing %s kind=%s line=%d, symbols=%v", want.name, want.kind, want.line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, forbidden := range tc.forbidden {
				if _, exists := got[forbidden]; exists {
					t.Errorf("phantom %s", forbidden)
				}
			}
		})
	}
}
