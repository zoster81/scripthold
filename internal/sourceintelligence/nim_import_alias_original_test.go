package sourceintelligence

import (
	"os"
	"strings"
	"testing"
)

type nimAliasModuleExpectation struct {
	module, alias string
	line          int
}

// Nim "import module as alias" imports the authored module, not a fabricated
// module whose name includes its local alias.
func TestNimImportAliasOriginalSourceDependencies(t *testing.T) {
	const fixture = "import os as sys, std/strutils\n" +
		"import ./local/math as lm, ./other\n" +
		"# import ghost as pretend\n" +
		"let decoy = \"import fake as fakeAlias\"\n"
	want := []nimAliasModuleExpectation{
		{"os", "sys", 1}, {"std/strutils", "", 1},
		{"./local/math", "lm", 2}, {"./other", "", 2},
	}
	for _, tc := range []struct{ name, content string }{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			result := nimOriginalFromImportAnalysis(t, tc.content)
			if len(result.Dependencies) != len(want) {
				t.Fatalf("unexpected Nim import count: got %+v, want %+v", result.Dependencies, want)
			}
			for _, expected := range want {
				assertNimAliasedModule(t, result.Dependencies, expected)
			}
		})
	}
	t.Run("unsupported-aliases-fail-closed", func(t *testing.T) {
		result := nimOriginalFromImportAnalysis(t,
			"import valid, missing as\n"+
				"import repeated as first as second\n"+
				"import foo bar as misleading\n"+
				"import std/[math, random] as grouped\n"+
				"import broken..module as doubledDot\n"+
				"import broken//module as doubledSlash\n")
		if result.Analysis.CoverageComplete {
			t.Fatalf("unsupported Nim import aliases incorrectly report full coverage: %+v", result.Analysis)
		}
		if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "valid" {
			t.Fatalf("malformed Nim import aliases fabricated module targets: %+v", result.Dependencies)
		}
		warnings := 0
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "nim-unsupported-import-alias" && diagnostic.Range != nil {
				warnings++
			}
		}
		if warnings != 6 {
			t.Fatalf("expected six source-ranged Nim alias warnings, got %d: %+v", warnings, result.Analysis.Diagnostics)
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		want              []nimAliasModuleExpectation
	}{
		{
			name: "bu", env: "SCRIPTHOLD_REAL_NIM_ALIAS_BU",
			marker: "import std/[math, random], memfiles as mf, cligen/sysUt",
			want: []nimAliasModuleExpectation{
				{"memfiles", "mf", 11}, {"cligen/sysUt", "", 11},
			},
		},
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_ALIAS_COMPILER",
			marker: "import lineinfos as astli",
			want: []nimAliasModuleExpectation{
				{"lineinfos", "astli", 36}, {"ast2nif", "", 37},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Nim alias source is not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 ||
				info.Size() > 16000 {
				t.Fatalf("original Nim alias file unavailable or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original Nim alias source marker changed")
			}
			result := nimOriginalFromImportAnalysis(t, string(body))
			for _, expected := range tc.want {
				assertNimAliasedModule(t, result.Dependencies, expected)
			}
		})
	}
}

func assertNimAliasedModule(t *testing.T, dependencies []StructuralDependency, want nimAliasModuleExpectation) {
	t.Helper()
	found := false
	for _, dep := range dependencies {
		if strings.Contains(dep.Value, " as ") || strings.HasSuffix(dep.Value, "as"+dep.Alias) ||
			strings.Contains(dep.Value, "asmf") || strings.Contains(dep.Value, "asastli") {
			t.Fatalf("Nim import invented a module from an alias: %+v", dep)
		}
		if dep.Value != want.module {
			continue
		}
		if dep.Kind != StructuralDependencyImport || dep.Alias != want.alias ||
			dep.Range.Start.Line != want.line || dep.Range.End.Line != want.line ||
			dep.Range.End.Column-dep.Range.Start.Column != len(want.module) {
			t.Fatalf("Nim import module source range or alias is incorrect: want %+v, got %+v", want, dep)
		}
		found = true
	}
	if !found {
		t.Fatalf("Nim original alias module %+v missing from %+v", want, dependencies)
	}
}
