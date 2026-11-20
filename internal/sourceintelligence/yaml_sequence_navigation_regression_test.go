package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// YAML mappings inside a sequence still have source-grounded keys. Repeated
// qualified names remain distinct through their original source selectors.
func TestYAMLSequenceMappingKeysRemainNavigableWithPartialCoverage(t *testing.T) {
	source := "jobs:\n" +
		"  build:\n" +
		"    steps:\n" +
		"      - name: Checkout\n" +
		"        uses: actions/checkout@v4\n" +
		"        with:\n" +
		"          fetch-depth: 0\n" +
		"      - uses: actions/cache@v4\n" +
		"        with:\n" +
		"          path: .cache\n" +
		"      - run: |\n" +
		"          echo 'fabricated: no'\n" +
		"    timeout-minutes: 20\n"
	result, err := (YAMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 80))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("YAML sequence item identities must remain explicitly partial: %+v", result.Analysis)
	}
	want := map[int]string{
		4:  "jobs.build.steps.name",
		5:  "jobs.build.steps.uses",
		6:  "jobs.build.steps.with",
		7:  "jobs.build.steps.with.fetch-depth",
		8:  "jobs.build.steps.uses",
		9:  "jobs.build.steps.with",
		10: "jobs.build.steps.with.path",
		11: "jobs.build.steps.run",
		13: "jobs.build.timeout-minutes",
	}
	found := make(map[int]bool)
	for _, sym := range result.Analysis.Symbols {
		line := sym.NameRange.Start.Line
		if qualified, ok := want[line]; ok && qualified == sym.QualifiedName && sym.ID != "" {
			found[line] = true
		}
		if sym.Name == "fabricated" || sym.QualifiedName == "jobs.build.steps.fabricated" {
			t.Fatalf("block-scalar body leaked YAML source key: %+v", sym)
		}
	}
	for line, qualified := range want {
		if !found[line] {
			t.Errorf("sequence mapping source key at line %d not indexed as %s", line, qualified)
		}
	}
	hasPartialDiagnostic := false
	for _, diag := range result.Analysis.Diagnostics {
		if diag.Code == "yaml-unmodeled-sequence" {
			hasPartialDiagnostic = true
		}
	}
	if !hasPartialDiagnostic {
		t.Fatal("YAML sequence mapping scope was incorrectly marked complete")
	}
}

// YAML also allows the dash at the same indent as the mapping key ("steps:").
// It must not be mistaken for a sibling of that key.
func TestYAMLIndentlessSequenceKeepsStepOwner(t *testing.T) {
	source := "jobs:\n" +
		"  build:\n" +
		"    steps:\n" +
		"    - uses: actions/checkout@v4\n" +
		"      with:\n" +
		"        fetch-depth: 0\n" +
		"    - name: Compile\n" +
		"      run: echo ok\n" +
		"    timeout-minutes: 20\n"
	result, err := (YAMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 70))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("indentless sequence must stay explicitly partial: %+v", result.Analysis)
	}
	want := map[int]string{
		4: "jobs.build.steps.uses",
		5: "jobs.build.steps.with",
		6: "jobs.build.steps.with.fetch-depth",
		7: "jobs.build.steps.name",
		8: "jobs.build.steps.run",
		9: "jobs.build.timeout-minutes",
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.NameRange.Start.Line == 4 && sym.ParentQualifiedName == "jobs.build" {
			t.Fatalf("indentless list entry lost its steps owner: %+v", sym)
		}
		if want[sym.NameRange.Start.Line] == sym.QualifiedName && sym.ID != "" {
			delete(want, sym.NameRange.Start.Line)
		}
	}
	if len(want) > 0 {
		t.Fatalf("YAML indentless sequence declarations missing: %+v", want)
	}
}

// Multi-line workflow commands must be readable through their mapping-key
// selectors, without hiding a following field of the same sequence item.
func TestYAMLBlockScalarRangesAndSequenceSiblings(t *testing.T) {
	const source = "jobs:\n" +
		"  release:\n" +
		"    steps:\n" +
		"      - run: |\n" +
		"          echo package\n" +
		"          name: not-a-step\n" +
		"\n" +
		"        env:\n" +
		"          CI: true\n" +
		"      - name: Publish\n" +
		"        run: >-\n" +
		"          echo first\n" +
		"          echo second\n" +
		"    timeout-minutes: 20\n" +
		"followup: ok\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (YAMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			want := map[int]struct {
				qualified string
				end       int
			}{
				4:  {"jobs.release.steps.run", 6},
				8:  {"jobs.release.steps.env", 8},
				9:  {"jobs.release.steps.env.CI", 9},
				11: {"jobs.release.steps.run", 13},
				14: {"jobs.release.timeout-minutes", 14},
				15: {"followup", 15},
			}
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == "not-a-step" {
					t.Fatalf("block content fabricated YAML key: %+v", sym)
				}
				at := sym.NameRange.Start.Line
				if expected, ok := want[at]; ok {
					if sym.QualifiedName != expected.qualified || sym.DeclarationRange.End.Line != expected.end {
						t.Errorf("bad multiline workflow mapping at line %d: %+v", at, sym)
					}
					delete(want, at)
				}
			}
			if len(want) != 0 {
				t.Errorf("missing multiline workflow keys: %+v", want)
			}
			if result.Analysis.CoverageComplete {
				t.Fatal("sequence item ownership still must be marked partial")
			}
		})
	}
}

func TestYAMLExplicitBlockIndentAndEmptyBlock(t *testing.T) {
	const source = "jobs:\n" +
		"  build:\n" +
		"    steps:\n" +
		"      - run: |2\n" +
		"            echo first\n" +
		"          echo second\n" +
		"        env:\n" +
		"          OK: true\n" +
		"empty: |\n" +
		"after: true\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (YAMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			want := map[int]struct {
				name string
				end  int
			}{
				4:  {"jobs.build.steps.run", 6},
				7:  {"jobs.build.steps.env", 7},
				8:  {"jobs.build.steps.env.OK", 8},
				9:  {"empty", 9},
				10: {"after", 10},
			}
			for _, sym := range result.Analysis.Symbols {
				line := sym.NameRange.Start.Line
				if expected, ok := want[line]; ok {
					if sym.QualifiedName != expected.name || sym.DeclarationRange.End.Line != expected.end {
						t.Errorf("explicit/empty YAML block wrong at %d: %+v", line, sym)
					}
					delete(want, line)
				}
			}
			if len(want) > 0 {
				t.Errorf("missing YAML block neighbors: %+v", want)
			}
		})
	}
}
