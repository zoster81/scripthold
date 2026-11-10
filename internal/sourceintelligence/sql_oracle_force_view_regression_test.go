package sourceintelligence

import (
	"context"
	"testing"
)

// Original-source pattern from Logger's Oracle views, with negative controls.
// This is declaration indexing, not Oracle SQL validation or execution.
func TestSQLOracleForceViewDeclarations(t *testing.T) {
	source := `create or replace force view logger_logs_5_min as
 select * from logger_logs where time_stamp > systimestamp - (5/1440);
create or replace noforce view logger_logs_60_min as
 select * from logger_logs where time_stamp > systimestamp - (1/24);
-- create or replace force view comment_ghost as select 1;
select 'create or replace force view string_ghost as select 1;';
`
	result, err := (SQLAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, symbol := range result.Analysis.Symbols {
		seen[symbol.Name] = symbol.NameRange.Start.Line
	}
	for name, line := range map[string]int{"logger_logs_5_min": 1, "logger_logs_60_min": 3} {
		if seen[name] != line {
			t.Errorf("missing original Oracle FORCE/NOFORCE view %s:%d: %v", name, line, seen)
		}
	}
	for _, name := range []string{"comment_ghost", "string_ghost"} {
		if _, exists := seen[name]; exists {
			t.Errorf("invented SQL view from comment or literal: %s", name)
		}
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Errorf("small supported Oracle views unexpectedly incomplete: %+v", result.Analysis)
	}
}
