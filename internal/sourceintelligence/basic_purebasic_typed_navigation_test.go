package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestPureBasicCharacterLiteralsPreserveNearbyProcedures(t *testing.T) {
	fixture := strings.Join([]string{
		"Module JSON",
		"Procedure.s xEscapeJSONString(Value.s)",
		"  Select Value",
		"    Case '\"'",
		"      Value = \"quote\"",
		"    Case '\\'",
		"      Value = \"slash\"",
		"  EndSelect",
		"  ProcedureReturn Value",
		"EndProcedure",
		"Procedure.i xParseJSONString()",
		"  ProcedureReturn 1",
		"EndProcedure",
		"EndModule",
	}, "\n") + "\n"
	result, err := (PureBasicAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(fixture), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid PureBasic character literals caused incomplete source: diagnostics=%+v", result.Analysis.Diagnostics)
	}
	for _, name := range []string{"JSON.xEscapeJSONString", "JSON.xParseJSONString"} {
		if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)[name]; !ok {
			t.Errorf("missing %s after PureBasic character literal; symbols=%v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestPureBasicTypedProceduresAndDeclareModuleNavigation(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
			fixture := strings.Join([]string{
				"DeclareModule JSON",
				"  Declare.i CreateJSON(MainNodeType.i = 0)",
				"  Declare.s EscapeJSON(Value.s)",
				"EndDeclareModule",
				"Module JSON",
				"  Procedure.s EscapeJSON(Value.s)",
				"    ProcedureReturn Value",
				"  EndProcedure",
				"  Procedure.i CreateJSON(MainNodeType.i = 0)",
				"    ProcedureReturn 1",
				"  EndProcedure",
				"  Procedure Normal()",
				"  EndProcedure",
				"EndModule",
			}, eol) + eol
			result, err := (PureBasicAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(fixture), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid PureBasic module reported partial: %+v", result.Analysis)
			}
			wanted := []struct {
				name, qualified string
				line            int
			}{
				{"CreateJSON", "JSON.CreateJSON", 2},
				{"EscapeJSON", "JSON.EscapeJSON", 3},
				{"EscapeJSON", "JSON.EscapeJSON", 6},
				{"CreateJSON", "JSON.CreateJSON", 9},
				{"Normal", "JSON.Normal", 12},
			}
			for _, w := range wanted {
				found := false
				for _, s := range result.Analysis.Symbols {
					if s.Name == w.name && s.QualifiedName == w.qualified &&
						s.DeclarationRange.Start.Line == w.line && s.NameRange.Start.Line == w.line {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("missing %s (%s) at line %d; symbols=%+v", w.name, w.qualified, w.line, result.Analysis.Symbols)
				}
			}
			for _, s := range result.Analysis.Symbols {
				if s.Name == "DeclareModule" || s.Name == "Procedure" || s.Name == "s" || s.Name == "i" {
					t.Errorf("fabricated symbol from PureBasic keyword/type suffix: %+v", s)
				}
			}
		})
	}
}
