package sourceintelligence

import (
	"os"
	"strings"
	"testing"
)

type nimExceptModuleWant struct {
	module, spelling string
	line             int
}

// "except" excludes imported member identifiers, not source-unit modules.
func TestNimExceptImportOriginalSourceModules(t *testing.T) {
	const fixture = "import system except TResult\n" +
		"import \"../lib/nifcore\" except symName, strVal\n" +
		"import std/strutils\n" +
		"# import phantom except fake\n" +
		"let decoy = \"import ghost except hidden\"\n"
	want := []nimExceptModuleWant{
		{"system", "system", 1},
		{"../lib/nifcore", "\"../lib/nifcore\"", 2},
		{"std/strutils", "std/strutils", 3},
	}
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			result := nimOriginalFromImportAnalysis(t, tc.source)
			for _, expected := range want {
				assertNimExceptImport(t, result.Dependencies, tc.source, expected)
			}
			if len(result.Dependencies) != len(want) {
				t.Fatalf("Nim except fabricated selector dependencies: %+v", result.Dependencies)
			}
		})
	}
	t.Run("unsupported-excepts-fail-closed", func(t *testing.T) {
		source := "import stable except ignored\n" +
			"import foo except\n" +
			"import foo except a,\n" +
			"import foo except a,, b\n" +
			"import foo, bar except x\n" +
			"import foo except a + b\n" +
			"import \"foo bar\" except x\n" +
			"import foo except a.b\n" +
			"import foo bar except x\n" +
			"import foo except a as b\n"
		result := nimOriginalFromImportAnalysis(t, source)
		if result.Analysis.CoverageComplete {
			t.Fatalf("unsupported Nim except syntax claimed full coverage: %+v", result.Analysis)
		}
		if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "stable" {
			t.Fatalf("malformed except invented source-unit imports: %+v", result.Dependencies)
		}
		warnings := 0
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "nim-unsupported-import-except" && diagnostic.Range != nil {
				warnings++
			}
		}
		if warnings != 9 {
			t.Fatalf("expected nine source-ranged Nim except warnings, got %d: %+v", warnings, result.Analysis.Diagnostics)
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		want              nimExceptModuleWant
	}{
		{
			name: "nimble", env: "SCRIPTHOLD_REAL_NIM_EXCEPT_NIMBLE",
			marker: "import system except TResult",
			want:   nimExceptModuleWant{"system", "system", 7},
		},
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_EXCEPT_COMPILER",
			marker: "import \"../dist/nimony/src/lib/nifcore\" except symName, strVal, poolSym, poolStr, lineInfoFile",
			want: nimExceptModuleWant{
				"../dist/nimony/src/lib/nifcore",
				"\"../dist/nimony/src/lib/nifcore\"", 39,
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Nim except source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 ||
				info.Size() > 16000 {
				t.Fatalf("original Nim source unavailable or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original Nim except source marker changed")
			}
			result := nimOriginalFromImportAnalysis(t, source)
			assertNimExceptImport(t, result.Dependencies, source, tc.want)
			for _, dep := range result.Dependencies {
				if dep.Range.Start.Line != tc.want.line {
					continue
				}
				if dep.Value != tc.want.module {
					t.Fatalf("Nim except generated a fake source unit: %+v", dep)
				}
			}
		})
	}
}

func assertNimExceptImport(t *testing.T, deps []StructuralDependency, source string, want nimExceptModuleWant) {
	t.Helper()
	lines := strings.Split(source, "\n")
	if want.line < 1 || want.line > len(lines) {
		t.Fatalf("invalid original Nim except line: %+v", want)
	}
	column := strings.Index(lines[want.line-1], want.spelling)
	if column < 0 {
		t.Fatalf("original Nim except source token not found: %+v", want)
	}
	found := false
	for _, dep := range deps {
		if dep.Value != want.module {
			continue
		}
		if dep.Kind != StructuralDependencyImport || dep.Alias != "" ||
			dep.Range.Start.Line != want.line || dep.Range.End.Line != want.line ||
			dep.Range.Start.Column != column+1 ||
			dep.Range.End.Column-dep.Range.Start.Column != len(want.spelling) {
			t.Fatalf("Nim except target or source range incorrect: %+v, want %+v", dep, want)
		}
		found = true
	}
	if !found {
		t.Fatalf("original Nim except module missing: %+v, got %+v", want, deps)
	}
}
