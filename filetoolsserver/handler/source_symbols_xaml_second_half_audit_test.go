package handler

import (
	"os"
	"testing"
)

// Both original projects are different from the earlier VCSamples/Rubberduck
// cases. Their selected XAML files have independent source and Git-blob oracles.
// XAML cannot infer C# code-behind execution or dynamic-resource resolution.
func TestSourceSymbolsXAMLSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original XAML project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "xaml84-blazor-samples-audit-selected", language: "xaml",
			search: "PageBackgroundColor", primary: "App.xaml",
			negativeFile: "MainPage.xaml", negativeQuery: "PageBackgroundColor",
			paths: []string{"App.xaml", "MainPage.xaml"},
			oracles: []extraAuditEvidence{
				{"App.xaml", "x:Class=\"MauiBlazorWeb.App\""},
				{"App.xaml", "x:Key=\"PageBackgroundColor\""},
				{"App.xaml", "x:Key=\"PrimaryTextColor\""},
				{"MainPage.xaml", "x:Class=\"MauiBlazorWeb.MainPage\""},
				{"MainPage.xaml", "BackgroundColor=\"{DynamicResource PageBackgroundColor}\""},
				{"MainPage.xaml", "x:Name=\"blazorWebView\""},
			},
			targets: []extraAuditSymbol{
				{"App.xaml", "MauiBlazorWeb.App", 5},
				{"App.xaml", "PageBackgroundColor", 9},
				{"App.xaml", "PrimaryTextColor", 10},
				{"MainPage.xaml", "MauiBlazorWeb.MainPage", 7},
				{"MainPage.xaml", "blazorWebView", 10},
			},
		},
		{
			project: "xaml84-windows-audit-selected", language: "xaml",
			search: "ShowToastButton", primary: "MainWindow.xaml",
			negativeFile: "App.xaml", negativeQuery: "ShowToastButton",
			paths: []string{"App.xaml", "MainWindow.xaml"},
			oracles: []extraAuditEvidence{
				{"App.xaml", "x:Class=\"DesktopToastsSample.App\""},
				{"App.xaml", "StartupUri=\"MainWindow.xaml\""},
				{"MainWindow.xaml", "x:Class=\"DesktopToastsSample.MainWindow\""},
				{"MainWindow.xaml", "x:Name=\"ShowToastButton\""},
				{"MainWindow.xaml", "x:Name=\"Output\""},
			},
			targets: []extraAuditSymbol{
				{"App.xaml", "DesktopToastsSample.App", 1},
				{"MainWindow.xaml", "DesktopToastsSample.MainWindow", 1},
				{"MainWindow.xaml", "ShowToastButton", 6},
				{"MainWindow.xaml", "Output", 7},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
