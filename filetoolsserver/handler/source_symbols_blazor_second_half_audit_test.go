package handler

import (
	"os"
	"testing"
)

// Provider 70: dotnet/blazor-samples and radzenhq/radzen-blazor are independent
// original project origins, not the earlier Blazorise and MudBlazor projects.
// Selected source files were fully read and pinned upstream before these checks.
func TestSourceSymbolsBlazorSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Blazor projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "blazor70-dotnet-samples-audit-selected", language: "blazor",
			search:        "OnInputFileChange",
			primary:       "10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor",
			negativeFile:  "10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload1.razor",
			negativeQuery: "OnInputFileChange",
			paths: []string{
				"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload1.razor",
				"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor",
				"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload3.razor",
			},
			oracles: []extraAuditEvidence{
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload1.razor", "<InputFile OnChange=\"LoadFiles\" multiple />"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload1.razor", "e.GetMultipleFiles(maxAllowedFiles)"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload1.razor", "file.OpenReadStream(maxFileSize).CopyToAsync(fs)"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "<InputFile OnChange=\"OnInputFileChange\" multiple />"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "protected override bool ShouldRender() => shouldRender;"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "new LazyBrowserFileStream(file, maxFileSize)"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "await client.PostAsync(\"https://localhost:5001/Filesave\","},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "result = uploadResults.SingleOrDefault(f => f.FileName == fileName) ?? new();"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload3.razor", "<InputFile OnChange=\"LoadFiles\" multiple />"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload3.razor", "progressPercent = Decimal.Divide(totalRead, file.Size);"},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload3.razor", "StateHasChanged();"},
			},
			targets: []extraAuditSymbol{
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload1.razor", "LoadFiles", 60},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "ShouldRender", 61},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "OnInputFileChange", 63},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload2.razor", "FileUpload", 141},
				{"10.0/BlazorSample_BlazorWebApp/Components/Pages/FileUpload3.razor", "LoadFiles", 58},
			},
		},
		{
			project: "blazor70-radzen-audit-selected", language: "blazor",
			search:       "Cases",
			primary:      "Radzen.Blazor.TrimTest/ODataQueryCheck.razor",
			negativeFile: "Radzen.Blazor/RadzenButton.razor", negativeQuery: "Cases",
			paths: []string{
				"Radzen.Blazor.TrimTest/App.razor",
				"Radzen.Blazor.TrimTest/ODataQueryCheck.razor",
				"Radzen.Blazor/RadzenAIPrompt.razor",
				"Radzen.Blazor/RadzenButton.razor",
				"Radzen.Blazor/RadzenCheckBox.razor",
			},
			oracles: []extraAuditEvidence{
				{"Radzen.Blazor.TrimTest/App.razor", "<ODataQueryCheck />"},
				{"Radzen.Blazor.TrimTest/App.razor", "Workbook workbook = CreateWorkbook();"},
				{"Radzen.Blazor.TrimTest/App.razor", "Task OpenConfirmAsync() => DialogService.Confirm("},
				{"Radzen.Blazor.TrimTest/App.razor", "void OpenSideDialog() => DialogService.OpenSide<RadzenCard>("},
				{"Radzen.Blazor.TrimTest/App.razor", "protected override Task OnAfterRenderAsync(bool firstRender)"},
				{"Radzen.Blazor.TrimTest/App.razor", "status = \"done\";"},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "static string Filter(Expression<Func<Ticket, bool>> predicate)"},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "IEnumerable<(string Actual, string Expected)> Cases()"},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "yield return (Filter(ticket => ticket.Subject.ToLower().Contains(search.ToLower()))"},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "foreach (var (actual, expected) in Run())"},
				{"Radzen.Blazor/RadzenAIPrompt.razor", "<RadzenMarkdown Text=\"@Output\" />"},
				{"Radzen.Blazor/RadzenButton.razor", "@onclick=\"@OnClick\""},
				{"Radzen.Blazor/RadzenCheckBox.razor", "@onclick=@Toggle"},
			},
			targets: []extraAuditSymbol{
				{"Radzen.Blazor.TrimTest/App.razor", "CreateWorkbook", 76},
				{"Radzen.Blazor.TrimTest/App.razor", "OpenConfirmAsync", 83},
				{"Radzen.Blazor.TrimTest/App.razor", "OpenSideDialog", 85},
				{"Radzen.Blazor.TrimTest/App.razor", "OnAfterRenderAsync", 87},
				{"Radzen.Blazor.TrimTest/App.razor", "SetError", 114},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "Member", 14},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "Team", 20},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "Ticket", 36},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "Filter", 62},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "Cases", 64},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "OnInitialized", 109},
				{"Radzen.Blazor.TrimTest/ODataQueryCheck.razor", "Run", 127},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
