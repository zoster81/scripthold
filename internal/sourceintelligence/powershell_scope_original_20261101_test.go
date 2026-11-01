package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPowerShellScopedFunctionNamesAndHashtableFields(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
			source := "function global:prompt { ' ' }\nfunction script:Get-Result { 1 }\n$export = @{\n    Function = @(\n        'Get-Result'\n    )\n    Filter = @('one')\n}\nfunction Get-Later { 2 }\n"
			result, err := (PowerShellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("unexpected partial coverage: %+v", result.Analysis.Diagnostics)
			}
			wanted := map[string]int{"prompt": 1, "Get-Result": 2, "Get-Later": 9}
			for _, s := range result.Analysis.Symbols {
				if s.Kind != SymbolKindFunction {
					continue
				}
				if line, ok := wanted[s.Name]; ok {
					if s.NameRange.Start.Line != line {
						t.Errorf("%q on line %d, want %d", s.Name, s.NameRange.Start.Line, line)
					}
					delete(wanted, s.Name)
				} else {
					t.Errorf("unexpected function %q at line %d", s.Name, s.NameRange.Start.Line)
				}
			}
			if len(wanted) > 0 {
				t.Errorf("missing functions %v; symbols: %v", wanted, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestPowerShellOriginalScopedPromptAndHashtable(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_POWERSHELL_POSHGIT_20261101")
	if path == "" {
		t.Skip("opt-in read-only original project not configured")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (PowerShellAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(string(b)), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range result.Analysis.Symbols {
		if s.Kind != SymbolKindFunction {
			continue
		}
		if s.Name == "prompt" && s.NameRange.Start.Line == 137 {
			found = true
		}
		if s.Name == "global" || s.Name == "@" {
			t.Errorf("spurious function %q at line %d", s.Name, s.NameRange.Start.Line)
		}
	}
	if !found {
		t.Errorf("missing original scoped function 'prompt'; symbols: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}
