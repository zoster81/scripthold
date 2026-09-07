//go:build !race

package sourceintelligence

import (
	"context"
	"testing"
)

func TestDetectLanguageAvoidsDefensiveDescriptorClones(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	cases := []DetectionInput{
		{Path: "shared.m", Text: "function y = run(x)\n  y = x;\nend\n"},
		{Path: "shared.h", Text: "struct Item { int value; };\n"},
		{Path: "module.bas", Text: "Public Sub Run()\r\nEnd Sub\r\n"},
		{Path: "shared.inc", Text: "type TPoint = record X: Integer; end;\n"},
		{Path: "config.yaml", Text: "name: demo\nitems:\n  - one\n"},
		{Path: "schema.sql", Text: "CREATE TABLE items (id INTEGER);\n"},
		{Path: "app.js", Text: "export class Service { run() {} }\n"},
	}
	observed := 0
	allocations := testing.AllocsPerRun(20, func() {
		observed = 0
		for _, input := range cases {
			result, detectErr := DetectLanguage(context.Background(), registry, input)
			if detectErr != nil {
				t.Fatal(detectErr)
			}
			observed += len(result.Candidates)
		}
	})
	if observed == 0 {
		t.Fatal("allocation guard produced no language candidates")
	}
	if allocations > 250 {
		t.Fatalf("DetectLanguage allocations = %.0f, want <= 250", allocations)
	}
}
