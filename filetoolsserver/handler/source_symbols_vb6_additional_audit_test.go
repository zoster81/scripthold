package handler

import (
	"os"
	"testing"
)

func TestSourceSymbolsOriginalVB6AdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original VB6 corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project:  "vb6-apress",
			language: "vb6", search: "openTheDatabase",
			primary:       "BegDB/Chapter10/modCode.bas",
			negativeFile:  "BegDB/Chapter10/frmCall.frm",
			negativeQuery: "openTheDatabase",
			paths: []string{
				"BegDB/Chapter10/modCode.bas",
				"BegDB/Chapter10/frmAddress.frm",
				"BegDB/Chapter10/frmCall.frm",
			},
			oracles: []extraAuditEvidence{
				{"BegDB/Chapter10/prjAddress.vbp", "Startup=\"frmAddress\""},
				{"BegDB/Chapter10/prjAddress.vbp", "Form=frmCall.frm"},
				{"BegDB/Chapter10/prjAddress.vbp", "Module=modCode; modCode.bas"},
				{"BegDB/Chapter10/modCode.bas", "Public Function openTheDatabase() As Boolean"},
				{"BegDB/Chapter10/modCode.bas", "dbPath = App.Path & \"\\contacts.mdb\""},
				{"BegDB/Chapter10/modCode.bas", "Set rsContactTable = dbContact.OpenRecordset(\"Contact\", dbOpenTable)"},
				{"BegDB/Chapter10/modCode.bas", "openTheDatabase = False"},
				{"BegDB/Chapter10/frmAddress.frm", "If (Not openTheDatabase()) Then"},
				{"BegDB/Chapter10/frmAddress.frm", "rsContactTable.AddNew"},
				{"BegDB/Chapter10/frmAddress.frm", "dbContact.Close"},
				{"BegDB/Chapter10/frmCall.frm", "Set rsNotes = dbContact.OpenRecordset(\"Notes\", dbOpenTable)"},
				{"BegDB/Chapter10/frmCall.frm", "rsNotes.Update"},
			},
			targets: []extraAuditSymbol{
				{"BegDB/Chapter10/modCode.bas", "openTheDatabase", 27},
				{"BegDB/Chapter10/frmAddress.frm", "Form_Load", 699},
				{"BegDB/Chapter10/frmAddress.frm", "postContact", 1284},
				{"BegDB/Chapter10/frmCall.frm", "cmdCallAdd_Click", 219},
			},
		},
		{
			project:  "vb6-gagniuc-discrete-probability-detector-in-vb6",
			language: "vb6", search: "Discrete_Probability_Detector",
			primary: "DPD.frm", negativeFile: "DPD.frm", negativeQuery: "Caption",
			paths: []string{"DPD.frm"},
			oracles: []extraAuditEvidence{
				{"DPD.vbp", "Form=DPD.frm"},
				{"DPD.vbp", "Startup=\"DPD\""},
				{"DPD.frm", "Private Sub MakeMatrix_Click()"},
				{"DPD.frm", "Discrete_Probability_Detector (InP.Text)"},
				{"DPD.frm", "Function Discrete_Probability_Detector(ByVal S As String)"},
				{"DPD.frm", "m(r, c) = Val(m(r, c)) + 1"},
				{"DPD.frm", "If Val(e(i, 1)) > 0 Then"},
				{"DPD.frm", "m(i, j) = Val(m(i, j)) / Val(e(i, 1))"},
				{"DPD.frm", "MatrixPaint(w, d, m, a, \"(P)\", \"Transition matrix M:\")"},
				{"DPD.frm", "OutPut.Text = tmp"},
				{"DPD.frm", "Private Sub Form_Resize()"},
				{"DPD.frm", "Caption         =   \"Discrete Probability Detector\""},
			},
			targets: []extraAuditSymbol{
				{"DPD.frm", "MakeMatrix_Click", 153},
				{"DPD.frm", "Discrete_Probability_Detector", 157},
				{"DPD.frm", "MatrixPaint", 223},
				{"DPD.frm", "Form_Resize", 267},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
