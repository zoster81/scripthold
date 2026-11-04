package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestLineNumberedBasicTargetsAreStructuralLabels(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"qbasic", QBasicAnalyzer{}},
		{"classic-basic", ClassicBasicAnalyzer{}},
	} {
		for _, eol := range []string{"\n", "\r\n"} {
			t.Run(tc.name+"/"+map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
				code := strings.Join([]string{
					"10 DIM A(3), B$(9)",
					"20 FOR I=1 TO 20: PRINT I",
					"30 IF I>2 THEN GOTO 20",
					"40 PRINT \"GOTO 999\": X=10",
					"50 PRINT 1: 200",
					"60 REM 888 IS NOT A LINE TARGET",
					"70 PRINT \"DONE\"",
				}, eol) + eol
				result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(code), testAnalyzeOptions(true, 80))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
					t.Fatalf("bounded line-numbered source should be complete: %+v", result.Analysis)
				}
				want := map[string]int{"10": 1, "20": 2, "30": 3, "40": 4, "50": 5, "60": 6, "70": 7}
				var found int
				for _, s := range result.Analysis.Symbols {
					if s.Kind != SymbolKindLabel {
						continue
					}
					line, ok := want[s.Name]
					if !ok {
						t.Errorf("invented numbered label %s at %+v", s.Name, s.NameRange)
						continue
					}
					if s.DeclarationRange.Start.Line != line || s.NameRange.Start.Line != line ||
						s.NativeKind != "line-number" {
						t.Errorf("incorrect numbered target %s: %+v", s.Name, s)
					}
					found++
				}
				if found != len(want) {
					t.Errorf("found %d line labels; want %d; all symbols=%+v", found, len(want), result.Analysis.Symbols)
				}
			})
		}
	}
}
