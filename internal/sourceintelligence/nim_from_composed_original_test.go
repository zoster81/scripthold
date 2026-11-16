package sourceintelligence

import (
	"os"
	"strings"
	"testing"
)

type nimComposedFromModuleWant struct {
	module, authored string
	line             int
}

// A Nim quoted-root / module source path is one module, never the names
// selectively imported from it; the range covers the entire authored path.
func TestNimComposedFromOriginalModules(t *testing.T) {
	const fixture = "from \"../lib\" / bif import BifModule, containsSym\n" +
		"from \"$nim\" / compiler/nimblecmd import getPathVersionChecksum\n" +
		"from std/strutils import toUpperAscii\n" +
		"# from \"phantom\" / fake import Ignored\n" +
		"let decoy = \"from \\\"ghost\\\" / fake import Ignored\"\n"
	want := []nimComposedFromModuleWant{
		{"../lib/bif", "\"../lib\" / bif", 1},
		{"$nim/compiler/nimblecmd", "\"$nim\" / compiler/nimblecmd", 2},
		{"std/strutils", "std/strutils", 3},
	}
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			result := nimOriginalFromImportAnalysis(t, tc.source)
			for _, expected := range want {
				checkNimComposedFromModule(t, result.Dependencies, tc.source, expected)
			}
			if len(result.Dependencies) != len(want) {
				t.Fatalf("Nim composed from fabricated selected-member modules: %+v", result.Dependencies)
			}
		})
	}
	t.Run("unsupported-composed-paths-fail-closed", func(t *testing.T) {
		source := "from \"bad space\" / mod import Type\n" +
			"from \"$other\" / mod import Type\n" +
			"from \"base\" / missing path import Type\n" +
			"from \"base\" / grouped/[one,two] import Type\n" +
			"from \"base\" /mod as alias import Type\n" +
			"from \"base\" / mod import\n" +
			"from \"base\" / mod import Type,\n"
		result := nimOriginalFromImportAnalysis(t, source)
		if result.Analysis.CoverageComplete || len(result.Dependencies) != 0 {
			t.Fatalf("unsupported composed Nim imports fabricated source evidence: %+v", result)
		}
		warnings := 0
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "nim-unsupported-from-import" && diagnostic.Range != nil {
				warnings++
			}
		}
		if warnings != 7 {
			t.Fatalf("expected seven source-ranged composed Nim warnings; got %d: %+v", warnings, result.Analysis.Diagnostics)
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		want              nimComposedFromModuleWant
	}{
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_COMPOSED_COMPILER",
			marker: "from \"../dist/nimony/src/lib\" / bif import BifModule, containsSym",
			want: nimComposedFromModuleWant{
				"../dist/nimony/src/lib/bif",
				"\"../dist/nimony/src/lib\" / bif", 40,
			},
		},
		{
			name: "nimble", env: "SCRIPTHOLD_REAL_NIM_COMPOSED_NIMBLE",
			marker: "from \"$nim\" / compiler/nimblecmd import getPathVersionChecksum",
			want: nimComposedFromModuleWant{
				"$nim/compiler/nimblecmd",
				"\"$nim\" / compiler/nimblecmd", 12,
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independently authored full Nim composed-from original not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 ||
				info.Size() > 24000 {
				t.Fatalf("original source missing or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original Nim composed from import marker drift")
			}
			result := nimOriginalFromImportAnalysis(t, source)
			checkNimComposedFromModule(t, result.Dependencies, source, tc.want)
			for _, dep := range result.Dependencies {
				if dep.Range.Start.Line == tc.want.line && dep.Value != tc.want.module {
					t.Fatalf("Nim composed from invented a module: %+v", dep)
				}
			}
		})
	}
}

func checkNimComposedFromModule(t *testing.T, deps []StructuralDependency, source string, want nimComposedFromModuleWant) {
	t.Helper()
	lines := strings.Split(source, "\n")
	if want.line < 1 || want.line > len(lines) {
		t.Fatalf("invalid original Nim module line: %+v", want)
	}
	column := strings.Index(lines[want.line-1], want.authored) + 1
	if column == 0 {
		t.Fatalf("authored composed Nim module not found: %+v", want)
	}
	found := false
	for _, dep := range deps {
		if dep.Value != want.module {
			continue
		}
		if dep.Kind != StructuralDependencyImport || dep.Alias != "" ||
			dep.Range.Start.Line != want.line || dep.Range.End.Line != want.line ||
			dep.Range.Start.Column != column ||
			dep.Range.End.Column-dep.Range.Start.Column != len(want.authored) {
			t.Fatalf("composed Nim module has incorrect source evidence: %+v; want %+v", dep, want)
		}
		found = true
	}
	if !found {
		t.Fatalf("composed Nim module missing: %+v; got %+v", want, deps)
	}
}
