package handler

import (
	"os"
	"testing"
)

func TestSourceSymbolsOriginalVBScriptAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original VBScript corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project:  "vbnet-rhino-samples",
			language: "vbscript", search: "BatchRender",
			primary:       "rhinoscript/BatchRender.vbs",
			negativeFile:  "rhinoscript/AddStartupScripts.vbs",
			negativeQuery: "BatchRender",
			paths: []string{
				"rhinoscript/BatchRender.vbs",
				"rhinoscript/RhinoRender.vbs",
				"rhinoscript/AddStartupScripts.vbs",
			},
			oracles: []extraAuditEvidence{
				{"rhinoscript/BatchRender.vbs", "Call BatchRender"},
				{"rhinoscript/BatchRender.vbs", "sFolder = \"C:\\Test\""},
				{"rhinoscript/BatchRender.vbs", "If (InStr(LCase(sFile), \".3dm\") > 0) Then"},
				{"rhinoscript/BatchRender.vbs", "sBitmap = Replace(sFile, \".3dm\", \".jpg\", 1, -1, 1)"},
				{"rhinoscript/BatchRender.vbs", "oRhinoScript.Command \"_-SaveRenderWindowAs \""},
				{"rhinoscript/BatchRender.vbs", "oRhinoScript.Command \"_-CloseRenderWindow\""},
				{"rhinoscript/RhinoRender.vbs", "sBitmap = Replace(sFile, \".3dm\", \".png\", 1, -1, 1)"},
				{"rhinoscript/RhinoRender.vbs", "Call oRhinoScript.Command(\"_-Render\")"},
				{"rhinoscript/AddStartupScripts.vbs", "Sub AddStartupScriptFile(strFile)"},
				{"rhinoscript/AddStartupScripts.vbs", "Call objShell.RegWrite(strKey, strFile, \"REG_SZ\")"},
			},
			targets: []extraAuditSymbol{
				{"rhinoscript/BatchRender.vbs", "BatchRender", 8},
				{"rhinoscript/AddStartupScripts.vbs", "AddStartupScripts", 21},
				{"rhinoscript/AddStartupScripts.vbs", "AddStartupScriptFile", 33},
			},
		},
		{
			project:  "vbscript-abbodi1406-batutil",
			language: "vbscript", search: "openMode",
			primary:       "OfficeUpsourcer/2007/_bin/WiRunSQL.vbs",
			negativeFile:  "OfficeUpsourcer/2007/_bin/WiSumInf.vbs",
			negativeQuery: "openMode",
			paths: []string{
				"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs",
				"OfficeUpsourcer/2007/_bin/WiCodepage.vbs",
				"OfficeUpsourcer/2007/_bin/WiSumInf.vbs",
			},
			oracles: []extraAuditEvidence{
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "Const msiOpenDatabaseModeReadOnly = 0"},
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "Dim openMode : openMode = msiOpenDatabaseModeReadOnly"},
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "If InStr(1, \"UPDATE INSERT DELETE CREATE ALTER DROP\", keyword, vbTextCompare) Then"},
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "Set database = installer.OpenDatabase(databasePath, openMode) : CheckError"},
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "If openMode = msiOpenDatabaseModeTransact Then database.Commit"},
				{"OfficeUpsourcer/2007/_bin/WiCodepage.vbs", "If argCount >= 2 Then openMode = msiOpenDatabaseModeTransact Else openMode = msiOpenDatabaseModeReadOnly"},
				{"OfficeUpsourcer/2007/_bin/WiCodepage.vbs", "Sub SetDatabaseCodepage(database, codepage)"},
				{"OfficeUpsourcer/2007/_bin/WiSumInf.vbs", "Set sumInfo = installer.SummaryInformation("},
				{"OfficeUpsourcer/2007/_bin/WiSumInf.vbs", "sumInfo.Persist"},
			},
			targets: []extraAuditSymbol{
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "openMode", 24},
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "CheckError", 70},
				{"OfficeUpsourcer/2007/_bin/WiRunSQL.vbs", "Fail", 81},
				{"OfficeUpsourcer/2007/_bin/WiCodepage.vbs", "openMode", 27},
				{"OfficeUpsourcer/2007/_bin/WiCodepage.vbs", "DatabaseCodepage", 42},
				{"OfficeUpsourcer/2007/_bin/WiCodepage.vbs", "SetDatabaseCodepage", 59},
				{"OfficeUpsourcer/2007/_bin/WiSumInf.vbs", "CheckError", 113},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
