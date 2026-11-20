package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Flow's ambient module boundary is token-based: spacing and line breaks
// between declaration keywords must not silently remove edit targets.
func TestFlowAmbientModuleWhitespacePreservesDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name, separation string
	}{
		{"two-spaces", "  "},
		{"tab", "\t"},
		{"LF", "\n"},
		{"CRLF", "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := "declare" + tc.separation + "module 'flow-ambient' {\n" +
				"  declare class Service {\n" +
				"    run(): void;\n" +
				"  }\n" +
				"}\n" +
				"const reference = module.exports;\n"
			document := testSourceDocument("ambient.js.flow", code)
			result, err := (FlowAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("well-formed ambient module lost coverage: %+v", result.Analysis)
			}
			want := map[string]bool{
				"flow-ambient":             false,
				"flow-ambient.Service":     false,
				"flow-ambient.Service.run": false,
			}
			for _, s := range result.Analysis.Symbols {
				if strings.Contains(s.QualifiedName, "exports") || s.Name == "reference" && s.ParentQualifiedName != "" {
					t.Fatalf("runtime module reference fabricated or misowned: %+v", s)
				}
				if _, ok := want[s.QualifiedName]; ok {
					want[s.QualifiedName] = true
				}
			}
			for name, found := range want {
				if !found {
					t.Errorf("ambient declaration missing despite complete result: %s", name)
				}
			}
		})
	}
}

// The shared TypeScript-family parser must preserve the same declaration
// boundary without claiming that an ordinary "async" line is a modifier.
func TestTypeScriptAmbientModuleAndECMAScriptAsyncLineBoundary(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "lf", "\r\n": "crlf"}[eol], func(t *testing.T) {
			ts := "declare" + eol + "module 'typed-ambient' {" + eol +
				"  declare class Keeper { find(): void; }" + eol + "}" + eol
			result, err := (TypeScriptAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("module.ts", ts), testAnalyzeOptions(true, 64))
			if err != nil || !result.Analysis.CoverageComplete {
				t.Fatalf("TypeScript declared module analysis failed: %v %+v", err, result.Analysis)
			}
			want := map[string]bool{"typed-ambient": false, "typed-ambient.Keeper": false, "typed-ambient.Keeper.find": false}
			for _, s := range result.Analysis.Symbols {
				if _, ok := want[s.QualifiedName]; ok {
					want[s.QualifiedName] = true
				}
			}
			for name, seen := range want {
				if !seen {
					t.Errorf("shared TypeScript parser lost ambient declaration %s", name)
				}
			}

			js := "async" + eol + "function ordinary() {}" + eol
			ordinary, err := (JavaScriptAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("ordinary.js", js), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, s := range ordinary.Analysis.Symbols {
				if s.Name != "ordinary" {
					continue
				}
				found = true
				for _, modifier := range s.Modifiers {
					if modifier == "async" {
						t.Errorf("line-separated async was misapplied to ordinary function: %+v", s)
					}
				}
			}
			if !found {
				t.Fatal("ordinary function after async line break was omitted")
			}
		})
	}
}
