package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestCILGenericArityClassOwnerAndNeighbor(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
			source := strings.Join([]string{
				".assembly Demo {}",
				".class public auto ansi sealed D`1<(I) T>",
				"  extends [mscorlib]System.MulticastDelegate",
				"{",
				"  .method public static void BeginInvoke() { ret }",
				"  .method public static void EndInvoke() { ret }",
				"}",
				".class public auto ansi Plain extends [mscorlib]System.Object",
				"{",
				"  .method public static void Neighbor() { ret }",
				"}",
			}, eol) + eol
			result, err := (CILAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("cil-generic-class.il", source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("supported CIL scope unexpectedly incomplete: %+v", result.Analysis)
			}
			want := map[string]int{
				"Demo.D`1":             2,
				"Demo.D`1.BeginInvoke": 5,
				"Demo.D`1.EndInvoke":   6,
				"Demo.Plain":           8,
				"Demo.Plain.Neighbor":  10,
			}
			for _, s := range result.Analysis.Symbols {
				if s.Name == "T" || s.QualifiedName == "Demo.T" || s.ParentQualifiedName == "Demo.T" {
					t.Errorf("CIL generic parameter fabricated as declared owner: %+v", s)
				}
				if line, ok := want[s.QualifiedName]; ok {
					if s.NameRange.Start.Line != line {
						t.Errorf("%s declared at line %d, expected %d", s.QualifiedName, s.NameRange.Start.Line, line)
					}
					delete(want, s.QualifiedName)
				}
			}
			if len(want) != 0 {
				t.Fatalf("real generic class and members absent: %v; got=%+v", want, result.Analysis.Symbols)
			}
		})
	}
}

func TestCILUnclosedGenericClassFailsConservatively(t *testing.T) {
	source := ".assembly Demo {}\n.class public D`1<(I) T\n{\n  .method public static void Ghost() { ret }\n}\n"
	result, err := (CILAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("cil-incomplete-generic.il", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatal("unclosed generic type header cannot have complete structural coverage")
	}
	for _, sym := range result.Analysis.Symbols {
		if sym.Name == "T" || sym.Name == "D`1" || sym.Name == "Ghost" {
			t.Fatalf("malformed generic header fabricated declaration: %+v", sym)
		}
	}
}
