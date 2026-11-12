package handler

import (
	"os"
	"testing"
)

// Provider 71: dotnet/aspnetcore and dotnet/AspNetCore.Docs are independent
// original Razor project origins, not previously qualified ABP or storefronts.
// Selected .cshtml files were fully read and pinned to upstream Git blobs
// before this source-derived handler navigation was tested.
func TestSourceSymbolsRazorSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Razor project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "razor71-aspnetcore-audit-selected", language: "razor",
			search:        "CompilationErrorPage",
			primary:       "src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml",
			negativeFile:  "src/Shared/ErrorPage/Views/ErrorPage.cshtml",
			negativeQuery: "CompilationErrorPage",
			paths: []string{
				"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml",
				"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/ErrorPage.cshtml",
				"src/Shared/ErrorPage/Views/ErrorPage.cshtml",
			},
			oracles: []extraAuditEvidence{
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml", "public CompilationErrorPage(CompilationErrorPageModel model)"},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml", "public CompilationErrorPageModel Model { get; set; }"},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml", "Response.StatusCode = 500;"},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml", "errorDetail.ErrorMessage"},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml", "Model.CompiledContent[i]"},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/ErrorPage.cshtml", "public ErrorPage(ErrorPageModel model)"},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/ErrorPage.cshtml", "@if (Model.Query.Any())"},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/ErrorPage.cshtml", "@if (Model.Endpoint != null)"},
				{"src/Shared/ErrorPage/Views/ErrorPage.cshtml", "public ErrorPage(ErrorPageModel model)"},
				{"src/Shared/ErrorPage/Views/ErrorPage.cshtml", "An error occurred while starting the application."},
				{"src/Shared/ErrorPage/Views/ErrorPage.cshtml", "@if (Model.ShowRuntimeDetails)"},
			},
			targets: []extraAuditSymbol{
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml", "CompilationErrorPage", 9},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/CompilationErrorPage.cshtml", "Model", 14},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/ErrorPage.cshtml", "ErrorPage", 10},
				{"src/Middleware/Diagnostics/src/DeveloperExceptionPage/Views/ErrorPage.cshtml", "Model", 15},
				{"src/Shared/ErrorPage/Views/ErrorPage.cshtml", "ErrorPage", 10},
				{"src/Shared/ErrorPage/Views/ErrorPage.cshtml", "Model", 15},
			},
		},
		{
			project: "razor71-aspnetcore-docs-audit-selected", language: "razor",
			search:        "GetAntiXsrfRequestToken",
			primary:       "aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml",
			negativeFile:  "aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Contact.cshtml",
			negativeQuery: "GetAntiXsrfRequestToken",
			paths: []string{
				"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml",
				"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Index.cshtml",
				"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Contact.cshtml",
			},
			oracles: []extraAuditEvidence{
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml", "@inject Microsoft.AspNetCore.Antiforgery.IAntiforgery Xsrf"},
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml", "public string GetAntiXsrfRequestToken()"},
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml", "Xsrf.GetAndStoreTokens(Context).RequestToken"},
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml", "name=\"RequestVerificationToken\" value=\"@GetAntiXsrfRequestToken()\""},
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml", "xhttp.setRequestHeader(\"RequestVerificationToken\","},
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Index.cshtml", "<form asp-action=\"SamplePost\" method=\"post\">"},
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Index.cshtml", "@Html.AntiForgeryToken()"},
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Contact.cshtml", "<h2>@ViewData[\"Title\"].</h2>"},
			},
			targets: []extraAuditSymbol{
				{"aspnetcore/security/anti-request-forgery/samples/2.x/MvcSample/Views/Home/Ajax.cshtml", "GetAntiXsrfRequestToken", 6},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
