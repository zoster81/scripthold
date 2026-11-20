package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

// These maintenance questions and expected edits are derived from the original
// projects, not from source_symbols or the installed connector's symbol totals.
func TestSourceSymbolsOriginalQBasicAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original QBasic corpus unavailable")
	}
	for _, scenario := range []extraAuditScenario{
		{
			project:  "qbasic-albertus82-vbesvga-oemsetup",
			language: "qbasic",
			search:   "660", primary: "OEMSETUP.BAS",
			negativeFile: "OEMSETUP.BAS", negativeQuery: "999",
			paths: []string{"OEMSETUP.BAS"},
			oracles: []extraAuditEvidence{
				{"README.md", "OEMSETUP.INF"},
				{"OEMSETUP.BAS", "220 GOSUB 620"},
				{"OEMSETUP.BAS", "350 FOR J = 0 TO I"},
				{"OEMSETUP.BAS", "470 REM Settings"},
				{"OEMSETUP.BAS", "530 PRINT #2, \",,system.ini,vbesvga.drv,\""},
				{"OEMSETUP.BAS", "620 REM Begin of the routine"},
				{"OEMSETUP.BAS", "630 IF LEN(A$) = 0 OR INSTR(A$, \"NG for VBESVGA.DRV\") <> 0 THEN RETURN"},
				{"OEMSETUP.BAS", "660 IF WI% < 640 OR HE% < 480 THEN RETURN"},
				{"OEMSETUP.BAS", "680 IF DE% > 24 THEN DE% = 24"},
				{"OEMSETUP.BAS", "700 IF MO(J, 0) = WI% AND MO(J, 1) = HE% AND MO(J, 2) = DE% THEN RETURN"},
				{"OEMSETUP.BAS", "730 MO(I, 0) = WI%"},
			},
			targets: []extraAuditSymbol{
				{"OEMSETUP.BAS", "MO", 3},
				{"OEMSETUP.BAS", "350", 26},
				{"OEMSETUP.BAS", "470", 38},
				{"OEMSETUP.BAS", "530", 44},
				{"OEMSETUP.BAS", "620", 53},
				{"OEMSETUP.BAS", "660", 57},
				{"OEMSETUP.BAS", "680", 59},
				{"OEMSETUP.BAS", "700", 61},
			},
		},
		{
			project:  "qbasic-deltacodegames-toadman-2",
			language: "qbasic",
			search:   "OnePlayerMode%", primary: "TOADMAN2.BAS",
			negativeFile: "TOAD2-BU.BAS", negativeQuery: "OnePlayerMode%",
			paths: []string{"TOADMAN2.BAS", "TOAD2-BU.BAS", "TD1-TD2.BAS"},
			oracles: []extraAuditEvidence{
				{"TOADMAN2.BAS", "CONST OnePlayerMode% = 0"},
				{"TOADMAN2.BAS", "IF OnePlayerMode% = 1 THEN Life%(1) = 500"},
				{"TOADMAN2.BAS", "IF OnePlayerMode% = 1 THEN"},
				{"TOADMAN2.BAS", "IF z% = 3 AND OnePlayerMode% = 0 THEN EXIT FOR"},
				{"TOADMAN2.BAS", "IF z% = 3 AND OnePlayerMode% = 1 THEN EXIT FOR"},
				{"TOADMAN2.BAS", "SUB MoveBadguys"},
				{"TOADMAN2.BAS", "SUB Restart"},
				{"TOAD2-BU.BAS", "SUB ChooseBorder"},
				{"TOAD2-BU.BAS", "SUB Save"},
				{"TOAD2-BU.BAS", "SUB Load"},
				{"TD1-TD2.BAS", "BSAVE nm2$, VARPTR(mz%(0, 0)), 5202"},
			},
			targets: []extraAuditSymbol{
				{"TOADMAN2.BAS", "OnePlayerMode%", 40},
				{"TOADMAN2.BAS", "MoveBadguys", 1030},
				{"TOADMAN2.BAS", "Restart", 1165},
				{"TOAD2-BU.BAS", "ChooseBorder", 164},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, scenario)
	}
	t.Run("numbered-program-label-kind", func(t *testing.T) {
		root := filepath.Join(corpus, "qbasic-albertus82-vbesvga-oemsetup")
		path := filepath.Join(root, "OEMSETUP.BAS")
		h := NewHandler([]string{root})
		for _, name := range []string{"620", "660", "680", "700"} {
			failure, got, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Query: name,
				Language: "qbasic", Encoding: "utf-8", Match: "exact", MaxFiles: 1, MaxSymbols: 4,
			})
			if err != nil || failure != nil || !got.CoverageComplete || len(got.Symbols) != 1 {
				t.Fatalf("numbered label %s not uniquely indexed: err=%v failed=%+v got=%+v", name, err, failure, got)
			}
			s := got.Symbols[0]
			if s.Kind != sourceintelligence.SymbolKindLabel || s.NativeKind != "line-number" ||
				s.Name != name || s.DeclarationRange.Start.Line != s.NameRange.Start.Line {
				t.Fatalf("incorrect numbered label %s: %+v", name, s)
			}
		}
	})
}
