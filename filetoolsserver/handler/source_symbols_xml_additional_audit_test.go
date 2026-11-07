package handler

import (
	"os"
	"testing"
)

// Independent original MSBuild configurations are read only: these tests
// qualify edit-location navigation, not XML schema or build evaluation.
func TestSourceSymbolsOriginalXMLAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original XML corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "vbnet-cryptolens-examples", language: "xml",
			search:        "TargetFrameworkVersion",
			primary:       "SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj",
			negativeFile:  "SKM Examples/SKM Examples VBNET/packages.config",
			negativeQuery: "TargetFrameworkVersion",
			paths: []string{
				"SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj",
				"SKM Examples/SKM Examples VBNET/packages.config",
			},
			oracles: []extraAuditEvidence{
				{"SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj", "<TargetFrameworkVersion>v4.0</TargetFrameworkVersion>"},
				{"SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj", "<OptionExplicit>On</OptionExplicit>"},
				{"SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj", "<Compile Include=\"Module1.vb\" />"},
				{"SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj", "Newtonsoft.Json.6.0.8\\lib\\net40\\Newtonsoft.Json.dll"},
				{"SKM Examples/SKM Examples VBNET/packages.config", "<package id=\"Newtonsoft.Json\" version=\"6.0.8\" targetFramework=\"net4\" />"},
				{"SKM Examples/SKM Examples VBNET/packages.config", "<package id=\"SKGLExtension\" version=\"4.0.1.2\" targetFramework=\"net40\" />"},
				{"SKM Examples/SKM Examples VBNET/Module1.vb", "Private Sub GetKeysExample()"},
			},
			targets: []extraAuditSymbol{
				{"SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj", "TargetFrameworkVersion", 14},
				{"SKM Examples/SKM Examples VBNET/SKM Examples VBNET.vbproj", "OptionExplicit", 37},
				{"SKM Examples/SKM Examples VBNET/packages.config", "Newtonsoft.Json", 3},
			},
		},
		{
			project: "vbnet-sonar-dotnet", language: "xml",
			search: "LangVersion", primary: "analyzers/Common.targets",
			negativeFile: "analyzers/CodeAnalysis.targets", negativeQuery: "LangVersion",
			paths: []string{
				"analyzers/Common.targets",
				"analyzers/Version.targets",
				"analyzers/CodeAnalysis.targets",
				"analyzers/src/Directory.Build.targets",
				"analyzers/tests/Directory.Build.targets",
			},
			oracles: []extraAuditEvidence{
				{"analyzers/Common.targets", "<LangVersion>14</LangVersion>"},
				{"analyzers/Common.targets", "<RestorePackagesWithLockFile>true</RestorePackagesWithLockFile>"},
				{"analyzers/Common.targets", "<Import Project=\"$(MSBuildThisFileDirectory)\\Version.targets\" />"},
				{"analyzers/Version.targets", "<ShortVersion>10.33</ShortVersion>"},
				{"analyzers/CodeAnalysis.targets", "<PackageReference Include=\"StyleCop.Analyzers\" Version=\"1.2.0-beta.556\">"},
				{"analyzers/src/Directory.Build.targets", "<Import Project=\"$(MSBuildThisFileDirectory)\\..\\Common.targets\" />"},
				{"analyzers/tests/Directory.Build.targets", "<Import Project=\"$(MSBuildThisFileDirectory)\\..\\Common.targets\" />"},
				{"analyzers/src/SonarAnalyzer.VisualBasic/SonarAnalyzer.VisualBasic.csproj", "<TargetFramework>netstandard2.0</TargetFramework>"},
			},
			targets: []extraAuditSymbol{
				{"analyzers/Common.targets", "LangVersion", 8},
				{"analyzers/Common.targets", "RestorePackagesWithLockFile", 13},
				{"analyzers/Version.targets", "ShortVersion", 5},
				{"analyzers/src/Directory.Build.targets", "BinariesFolder", 7},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
