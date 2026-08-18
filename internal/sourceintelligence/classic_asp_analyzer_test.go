package sourceintelligence

import (
	"context"
	"reflect"
	"testing"
)

var _ SourceAnalyzer = ClassicASPAnalyzer{}

func TestClassicASPAnalyzerSegmentsAndDelegatesVBScript(t *testing.T) {
	text := "<%@ Language=\"VBScript\" %>\r\n" +
		"<html>\r\n" +
		"<!--#include file=\"inc/common.asp\" -->\r\n" +
		"<body>\r\n" +
		"<%\r\nSub Hello(name)\r\n    Response.Write name\r\nEnd Sub\r\n%>\r\n" +
		"<%= Hello(\"world\") %>\r\n" +
		"<script language=\"VBScript\" runat=\"server\">\r\nFunction Twice(x)\r\n    Twice = x * 2\r\nEnd Function\r\n</script>\r\n" +
		"</body>\r\n</html>\r\n"
	document := sourceDocumentForScanner(text)
	document.Path = "fixture.asp"
	options := AnalyzeOptions{IncludeSignatures: true, Limits: SymbolBuilderLimits{MaxSymbols: 128, MaxSignatureBytes: 4096, MaxDiagnostics: 32}}
	result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("Classic ASP analysis unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"Hello", "Twice"} {
		symbol, ok := byName[name]
		if !ok || symbol.Kind != SymbolKindMethod || symbol.Language != "vbscript" || symbol.Analyzer != string(AnalyzerClassicASP) || symbol.RegionID == "" {
			t.Fatalf("ASP symbol %s = %+v exists=%v", name, symbol, ok)
		}
	}
	if byName["Hello"].NameRange.Start.Line != 6 || byName["Twice"].NameRange.Start.Line != 12 {
		t.Fatalf("host coordinate mapping Hello=%+v Twice=%+v", byName["Hello"].NameRange.Start, byName["Twice"].NameRange.Start)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Kind != StructuralDependencyInclude || result.Dependencies[0].Value != "inc/common.asp" {
		t.Fatalf("ASP include dependencies = %+v", result.Dependencies)
	}
	var server, expression, directive, host int
	for _, region := range result.Regions {
		switch region.Kind {
		case "server-script":
			server++
		case "expression":
			expression++
		case "directive":
			directive++
		case "host":
			host++
		}
		if region.Range.Start.Line < 1 || region.Range.End.Line < region.Range.Start.Line {
			t.Fatalf("invalid region range: %+v", region)
		}
	}
	if server != 2 || expression != 1 || directive != 1 || host == 0 {
		t.Fatalf("region counts server=%d expression=%d directive=%d host=%d regions=%+v", server, expression, directive, host, result.Regions)
	}
	second, err := (ClassicASPAnalyzer{}).Analyze(context.Background(), document, options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, second) {
		t.Fatal("Classic ASP analyzer output is nondeterministic")
	}
}

func TestClassicASPAnalyzerPreservesOffsetsAcrossUnicodeCaseFolding(t *testing.T) {
	text := "<%@ȺȺȺ%>"
	document := sourceDocumentForScanner(text)
	document.Path = "unicode-offset.asp"
	result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 32, MaxSignatureBytes: 1024, MaxDiagnostics: 8}})
	if err != nil {
		t.Fatal(err)
	}
	for _, region := range result.Regions {
		if region.Range.Start.Line < 1 || region.Range.End.Line < region.Range.Start.Line {
			t.Fatalf("invalid region range after Unicode case folding: %+v", region)
		}
	}
}

func TestClassicASPAnalyzerReportsUnsupportedEmbeddedLanguage(t *testing.T) {
	text := `<%@ Language="PerlScript" %>
<html><body>
<% sub nope : end sub %>
<script language="PerlScript" runat="server">sub alsoNope</script>
</body></html>`
	document := sourceDocumentForScanner(text)
	document.Path = "unsupported.asp"
	result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(), document, AnalyzeOptions{Limits: SymbolBuilderLimits{MaxSymbols: 32, MaxSignatureBytes: 1024, MaxDiagnostics: 8}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
		t.Fatalf("unsupported PerlScript did not lower coverage: %+v", result.Analysis)
	}
	if len(result.Analysis.Symbols) != 0 {
		t.Fatalf("unsupported PerlScript was misparsed: %+v", result.Analysis.Symbols)
	}
	unsupported := 0
	for _, region := range result.Regions {
		if region.Kind == "server-script" && !region.Supported && region.Language == "perlscript" {
			unsupported++
		}
	}
	if unsupported != 2 {
		t.Fatalf("unsupported PerlScript regions=%d, regions=%+v", unsupported, result.Regions)
	}
}
