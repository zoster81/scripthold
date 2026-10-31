package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestBashNvmWrappedOriginal(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_BASH_NVM_INSTALL")
	if path == "" {
		t.Skip("set SCRIPTHOLD_REAL_BASH_NVM_INSTALL to the qualified original NVM install.sh")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		t.Fatal("original Bash fixture must be a regular file of at most 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(data))
	doc.Path = path
	result, err := (BashAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("original NVM analysis incomplete: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for name, line := range map[string]int{"nvm_has": 5, "nvm_has_executable": 11, "nvm_reset": 524} {
		got, ok := byName[name]
		if !ok || got.Kind != SymbolKindFunction || got.NameRange.Start.Line != line {
			t.Errorf("%s: found=%v symbol=%+v, want function at line %d", name, ok, got, line)
		}
	}
}

func TestBashGroupingDeclarationsAndNoNestedLeaks(t *testing.T) {
	source := "{ # protective script group\n" +
		"first() {\n  inner() { :; }\n}\n" +
		"{ second() { :; } }\n" +
		"echo \"$(printf 'fake() { :; }')\"\n" +
		"}\n" +
		"after() { :; }\n"
	for _, tc := range []struct{ name, code string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			for _, analyzer := range []SourceAnalyzer{BashAnalyzer{}, ShellAnalyzer{}} {
				t.Run(analyzer.Language(), func(t *testing.T) {
					result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.code), testAnalyzeOptions(true, 128))
					if err != nil {
						t.Fatal(err)
					}
					if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
						t.Fatalf("group analysis incomplete: %+v", result.Analysis)
					}
					byName := symbolsByQualifiedName(result.Analysis.Symbols)
					for _, name := range []string{"first", "second", "after"} {
						if got, ok := byName[name]; !ok || got.Kind != SymbolKindFunction {
							t.Errorf("missing grouped function %s: %v; all=%v", name, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
						}
					}
					for _, name := range []string{"inner", "fake"} {
						if got, ok := byName[name]; ok {
							t.Errorf("leaked function %s: %+v", name, got)
						}
					}
				})
			}
		})
	}
}
