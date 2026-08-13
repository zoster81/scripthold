package sourceintelligence

import (
	"context"
	"reflect"
	"testing"
)

var _ SourceAnalyzer = VBNetAnalyzer{}

func TestVBNetAnalyzerScopesContinuationColonAndRelations(t *testing.T) {
	text := `Imports System
Namespace Demo
Public Partial Class Box(Of T)
    Inherits BaseBox
    Implements IDisposable
    Public Const Answer As Integer = 42
    Private ReadOnly value As T
    Public Event Changed()

    Public Sub New(value As T)
        Me.value = value
    End Sub

    Public Function GetValue( _
        Optional fallback As T = Nothing) As T
        Return value
    End Function

    Public Property Value As T
        Get
            Return value
        End Get
        Private Set(value As T)
            Me.value = value
        End Set
    End Property

    Public Sub First(): End Sub : Public Sub Second(): End Sub
End Class

Friend Structure Pair
    Public X As Integer
    Public Y As Integer
End Structure

Public Interface IService
    Sub Run()
End Interface
End Namespace
`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.vb"
	options := AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 256, MaxSignatureBytes: 4096, MaxDiagnostics: 32}}
	result, err := (VBNetAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("VB.NET analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	want := map[string]SymbolKind{
		"Demo":              SymbolKindNamespace,
		"Demo.Box":          SymbolKindClass,
		"Demo.Box.Answer":   SymbolKindConstant,
		"Demo.Box.value":    SymbolKindField,
		"Demo.Box.Changed":  SymbolKindEvent,
		"Demo.Box.Box":      SymbolKindConstructor,
		"Demo.Box.GetValue": SymbolKindMethod,
		"Demo.Box.Value":    SymbolKindProperty,
		"Demo.Box.First":    SymbolKindMethod,
		"Demo.Box.Second":   SymbolKindMethod,
		"Demo.Pair":         SymbolKindStruct,
		"Demo.Pair.X":       SymbolKindField,
		"Demo.Pair.Y":       SymbolKindField,
		"Demo.IService":     SymbolKindInterface,
		"Demo.IService.Run": SymbolKindMethod,
	}
	for qualified, kind := range want {
		symbol, ok := byName[qualified]
		if !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if symbol.Language != "vbnet" || symbol.Analyzer != string(AnalyzerVBNet) || symbol.Evidence != SymbolEvidenceStructural {
			t.Fatalf("%s metadata = %+v", qualified, symbol)
		}
	}
	if !containsString(byName["Demo.Box"].Modifiers, "partial") {
		t.Fatalf("Box modifiers = %v", byName["Demo.Box"].Modifiers)
	}
	if byName["Demo.Pair"].Visibility != VisibilityFriend || byName["Demo.Box.value"].Visibility != VisibilityPrivate {
		t.Fatalf("visibility Pair=%q value=%q", byName["Demo.Pair"].Visibility, byName["Demo.Box.value"].Visibility)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "System" {
		t.Fatalf("Imports dependencies = %+v", result.Dependencies)
	}
	if len(result.Relations) != 2 || result.Relations[0].Kind != "inherits" || result.Relations[0].Target != "BaseBox" || result.Relations[1].Kind != "implements" || result.Relations[1].Target != "IDisposable" {
		t.Fatalf("VB relations = %+v", result.Relations)
	}
	if byName["Demo.Box.GetValue"].Signature == "" || byName["Demo.Box.GetValue"].BodyRange == nil {
		t.Fatalf("continued function ranges = %+v", byName["Demo.Box.GetValue"])
	}
	second, err := (VBNetAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, second) {
		t.Fatal("VB.NET analyzer output is nondeterministic")
	}
}

func TestVBNetAnalyzerEscapedNamesAndDeclarationModifiers(t *testing.T) {
	text := `MustInherit Class Container
    Private NotInheritable Class [Class]
    End Class

    Public Shared Iterator Function Items() As IEnumerable(Of Integer)
        Yield 1
    End Function

    Default Public ReadOnly Property Item(index As Integer) As Integer
        Get
            Return index
        End Get
    End Property

    Public Shadows Sub Reset()
    End Sub

    Public Overridable Function Compute() As Integer
        Return 1
    End Function

    Public MustOverride Function Required() As Integer
End Class
`
	document := sourceDocumentForScanner(text)
	document.Path = "modifiers.vb"
	result, err := (VBNetAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 64, MaxSignatureBytes: 2048, MaxDiagnostics: 16}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("VB.NET modifier analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	want := map[string]SymbolKind{
		"Container":          SymbolKindClass,
		"Container.Class":    SymbolKindClass,
		"Container.Items":    SymbolKindMethod,
		"Container.Item":     SymbolKindProperty,
		"Container.Reset":    SymbolKindMethod,
		"Container.Compute":  SymbolKindMethod,
		"Container.Required": SymbolKindMethod,
	}
	for qualified, kind := range want {
		if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind {
			t.Fatalf("%s = %+v exists=%v; symbols=%v", qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if !containsString(byName["Container.Items"].Modifiers, "iterator") || !containsString(byName["Container.Item"].Modifiers, "default") {
		t.Fatalf("iterator/default modifiers missing: Items=%v Item=%v", byName["Container.Items"].Modifiers, byName["Container.Item"].Modifiers)
	}
	if !containsString(byName["Container.Required"].Modifiers, "mustoverride") || byName["Container.Required"].BodyRange != nil {
		t.Fatalf("MustOverride function = %+v", byName["Container.Required"])
	}
}

func TestVBNetAnalyzerDeclareProceduresAndCustomEvent(t *testing.T) {
	text := `Class Interop
    Friend Declare Auto Function NativeCall Lib "kernel32" (value As Integer) As Integer

    Public Custom Event Changed As EventHandler
        AddHandler(value As EventHandler)
        End AddHandler
        RemoveHandler(value As EventHandler)
        End RemoveHandler
        RaiseEvent(sender As Object, e As EventArgs)
        End RaiseEvent
    End Event
End Class
`
	document := sourceDocumentForScanner(text)
	document.Path = "interop.vb"
	result, err := (VBNetAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 64, MaxSignatureBytes: 2048, MaxDiagnostics: 16}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("VB.NET interop/event analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	nativeCall, ok := byName["Interop.NativeCall"]
	if !ok || nativeCall.Kind != SymbolKindMethod || nativeCall.BodyRange != nil || !containsString(nativeCall.Modifiers, "declare") || !containsString(nativeCall.Modifiers, "auto") {
		t.Fatalf("Declare Function = %+v exists=%v", nativeCall, ok)
	}
	changed, ok := byName["Interop.Changed"]
	if !ok || changed.Kind != SymbolKindEvent || changed.BodyRange == nil || !containsString(changed.Modifiers, "custom") {
		t.Fatalf("Custom Event = %+v exists=%v", changed, ok)
	}
	for _, unexpected := range []string{"Interop.AddHandler", "Interop.RemoveHandler", "Interop.RaiseEvent"} {
		if _, exists := byName[unexpected]; exists {
			t.Fatalf("custom-event accessor became declaration %s: %v", unexpected, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestVBNetAnalyzerMultilineLambdasDoNotCloseOuterCallable(t *testing.T) {
	text := `Class Container
    Public Sub Outer()
        Dim singleFunction As Func(Of Integer, Integer) = Function(value As Integer) value + 1
        Dim singleSub As Action(Of Integer) = Sub(value As Integer) Console.WriteLine(value)
        Dim action As Action = Sub()
                                   Dim nested As Integer = 1
                               End Sub
        Dim producer As Func(Of Integer) = Function() As Integer
                                               Return 2
                                           End Function
        Dim localAfter As Integer = producer()
    End Sub

    Public Function After() As Integer
        Return 3
    End Function
End Class
`
	document := sourceDocumentForScanner(text)
	document.Path = "lambdas.vb"
	result, err := (VBNetAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 64, MaxSignatureBytes: 2048, MaxDiagnostics: 16}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("VB.NET lambda analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, expected := range []string{"Container", "Container.Outer", "Container.After"} {
		if _, ok := byName[expected]; !ok {
			t.Fatalf("missing %s: %v", expected, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, unexpected := range []string{"Container.nested", "Container.producer", "Container.localAfter"} {
		if _, ok := byName[unexpected]; ok {
			t.Fatalf("method-local symbol leaked as %s: %v", unexpected, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestVBNetAnalyzerIgnoresMethodLocalsAndRecoversMissingEnd(t *testing.T) {
	text := `Class Broken
    Private field As Integer
    Sub Work()
        Dim local As Integer
        local = 1
    End Sub
    Function StillVisible() As Integer
        Return field
    End Function
`
	document := sourceDocumentForScanner(text)
	document.Path = "broken.vb"
	result, err := (VBNetAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 32, MaxSignatureBytes: 1024, MaxDiagnostics: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("missing End Class did not lower coverage: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Broken", "Broken.field", "Broken.Work", "Broken.StillVisible"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing recovered symbol %s: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if _, exists := byName["Broken.Work.local"]; exists {
		t.Fatalf("method local became field: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}
