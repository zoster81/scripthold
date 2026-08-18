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

func TestConformanceAcrossLegacyEncodingsUnicodeAndDeterminism(t *testing.T) {
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
			options := testAnalyzeOptions(true, 256)
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

func TestDetectorKeepsSharedBASAmbiguousAndRoutesDistinctFormats(t *testing.T) {
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

func TestOpaqueAndDynamicBoundariesDoNotLeakDeclarations(t *testing.T) {
	powerShellText := "@\"\nfunction Fake-InHereString { }\n\"@\nfunction Real-Function { }\n"
	ps, err := (PowerShellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(powerShellText), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	psNames := sortedSymbolQualifiedNames(ps.Analysis.Symbols)
	if containsSortedString(psNames, "Fake-InHereString") || !containsSortedString(psNames, "Real-Function") {
		t.Fatalf("PowerShell here-string boundary symbols=%v", psNames)
	}

	assignedHereString := "$newPrompt = @'\nfunction Fake-InAssignedHereString { }\n'@\nfunction Real-AssignedFunction { }\n"
	assigned, err := (PowerShellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(assignedHereString), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	assignedNames := sortedSymbolQualifiedNames(assigned.Analysis.Symbols)
	if !assigned.Analysis.CoverageComplete || containsSortedString(assignedNames, "Fake-InAssignedHereString") || !containsSortedString(assignedNames, "Real-AssignedFunction") {
		t.Fatalf("PowerShell assigned here-string boundary analysis=%+v symbols=%v", assigned.Analysis, assignedNames)
	}

	commentMarker := "# @'\nfunction Real-AfterComment { }\n"
	commented, err := (PowerShellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(commentMarker), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	commentedNames := sortedSymbolQualifiedNames(commented.Analysis.Symbols)
	if !commented.Analysis.CoverageComplete || !containsSortedString(commentedNames, "Real-AfterComment") {
		t.Fatalf("PowerShell comment marker was treated as a here-string: analysis=%+v symbols=%v", commented.Analysis, commentedNames)
	}

	fsharpText := "(* outer (* type Fake = class end *) *)\nlet real value = \"type StringFake = class end\"\n"
	fs, err := (FSharpAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(fsharpText), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	fsNames := sortedSymbolQualifiedNames(fs.Analysis.Symbols)
	if containsSortedString(fsNames, "Fake") || containsSortedString(fsNames, "StringFake") || !containsSortedString(fsNames, "real") {
		t.Fatalf("F# opaque boundary symbols=%v", fsNames)
	}

	vbscriptText := "Execute \"Class DynamicFake : End Class\"\nClass Real\nEnd Class\n"
	vb, err := (VBScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(vbscriptText), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	vbNames := sortedSymbolQualifiedNames(vb.Analysis.Symbols)
	if containsSortedString(vbNames, "DynamicFake") || !containsSortedString(vbNames, "Real") {
		t.Fatalf("VBScript runtime/string boundary symbols=%v", vbNames)
	}
}

func TestRealWorldFSharpCompilerDirectivesAreIndentationNeutral(t *testing.T) {
	text := "type Service() =\n" +
		"    member _.Run() =\n" +
		"#if DEBUG\n" +
		"        let value = 1\n" +
		"#else\n" +
		"        let value = 2\n" +
		"#endif\n" +
		"        value\n" +
		"    member _.Next() = 2\n" +
		"let top value = value\n"
	result, err := (FSharpAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid F# compiler directives changed indentation state: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Service", "Service.Run", "Service.Next", "top"} {
		if _, ok := byName[name]; !ok {
			t.Fatalf("missing %s; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestMalformedAndCancellationBoundaries(t *testing.T) {
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
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
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
		_, err := analyzer.Analyze(ctx, sourceDocumentForScanner("class A {}\n"), testAnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}
}

func TestBasicDotNetCompositeGeneratedSourcesRespectSymbolLimits(t *testing.T) {
	cases := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"vb6", VB6Analyzer{}, generatedVBModule(1200, "VB6Module")},
		{"vba", VBAAnalyzer{}, generatedVBModule(1200, "VBAModule")},
		{"vbscript", VBScriptAnalyzer{}, generatedBasicFunctions(1200, "Function", "End Function")},
		{"qbasic", QBasicAnalyzer{}, generatedBasicFunctions(1200, "SUB", "END SUB")},
		{"classic-basic", ClassicBasicAnalyzer{}, generatedClassicBasic(1200)},
		{"freebasic", FreeBasicAnalyzer{}, generatedBasicFunctions(1200, "Sub", "End Sub")},
		{"purebasic", PureBasicAnalyzer{}, generatedBasicFunctions(1200, "Procedure", "EndProcedure")},
		{"fsharp", FSharpAnalyzer{}, generatedFSharp(1200)},
		{"cpp-cli", CPPCLIAnalyzer{}, generatedCPPCLI(1200)},
		{"jscript-net", JScriptNetAnalyzer{}, generatedJScriptNet(1200)},
		{"cil", CILAnalyzer{}, generatedCIL(1200)},
		{"powershell", PowerShellAnalyzer{}, generatedPowerShell(1200)},
		{"classic-asp", ClassicASPAnalyzer{}, generatedClassicASP(1200)},
		{"aspnet-webforms", ASPNetWebFormsAnalyzer{}, generatedWebForms(1200)},
		{"razor", RazorAnalyzer{}, generatedRazor(1200, "functions")},
		{"blazor", BlazorAnalyzer{}, generatedRazor(1200, "code")},
		{"xaml", XAMLAnalyzer{}, generatedXAML(1200)},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result: symbols=%d truncated=%v complete=%v diagnostics=%+v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
		})
	}
}

func generatedVBModule(count int, module string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Attribute VB_Name = %q\n", module)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "Public Sub S%04d()\nEnd Sub\n", i)
	}
	return b.String()
}

func generatedBasicFunctions(count int, open, close string) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "%s F%04d()\n%s\n", open, i, close)
	}
	return b.String()
}

func generatedClassicBasic(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "%d DEF FN%04d(X)=X\n", 10+i*10, i)
	}
	return b.String()
}

