package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// The comma-delimited Dim syntax is present in original AVG 8 and AVStatus VBScript
// sources. All names must be searchable, not only the first name on each line.
func TestVBScriptCommaSeparatedDimDeclarations(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(map[string]string{"\n": "LF", "\r\n": "CRLF"}[eol], func(t *testing.T) {
			text := strings.Join([]string{
				"Option Explicit",
				"Dim wshShell, wshSysEnv, ProgramFiles, Executable, updatecmd, scancmd, OSType",
				"Dim entries(1, 2), NoAVBehavior, strWMINamespace2",
				"Sub Run()",
				"  Dim localOne, localTwo",
				"  message = \"Dim Ghost, Wrong\"",
				"End Sub",
			}, eol) + eol
			result, err := (VBScriptAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid VBScript Dim syntax was incomplete: %+v", result.Analysis)
			}
			want := []struct {
				name      string
				line      int
				qualified string
			}{
				{"wshShell", 2, "wshShell"},
				{"wshSysEnv", 2, "wshSysEnv"},
				{"ProgramFiles", 2, "ProgramFiles"},
				{"Executable", 2, "Executable"},
				{"updatecmd", 2, "updatecmd"},
				{"scancmd", 2, "scancmd"},
				{"OSType", 2, "OSType"},
				{"entries", 3, "entries"},
				{"NoAVBehavior", 3, "NoAVBehavior"},
				{"strWMINamespace2", 3, "strWMINamespace2"},
				{"localOne", 5, "Run.localOne"},
				{"localTwo", 5, "Run.localTwo"},
			}
			byName := map[string]NormalizedSymbol{}
			for _, sym := range result.Analysis.Symbols {
				if sym.Kind == SymbolKindVariable {
					byName[sym.QualifiedName] = sym
				}
				if sym.Name == "Ghost" || sym.Name == "Wrong" {
					t.Errorf("string literal misread as variable: %+v", sym)
				}
			}
			for _, w := range want {
				got, ok := byName[w.qualified]
				if !ok || got.Name != w.name || got.DeclarationRange.Start.Line != w.line ||
					got.NameRange.Start.Line != w.line {
					t.Errorf("missing/wrong Dim variable %s at line %d, got=%+v found=%v", w.qualified, w.line, got, ok)
				}
			}
		})
	}
}
