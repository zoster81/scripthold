package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase6ConformanceAcrossLegacyEncodingsUnicodeAndDeterminism(t *testing.T) {
	tests := []struct {
		name, language, extension, encoding, text string
		bom                                       bool
		want                                      []string
	}{
		{name: "vb6-windows1252-crlf", language: "vb6", extension: ".bas", encoding: "windows-1252", text: "Attribute VB_Name = \"Démo\"\r\nPublic Sub Café()\r\nEnd Sub\r\n", want: []string{"Démo", "Démo.Café"}},
		{name: "vbscript-utf16le", language: "vbscript", extension: ".vbs", encoding: "utf-16le", bom: true, text: "Class Résumé\r\nPublic Sub Café()\r\nEnd Sub\r\nEnd Class\r\n", want: []string{"Résumé", "Résumé.Café"}},
		{name: "qbasic-ibm850-crlf", language: "qbasic", extension: ".bas", encoding: "ibm850", text: "TYPE TRésumé\r\nCafé AS INTEGER\r\nEND TYPE\r\n", want: []string{"TRésumé", "TRésumé.Café"}},
		{name: "fsharp-utf16be", language: "fsharp", extension: ".fs", encoding: "utf-16be", bom: true, text: "namespace Démo\nlet café value = value\n", want: []string{"Démo", "Démo.café"}},
		{name: "powershell-utf16le", language: "powershell", extension: ".ps1", encoding: "utf-16le", bom: true, text: "function Get-Café { 1 }\r\n", want: []string{"Get-Café"}},
		{name: "purebasic-windows1252", language: "purebasic", extension: ".pb", encoding: "windows-1252", text: "Procedure Résumé()\r\nEndProcedure\r\n", want: []string{"Résumé"}},
		{name: "xaml-utf16le", language: "xaml", extension: ".xaml", encoding: "utf-16le", bom: true, text: `<Window x:Class="Café.View" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"><Grid x:Name="Résumé" /></Window>` + "\r\n", want: []string{"Café.View", "Café.View.Résumé"}},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture"+tc.extension)
			if err := os.WriteFile(path, encodeSourceFixture(t, tc.encoding, tc.text, tc.bom), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{RequestedEncoding: tc.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000})
			if err != nil {
				t.Fatal(err)
			}
			descriptor, _ := registry.Resolve(tc.language)
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatalf("missing analyzer %s", tc.language)
			}
			options := phase3AnalyzeOptions(true, 256)
			first, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			second, err := analyzer.Analyze(context.Background(), document, options)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("%s output is nondeterministic", tc.language)
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("%s conformance partial: %+v", tc.language, first.Analysis)
			}
			names := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("%s missing %s; symbols=%v", tc.language, want, names)
				}
			}
		})
	}
}

func TestR27Phase6DetectorKeepsSharedBASAmbiguousAndRoutesDistinctFormats(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ambiguous, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "module.bas", Text: "Public Sub Run()\nEnd Sub\n"})
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.State != DetectionAmbiguous || ambiguous.Language != "" {
		t.Fatalf("shared .bas = %+v, want ambiguous", ambiguous)
	}
	for _, tc := range []struct{ path, text, want string }{
		{"script.vbs", "Class Service\nEnd Class\n", "vbscript"},
		{"module.fs", "namespace Demo\nlet run x = x\n", "fsharp"},
		{"code.il", ".assembly Demo {}\n.class public Service {}\n", "cil"},
		{"profile.ps1", "function Get-Value { 1 }\nclass Service { [int] Run() { return 1 } }\n", "powershell"},
		{"lib.pb", "Module Demo\nEndModule\n", "purebasic"},
		{"page.aspx", `<%@ Page Language="C#" %><script runat="server">public void Run() {}</script>`, "aspnet-webforms"},
		{"page.cshtml", "@functions { public void Run() {} }\n", "razor"},
		{"Widget.razor", "@code { private void Run() {} }\n", "blazor"},
		{"View.xaml", `<Window x:Class="Demo.View" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml" />`, "xaml"},
	} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionProbable || result.Language != tc.want {
			t.Fatalf("%s detection=%+v want probable %s", tc.path, result, tc.want)
		}
	}
	for _, language := range []string{"vb6", "vba", "qbasic", "classic-basic", "cpp-cli", "jscript-net"} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "ambiguous.bas", Text: "Public Sub Run()\nEnd Sub\n", ExplicitLanguage: language})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionExact || result.Language != language {
			t.Fatalf("explicit %s detection=%+v", language, result)
		}
	}
}

