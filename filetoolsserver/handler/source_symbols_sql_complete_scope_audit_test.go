package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Complete original-file audit of directly declared SQL schema objects,
// independently inventoried from five full source files before invoking the
// development MCP handler. PL/SQL block internals and dynamic DDL strings
// are outside this SQL DDL declaration inventory, not inferred as objects.
func TestSQLSelectedOriginalCompleteDDLInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original SQL source corpus not configured")
	}
	type declaration struct {
		name, kind string
		line       int
	}
	cases := []struct {
		project, file string
		want          []declaration
	}{
		{"sql56-sample-schemas-audit-selected", "customer_orders/co_create.sql", []declaration{
			{"customers", "type", 62}, {"stores", "type", 75},
			{"products", "type", 96}, {"orders", "type", 114},
			{"shipments", "type", 128}, {"order_items", "type", 142},
			{"inventory", "type", 157}, {"customer_order_products", "type", 173},
			{"store_orders", "type", 195}, {"product_reviews", "type", 223},
			{"product_orders", "type", 247},
			{"customers_name_i", "resource", 266},
			{"orders_customer_id_i", "resource", 267},
			{"orders_store_id_i", "resource", 268},
			{"shipments_store_id_i", "resource", 269},
			{"shipments_customer_id_i", "resource", 270},
			{"order_items_shipment_id_i", "resource", 271},
			{"inventory_product_id_i", "resource", 272},
		}},
		{"sql56-sample-schemas-audit-selected", "customer_orders/co_uninstall.sql", nil},
		{"sql56-logger-audit-selected", "source/views/logger_logs_5_min.sql",
			[]declaration{{"logger_logs_5_min", "type", 1}}},
		{"sql56-logger-audit-selected", "source/views/logger_logs_60_min.sql",
			[]declaration{{"logger_logs_60_min", "type", 1}}},
		{"sql56-logger-audit-selected", "source/tables/logger_logs.sql", nil},
	}
	type original struct {
		path  string
		text  string
		lines []string
	}
	originals := make([]original, len(cases))
	for i, tc := range cases {
		path := filepath.Join(root, tc.project, filepath.FromSlash(tc.file))
		requireAdditionalOriginalFile(t, path)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := strings.ReplaceAll(string(bytes), "\r\n", "\n")
		originals[i] = original{path, source, strings.Split(source, "\n")}
		for _, decl := range tc.want {
			if decl.line < 1 || decl.line > len(originals[i].lines) {
				t.Fatalf("independent DDL location outside original %s:%d", tc.file, decl.line)
			}
			line := strings.TrimSpace(originals[i].lines[decl.line-1])
			if !strings.Contains(strings.ToLower(line), "create") ||
				!strings.Contains(strings.ToLower(line), decl.name) {
				t.Fatalf("incorrect independently inventoried DDL %s:%d %q", tc.file, decl.line, line)
			}
		}
	}
	for i, tc := range cases {
		t.Run(tc.project+"/"+tc.file, func(t *testing.T) {
			src := originals[i]
			handler := NewHandler([]string{filepath.Dir(src.path)})
			failed, out, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{src.path}, Language: "sql",
				Encoding: "utf-8", Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete ||
				out.FilesParsed != 1 {
				t.Fatalf("full original source outline incomplete: err=%v failed=%+v result=%+v", err, failed, out)
			}
			expected := map[int]declaration{}
			for _, decl := range tc.want {
				if _, exists := expected[decl.line]; exists {
					t.Fatalf("duplicate independent location: %d", decl.line)
				}
				expected[decl.line] = decl
			}
			actual := map[int]bool{}
			for _, symbol := range out.Symbols {
				line := symbol.NameRange.Start.Line
				if filepath.Clean(symbol.Path) != filepath.Clean(src.path) {
					t.Errorf("declaration references wrong original file: %+v", symbol)
					continue
				}
				if actual[line] {
					t.Errorf("duplicate declaration reported at %d: %+v", line, symbol)
				}
				actual[line] = true
				want, exists := expected[line]
				if !exists {
					t.Errorf("invented supported DDL at %d: %s %s", line, symbol.Kind, symbol.Name)
					continue
				}
				if symbol.Name != want.name || string(symbol.Kind) != want.kind ||
					symbol.ParentQualifiedName != "" {
					t.Errorf("wrong declaration at %d: got %s %s parent %q, expected %s %s",
						line, symbol.Kind, symbol.Name, symbol.ParentQualifiedName, want.kind, want.name)
				}
				// Derive dialect classification independently from original DDL.
				originalLine := strings.ToLower(strings.TrimSpace(src.lines[line-1]))
				wantNative := ""
				switch {
				case strings.HasPrefix(originalLine, "create table "):
					wantNative = "table"
				case strings.HasPrefix(originalLine, "create or replace view "):
					wantNative = "view"
				case strings.HasPrefix(originalLine, "create or replace force view "):
					wantNative = "oracle-view"
				case strings.HasPrefix(originalLine, "create index "):
					wantNative = "index"
				default:
					t.Errorf("independent original DDL at %d unsupported: %q", line, originalLine)
				}
				if wantNative != "" && symbol.NativeKind != wantNative {
					t.Errorf("wrong SQL dialect/object type at %d: %q, expected %q", line, symbol.NativeKind, wantNative)
				}
			}
			for _, decl := range tc.want {
				if !actual[decl.line] {
					t.Errorf("missing DDL %s:%d", decl.name, decl.line)
				}
			}
			if len(out.Symbols) != len(tc.want) {
				t.Errorf("unexpected object count, source %d, tool %d", len(tc.want), len(out.Symbols))
			}
			t.Logf("originalCompleteFileDDL=%d returned=%d coverageComplete=%v",
				len(tc.want), len(out.Symbols), out.CoverageComplete)
		})
	}
}
