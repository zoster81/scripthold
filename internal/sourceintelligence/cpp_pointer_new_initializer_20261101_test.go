package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCPPPointerDirectNewInitializerIsVariable(t *testing.T) {
	source := "namespace FB { namespace AssociationManager {\nstatic std::mutex *hookMutex(new std::mutex);\nvoid realFunc(int n);\n} }\n"
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"cpp", CPPAnalyzer{}},
		{"objective-cpp", ObjectiveCPPAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, eol := range []string{"\n", "\r\n"} {
				result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol)), testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete {
					t.Fatalf("unexpected incomplete analysis: %+v", result.Analysis.Diagnostics)
				}
				foundVariable, foundFunction := false, false
				for _, sym := range result.Analysis.Symbols {
					if sym.Name == "hookMutex" {
						if sym.Kind != SymbolKindVariable || sym.NameRange.Start.Line != 2 {
							t.Errorf("pointer new initializer is a variable, got %+v", sym)
						}
						foundVariable = true
					}
					if sym.Name == "realFunc" && sym.Kind == SymbolKindFunction && sym.NameRange.Start.Line == 3 {
						foundFunction = true
					}
				}
				if !foundVariable || !foundFunction {
					t.Errorf("missing initialized variable or actual function, symbols=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestObjectiveCPPOriginalNewInitializer(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_OBJC_CPP_ASSOCIATIONS_20261101")
	if path == "" {
		t.Skip("qualified original Objective-C++ source not configured")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(string(data)), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, sym := range result.Analysis.Symbols {
		if sym.Name == "hookMutex" {
			if sym.Kind != SymbolKindVariable || sym.NameRange.Start.Line != 34 || sym.ParentQualifiedName != "FB.AssociationManager" {
				t.Errorf("wrong original new initializer kind/position/owner: %+v", sym)
			}
			found = true
		}
	}
	if !found {
		t.Errorf("missing qualified original pointer initializer: symbols=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}
