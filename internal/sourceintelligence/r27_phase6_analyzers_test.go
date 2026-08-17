package sourceintelligence

import (
	"context"
	"reflect"
	"testing"
)

func TestR27Phase6BasicDialectAnalyzersRemainDistinct(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     map[string]SymbolKind
	}{
		{"vb6", VB6Analyzer{}, "Attribute VB_Name = \"DemoModule\"\nPublic Sub Hello()\nEnd Sub\nPrivate Function Twice(ByVal x As Integer) As Integer\nEnd Function\n", map[string]SymbolKind{"DemoModule": SymbolKindModule, "DemoModule.Hello": SymbolKindMethod, "DemoModule.Twice": SymbolKindMethod}},
		{"vba", VBAAnalyzer{}, "Attribute VB_Name = \"SheetCode\"\nPublic Const Answer = 42\nPublic Function Compute(ByVal x As Long) As Long\nEnd Function\n", map[string]SymbolKind{"SheetCode": SymbolKindModule, "SheetCode.Answer": SymbolKindConstant, "SheetCode.Compute": SymbolKindMethod}},
		{"vbscript", VBScriptAnalyzer{}, "Class Service\n  Public Sub Run()\n  End Sub\n  Private Function Twice(x)\n  End Function\nEnd Class\nFunction Top(x)\nEnd Function\n", map[string]SymbolKind{"Service": SymbolKindClass, "Service.Run": SymbolKindMethod, "Service.Twice": SymbolKindMethod, "Top": SymbolKindFunction}},
		{"qbasic", QBasicAnalyzer{}, "DECLARE SUB Hello (name$)\nTYPE Point\n  X AS INTEGER\nEND TYPE\nSUB Hello (name$)\nEND SUB\nFUNCTION Twice% (x%)\nEND FUNCTION\n", map[string]SymbolKind{"Point": SymbolKindType, "Point.X": SymbolKindField, "Hello": SymbolKindFunction, "Twice": SymbolKindFunction}},
		{"classic-basic", ClassicBasicAnalyzer{}, "10 DEF FNDOUBLE(X)=X*2\n20 DIM VALUE\n30 END\n", map[string]SymbolKind{"FNDOUBLE": SymbolKindFunction, "VALUE": SymbolKindVariable}},
		{"freebasic", FreeBasicAnalyzer{}, "#include \"common.bi\"\nNamespace Demo\nType Box\n  value As Integer\nEnd Type\nSub Run()\nEnd Sub\nEnd Namespace\n", map[string]SymbolKind{"Demo": SymbolKindNamespace, "Demo.Box": SymbolKindType, "Demo.Box.value": SymbolKindField, "Demo.Run": SymbolKindFunction}},
		{"purebasic", PureBasicAnalyzer{}, "XIncludeFile \"common.pbi\"\nModule Demo\nProcedure Run(value.i)\nEndProcedure\nEndModule\nStructure Item\n  value.i\nEndStructure\n", map[string]SymbolKind{"Demo": SymbolKindModule, "Demo.Run": SymbolKindFunction, "Item": SymbolKindStruct, "Item.value": SymbolKindField}},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			document := sourceDocumentForScanner(tc.text)
			result, err := tc.analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s analysis partial: %+v", tc.language, result.Analysis)
			}
			if tc.analyzer.Language() != tc.language {
				t.Fatalf("analyzer language=%q want=%q", tc.analyzer.Language(), tc.language)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				if symbol, ok := byName[name]; !ok || symbol.Kind != kind {
					t.Fatalf("%s %s=%+v exists=%v symbols=%v", tc.language, name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestR27RealWorldFreeBasicTypeAliasesDoNotOpenScopes(t *testing.T) {
	text := "Type pCall As Function (xy As Any Ptr) As Long\n" +
		"Type ValueAlias As Long\n" +
		"Type DLLVERSIONINFO\n  cbSize As Long\nEnd Type\n" +
		"Function WinMain() As Integer\n  Return 0\nEnd Function\n"
	result, err := (FreeBasicAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid FreeBASIC type aliases reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"pCall", "ValueAlias", "DLLVERSIONINFO", "DLLVERSIONINFO.cbSize", "WinMain"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing %s; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, wrong := range []string{"pCall.ValueAlias", "pCall.DLLVERSIONINFO", "ValueAlias.DLLVERSIONINFO", "DLLVERSIONINFO.WinMain"} {
		if _, ok := byName[wrong]; ok {
			t.Fatalf("FreeBASIC type alias leaked scope through %s; symbols=%v", wrong, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestR27RealWorldVBDesignerKeywordAssignmentsDoNotOpenScopes(t *testing.T) {
	text := "VERSION 5.00\r\n" +
		"Begin {C0E45035-5775-11D0-B388-00A0C9055D8E} DataEnvironment1\r\n" +
		"   BeginProperty Field1\r\n" +
		"      Type = 200\r\n" +
		"      Name = \"au_id\"\r\n" +
		"   EndProperty\r\n" +
		"End\r\n" +
		"Attribute VB_Name = \"DataEnvironment1\"\r\n" +
		"Private Sub Run()\r\nEnd Sub\r\n"
	result, err := (VB6Analyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid exported VB6 designer source reported partial coverage: diagnostics=%+v", result.Analysis.Diagnostics)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := byName["DataEnvironment1.Run"]; !ok {
		t.Fatalf("code-behind method missing: symbols=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestR27RealWorldVBPropertyProceduresUseDeclaredPropertyName(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
		want     []string
	}{
		{
			name:     "vb6",
			analyzer: VB6Analyzer{},
			text: "Attribute VB_Name = \"ArrayDataSource\"\n" +
				"Public Property Get EOF() As Boolean\nEnd Property\n" +
				"Public Property Let Bookmark(ByVal newValue As Variant)\nEnd Property\n" +
				"Public Property Set Recordset(ByVal newValue As Object)\nEnd Property\n",
			want: []string{"ArrayDataSource.EOF", "ArrayDataSource.Bookmark", "ArrayDataSource.Recordset"},
		},
		{
			name:     "vba",
			analyzer: VBAAnalyzer{},
			text: "Attribute VB_Name = \"SheetCode\"\n" +
				"Public Property Get EOF() As Boolean\nEnd Property\n" +
				"Public Property Let Bookmark(ByVal newValue As Variant)\nEnd Property\n" +
				"Public Property Set Recordset(ByVal newValue As Object)\nEnd Property\n",
			want: []string{"SheetCode.EOF", "SheetCode.Bookmark", "SheetCode.Recordset"},
		},
		{
			name:     "vbscript",
			analyzer: VBScriptAnalyzer{},
			text: "Class Service\n" +
				"Public Property Get EOF()\nEnd Property\n" +
				"Public Property Let Bookmark(value)\nEnd Property\n" +
				"Public Property Set Recordset(value)\nEnd Property\n" +
				"End Class\n",
			want: []string{"Service.EOF", "Service.Bookmark", "Service.Recordset"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, qualifiedName := range tc.want {
				symbol, ok := byName[qualifiedName]
				if !ok || symbol.Kind != SymbolKindProperty {
					t.Fatalf("%s property %q=%+v exists=%v symbols=%v", tc.name, qualifiedName, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			for _, forbidden := range []string{"Get", "Let", "Set"} {
				for _, symbol := range result.Analysis.Symbols {
					if symbol.Kind == SymbolKindProperty && symbol.Name == forbidden {
						t.Fatalf("%s property procedure verb leaked as symbol name: %+v", tc.name, symbol)
					}
				}
			}
		})
	}
}

func TestR27Phase6DotNetAdjacentAnalyzers(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     map[string]SymbolKind
	}{
		{"fsharp", FSharpAnalyzer{}, "namespace Demo\nopen System\ntype IService =\n    abstract member Run : int -> int\ntype Service() =\n    inherit System.Object()\n    interface IService\n    member _.Run(value:int) = value\nlet top value = value\n", map[string]SymbolKind{"Demo": SymbolKindNamespace, "Demo.IService": SymbolKindType, "Demo.IService.Run": SymbolKindMethod, "Demo.Service": SymbolKindType, "Demo.Service.Run": SymbolKindMethod, "Demo.top": SymbolKindFunction}},
		{"cpp-cli", CPPCLIAnalyzer{}, "using namespace System;\npublic ref class Service : public Object {\npublic:\n    Service() {}\n    int Run(int value) { return value; }\n};\n", map[string]SymbolKind{"Service": SymbolKindClass, "Service.Service": SymbolKindConstructor, "Service.Run": SymbolKindMethod}},
		{"jscript-net", JScriptNetAnalyzer{}, "package Demo {\nimport System;\npublic class Service extends BaseService {\n    public function Run(value : int) : int { return value; }\n}\n}\n", map[string]SymbolKind{"Demo": SymbolKindPackage, "Demo.Service": SymbolKindClass, "Demo.Service.Run": SymbolKindMethod}},
		{"cil", CILAnalyzer{}, ".assembly extern mscorlib {}\n.assembly Demo {}\n.module Demo.dll\n.class public auto ansi Service extends [mscorlib]System.Object {\n  .field private int32 value\n  .method public hidebysig instance int32 Run(int32 x) cil managed { ret }\n}\n", map[string]SymbolKind{"Demo": SymbolKindModule, "Demo.Service": SymbolKindClass, "Demo.Service.value": SymbolKindField, "Demo.Service.Run": SymbolKindMethod}},
		{"powershell", PowerShellAnalyzer{}, "using module ./Common.psm1\nclass Service : BaseService {\n    [string] $Name\n    [int] Run([int] $value) { return $value }\n}\nfunction Get-Value { param([int]$Value) $Value }\nfilter Select-Value { $_ }\n", map[string]SymbolKind{"Service": SymbolKindClass, "Service.Name": SymbolKindField, "Service.Run": SymbolKindMethod, "Get-Value": SymbolKindFunction, "Select-Value": SymbolKindFunction}},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s analysis partial: %+v", tc.language, result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				if symbol, ok := byName[name]; !ok || symbol.Kind != kind {
					t.Fatalf("%s %s=%+v exists=%v symbols=%v", tc.language, name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestR27Phase6ClassicASPDelegatesVBScriptAndJScript(t *testing.T) {
	text := `<%@ Language="VBScript" %>
<% Sub VBHello() : End Sub %>
<script language="JScript" runat="server">
function JSHello(value) { return value; }
</script>`
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.asp"
	result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(), document, phase3AnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("Classic ASP Phase 6 partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := byName["VBHello"]; !ok {
		t.Fatalf("missing VBScript symbol: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if symbol, ok := byName["JSHello"]; !ok || symbol.Language != "jscript" {
		t.Fatalf("JScript symbol=%+v exists=%v", symbol, ok)
	}
	supported := map[string]bool{}
	for _, region := range result.Regions {
		if region.Kind == "server-script" {
			supported[region.Language] = region.Supported
		}
	}
	if !supported["vbscript"] || !supported["jscript"] {
		t.Fatalf("ASP supported regions=%v", supported)
	}
}

func TestR27Phase6CompositeDotNetFormatsPreserveHostCoordinates(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		path     string
		text     string
		want     []string
	}{
		{"aspnet-webforms", ASPNetWebFormsAnalyzer{}, "page.aspx", `<%@ Page Language="C#" %>
<html><body>
<script runat="server">
protected void Page_Load(object sender, EventArgs e) { }
</script>
</body></html>`, []string{"Page_Load"}},
		{"razor", RazorAnalyzer{}, "page.cshtml", `@using System.Text
@functions {
    public string Format(int value) { return value.ToString(); }
}
<div>@DateTime.Now</div>`, []string{"Format"}},
		{"blazor", BlazorAnalyzer{}, "Widget.razor", `@using System
<h1>@Title</h1>
@code {
    private string Title { get; set; }
    private void Refresh() { }
}`, []string{"Title", "Refresh"}},
		{"xaml", XAMLAnalyzer{}, "View.xaml", `<Window x:Class="Demo.MainWindow" xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"><Grid x:Name="Root" /></Window>`, []string{"Demo.MainWindow", "Demo.MainWindow.Root"}},
	}
	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			document := sourceDocumentForScanner(tc.text)
			document.Path = tc.path
			result, err := tc.analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("%s partial: %+v", tc.language, result.Analysis)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("%s missing %s symbols=%v", tc.language, want, names)
				}
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.DeclarationRange.Start.Line <= 0 || symbol.NameRange.Start.Line <= 0 {
					t.Fatalf("%s invalid host range: %+v", tc.language, symbol)
				}
			}
		})
	}
}

func TestR27Phase6RegistryActivatesEveryRequestedDialectSeparately(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	expected := map[string]AnalyzerID{
		"vb6": AnalyzerVB6, "vba": AnalyzerVBA, "vbscript": AnalyzerVBScript, "qbasic": AnalyzerQBasic,
		"classic-basic": AnalyzerClassicBasic, "freebasic": AnalyzerFreeBasic, "purebasic": AnalyzerPureBasic,
		"fsharp": AnalyzerFSharp, "cpp-cli": AnalyzerCPPCLI, "jscript-net": AnalyzerJScriptNet, "cil": AnalyzerCIL,
		"powershell": AnalyzerPowerShell, "classic-asp": AnalyzerClassicASP, "aspnet-webforms": AnalyzerASPNetWebForms,
		"razor": AnalyzerRazor, "blazor": AnalyzerBlazor, "xaml": AnalyzerXAML,
	}
	identities := map[AnalyzerID]string{}
	for language, wantID := range expected {
		descriptor, ok := registry.Resolve(language)
		if !ok {
			t.Fatalf("missing Phase 6 registry row %s", language)
		}
		analyzer, available := AnalyzerFor(descriptor)
		if !available || analyzer.ID() != wantID || analyzer.Language() != language {
			t.Fatalf("%s analyzer=%#v available=%v descriptor=%+v", language, analyzer, available, descriptor)
		}
		if previous, duplicate := identities[wantID]; duplicate && language != "classic-asp" {
			t.Fatalf("dialects %s and %s share analyzer identity %s", previous, language, wantID)
		}
		identities[wantID] = language
		caps := descriptor.Capabilities
		if !caps.SourceAnalysis || !caps.Declarations || !caps.Ranges {
			t.Fatalf("%s incomplete capability row: %+v", language, caps)
		}
		if caps.ProjectResolvedReferences || caps.ProjectResolvedDefinitions || caps.SemanticRelations {
			t.Fatalf("%s overclaims unproven project/semantic capability: %+v", language, caps)
		}
	}
	for _, language := range []string{"vb6", "vba", "vbscript", "qbasic", "classic-basic", "freebasic", "purebasic"} {
		descriptor, _ := registry.Resolve(language)
		if !descriptor.Capabilities.CaseInsensitive {
			t.Fatalf("%s must be case insensitive", language)
		}
	}
}

func TestR27Phase6RepresentativeDependenciesRemainLiteralAndDeterministic(t *testing.T) {
	first, err := (FreeBasicAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("#include \"one.bi\"\nSub Run()\nEnd Sub\n"), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	second, err := (FreeBasicAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner("#include \"one.bi\"\nSub Run()\nEnd Sub\n"), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("Phase 6 analyzer output is nondeterministic")
	}
	if got := dependencyValues(first.Dependencies); !reflect.DeepEqual(got, []string{"one.bi"}) {
		t.Fatalf("FreeBASIC dependencies=%v", got)
	}
}
