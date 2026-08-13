package sourceintelligence

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

var _ SourceAnalyzer = CSharpAnalyzer{}

func TestCSharpAnalyzerModernDeclarationsAndFalsePositiveResistance(t *testing.T) {
	text := `using System;
using IO = System.IO;

namespace Demo.Core {
    [Obsolete]
    public partial class Box<T> : IDisposable {
        public const int Answer = 42;
        private readonly T value;
        public event EventHandler? Changed;

        public Box(T value) { this.value = value; }
        public T Value { get; private set; }
        public T Get() => value;
        public static string Extend(this Box<string> box, int count) => box.ToString();

        public record Nested<U>(U Item);

        public void Noise() {
            var fake = "public class Fake { public void Wrong() {} }";
            // public void Commented() {}
        }
    }

    internal interface IService {
        void Run();
    }

    public struct Pair { public int X; public int Y; }
    public enum State { Ready, Done }
}
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.cs"
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{
		IncludeSignatures: true,
		Limits:            SymbolBuilderLimits{MaxSymbols: 256, MaxSignatureBytes: 4096, MaxDiagnostics: 32},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("C# analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	want := map[string]SymbolKind{
		"Demo.Core":              SymbolKindNamespace,
		"Demo.Core.Box":          SymbolKindClass,
		"Demo.Core.Box.Answer":   SymbolKindConstant,
		"Demo.Core.Box.value":    SymbolKindField,
		"Demo.Core.Box.Changed":  SymbolKindEvent,
		"Demo.Core.Box.Box":      SymbolKindConstructor,
		"Demo.Core.Box.Value":    SymbolKindProperty,
		"Demo.Core.Box.Get":      SymbolKindMethod,
		"Demo.Core.Box.Extend":   SymbolKindMethod,
		"Demo.Core.Box.Nested":   SymbolKindRecord,
		"Demo.Core.Box.Noise":    SymbolKindMethod,
		"Demo.Core.IService":     SymbolKindInterface,
		"Demo.Core.IService.Run": SymbolKindMethod,
		"Demo.Core.Pair":         SymbolKindStruct,
		"Demo.Core.Pair.X":       SymbolKindField,
		"Demo.Core.Pair.Y":       SymbolKindField,
		"Demo.Core.State":        SymbolKindEnum,
	}
	for qualified, kind := range want {
		symbol, ok := byName[qualified]
		if !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v, exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if symbol.Evidence != SymbolEvidenceStructural || symbol.Analyzer != string(AnalyzerCSharp) || symbol.Language != "csharp" {
			t.Fatalf("%s evidence/analyzer = %+v", qualified, symbol)
		}
	}
	for _, forbidden := range []string{"Demo.Core.Box.Fake", "Demo.Core.Box.Wrong", "Demo.Core.Box.Commented"} {
		if _, exists := byName[forbidden]; exists {
			t.Fatalf("false positive symbol %s", forbidden)
		}
	}
	if !containsString(byName["Demo.Core.Box"].Modifiers, "partial") {
		t.Fatalf("Box modifiers = %v", byName["Demo.Core.Box"].Modifiers)
	}
	if !containsString(byName["Demo.Core.Box.Extend"].Modifiers, "extension") {
		t.Fatalf("extension method modifiers = %v", byName["Demo.Core.Box.Extend"].Modifiers)
	}
	if byName["Demo.Core.Box.Value"].Visibility != VisibilityPublic || byName["Demo.Core.Box.value"].Visibility != VisibilityPrivate {
		t.Fatalf("visibility Value=%q value=%q", byName["Demo.Core.Box.Value"].Visibility, byName["Demo.Core.Box.value"].Visibility)
	}
	if !strings.Contains(byName["Demo.Core.Box.Get"].Signature, "public T Get()") {
		t.Fatalf("Get signature = %q", byName["Demo.Core.Box.Get"].Signature)
	}
	if len(result.Dependencies) != 2 || result.Dependencies[0].Value != "System" || result.Dependencies[1].Value != "System.IO" || result.Dependencies[1].Alias != "IO" {
		t.Fatalf("using dependencies = %+v", result.Dependencies)
	}
	second, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 256, MaxSignatureBytes: 4096, MaxDiagnostics: 32}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, second) {
		t.Fatal("C# analyzer output is nondeterministic")
	}
}

func TestCSharpAnalyzerAttributedMembers(t *testing.T) {
	text := `class Container {
    [Obsolete]
    public Container() { }

    [DebuggerStepThrough]
    [return: MaybeNull]
    public string? Attributed([NotNull] string? value) {
        Helper();
        return value;
    }

    [Obsolete]
    public int Count { get; set; }

    [Obsolete]
    public event EventHandler? Changed;

    [Obsolete]
    private int field;

    private void Helper() { }
}`
	document := sourceDocumentForScanner(text)
	document.Path = "attributes.cs"
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 64, MaxSignatureBytes: 4096, MaxDiagnostics: 16}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("attributed C# analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	want := map[string]SymbolKind{
		"Container":            SymbolKindClass,
		"Container.Container":  SymbolKindConstructor,
		"Container.Attributed": SymbolKindMethod,
		"Container.Count":      SymbolKindProperty,
		"Container.Changed":    SymbolKindEvent,
		"Container.field":      SymbolKindField,
		"Container.Helper":     SymbolKindMethod,
	}
	for qualified, kind := range want {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if len(result.Analysis.Symbols) != len(want) {
		t.Fatalf("attributed members produced extra symbols: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCSharpAnalyzerAttributedTypesDoNotParseAttributeKeywordsAsDeclarations(t *testing.T) {
	text := `[module: System.Runtime.CompilerServices.SkipLocalsInit]
namespace Demo {
    [AttributeUsage(AttributeTargets.Class | AttributeTargets.Interface)]
    public sealed class Container : Attribute {
        public Container() { }
        public Container(string id) : base() { }
    }
}`
	document := sourceDocumentForScanner(text)
	document.Path = "attributed-type.cs"
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 64, MaxSignatureBytes: 4096, MaxDiagnostics: 16}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("attributed type analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := byName["Demo.Container"]; !ok {
		t.Fatalf("attributed type missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	constructors := 0
	for _, symbol := range result.Analysis.Symbols {
		if symbol.QualifiedName == "Demo.Container.Container" && symbol.Kind == SymbolKindConstructor {
			constructors++
		}
	}
	if constructors != 2 {
		t.Fatalf("constructors=%d; symbols=%v", constructors, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCSharpAnalyzerIndexersAndDestructor(t *testing.T) {
	text := `class Container {
    public string this[int index] { get => ""; set { } }
    public string this[string key] => key;

    ~Container() { Dispose(); }
    private void Dispose() { }
}`
	document := sourceDocumentForScanner(text)
	document.Path = "indexers.cs"
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 64, MaxSignatureBytes: 4096, MaxDiagnostics: 16}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("indexer/destructor analysis unexpectedly partial: %+v", result.Analysis)
	}
	var indexers int
	var destructors int
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "this" && symbol.Kind == SymbolKindProperty && symbol.NativeKind == "indexer" {
			indexers++
		}
		if symbol.Name == "Container" && symbol.Kind == SymbolKindDestructor && symbol.NativeKind == "destructor" {
			destructors++
		}
		if symbol.QualifiedName == "Container.index" || symbol.QualifiedName == "Container.key" {
			t.Fatalf("indexer parameter leaked as member: %+v", symbol)
		}
	}
	if indexers != 2 || destructors != 1 {
		t.Fatalf("indexers=%d destructors=%d; symbols=%v", indexers, destructors, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCSharpAnalyzerFileScopedNamespaceMalformedAndLimits(t *testing.T) {
	text := `namespace App.Models;
public class Good { public int Id { get; set; } }
public class Broken {
    public void StillVisible() { }
    string s = "unterminated
`
	document := sourceDocumentForScanner(text)
	document.Path = "broken.cs"
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{
		IncludeSignatures: true,
		Limits:            SymbolBuilderLimits{MaxSymbols: 32, MaxSignatureBytes: 1024, MaxDiagnostics: 8},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("malformed C# did not report partial coverage: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"App.Models", "App.Models.Good", "App.Models.Good.Id", "App.Models.Broken", "App.Models.Broken.StillVisible"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("malformed recovery lost %s: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}

	limited := AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 2, MaxSignatureBytes: 1024, MaxDiagnostics: 8}}
	limitedResult, err := (CSharpAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("class A {} class B {} class C {}"), limited)
	if err != nil {
		t.Fatal(err)
	}
	if !limitedResult.Analysis.Truncated || limitedResult.Analysis.CoverageComplete || len(limitedResult.Analysis.Symbols) != 2 {
		t.Fatalf("C# bounded result = %+v", limitedResult.Analysis)
	}
}
