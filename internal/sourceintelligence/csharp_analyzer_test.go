package sourceintelligence

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

var _ SourceAnalyzer = CSharpAnalyzer{}

func TestCSharpAnalyzerGenericMethodNames(t *testing.T) {
	const source = `namespace Demo {
public class Converter {
    public static T? DeserializeObject<T>(string value) => default;
    public static T? DeserializeAnonymousType<T>(string value, T item) { return default; }
    public static T? DeserializeObject<T>
        (string value, int count) => default;
    public static TResult? Map<TResult, TKey>(TKey key) where TResult : class { return default; }
    public static int Compare(int left, int right) => left < right ? left : right;
    public static TValue? Parse
        <TValue>
        (string value) => default;
}
}`
	for _, tc := range []struct {
		name, source string
	}{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CSharpAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("generic method source analysis partial: %+v", result.Analysis)
			}
			want := map[int]string{
				3: "DeserializeObject",
				4: "DeserializeAnonymousType",
				5: "DeserializeObject",
				7: "Map",
				8: "Compare",
				9: "Parse",
			}
			for _, symbol := range result.Analysis.Symbols {
				line := symbol.NameRange.Start.Line
				if expected, ok := want[line]; ok {
					if symbol.Name != expected || symbol.Kind != SymbolKindMethod ||
						symbol.QualifiedName != "Demo.Converter."+expected ||
						symbol.ParentQualifiedName != "Demo.Converter" {
						t.Fatalf("generic method at line %d: got %+v, want %s", line, symbol, expected)
					}
					delete(want, line)
				}
				if symbol.Kind == SymbolKindMethod && (symbol.Name == "T" || symbol.Name == "TResult" || symbol.Name == "TKey") {
					t.Fatalf("generic parameter indexed as method: %+v", symbol)
				}
			}
			if len(want) != 0 {
				t.Fatalf("generic methods missing at lines %v; symbols=%v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestCSharpOriginalNewtonsoftGenericMethods(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_CSHARP_NEWTONSOFT_JSONCONVERT")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_CSHARP_NEWTONSOFT_JSONCONVERT to the original JsonConvert.cs")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("original C# fixture exceeds 1 MiB")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	document := sourceDocumentForScanner(string(contents))
	document.Path = path
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("original Newtonsoft analysis partial: %+v", result.Analysis)
	}
	want := map[int]string{
		744: "DeserializeObject",
		763: "DeserializeAnonymousType",
		786: "DeserializeAnonymousType",
		801: "DeserializeObject",
		819: "DeserializeObject",
	}
	for _, symbol := range result.Analysis.Symbols {
		line := symbol.NameRange.Start.Line
		if expected, ok := want[line]; ok {
			if symbol.Name != expected || symbol.Kind != SymbolKindMethod ||
				symbol.QualifiedName != "Newtonsoft.Json.JsonConvert."+expected ||
				symbol.ParentQualifiedName != "Newtonsoft.Json.JsonConvert" {
				t.Fatalf("original Newtonsoft generic declaration line %d: got %+v, want %q", line, symbol, expected)
			}
			delete(want, line)
		}
	}
	if len(want) != 0 {
		t.Fatalf("original Newtonsoft generic methods missing at lines %v; symbols=%v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestCSharpOriginalSerilogGenericMethods(t *testing.T) {
	for _, tc := range []struct {
		name, env, qualified string
		line                 int
	}{
		{"extensions", "SCRIPTHOLD_REAL_CSHARP_SERILOG_EXTENSIONS", "Serilog.LoggerExtensions.ForContext", 20},
		{"interface", "SCRIPTHOLD_REAL_CSHARP_SERILOG_ILOGGER", "Serilog.ILogger.ForContext", 105},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skipf("set %s to an original pinned Serilog C# source file", tc.env)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > 1<<20 {
				t.Fatal("original Serilog C# fixture exceeds 1 MiB")
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			document := sourceDocumentForScanner(string(contents))
			document.Path = path
			result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("original Serilog analysis partial: %+v", result.Analysis)
			}
			found := false
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Kind == SymbolKindMethod && (symbol.Name == "TSource" || symbol.Name == "TValue") {
					t.Fatalf("Serilog generic argument misnamed as method: %+v", symbol)
				}
				if symbol.NameRange.Start.Line == tc.line &&
					symbol.QualifiedName == tc.qualified && symbol.Kind == SymbolKindMethod {
					found = true
				}
			}
			if !found {
				t.Fatalf("original Serilog generic method %q at line %d missing; symbols=%v", tc.qualified, tc.line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestCSharpMemberInitializersAreNotCallableDeclarations(t *testing.T) {
	const source = `class Pipeline {
    public static readonly Pipeline Empty = new(1);
    public Func<int> Callback = () => 1;
    public int Value => Compute();
    public static Pipeline Create(int value) => new(value);
    private int Compute() => 7;
}`
	for _, tc := range []struct{ name, source string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CSharpAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("member initializer source partial: %+v", result.Analysis)
			}
			want := map[string]SymbolKind{
				"Pipeline":          SymbolKindClass,
				"Pipeline.Empty":    SymbolKindField,
				"Pipeline.Callback": SymbolKindField,
				"Pipeline.Value":    SymbolKindProperty,
				"Pipeline.Create":   SymbolKindMethod,
				"Pipeline.Compute":  SymbolKindMethod,
			}
			for _, symbol := range result.Analysis.Symbols {
				kind, ok := want[symbol.QualifiedName]
				if !ok || symbol.Kind != kind {
					t.Fatalf("unexpected member declaration %+v; want=%v", symbol, want)
				}
				delete(want, symbol.QualifiedName)
			}
			if len(want) != 0 {
				t.Fatalf("members missing: %v; symbols=%v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestCSharpOriginalPollyPipelineMembers(t *testing.T) {
	for _, tc := range []struct {
		name, env string
		want      map[int]struct {
			qualified string
			kind      SymbolKind
		}
	}{
		{"type", "SCRIPTHOLD_REAL_CSHARP_POLLY_PIPELINE", map[int]struct {
			qualified string
			kind      SymbolKind
		}{
			13: {"Polly.ResiliencePipeline", SymbolKindClass},
			18: {"Polly.ResiliencePipeline.Empty", SymbolKindField},
		}},
		{"async", "SCRIPTHOLD_REAL_CSHARP_POLLY_PIPELINE_ASYNC", map[int]struct {
			qualified string
			kind      SymbolKind
		}{
			17: {"Polly.ResiliencePipeline.ExecuteAsync", SymbolKindMethod},
			32: {"Polly.ResiliencePipeline.ExecuteAsync", SymbolKindMethod},
			48: {"Polly.ResiliencePipeline.ExecuteAsync", SymbolKindMethod},
			63: {"Polly.ResiliencePipeline.ExecuteAsync", SymbolKindMethod},
			86: {"Polly.ResiliencePipeline.ExecuteOutcomeAsync", SymbolKindMethod},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skipf("set %s to a qualified original Polly C# file", tc.env)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > 1<<20 {
				t.Fatal("original Polly fixture exceeds 1 MiB")
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			document := sourceDocumentForScanner(string(contents))
			document.Path = path
			result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("original Polly analysis partial: %+v", result.Analysis)
			}
			seen := make(map[int]bool)
			for _, symbol := range result.Analysis.Symbols {
				if want, ok := tc.want[symbol.NameRange.Start.Line]; ok {
					if symbol.QualifiedName != want.qualified || symbol.Kind != want.kind {
						t.Fatalf("Polly declaration line %d: got %+v, want %+v", symbol.NameRange.Start.Line, symbol, want)
					}
					seen[symbol.NameRange.Start.Line] = true
				}
			}
			for line := range tc.want {
				if !seen[line] {
					t.Fatalf("Polly declaration line %d missing; symbols=%v", line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestCSharpOriginalMediatRSendOverloads(t *testing.T) {
	for _, tc := range []struct {
		name, env, owner string
		lines            []int
	}{
		{"interface", "SCRIPTHOLD_REAL_CSHARP_MEDIATR_ISENDER", "MediatR.ISender", []int{19, 27, 36}},
		{"implementation", "SCRIPTHOLD_REAL_CSHARP_MEDIATR_MEDIATOR", "MediatR.Mediator", []int{50, 67, 85}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skipf("set %s to the qualified original MediatR C# source", tc.env)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Size() > 1<<20 {
				t.Fatal("MediatR fixture exceeds 1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = path
			result, err := (CSharpAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("MediatR analysis incomplete: %+v", result.Analysis)
			}
			want := make(map[int]bool, len(tc.lines))
			for _, line := range tc.lines {
				want[line] = false
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Kind == SymbolKindMethod && (symbol.Name == "TResponse" || symbol.Name == "TRequest") {
					t.Fatalf("generic parameter became a method: %+v", symbol)
				}
				line := symbol.NameRange.Start.Line
				if _, exists := want[line]; !exists {
					continue
				}
				if symbol.Kind != SymbolKindMethod || symbol.Name != "Send" ||
					symbol.QualifiedName != tc.owner+".Send" || symbol.ParentQualifiedName != tc.owner {
					t.Fatalf("MediatR Send at line %d: %+v", line, symbol)
				}
				want[line] = true
			}
			for line, found := range want {
				if !found {
					t.Fatalf("missing %s.Send at line %d; symbols=%v", tc.owner, line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestCSharpOriginalDapperQueryAsyncOverloads(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_CSHARP_DAPPER_SQLMAPPER_ASYNC")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_CSHARP_DAPPER_SQLMAPPER_ASYNC to the qualified Dapper SqlMapper.Async.cs")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 1<<20 {
		t.Fatal("Dapper original fixture exceeds 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(data))
	doc.Path = path
	result, err := (CSharpAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Dapper original analysis partial: %+v", result.Analysis)
	}
	lines := []int{27, 36, 90, 213, 298, 307, 691, 708, 730, 748, 771, 790, 814, 834, 859, 880, 906, 928, 969}
	want := make(map[int]bool, len(lines))
	for _, line := range lines {
		want[line] = false
	}
	for _, symbol := range result.Analysis.Symbols {
		line := symbol.NameRange.Start.Line
		if _, exists := want[line]; !exists {
			continue
		}
		if symbol.Name != "QueryAsync" || symbol.Kind != SymbolKindMethod ||
			symbol.QualifiedName != "Dapper.SqlMapper.QueryAsync" ||
			symbol.ParentQualifiedName != "Dapper.SqlMapper" {
			t.Fatalf("Dapper QueryAsync at %d: %+v", line, symbol)
		}
		want[line] = true
	}
	for line, found := range want {
		if !found {
			t.Fatalf("missing Dapper QueryAsync at line %d; symbols=%v", line, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

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

func TestCSharpAnalyzerAvoidsSpeculativeTypeProbeModifierAllocations(t *testing.T) {
	document := sourceDocumentForScanner(generatedCSharpSource(400))
	document.Path = "type-probe-allocation.cs"
	options := AnalyzeOptions{MaxNesting: 256, Limits: SymbolBuilderLimits{MaxSymbols: 10_000, MaxSignatureBytes: 8192, MaxDiagnostics: 256}}
	allocations := testing.AllocsPerRun(5, func() {
		result, err := (CSharpAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil {
			panic(err)
		}
		if len(result.Analysis.Symbols) != 401 {
			panic("unexpected C# allocation-guard symbol count")
		}
	})
	if allocations > 6000 {
		t.Fatalf("allocations = %.0f, want <= 6000", allocations)
	}
}
