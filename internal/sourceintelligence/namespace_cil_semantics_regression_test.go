package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCPPUsingNamespaceIsNotAVariable(t *testing.T) {
	source := `using namespace System;
using namespace System::Reflection;
using facebook::react;
using Alias = int;
namespace Demo { class Service {}; }
int count;
`
	result, err := (CPPAnalyzer{}).Analyze(context.Background(), testSourceDocument("declarations.cpp", source), testAnalyzeOptions(true, 96))
	if err != nil {
		t.Fatal(err)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"System", "Reflection", "react", "System::Reflection"} {
		if sym, ok := got[name]; ok {
			t.Errorf("namespace import %q is a false declaration: %+v", name, sym)
		}
	}
	for name, kind := range map[string]SymbolKind{"Alias": SymbolKindAlias, "Demo.Service": SymbolKindClass, "count": SymbolKindVariable} {
		if sym, ok := got[name]; !ok || sym.Kind != kind {
			t.Errorf("legitimate C++ declaration %q got %+v, present=%v, want %s", name, sym, ok, kind)
		}
	}
}

func TestCPPDirectiveBeforeUsingDoesNotLeakVariable(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		newline  string
	}{
		{"cpp-lf", CPPAnalyzer{}, "\n"},
		{"cpp-cli-lf", CPPCLIAnalyzer{}, "\n"},
		{"cpp-cli-crlf", CPPCLIAnalyzer{}, "\r\n"},
		{"cpp-cli-blank", CPPCLIAnalyzer{}, "\n"},
		{"cpp-cli-bom", CPPCLIAnalyzer{}, "\n"},
		{"cpp-bom", CPPAnalyzer{}, "\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := strings.Join([]string{"#include \"stdafx.h\"", "using namespace System;", "using namespace System::Reflection;", "class Valid {};", ""}, tc.newline)
			if tc.name == "cpp-cli-blank" {
				source = strings.Replace(source, tc.newline+"using namespace System;", tc.newline+tc.newline+"using namespace System;", 1)
			}
			if strings.HasSuffix(tc.name, "-bom") {
				source = "\ufeff" + source
			}
			result, err := tc.analyzer.Analyze(context.Background(), testSourceDocument("prelude.cpp", source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Kind == SymbolKindVariable && (symbol.Name == "System" || symbol.Name == "Reflection") {
					t.Errorf("preprocessor directive leaked into a fake namespace variable: %+v", symbol)
				}
			}
			if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Valid"]; !ok || symbol.Kind != SymbolKindClass {
				t.Errorf("valid class after directives missing: %+v exists=%v", symbol, ok)
			}
		})
	}
}

func TestCILQuotedAssemblyHasActualName(t *testing.T) {
	source := ".assembly extern mscorlib\n{\n}\n.assembly 'ConvTest'\n{\n}\n.module ConvTest.exe\n.class public Program\n{\n}\n"
	result, err := (CILAnalyzer{}).Analyze(context.Background(), testSourceDocument("ConvTest.il", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	assembly, exists := got["ConvTest"]
	if !exists || assembly.Kind != SymbolKindModule || assembly.DeclarationRange.Start.Line != 4 {
		t.Errorf("CIL assembly name not parsed: got %+v, exists=%v", assembly, exists)
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Kind == SymbolKindModule && strings.ContainsAny(sym.Name, "'\"") {
			t.Errorf("punctuation emitted as CIL assembly module: %+v", sym)
		}
	}
}

func TestCILQuotedAssemblyMalformedFailsClosed(t *testing.T) {
	source := ".assembly 'MissingClose\n{\n}\n"
	result, err := (CILAnalyzer{}).Analyze(context.Background(), testSourceDocument("broken.il", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatal("unclosed quoted CIL assembly incorrectly reports complete coverage")
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Kind == SymbolKindModule {
			t.Errorf("malformed quoted assembly emitted module: %+v", sym)
		}
	}
}

func TestCILQuotedExternalAssemblyDependency(t *testing.T) {
	source := ".assembly extern 'Core.Library'\n{\n}\n.assembly 'Main Demo'\n{\n}\n"
	result, err := (CILAnalyzer{}).Analyze(context.Background(), testSourceDocument("quoted.il", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "Core.Library" {
		t.Fatalf("quoted external assembly was not imported correctly: %+v", result.Dependencies)
	}
	if sym, ok := symbolsByQualifiedName(result.Analysis.Symbols)["Main Demo"]; !ok || sym.Kind != SymbolKindModule {
		t.Errorf("quoted assembly with spaces not recognized, exists=%v: %+v", ok, sym)
	}
}

func TestCPPCLINamespaceDirectiveRealCorpusOracles(t *testing.T) {
	for _, env := range []string{"SCRIPTHOLD_CPPCLI_ASSEMBLY_ORACLE_A", "SCRIPTHOLD_CPPCLI_ASSEMBLY_ORACLE_B"} {
		t.Run(env, func(t *testing.T) {
			path := os.Getenv(env)
			if path == "" {
				t.Skip("set corpus path for independent oracle")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(data)), testAnalyzeOptions(false, 256))
			if err != nil {
				t.Fatal(err)
			}
			for _, sym := range result.Analysis.Symbols {
				if sym.Kind == SymbolKindVariable {
					for _, name := range []string{"System", "Reflection", "CompilerServices", "InteropServices", "Permissions"} {
						if sym.Name == name {
							t.Errorf("namespace directive falsely classified as %s variable: %+v", name, sym)
						}
					}
				}
			}
		})
	}
}

func TestObjectiveCPPNamespaceDirectiveRealCorpusOracle(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_OBJCPP_NAMESPACE_ORACLE")
	if path == "" {
		t.Skip("set corpus path for independent oracle")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (ObjectiveCPPAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(data)), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	foundImplementation := false
	for _, sym := range result.Analysis.Symbols {
		if sym.Kind == SymbolKindVariable && sym.Name == "react" {
			t.Errorf("Objective-C++ imported namespace became variable: %+v", sym)
		}
		if sym.Kind == SymbolKindImplementation && sym.Name == "RCTAppDelegate" {
			foundImplementation = true
		}
	}
	if !foundImplementation {
		t.Fatal("legitimate Objective-C implementation missing")
	}
}

func TestCILQuotedAssemblyRealCorpusOracle(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_CIL_QUOTED_ASSEMBLY_ORACLE")
	if path == "" {
		t.Skip("set corpus path for independent oracle")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (CILAnalyzer{}).Analyze(context.Background(), testSourceDocument(path, string(data)), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	if actual, exists := symbols["ConvTest"]; !exists || actual.Kind != SymbolKindModule || actual.DeclarationRange.Start.Line != 7 {
		t.Errorf("real CIL quoted assembly not recognized, present=%v: %+v", exists, actual)
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Kind == SymbolKindModule && sym.Name == "'" {
			t.Errorf("real CIL assembly parsed as apostrophe: %+v", sym)
		}
	}
}