func TestR27Phase6OpaqueAndDynamicBoundariesDoNotLeakDeclarations(t *testing.T) {
	powerShellText := "@\"\nfunction Fake-InHereString { }\n\"@\nfunction Real-Function { }\n"
	ps, err := (PowerShellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(powerShellText), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	psNames := sortedSymbolQualifiedNames(ps.Analysis.Symbols)
	if containsSortedString(psNames, "Fake-InHereString") || !containsSortedString(psNames, "Real-Function") {
		t.Fatalf("PowerShell here-string boundary symbols=%v", psNames)
	}

	fsharpText := "(* outer (* type Fake = class end *) *)\nlet real value = \"type StringFake = class end\"\n"
	fs, err := (FSharpAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(fsharpText), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	fsNames := sortedSymbolQualifiedNames(fs.Analysis.Symbols)
	if containsSortedString(fsNames, "Fake") || containsSortedString(fsNames, "StringFake") || !containsSortedString(fsNames, "real") {
		t.Fatalf("F# opaque boundary symbols=%v", fsNames)
	}

	vbscriptText := "Execute \"Class DynamicFake : End Class\"\nClass Real\nEnd Class\n"
	vb, err := (VBScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(vbscriptText), phase3AnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	vbNames := sortedSymbolQualifiedNames(vb.Analysis.Symbols)
	if containsSortedString(vbNames, "DynamicFake") || !containsSortedString(vbNames, "Real") {
		t.Fatalf("VBScript runtime/string boundary symbols=%v", vbNames)
	}
}

func TestR27Phase6MalformedAndCancellationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"vbscript", VBScriptAnalyzer{}, "Class Good\nEnd Class\nClass Broken\n"},
		{"powershell", PowerShellAnalyzer{}, "function Good {}\n@\"\nfunction Fake {}\n"},
		{"razor", RazorAnalyzer{}, "@functions { public void Broken() { }\n"},
		{"xaml", XAMLAnalyzer{}, "<!-- unterminated\n<Window x:Class=\"Fake.View\" />"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("%s malformed source did not lower coverage: %+v", tc.name, result.Analysis)
			}
		})
	}

	analyzers := []SourceAnalyzer{
		VB6Analyzer{}, VBAAnalyzer{}, VBScriptAnalyzer{}, QBasicAnalyzer{}, ClassicBasicAnalyzer{}, FreeBasicAnalyzer{}, PureBasicAnalyzer{},
		FSharpAnalyzer{}, CPPCLIAnalyzer{}, JScriptNetAnalyzer{}, CILAnalyzer{}, PowerShellAnalyzer{}, ClassicASPAnalyzer{}, ASPNetWebFormsAnalyzer{}, RazorAnalyzer{}, BlazorAnalyzer{}, XAMLAnalyzer{},
	}
	for _, analyzer := range analyzers {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := analyzer.Analyze(ctx, sourceDocumentForScanner("class A {}\n"), phase3AnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}
}

func TestR27Phase6LargeGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"vb6", VB6Analyzer{}, generatedPhase6VBModule(1200, "VB6Module")},
		{"vba", VBAAnalyzer{}, generatedPhase6VBModule(1200, "VBAModule")},
		{"vbscript", VBScriptAnalyzer{}, generatedPhase6BasicFunctions(1200, "Function", "End Function")},
		{"qbasic", QBasicAnalyzer{}, generatedPhase6BasicFunctions(1200, "SUB", "END SUB")},
		{"classic-basic", ClassicBasicAnalyzer{}, generatedPhase6ClassicBasic(1200)},
		{"freebasic", FreeBasicAnalyzer{}, generatedPhase6BasicFunctions(1200, "Sub", "End Sub")},
		{"purebasic", PureBasicAnalyzer{}, generatedPhase6BasicFunctions(1200, "Procedure", "EndProcedure")},
		{"fsharp", FSharpAnalyzer{}, generatedPhase6FSharp(1200)},
		{"cpp-cli", CPPCLIAnalyzer{}, generatedPhase6CPPCLI(1200)},
		{"jscript-net", JScriptNetAnalyzer{}, generatedPhase6JScriptNet(1200)},
		{"cil", CILAnalyzer{}, generatedPhase6CIL(1200)},
		{"powershell", PowerShellAnalyzer{}, generatedPhase6PowerShell(1200)},
		{"classic-asp", ClassicASPAnalyzer{}, generatedPhase6ClassicASP(1200)},
		{"aspnet-webforms", ASPNetWebFormsAnalyzer{}, generatedPhase6WebForms(1200)},
		{"razor", RazorAnalyzer{}, generatedPhase6Razor(1200, "functions")},
		{"blazor", BlazorAnalyzer{}, generatedPhase6Razor(1200, "code")},
		{"xaml", XAMLAnalyzer{}, generatedPhase6XAML(1200)},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result: symbols=%d truncated=%v complete=%v diagnostics=%+v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
		})
	}
}

