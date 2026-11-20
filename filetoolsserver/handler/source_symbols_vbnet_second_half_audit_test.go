package handler

import (
	"os"
	"testing"
)

// Provider 67: independent Mono Basic and dotnet/samples origins, not the
// initially qualified Community.VisualBasic and SmallVisualBasic projects.
// Original selected sources were read in full and independently grep-checked
// before these bounded navigation oracles were recorded; nothing is executed.
func TestSourceSymbolsVBNetSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original VB.NET projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "vbnet67-mono-basic-audit-selected", language: "vbnet",
			search: "RunProcess", primary: "vbnc/rt/source/ExternalProcessExecutor.vb",
			negativeFile: "vbnc/rt/source/TestExecutor.vb", negativeQuery: "RunProcess",
			paths: []string{
				"vbnc/rt/source/ExternalProcessExecutor.vb",
				"vbnc/rt/source/ExternalProcessVerification.vb",
				"vbnc/rt/source/TestExecutor.vb",
			},
			oracles: []extraAuditEvidence{
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "Public Function RunProcess() As Boolean"},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "process.StartInfo.RedirectStandardError = True"},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "m_TimedOut = Not process.WaitForExit(m_TimeOut)"},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "m_ExitCode = Integer.MinValue"},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "Private Sub DeleteFilesAndDirectories(ByVal state As Object)"},
				{"vbnc/rt/source/ExternalProcessVerification.vb", "Protected Overrides Function RunVerification() As Boolean"},
				{"vbnc/rt/source/ExternalProcessVerification.vb", "result = m_Process.RunProcess()"},
				{"vbnc/rt/source/ExternalProcessVerification.vb", "If m_Process.ExitCode <> Me.ExpectedExitCode Then"},
				{"vbnc/rt/source/TestExecutor.vb", "Sub RunAsync(ByVal Test As Test, Optional ByVal Priority As Boolean = False)"},
				{"vbnc/rt/source/TestExecutor.vb", "Private Sub Runner()"},
				{"vbnc/rt/source/TestExecutor.vb", "Test.DoTest()"},
			},
			targets: []extraAuditSymbol{
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "ExternalProcessExecutor", 21},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "ExpandCmdLine", 147},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "ExpandCmdLine", 158},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "OutputReader", 170},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "ErrorReader", 185},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "RunProcess", 201},
				{"vbnc/rt/source/ExternalProcessExecutor.vb", "DeleteFilesAndDirectories", 294},
				{"vbnc/rt/source/ExternalProcessVerification.vb", "RunVerification", 47},
				{"vbnc/rt/source/TestExecutor.vb", "TestExecutor", 20},
				{"vbnc/rt/source/TestExecutor.vb", "Runner", 99},
				{"vbnc/rt/source/TestExecutor.vb", "RunAsync", 151},
				{"vbnc/rt/source/TestExecutor.vb", "RunAsync", 182},
				{"vbnc/rt/source/TestExecutor.vb", "RunAsyncTree", 206},
				{"vbnc/rt/source/TestExecutor.vb", "Pause", 238},
			},
		},
		{
			project: "vbnet67-dotnet-samples-audit-selected", language: "vbnet",
			search: "CreateFallbackBuffer", primary: "core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb",
			negativeFile: "core/encoding/cyrillic-to-latin/vb/ConsoleModule.vb", negativeQuery: "CreateFallbackBuffer",
			paths: []string{
				"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb",
				"core/encoding/cyrillic-to-latin/vb/ConsoleModule.vb",
			},
			oracles: []extraAuditEvidence{
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Public Class CyrillicToLatinFallback : Inherits EncoderFallback"},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Public Overrides Function CreateFallbackBuffer() As System.Text.EncoderFallbackBuffer"},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Return New CyrillicToLatinFallbackBuffer(table)"},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Public Overloads Overrides Function Fallback(ByVal charUnknownHigh As Char, ByVal charUnknownLow As Char, ByVal index As Integer) As Boolean"},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Public Overrides Function GetNextChar() As Char"},
				{"core/encoding/cyrillic-to-latin/vb/ConsoleModule.vb", "Sub Main()"},
				{"core/encoding/cyrillic-to-latin/vb/ConsoleModule.vb", "New CyrillicToLatinFallback(), New DecoderExceptionFallback()"},
				{"core/encoding/cyrillic-to-latin/vb/ConsoleModule.vb", "Private Sub ShowSyntax()"},
			},
			targets: []extraAuditSymbol{
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "CyrillicToLatinFallback", 4},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "CreateFallbackBuffer", 79},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "MaxCharCount", 83},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "CyrillicToLatinFallbackBuffer", 90},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Fallback", 102},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Fallback", 107},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "GetNextChar", 117},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "MovePrevious", 129},
				{"core/encoding/cyrillic-to-latin/vb/CyrillicToLatinFallback.vb", "Remaining", 138},
				{"core/encoding/cyrillic-to-latin/vb/ConsoleModule.vb", "Main", 5},
				{"core/encoding/cyrillic-to-latin/vb/ConsoleModule.vb", "ShowSyntax", 68},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
