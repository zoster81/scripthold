package sourceintelligence

import (
	"context"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func FuzzR27Phase10AnalyzersNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{"CREATE TABLE users(id INTEGER);\n", 0},
		{"CREATE OR REPLACE PACKAGE demo AS\nPROCEDURE run;\nEND demo;\n/\n", 1},
		{"schema { query: Query }\ntype Query { ping: String }\n", 2},
		{"resource \"demo\" \"main\" {}\n", 3},
		{"let\n answer = 42;\nin {}\n", 4},
		{"syntax = \"proto3\";\nmessage Item { string id = 1; }\n", 5},
		{"entity Counter is\nend Counter;\n", 6},
		{"module counter; wire ready; endmodule\n", 7},
		{"interface bus_if; logic ready; endinterface\n", 8},
		{"main:\n nop\n", 9},
		{"<main id=\"hero\"></main>\n", 10},
		{"<item id=\"root\"/>\n", 11},
		{".card { display: block; }\n", 12},
		{"$gap: 1rem;\n.card { color: red; }\n", 13},
		{"$gap: 1rem\n.card\n color: red\n", 14},
		{"@gap: 1rem;\n.card { margin: @gap; }\n", 15},
		{"{\"service\":{\"name\":\"api\"}}\n", 16},
		{"service:\n  name: api\n", 17},
		{"[server]\nhost = \"localhost\"\n", 18},
		{"# Project\n\n## Usage\n", 19},
		{"openapi: 3.1.0\npaths:\n  /users:\n    get:\n      operationId: listUsers\n", 20},
		{"- name: Play\n  hosts: all\n  tasks:\n    - name: Ping\n      debug:\n", 21},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := phase10Analyzers()
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), phase9TestDocument("fuzz.fixture", text), phase3AnalyzeOptions(false, 128))
		if err != nil {
			if kind := operation.KindOf(err); kind != operation.KindInvalidInput && kind != operation.KindLimit && kind != operation.KindUnsupported {
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, kind)
			}
			return
		}
		if len(result.Analysis.Symbols) > 128 {
			t.Fatalf("%s fuzz result exceeded symbol bound: %d", analyzer.Language(), len(result.Analysis.Symbols))
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "" || symbol.QualifiedName == "" || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 || symbol.DeclarationRange.End.Line <= 0 || symbol.DeclarationRange.End.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid symbol: %+v", analyzer.Language(), symbol)
			}
		}
	})
}