func generatedPhase6VBModule(count int, module string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Attribute VB_Name = %q\n", module)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "Public Sub S%04d()\nEnd Sub\n", i)
	}
	return b.String()
}

func generatedPhase6BasicFunctions(count int, open, close string) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "%s F%04d()\n%s\n", open, i, close)
	}
	return b.String()
}

func generatedPhase6ClassicBasic(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "%d DEF FN%04d(X)=X\n", 10+i*10, i)
	}
	return b.String()
}

func generatedPhase6FSharp(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "let f%04d value = value\n", i)
	}
	return b.String()
}

func generatedPhase6CPPCLI(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "public ref class C%04d {};\n", i)
	}
	return b.String()
}

func generatedPhase6JScriptNet(count int) string {
	var b strings.Builder
	b.WriteString("package Generated {\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d() {}\n", i)
	}
	b.WriteString("}\n")
	return b.String()
}

func generatedPhase6CIL(count int) string {
	var b strings.Builder
	b.WriteString(".assembly Generated {}\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, ".class public C%04d {}\n", i)
	}
	return b.String()
}

func generatedPhase6PowerShell(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function Get-F%04d {}\n", i)
	}
	return b.String()
}

func generatedPhase6ClassicASP(count int) string {
	var b strings.Builder
	b.WriteString("<%@ Language=\"VBScript\" %>\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "<%% Sub S%04d()\nEnd Sub %%>\n", i)
	}
	return b.String()
}

func generatedPhase6WebForms(count int) string {
	var b strings.Builder
	b.WriteString("<%@ Page Language=\"C#\" %>\n<script runat=\"server\">\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "public void M%04d() {}\n", i)
	}
	b.WriteString("</script>\n")
	return b.String()
}

func generatedPhase6Razor(count int, directive string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "@%s {\n", directive)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "public void M%04d() {}\n", i)
	}
	b.WriteString("}\n")
	return b.String()
}

func generatedPhase6XAML(count int) string {
	var b strings.Builder
	b.WriteString(`<Window x:Class="Generated.View" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"><Grid>`)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, `<Button x:Name="B%04d" />`, i)
	}
	b.WriteString("</Grid></Window>\n")
	return b.String()
}

func FuzzR27Phase6AnalyzersNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{"Attribute VB_Name = \"M\"\nPublic Sub Run()\nEnd Sub\n", 0},
		{"Class Box\nPublic Sub Run()\nEnd Sub\nEnd Class\n", 2},
		{"namespace Demo\nlet run x = x\n", 7},
		{"public ref class Box {};\n", 8},
		{"package Demo { function run() {} }\n", 9},
		{".assembly Demo {}\n.class public Box {}\n", 10},
		{"function Get-Value {}\n", 11},
		{"<%@ Language=\"JScript\" %><% function run() {} %>", 12},
		{"@functions { public void Run() {} }\n", 14},
		{`<Window x:Class="Demo.View" />`, 16},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := []SourceAnalyzer{
			VB6Analyzer{}, VBAAnalyzer{}, VBScriptAnalyzer{}, QBasicAnalyzer{}, ClassicBasicAnalyzer{}, FreeBasicAnalyzer{}, PureBasicAnalyzer{},
			FSharpAnalyzer{}, CPPCLIAnalyzer{}, JScriptNetAnalyzer{}, CILAnalyzer{}, PowerShellAnalyzer{}, ClassicASPAnalyzer{}, ASPNetWebFormsAnalyzer{}, RazorAnalyzer{}, BlazorAnalyzer{}, XAMLAnalyzer{},
		}
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 128))
		if err != nil {
			if kind := operation.KindOf(err); kind != operation.KindInvalidInput && kind != operation.KindLimit && kind != operation.KindUnsupported {
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, kind)
			}
			return
		}
		if len(result.Analysis.Symbols) > 128 {
			t.Fatalf("%s fuzz result exceeded symbol bound: %d", analyzer.Language(), len(result.Analysis.Symbols))
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "" || symbol.QualifiedName == "" || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid symbol: %+v", analyzer.Language(), symbol)
			}
		}
	})
}
