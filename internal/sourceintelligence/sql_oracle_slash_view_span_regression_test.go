package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Logger uses SQL*Plus's standalone slash instead of a semicolon to execute
// its Oracle views. Only the view definition belongs to the source excerpt.
func TestSQLOracleSlashTerminatedViewSourceRanges(t *testing.T) {
	source := "create or replace force view logger_logs_5_min as\n" +
		"  select *\n" +
		"  from logger_logs\n" +
		"  where time_stamp > systimestamp - (5/1440)\n" +
		"/\n" +
		"-- / is not an Oracle terminator in this comment\n" +
		"create or replace force view logger_logs_60_min as\n" +
		"  select '/' as delimiter, time_stamp\n" +
		"  from logger_logs\n" +
		"  where time_stamp > systimestamp - (1/24);\n" +
		"create table unrelated (id number);\n"
	for _, eol := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(eol.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.newline))
			result, err := (SQLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid SQL source was marked incomplete: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, tc := range []struct {
				name                string
				startLine, endLine  int
				contains, forbidden string
			}{
				{"logger_logs_5_min", 1, 4, "where time_stamp > systimestamp - (5/1440)", "create or replace force view logger_logs_60_min"},
				{"logger_logs_60_min", 7, 10, "select '/' as delimiter, time_stamp", "create table unrelated"},
				{"unrelated", 11, 11, "create table unrelated (id number);", "logger_logs_60_min"},
			} {
				sym, ok := symbols[tc.name]
				if !ok {
					t.Errorf("missing source symbol %q: %+v", tc.name, result.Analysis.Symbols)
					continue
				}
				if sym.DeclarationRange.Start.Line != tc.startLine || sym.DeclarationRange.End.Line != tc.endLine ||
					!strings.Contains(sym.Signature, tc.contains) || strings.Contains(sym.Signature, tc.forbidden) {
					t.Errorf("incorrect bounded declaration for %q: %+v", tc.name, sym)
				}
			}
		})
	}
}

func TestSQLOracleSlashLikeTextMustNotTerminateView(t *testing.T) {
	source := "create or replace force view doc as\n" +
		"  select '/' as quoted_slash\n" +
		"  from dual\n" +
		"  -- / comment looks like a separator\n" +
		"  where '/' <> 'x';\n" +
		"create table neighbor (id number);\n"
	for _, newline := range []string{"\n", "\r\n"} {
		result, err := (SQLAnalyzer{}).Analyze(context.Background(),
			sourceDocumentForScanner(strings.ReplaceAll(source, "\n", newline)), testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		got := symbolsByQualifiedName(result.Analysis.Symbols)["doc"]
		if got.DeclarationRange.End.Line != 5 ||
			!strings.Contains(got.Signature, "where '/' <> 'x';") ||
			strings.Contains(got.Signature, "create table neighbor") {
			t.Errorf("quoted/comment slash shortened or broadened SQL view: %+v", got)
		}
	}
}
