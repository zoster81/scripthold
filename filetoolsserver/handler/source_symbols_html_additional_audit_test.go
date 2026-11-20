package handler

import (
	"os"
	"testing"
)

// The independent HTML source is read only. ASP.NET, EJS and browser code are
// not executed; the checks establish bounded markup navigation, not rendering.
func TestSourceSymbolsOriginalHTMLAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original HTML corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "classic-asp-zynk-samples", language: "html",
			search: "Form1", primary: "ASP.Net/Upload.aspx",
			negativeFile: "ASP/upload.asp", negativeQuery: "Form1",
			paths: []string{"ASP.Net/Upload.aspx", "ASP.Net/Download.aspx"},
			oracles: []extraAuditEvidence{
				{"ASP.Net/Upload.aspx", "Codebehind=\"Upload.aspx.cs\""},
				{"ASP.Net/Upload.aspx", "Inherits=\"ConnectSamples.Upload\""},
				{"ASP.Net/Upload.aspx", "<title>Upload</title>"},
				{"ASP.Net/Upload.aspx", "<form id=\"Form1\" method=\"post\" runat=\"server\">"},
				{"ASP.Net/Download.aspx", "Codebehind=\"Download.aspx.cs\""},
				{"ASP.Net/Download.aspx", "Inherits=\"ConnectSamples.Download\""},
				{"ASP.Net/Download.aspx", "<title>Download</title>"},
				{"ASP.Net/Download.aspx", "<form id=\"Form1\" method=\"post\" runat=\"server\">"},
				{"ASP.Net/Upload.aspx.cs", "private void UpdateProductGroups(XmlDocument document)"},
				{"ASP.Net/Download.aspx.cs", "private void UpdateRecords(string input)"},
				{"ASP/upload.asp", "Function CleanXML(xml)"},
			},
			targets: []extraAuditSymbol{
				{"ASP.Net/Upload.aspx", "Form1", 12},
				{"ASP.Net/Download.aspx", "Form1", 12},
			},
		},
		{
			project: "html-gchq-cyberchef", language: "html",
			search: "preloader-msg", primary: "src/web/html/index.html",
			negativeFile: "src/web/static/ga.html", negativeQuery: "preloader-msg",
			paths: []string{"src/web/html/index.html", "src/web/static/ga.html"},
			oracles: []extraAuditEvidence{
				{"src/web/html/index.html", "function changeLoadingMsg() {"},
				{"src/web/html/index.html", "document.getElementById(\"preloader-msg\")"},
				{"src/web/html/index.html", "window.addEventListener(\"error\", loadingErrorHandler);"},
				{"src/web/html/index.html", "<div id=\"loader-wrapper\">"},
				{"src/web/html/index.html", "<div id=\"preloader-msg\" class=\"loading-msg\"></div>"},
				{"src/web/html/index.html", "<div id=\"preloader-error\" class=\"loading-error\"></div>"},
				{"src/web/html/index.html", "<div id=\"recipe\" class=\"split split-horizontal no-select\""},
				{"src/web/html/index.html", "<div id=\"input\" class=\"split no-select\""},
				{"src/web/html/index.html", "<div id=\"output\" class=\"split\""},
				{"src/web/App.mjs", "// Trigger CSS animations to remove preloader"},
				{"src/web/static/ga.html", "<!-- Begin Google Analytics -->"},
			},
			targets: []extraAuditSymbol{
				{"src/web/html/index.html", "loader-wrapper", 139},
				{"src/web/html/index.html", "preloader", 140},
				{"src/web/html/index.html", "preloader-msg", 141},
				{"src/web/html/index.html", "preloader-error", 142},
				{"src/web/html/index.html", "recipe", 181},
				{"src/web/html/index.html", "input", 225},
				{"src/web/html/index.html", "input-text", 275},
				{"src/web/html/index.html", "output", 279},
				{"src/web/html/index.html", "output-text", 333},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
