package handler

import (
	"os"
	"testing"
)

// Provider 56: original SQL DDL from Oracle Sample Schemas and Logger,
// independent of the first-stage veterinary clinic and Le Mans repositories.
// No SQL interpreter, database, or project script is ever executed.
func TestSourceSymbolsSQLSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original SQL projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "sql56-sample-schemas-audit-selected", language: "sql",
			search: "order_items", primary: "customer_orders/co_create.sql",
			negativeFile: "customer_orders/co_uninstall.sql", negativeQuery: "order_items",
			paths: []string{
				"customer_orders/co_create.sql",
				"customer_orders/co_uninstall.sql",
			},
			oracles: []extraAuditEvidence{
				{"customer_orders/co_create.sql", "CREATE TABLE customers"},
				{"customer_orders/co_create.sql", "CREATE TABLE orders"},
				{"customer_orders/co_create.sql", "CREATE TABLE order_items"},
				{"customer_orders/co_create.sql", "CREATE TABLE inventory"},
				{"customer_orders/co_create.sql", "CREATE OR REPLACE VIEW customer_order_products AS"},
				{"customer_orders/co_create.sql", "CREATE OR REPLACE VIEW store_orders AS"},
				{"customer_orders/co_create.sql", "CREATE INDEX order_items_shipment_id_i"},
			},
			targets: []extraAuditSymbol{
				{"customer_orders/co_create.sql", "orders", 114},
				{"customer_orders/co_create.sql", "order_items", 142},
				{"customer_orders/co_create.sql", "inventory", 157},
				{"customer_orders/co_create.sql", "customer_order_products", 173},
				{"customer_orders/co_create.sql", "store_orders", 195},
			},
		},
		{
			project: "sql56-logger-audit-selected", language: "sql",
			search: "logger_logs_5_min", primary: "source/views/logger_logs_5_min.sql",
			negativeFile: "source/views/logger_logs_60_min.sql", negativeQuery: "logger_logs_5_min",
			paths: []string{
				"source/views/logger_logs_5_min.sql",
				"source/views/logger_logs_60_min.sql",
			},
			oracles: []extraAuditEvidence{
				{"source/views/logger_logs_5_min.sql", "create or replace force view logger_logs_5_min as"},
				{"source/views/logger_logs_5_min.sql", "from logger_logs"},
				{"source/views/logger_logs_5_min.sql", "where time_stamp > systimestamp - (5/1440)"},
				{"source/views/logger_logs_60_min.sql", "create or replace force view logger_logs_60_min as"},
				{"source/views/logger_logs_60_min.sql", "where time_stamp > systimestamp - (1/24)"},
				{"source/tables/logger_logs.sql", "create table logger_logs("},
			},
			targets: []extraAuditSymbol{
				{"source/views/logger_logs_5_min.sql", "logger_logs_5_min", 1},
				{"source/views/logger_logs_60_min.sql", "logger_logs_60_min", 1},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