func generatedFSharp(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "let f%04d value = value\n", i)
	}
	return b.String()
}

func generatedCPPCLI(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "public ref class C%04d {};\n", i)
	}
	return b.String()
}

func generatedJScriptNet(count int) string {
	var b strings.Builder
	b.WriteString("package Generated {\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d() {}\n", i)
	}
	b.WriteString("}\n")
	return b.String()
}

func generatedCIL(count int) string {
	var b strings.Builder
	b.WriteString(".assembly Generated {}\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, ".class public C%04d {}\n", i)
	}
	return b.String()
}

func generatedPowerShell(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function Get-F%04d {}\n", i)
	}
	return b.String()
}

func generatedClassicASP(count int) string {
	var b strings.Builder
	b.WriteString("<%@ Language=\"VBScript\" %>\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "<%% Sub S%04d()\nEnd Sub %%>\n", i)
	}
	return b.String()
}

func generatedWebForms(count int) string {
	var b strings.Builder
	b.WriteString("<%@ Page Language=\"C#\" %>\n<script runat=\"server\">\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "public void M%04d() {}\n", i)
	}
	b.WriteString("</script>\n")
	return b.String()
}

func generatedRazor(count int, directive string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "@%s {\n", directive)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "public void M%04d() {}\n", i)
	}
	b.WriteString("}\n")
	return b.String()
}

func generatedXAML(count int) string {
	var b strings.Builder
	b.WriteString(`<Window x:Class="Generated.View" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml"><Grid>`)
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, `<Button x:Name="B%04d" />`, i)
	}
	b.WriteString("</Grid></Window>\n")
	return b.String()
}
