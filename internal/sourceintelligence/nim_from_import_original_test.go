package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// A Nim "from module import symbol" directive names one module dependency,
// not the selectively imported symbols.
func TestNimFromImportOriginalSourceModules(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []struct {
			value string
			line  int
		}
	}{
		{
			name: "portable",
			source: "from std/strutils import toUpperAscii, strip\n" +
				"import std/math\n" +
				"# from ignored import Fake\n" +
				"const decoy = \"from hidden import Wrong\"\n" +
				"from ./local/module import nil\n",
			want: []struct {
				value string
				line  int
			}{
				{"std/strutils", 1}, {"std/math", 2}, {"./local/module", 5},
			},
		},
	} {
		for _, eol := range []struct{ name, sep string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				result := nimOriginalFromImportAnalysis(t, strings.ReplaceAll(tc.source, "\n", eol.sep))
				checkNimFromImportModules(t, result.Dependencies, tc.want)
			})
		}
	}
	t.Run("unsupported-source-forms", func(t *testing.T) {
		result := nimOriginalFromImportAnalysis(t,
			"from std/strutils as su import nil\n"+
				"from broken/[one, two] import item\n"+
				"from missing import\n"+
				"from foo bar import item\n")
		if result.Analysis.CoverageComplete {
			t.Fatalf("unsupported Nim selective import syntax incorrectly reported complete: %+v", result.Analysis)
		}
		if len(result.Dependencies) != 0 {
			t.Fatalf("unsupported Nim source imports fabricated modules: %+v", result.Dependencies)
		}
		found := false
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "nim-unsupported-from-import" && diagnostic.Range != nil {
				found = true
			}
		}
		if !found {
			t.Fatalf("unsupported Nim selective import lacks source-ranged warning: %+v", result.Analysis.Diagnostics)
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		want              []struct {
			value string
			line  int
		}
	}{
		{
			name: "aporia", env: "SCRIPTHOLD_REAL_NIM_FROM_APORIA",
			marker: "from gdk2 import WINDOW_TYPE_HINT_MENU",
			want: []struct {
				value string
				line  int
			}{
				{"gtk2", 1}, {"gdk2", 2},
			},
		},
		{
			name: "cligen", env: "SCRIPTHOLD_REAL_NIM_FROM_CLIGEN",
			marker: "from std/strutils import toUpperAscii",
			want: []struct {
				value string
				line  int
			}{
				{"std/strutils", 1}, {"cligen/strUt", 2},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Nim source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 50 || info.Size() > 16000 {
				t.Fatalf("original Nim source unavailable or unbounded: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.marker) {
				t.Fatal("original Nim source oracle drift")
			}
			result := nimOriginalFromImportAnalysis(t, string(data))
			checkNimFromImportModules(t, result.Dependencies, tc.want)
		})
	}
}

func nimOriginalFromImportAnalysis(t *testing.T, src string) AnalyzerResult {
	t.Helper()
	result, err := (NimAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("original.nim", src), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("Nim analysis unexpectedly truncated")
	}
	return result
}

func checkNimFromImportModules(t *testing.T, deps []StructuralDependency, want []struct {
	value string
	line  int
}) {
	t.Helper()
	if len(deps) != len(want) {
		t.Fatalf("Nim module imports got %d want %d: %+v", len(deps), len(want), deps)
	}
	for _, expected := range want {
		found := false
		for _, dep := range deps {
			if dep.Value != expected.value {
				continue
			}
			if dep.Kind != StructuralDependencyImport || dep.Alias != "" ||
				dep.Range.Start.Line != expected.line || dep.Range.End.Line != expected.line {
				t.Fatalf("Nim module import has wrong evidence for %+v: %+v", expected, dep)
			}
			found = true
		}
		if !found {
			t.Errorf("Nim original module dependency missing: %+v; got %+v", expected, deps)
		}
	}
}
