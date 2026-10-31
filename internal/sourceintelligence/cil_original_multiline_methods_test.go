package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCILQualifiedOriginalMethods(t *testing.T) {
	cases := []struct {
		name, env, method string
		line              int
	}{
		{"TestMath", "SCRIPTHOLD_REAL_CIL_TESTMATH", "UseProperty", 33},
		{"ILSpy", "SCRIPTHOLD_REAL_CIL_ILSPY_IDSTRING", "ConstValue", 25},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("qualified original CIL source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatal("original CIL source must be regular and <=1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = path
			res, err := (CILAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if res.Analysis.Truncated || !res.Analysis.CoverageComplete {
				t.Fatalf("incomplete: %+v", res.Analysis)
			}
			found := false
			for _, sym := range res.Analysis.Symbols {
				if sym.Name == tc.method && sym.Kind == SymbolKindMethod && sym.NameRange.Start.Line == tc.line {
					found = true
				}
			}
			if !found {
				t.Errorf("qualified original CIL method %s line %d missing; parsed names=%v", tc.method, tc.line, sortedSymbolQualifiedNames(res.Analysis.Symbols))
			}
		})
	}
}
func TestCILAdjacentAndMultilineMethodsLFCRLF(t *testing.T) {
	const text = ".assembly Demo {}\n.class public DemoClass\n{\n .method public static void First() cil managed\n {\n ret\n }\n .method public hidebysig static\n   int32 Second(\n     int32 x\n   ) cil managed\n {\n ret\n }\n .method public static void Third() cil managed\n {\n ret\n }\n}\n"
	for _, tc := range []struct{ name, body string }{{"LF", text}, {"CRLF", strings.ReplaceAll(text, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (CILAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.body), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if res.Analysis.Truncated || !res.Analysis.CoverageComplete {
				t.Fatalf("incomplete: %+v", res.Analysis)
			}
			found := map[string]bool{}
			for _, sym := range res.Analysis.Symbols {
				if sym.Kind == SymbolKindMethod && sym.ParentQualifiedName != "" {
					found[sym.Name] = true
				}
			}
			for _, name := range []string{"First", "Second", "Third"} {
				if !found[name] {
					t.Errorf("missing method %s from CIL class: %+v", name, res.Analysis.Symbols)
				}
			}
		})
	}
}
