package handler

import (
	"os"
	"testing"
)

// Source oracles below are taken from the independent original BASIC programs,
// not from an installed connector or source_symbols output.
func TestSourceSymbolsOriginalClassicBasicAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original classic BASIC corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "cb-dualbrain", language: "classic-basic",
			search: "FNSS4", primary: "lanturn.bas",
			negativeFile: "spacesc.bas", negativeQuery: "FNSS4",
			paths: []string{"lanturn.bas", "spacesc.bas"},
			oracles: []extraAuditEvidence{
				{"lanturn.bas", "20 DEF FNSS4(W,H) = 4 + FIX("},
				{"lanturn.bas", "25 DEF FNCOLLIDE(X1,W1,O1,X2,W2,O2)"},
				{"lanturn.bas", "320   DIM SP1(FNSS4(32,64))"},
				{"lanturn.bas", "335   CLS: W = 64: H = 32: GOSUB 8000"},
				{"lanturn.bas", "8035 DIM SW1(FNSS4(64,32))"},
				{"lanturn.bas", "2000 ' -- MAIN LOOP --"},
				{"spacesc.bas", "50   DEF FNRAND (N)"},
				{"spacesc.bas", "60   DEF FNMOVE (N)"},
				{"spacesc.bas", "70   DEF FNSEEK (ME, YOU)"},
				{"spacesc.bas", "2140   IF WX = 0 THEN GOSUB 8500 ELSE GOSUB 8000"},
			},
			targets: []extraAuditSymbol{
				{"lanturn.bas", "FNSS4", 5},
				{"lanturn.bas", "FNCOLLIDE", 6},
				{"lanturn.bas", "310", 30},
				{"lanturn.bas", "335", 33},
				{"spacesc.bas", "FNRAND", 5},
			},
		},
		{
			project: "cb-gwbasic3d", language: "classic-basic",
			search: "5100", primary: "G3D.BAS",
			negativeFile: "G3D.BAS", negativeQuery: "77777",
			paths: []string{"G3D.BAS"},
			oracles: []extraAuditEvidence{
				{"README.md", "G3D.BAS"},
				{"G3D.BAS", " 1110 SZ = 4: SX = 2: SY = 1: GOSUB 5100"},
				{"G3D.BAS", " 1120 SZ = 5: SX = 3: SY = 4: GOSUB 5100"},
				{"G3D.BAS", " 1132 SY = FRAME: GOSUB 5825"},
				{"G3D.BAS", " 5030 ACTIVE = 1 - ACTIVE"},
				{"G3D.BAS", " 5100 M(SZ,0,0) = M(SX,0,0)*M(SY,0,0)"},
				{"G3D.BAS", " 5825 SF = -150"},
				{"G3D.BAS", " 5870 FOR SI = 0 TO BASE - 2"},
			},
			targets: []extraAuditSymbol{
				{"G3D.BAS", "5030", 53},
				{"G3D.BAS", "5100", 61},
				{"G3D.BAS", "5825", 90},
				{"G3D.BAS", "5870", 98},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
