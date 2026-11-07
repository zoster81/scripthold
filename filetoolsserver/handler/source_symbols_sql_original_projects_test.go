package handler

import (
	"os"
	"testing"
)

// Original PostgreSQL and MySQL repositories are read-only evidence: SQL
// migrations, views, functions, databases, and scripts are never executed.
func TestSourceSymbolsOriginalSQLInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original SQL corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "sql-iturres-vet-clinic-db", language: "sql",
			search: "invoice_items", primary: "schema_based_on_diagram.sql",
			negativeFile: "schema.sql", negativeQuery: "invoice_items",
			paths: []string{
				"schema_based_on_diagram.sql",
				"schema.sql",
				"data.sql",
			},
			oracles: []extraAuditEvidence{
				{"schema_based_on_diagram.sql", "CREATE TABLE medical_histories ("},
				{"schema_based_on_diagram.sql", "CREATE TABLE patients ("},
				{"schema_based_on_diagram.sql", "CREATE TABLE invoices ("},
				{"schema_based_on_diagram.sql", "CONSTRAINT fk_medical_history_id FOREIGN KEY (medical_history_id) REFERENCES medical_histories(id) ON DELETE CASCADE"},
				{"schema_based_on_diagram.sql", "CREATE INDEX idx_invoices_medical_history_id ON invoices(medical_history_id);"},
				{"schema_based_on_diagram.sql", "CREATE TABLE invoice_items ("},
				{"schema_based_on_diagram.sql", "CONSTRAINT fk_invoice_id FOREIGN KEY (invoice_id) REFERENCES invoices(id) ON DELETE CASCADE"},
				{"schema_based_on_diagram.sql", "ADD CONSTRAINT fk_in_treatment_id FOREIGN KEY (treatment_id) REFERENCES treatments(id) ON DELETE CASCADE;"},
				{"schema_based_on_diagram.sql", "CREATE INDEX idx_invoice_items_treatment_id ON invoice_items(treatment_id);"},
				{"schema_based_on_diagram.sql", "CREATE TABLE treatments ("},
				{"schema_based_on_diagram.sql", "CREATE TABLE treatments_history ("},
				{"schema.sql", "CREATE TABLE"},
				{"schema.sql", "animals ("},
				{"data.sql", "INSERT INTO"},
				{"data.sql", "animals ("},
			},
			targets: []extraAuditSymbol{
				{"schema_based_on_diagram.sql", "medical_histories", 1},
				{"schema_based_on_diagram.sql", "patients", 9},
				{"schema_based_on_diagram.sql", "invoices", 21},
				{"schema_based_on_diagram.sql", "invoice_items", 33},
				{"schema_based_on_diagram.sql", "treatments", 51},
				{"schema_based_on_diagram.sql", "treatments_history", 59},
				{"schema.sql", "animals", 2},
				{"schema.sql", "visits", 66},
			},
		},
		{
			project: "sql-gdsotirov-lemans24", language: "sql",
			search: "WinnersByClass", primary: "db/views/WinnersByClass.sql",
			negativeFile: "db/views/Winners.sql", negativeQuery: "WinnersByClass",
			paths: []string{
				"db/views/WinnersByClass.sql",
				"db/views/Winners.sql",
				"db/tables/results.sql",
				"db/tables/races.sql",
				"db/tables/cars.sql",
				"db/routines/pos_to_num.sql",
			},
			oracles: []extraAuditEvidence{
				{"db/views/WinnersByClass.sql", "CREATE OR REPLACE VIEW WinnersByClass AS"},
				{"db/views/WinnersByClass.sql", "DENSE_RANK() OVER (PARTITION BY R.id, C.car_class"},
				{"db/views/WinnersByClass.sql", "pos_to_num(RES.pos)  AS Overall"},
				{"db/views/WinnersByClass.sql", "R.cancelled    = 0 /* not cancelled */"},
				{"db/views/WinnersByClass.sql", "RES.pos NOT IN ('DNA', 'DNF', 'DNP', 'DNQ', 'DNS', 'DSQ', 'NC', 'RES')"},
				{"db/views/WinnersByClass.sql", "WHERE InClass = 1"},
				{"db/tables/results.sql", "CREATE TABLE IF NOT EXISTS results ("},
				{"db/tables/results.sql", "pos         VARCHAR(3)    NOT NULL"},
				{"db/tables/races.sql", "CREATE TABLE IF NOT EXISTS races ("},
				{"db/tables/races.sql", "cancelled           TINYINT       NOT NULL DEFAULT 0"},
				{"db/tables/cars.sql", "CREATE TABLE IF NOT EXISTS cars ("},
				{"db/tables/cars.sql", "car_class   VARCHAR(12) NULL"},
				{"db/routines/pos_to_num.sql", "CREATE FUNCTION pos_to_num(pos VARCHAR(3) CHARACTER SET ascii)"},
				{"db/routines/pos_to_num.sql", "WHEN pos = 'DNF' THEN 93"},
				{"db/views/Winners.sql", "CREATE OR REPLACE VIEW Winners AS"},
				{"db/views/Winners.sql", "RES.pos        = 1 /* winners */"},
			},
			targets: []extraAuditSymbol{
				{"db/views/WinnersByClass.sql", "WinnersByClass", 1},
				{"db/views/Winners.sql", "Winners", 1},
				{"db/tables/results.sql", "results", 1},
				{"db/tables/races.sql", "races", 1},
				{"db/tables/cars.sql", "cars", 1},
				{"db/routines/pos_to_num.sql", "pos_to_num", 5},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
