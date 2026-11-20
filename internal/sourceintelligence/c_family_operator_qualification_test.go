package sourceintelligence

import (
	"context"
	"testing"
)

// A compact oracle based on genuine MQL5 operator overloads in a complete
// project file. Assignments in a method body are never field declarations.
func TestCFamilyOverloadedOperatorsPreserveFollowingMembers(t *testing.T) {
	text := "class CJAVal {\n" +
		"  int m_type;\n" +
		"  long m_iv;\n" +
		"public:\n" +
		"  void operator=(const int a) {\n" +
		"    m_type=1;\n" +
		"    m_iv=a;\n" +
		"  }\n" +
		"  bool operator==(const int a) { return m_iv==a; }\n" +
		"  bool operator!=(const int a) { return m_iv!=a; }\n" +
		"  long ToInt() const { return m_iv; }\n" +
		"  void Set(const int a) { m_iv=a; }\n" +
		"};\n"
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{{"mql5", MQL5Analyzer{}}, {"cpp", CPPAnalyzer{}}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("incorrectly reports missing or partial source coverage: %+v", result.Analysis)
			}
			seen := make(map[string]int)
			for _, sym := range result.Analysis.Symbols {
				seen[sym.QualifiedName+"|"+string(sym.Kind)]++
				if sym.ParentQualifiedName != "" && sym.ParentQualifiedName != "CJAVal" {
					t.Errorf("unexpected owner for %s: %s", sym.QualifiedName, sym.ParentQualifiedName)
				}
			}
			required := []string{
				"CJAVal|class",
				"CJAVal.m_type|field",
				"CJAVal.m_iv|field",
				"CJAVal.operator=|operator",
				"CJAVal.operator==|operator",
				"CJAVal.operator!=|operator",
				"CJAVal.ToInt|method",
				"CJAVal.Set|method",
			}
			for _, want := range required {
				if seen[want] != 1 {
					t.Errorf("declaration %q count %d, want 1; all=%v", want, seen[want], seen)
				}
			}
			if len(result.Analysis.Symbols) != len(required) {
				t.Errorf("returned %d symbols, want exactly %d; all=%v", len(result.Analysis.Symbols), len(required), seen)
			}
		})
	}
}
