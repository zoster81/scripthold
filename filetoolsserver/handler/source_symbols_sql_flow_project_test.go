package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSourceSymbolsSQLMultilineAndFlowGenericHandler(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	for _, tc := range []struct {
		name, language, source, want string
		count, line                  int
	}{
		{"sql", "sql", "CREATE TABLE\r\n animals (id INT);\r\n-- CREATE TABLE ghost (id INT);\r\n", "animals", 1, 1},
		{"flow", "flow", "// @flow\r\nexport class Container<State: {}> {\r\n state: State;\r\n _listeners: Array<Listener> = [];\r\n constructor() {}\r\n setState(value: State) { return value; }\r\n subscribe(fn: Listener) {}\r\n unsubscribe(fn: Listener) {}\r\n}\r\n", "Container.setState", 7, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.name+".txt")
			if err := os.WriteFile(path, []byte(tc.source), 0o600); err != nil {
				t.Fatal(err)
			}
			h := NewHandler([]string{root})
			ctx := context.Background()
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Presentation: "compact", Language: tc.language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64})
			if err != nil || failed != nil || !out.CoverageComplete || out.SymbolCount != tc.count || len(out.OutlineFiles) != 1 {
				t.Fatalf("outline %s: %v %+v %+v", tc.name, err, failed, out)
			}
			var selected *SourceOutlineNode
			for _, s := range out.OutlineFiles[0].Symbols {
				if s.QualifiedName == tc.want {
					selected = s
					break
				}
			}
			if selected == nil || selected.Line != tc.line {
				t.Fatalf("expected %s line %d: %+v", tc.want, tc.line, out.OutlineFiles[0].Symbols)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: tc.language, Encoding: "utf-8", SymbolID: selected.SymbolID, SourceFingerprint: out.OutlineFiles[0].SourceFingerprint, MaxBytes: 4096})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, selected.Name) {
				t.Fatalf("show failed %v %+v %+v", err, failed, shown.Show)
			}
			failed, q, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Paths: []string{path}, Language: tc.language, Query: selected.Name, Mode: "structural", Match: "exact", MaxResults: 5})
			if err != nil || failed != nil || q.Search == nil || len(q.Search.Matches) == 0 {
				t.Fatalf("search failed %v %+v %+v", err, failed, q.Search)
			}
		})
	}
}

func TestSourceSymbolsRealSQLClinicFullProject(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SQL_VET_PROJECT")
	if root == "" {
		t.Skip("original SQL project not provided")
	}
	paths := []string{}
	for _, n := range []string{"schema.sql", "schema_based_on_diagram.sql", "data.sql", "queries.sql", "logs/animals-query-results.sql"} {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(n)))
	}
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Paths: paths, Presentation: "compact", Language: "sql", MaxFiles: 5, MaxSymbols: 64})
	if err != nil || failed != nil || !out.CoverageComplete || out.FilesParsed != 5 || out.SymbolCount != 18 {
		t.Fatalf("complete SQL project: %v %+v %+v", err, failed, out)
	}
	want := map[string]map[string]int{
		"schema.sql":                  {"animals": 1, "species": 12, "owners": 19, "vets": 38, "specializations": 47, "visits": 65},
		"schema_based_on_diagram.sql": {"medical_histories": 1, "patients": 9, "invoices": 21, "invoice_items": 33, "treatments": 51, "treatments_history": 59, "idx_medical_histories_patient_id": 19, "idx_invoices_medical_history_id": 31, "idx_invoice_items_invoice_id": 47, "idx_invoice_items_treatment_id": 49, "idx_treatments_history_medical_history_id": 67, "idx_treatments_history_treatment_id": 69},
		"data.sql":                    {}, "queries.sql": {}, "animals-query-results.sql": {},
	}
	for _, f := range out.OutlineFiles {
		expected, ok := want[filepath.Base(f.Path)]
		if !ok || len(f.Symbols) != len(expected) {
			t.Errorf("file=%s count=%d", f.Path, len(f.Symbols))
			continue
		}
		for _, s := range f.Symbols {
			line, ok := expected[s.QualifiedName]
			kind := "type"
			if strings.HasPrefix(s.QualifiedName, "idx_") {
				kind = "resource"
			}
			if !ok || s.Line != line || string(s.Kind) != kind {
				t.Errorf("wrong SQL declaration %+v", s)
			}
		}
	}
	var index *SourceOutlineNode
	var indexFile SourceOutlineFile
	for _, f := range out.OutlineFiles {
		for _, s := range f.Symbols {
			if s.QualifiedName == "idx_invoices_medical_history_id" {
				index, indexFile = s, f
				break
			}
		}
	}
	if index == nil || indexFile.Path == "" {
		t.Fatal("the original SQL index is missing")
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: indexFile.Path, Language: "sql", Encoding: "utf-8",
		SymbolID: index.SymbolID, SourceFingerprint: indexFile.SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.HasPrefix(shown.Show.Text, "CREATE INDEX idx_invoices_medical_history_id ON invoices(medical_history_id);") {
		t.Fatalf("index show mismatch: err=%v tool=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: paths, Language: "sql", Query: index.Name,
		Mode: "structural", Match: "exact", MaxResults: 10,
	})
	if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) != 1 ||
		string(query.Search.Matches[0].Kind) != "resource" {
		t.Fatalf("index search mismatch: err=%v tool=%+v search=%+v", err, failed, query.Search)
	}
}

func TestSourceSymbolsRealFlowUnstatedContainer(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_FLOW_UNSTATED_PROJECT")
	if root == "" {
		t.Skip("original Flow project not provided")
	}
	path := filepath.Join(root, "src", "unstated.js")
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Presentation: "compact", Language: "flow", MaxFiles: 1, MaxSymbols: 128})
	if err != nil || failed != nil || !out.CoverageComplete || len(out.OutlineFiles) != 1 {
		t.Fatalf("Flow outline %v %+v %+v", err, failed, out)
	}
	want := map[string]int{"Container": 9, "Container.state": 10, "Container._listeners": 11, "Container.Container": 13, "Container.setState": 17, "Container.subscribe": 47, "Container.unsubscribe": 51}
	found := map[string]*SourceOutlineNode{}
	for _, s := range out.OutlineFiles[0].Symbols {
		found[s.QualifiedName] = s
	}
	for name, line := range want {
		if found[name] == nil || found[name].Line != line {
			t.Errorf("missing real declaration %s at %d: %+v", name, line, found[name])
		}
	}
	if found["Container"] == nil {
		t.Fatal("missing class")
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "flow", Encoding: "utf-8", SymbolID: found["Container"].SymbolID, SourceFingerprint: out.OutlineFiles[0].SourceFingerprint, MaxBytes: 8192})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "unsubscribe(fn: Listener)") {
		t.Fatalf("truncated class body: %v %+v %+v", err, failed, shown.Show)
	}
}
