package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These original project sources establish the SQL view's authored body before
// querying the development handler; the SQL is never executed or modified.
func TestSQLOriginalOracleViewSourceShowStopsAtStatementEnd(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original SQL snapshots not configured")
	}
	cases := []struct {
		name, project, file, view, opening, required, negative string
		startLine, endLine                                     int
		terminator                                             string
	}{
		{
			name:    "Logger five-minute view, SQLPlus slash",
			project: "sql56-logger-audit-selected", file: "source/views/logger_logs_5_min.sql",
			view: "logger_logs_5_min", opening: "create or replace force view logger_logs_5_min as",
			required:  "where time_stamp > systimestamp - (5/1440)",
			negative:  "create or replace force view logger_logs_60_min",
			startLine: 1, endLine: 4, terminator: "/",
		},
		{
			name:    "Logger sixty-minute view, SQLPlus slash",
			project: "sql56-logger-audit-selected", file: "source/views/logger_logs_60_min.sql",
			view: "logger_logs_60_min", opening: "create or replace force view logger_logs_60_min as",
			required:  "where time_stamp > systimestamp - (1/24)",
			negative:  "create table logger_logs",
			startLine: 1, endLine: 4, terminator: "/",
		},
		{
			name:    "Oracle customer order summary and next view boundary",
			project: "sql56-sample-schemas-audit-selected", file: "customer_orders/co_create.sql",
			view: "customer_order_products", opening: "CREATE OR REPLACE VIEW customer_order_products AS",
			required:  "GROUP  BY o.order_id, o.order_tms, o.order_status,",
			negative:  "CREATE OR REPLACE VIEW store_orders AS",
			startLine: 173, endLine: 189, terminator: ";",
		},
		{
			name:    "Oracle JSON_TABLE review view",
			project: "sql56-sample-schemas-audit-selected", file: "customer_orders/co_create.sql",
			view: "product_reviews", opening: "CREATE OR REPLACE VIEW product_reviews AS",
			required:  "JSON_TABLE (",
			negative:  "CREATE OR REPLACE VIEW product_orders AS",
			startLine: 223, endLine: 242, terminator: ";",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original SQL file unavailable or unsafe: %v %+v", err, info)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.startLine < 1 || tc.endLine >= len(lines) ||
				strings.TrimSpace(lines[tc.startLine-1]) != tc.opening ||
				!strings.Contains(strings.Join(lines[tc.startLine-1:tc.endLine], "\n"), tc.required) {
				t.Fatalf("independent original SQL view oracle changed: %s:%d-%d", tc.file, tc.startLine, tc.endLine)
			}
			last := strings.TrimSpace(lines[tc.endLine-1])
			if tc.terminator == "/" {
				if strings.TrimSpace(lines[tc.endLine]) != "/" {
					t.Fatalf("missing SQLPlus slash after original view at %s:%d", tc.file, tc.endLine)
				}
			} else if !strings.HasSuffix(last, ";") {
				t.Fatalf("missing SQL semicolon at original view %s:%d", tc.file, tc.endLine)
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "sql", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.Truncated || digest.FilesParsed != 1 {
				t.Fatalf("original SQL digest incomplete: %v %+v", err, digest)
			}
			_, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "sql",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || !outlined.CoverageComplete || outlined.Truncated {
				t.Fatalf("original SQL outline incomplete: %v %+v", err, outlined)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "sql",
				Query: tc.view, Match: "exact", FileDetails: "relevant",
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original SQL find incomplete: %v %+v", err, found)
			}
			symbolID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.view && sym.NameRange.Start.Line == tc.startLine &&
					sym.DeclarationRange.Start.Line == tc.startLine &&
					sym.DeclarationRange.End.Line == tc.endLine && string(sym.Kind) == "type" {
					symbolID = sym.ID
				}
			}
			if symbolID == "" {
				t.Fatalf("original view not found with expected range: %s:%d %+v", tc.file, tc.startLine, found.Symbols)
			}
			present := false
			for _, sym := range outlined.Symbols {
				if sym.ID == symbolID {
					present = true
					break
				}
			}
			if !present {
				t.Fatalf("view missing from complete outline: %s:%d", tc.file, tc.startLine)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint == "" {
				t.Fatalf("unsafe SQL evidence: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "sql",
				Encoding: file.Encoding, SymbolID: symbolID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.opening) ||
				!strings.Contains(shown.Show.Text, tc.required) ||
				strings.Contains(shown.Show.Text, tc.negative) {
				t.Fatalf("original SQL show missing source or crossed statements: %s:%d err=%v show=%+v",
					tc.file, tc.startLine, err, shown.Show)
			}
			if tc.terminator == "/" && strings.Contains(shown.Show.Text, "\n/") {
				t.Fatalf("SQLPlus execute command belongs outside view declaration: %s:%d", tc.file, tc.startLine)
			}
		})
	}
}
