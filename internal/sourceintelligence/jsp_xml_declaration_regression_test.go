package sourceintelligence

import (
	"context"
	"os"
	"testing"
)

func TestJSPXMLDeclarationExportsJavaMethod(t *testing.T) {
	source := `<?xml version="1.0"?>
<jsp:root xmlns:jsp="http://java.sun.com/JSP/Page" version="1.2">
<jsp:declaration>
  String getDateTimeStr(Locale l) {
    return l.toString();
  }
</jsp:declaration>
<main id="hero"></main>
<jsp:scriptlet><![CDATA[
  for (int i = 0; i < 10; i++) { out.println(i); }
]]></jsp:scriptlet>
<jsp:expression>getDateTimeStr(request.getLocale())</jsp:expression>
</jsp:root>`
	result, err := (JSPAnalyzer{}).Analyze(context.Background(), testSourceDocument("xml.jsp", source), testAnalyzeOptions(true, 96))
	if err != nil {
		t.Fatal(err)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, kind := range map[string]SymbolKind{
		"hero":           SymbolKindEntity,
		"getDateTimeStr": SymbolKindMethod,
	} {
		symbol, found := symbols[name]
		if !found || symbol.Kind != kind {
			t.Errorf("%s = %+v present=%v; symbols=%v", name, symbol, found, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		if name == "getDateTimeStr" && found {
			if symbol.RegionID == "" || symbol.DeclarationRange.Start.Line != 4 {
				t.Errorf("Java declaration lost XML region or source position: %+v", symbol)
			}
		}
	}
	if result.Analysis.CoverageComplete {
		t.Fatal("unsupported JSP XML scriptlet/expression must retain incomplete coverage")
	}
	if !containsJSPDiagnostic(result.Analysis.Diagnostics, "jsp-xml-code-unsupported") {
		t.Fatalf("missing unsupported XML code diagnostic: %+v", result.Analysis.Diagnostics)
	}
}

func TestJSPXMLDeclarationMixedWithClassicScriptlets(t *testing.T) {
	source := `<%! class ClassicHelper { void run() {} } %>
<jsp:declaration>
class XMLHelper { void call() {} }
</jsp:declaration>
<main id="target"></main>`
	result, err := (JSPAnalyzer{}).Analyze(context.Background(), testSourceDocument("mixed.jsp", source), testAnalyzeOptions(true, 96))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("supported JSP regions unexpectedly incomplete: %+v", result.Analysis)
	}
	found := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, kind := range map[string]SymbolKind{
		"ClassicHelper": SymbolKindClass, "ClassicHelper.run": SymbolKindMethod,
		"XMLHelper": SymbolKindClass, "XMLHelper.call": SymbolKindMethod,
		"target": SymbolKindEntity,
	} {
		symbol, ok := found[name]
		if !ok || symbol.Kind != kind {
			t.Errorf("%s: present=%v kind=%q want=%q", name, ok, symbol.Kind, kind)
		}
		if ok && name != "target" && symbol.RegionID == "" {
			t.Errorf("%s missing JSP region ID", name)
		}
	}
}

func TestJSPXMLDeclarationCDATAAndMalformedStayIncomplete(t *testing.T) {
	for _, input := range []string{
		"<jsp:declaration><![CDATA[ class Ghost {} ]]></jsp:declaration>",
		"<jsp:declaration>class Ghost {}",
		"<jsp:declaration attr=\"x\">class Ghost {}</jsp:declaration>",
	} {
		result, err := (JSPAnalyzer{}).Analyze(context.Background(), testSourceDocument("unsupported.jsp", input), testAnalyzeOptions(false, 48))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || !containsJSPDiagnostic(result.Analysis.Diagnostics, "jsp-xml-code-unsupported") {
			t.Errorf("incomplete XML declaration was claimed complete: %+v", result.Analysis)
		}
		if _, leaked := symbolsByQualifiedName(result.Analysis.Symbols)["Ghost"]; leaked {
			t.Error("unsupported JSP XML syntax leaked a Java declaration")
		}
	}
}

func TestJSPXMLDeclarationTextInsideScriptletIsNotARegion(t *testing.T) {
	source := `<jsp:scriptlet><![CDATA[
String template = "<jsp:declaration>class Fake {}</jsp:declaration>";
]]></jsp:scriptlet>
<main id="real"></main>`
	result, err := (JSPAnalyzer{}).Analyze(context.Background(), testSourceDocument("scriptlet.jsp", source), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || !containsJSPDiagnostic(result.Analysis.Diagnostics, "jsp-xml-code-unsupported") {
		t.Fatalf("unsupported scriptlet should retain incomplete coverage: %+v", result.Analysis)
	}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Name == "Fake" {
			t.Errorf("scriptlet text generated a fake declaration: %+v", symbol)
		}
	}
}

// Opt-in: replay an actual JSP XML document without depending on a private
// corpus path in the repository's ordinary regression suite.
func TestJSPXMLRealFileOracle(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_JSP_XML_PATH")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_JSP_XML_PATH to run the real-file oracle")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (JSPAnalyzer{}).Analyze(context.Background(), testSourceDocument("xml.jsp", string(data)), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	method, found := byName["getDateTimeStr"]
	if !found || method.Kind != SymbolKindMethod || method.DeclarationRange.Start.Line != 25 || method.RegionID == "" {
		t.Fatalf("real JSP XML declaration not located at line 25: %+v exists=%v", method, found)
	}
	if result.Analysis.CoverageComplete || !containsJSPDiagnostic(result.Analysis.Diagnostics, "jsp-xml-code-unsupported") {
		t.Fatalf("real JSP XML CDATA/expression unsupported boundaries were lost: %+v", result.Analysis)
	}
}

func containsJSPDiagnostic(diagnostics []AnalysisDiagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
