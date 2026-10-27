package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestJSPXMLCodeRegionsReportIncompleteCoverage(t *testing.T) {
	for _, tag := range []string{"scriptlet", "expression"} {
		t.Run(tag, func(t *testing.T) {
			source := "<main id=\"home\"></main>\n<jsp:" + tag + ">\n" +
				"String getDateTimeStr(java.util.Locale locale) { return \"\"; }\n" +
				"</jsp:" + tag + ">\n"
			result, err := (JSPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete {
				t.Fatal("unparsed JSP XML code was reported completely analyzed")
			}
			found := false
			for _, diagnostic := range result.Analysis.Diagnostics {
				if diagnostic.Code == "jsp-xml-code-unsupported" {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected JSP XML coverage diagnostic, got %+v", result.Analysis.Diagnostics)
			}
			if _, exists := symbolsByQualifiedName(result.Analysis.Symbols)["home"]; !exists {
				t.Fatal("JSP host HTML symbol was lost")
			}
		})
	}
}

func TestJSPXMLCodeInsideHTMLCommentDoesNotReduceCoverage(t *testing.T) {
	source := "<!-- <jsp:declaration>String fake() { return \"\"; }</jsp:declaration> -->\n<main id=\"home\"></main>"
	result, err := (JSPAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("comment-only JSP XML text reduced coverage: %+v", result.Analysis.Diagnostics)
	}
	for _, diagnostic := range result.Analysis.Diagnostics {
		if strings.HasPrefix(diagnostic.Code, "jsp-xml-code") {
			t.Fatalf("comment-only JSP XML tag reported unsupported: %+v", diagnostic)
		}
	}
}
