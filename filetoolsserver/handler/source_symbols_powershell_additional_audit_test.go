package handler

import (
	"os"
	"testing"
)

// Source-first original PowerShell scripts are read only, never executed.
func TestSourceSymbolsOriginalPowerShellAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original PowerShell corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "csharp-automapper", language: "powershell",
			search: "Exec", primary: "Build.ps1",
			negativeFile: "Push.ps1", negativeQuery: "Exec",
			paths: []string{"Build.ps1", "Setup.ps1", "Push.ps1"},
			oracles: []extraAuditEvidence{
				{"Build.ps1", "function Exec"},
				{"Build.ps1", "if ($lastexitcode -ne 0)"},
				{"Build.ps1", "dotnet test $solution"},
				{"Setup.ps1", "function Exec"},
				{"Setup.ps1", "Start-BitsTransfer"},
				{"Push.ps1", "dotnet nuget push"},
				{"src/AutoMapper/ApiCompat/PreBuild.ps1", "param([string]$version)"},
			},
			targets: []extraAuditSymbol{{"Build.ps1", "Exec", 12}, {"Setup.ps1", "Exec", 12}},
		},
		{
			project: "csharp-humanizer", language: "powershell",
			search: "Get-ApiDigest", primary: "tools/docs/verify-api.ps1",
			negativeFile:  "scripts/coverage-branch-hotspots.ps1",
			negativeQuery: "Get-ApiDigest",
			paths:         []string{"tools/docs/verify-api.ps1", "tools/docs/verify-aot.ps1", "scripts/coverage-branch-hotspots.ps1"},
			oracles: []extraAuditEvidence{
				{"tools/docs/verify-api.ps1", "function Get-ApiDigest"},
				{"tools/docs/verify-api.ps1", "function Get-CheckoutApiInput"},
				{"tools/docs/verify-api.ps1", "snapshot-state.ps1"},
				{"tools/docs/build.ps1", "verify-manifest.ps1"},
				{"tools/docs/verify-aot.ps1", "function Invoke-PublishedExample"},
				{"scripts/coverage-branch-hotspots.ps1", "function Get-AttributeValue"},
			},
			targets: []extraAuditSymbol{
				{"tools/docs/verify-api.ps1", "Get-ApiDigest", 44},
				{"tools/docs/verify-api.ps1", "Get-CheckoutApiInput", 68},
				{"tools/docs/verify-aot.ps1", "Invoke-PublishedExample", 78},
				{"scripts/coverage-branch-hotspots.ps1", "Get-AttributeValue", 13},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
