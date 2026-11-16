package sourceintelligence

import (
	"os"
	"strings"
	"testing"
)

type nimGroupedImportExpected struct {
	module string
	member string
	line   int
}

// Grouped Nim imports refer to each authored module; bracket text is not a
// navigable module path. Ranges are anchored to each member token in source.
func TestNimGroupedImportOriginalSourceModules(t *testing.T) {
	portable := "import std/[math, random], os as sys\n" +
		"import std / [strutils, sets]\n" +
		"# import phantom/[hidden, fake]\n" +
		"let decoy = \"import secret/[ignored]\"\n"
	want := []nimGroupedImportExpected{
		{"std/math", "math", 1},
		{"std/random", "random", 1},
		{"std/strutils", "strutils", 2},
		{"std/sets", "sets", 2},
	}
	for _, tc := range []struct{ name, source string }{
		{"LF", portable},
		{"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			result := nimOriginalFromImportAnalysis(t, tc.source)
			for _, expected := range want {
				checkNimGroupedImport(t, result.Dependencies, tc.source, expected)
			}
			for _, dep := range result.Dependencies {
				if strings.Contains(dep.Value, "[") || strings.Contains(dep.Value, "]") {
					t.Fatalf("group was invented as one Nim module: %+v", dep)
				}
			}
		})
	}
	t.Run("malformed-groups-fail-closed", func(t *testing.T) {
		source := "import valid\n" +
			"import std/[math,]\n" +
			"import std/[math,, random]\n" +
			"import std/[]\n" +
			"import std/[math, other/[inner]]\n" +
			"import foo bar/[math, random]\n" +
			"import std/[math, random] except item\n" +
			"import std/[math, random]/suffix\n"
		result := nimOriginalFromImportAnalysis(t, source)
		if result.Analysis.CoverageComplete {
			t.Fatalf("unsupported grouped Nim imports reported complete: %+v", result.Analysis)
		}
		if len(result.Dependencies) != 1 || result.Dependencies[0].Value != "valid" {
			t.Fatalf("malformed grouped imports fabricated source modules: %+v", result.Dependencies)
		}
		warnings := 0
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "nim-unsupported-grouped-import" && diagnostic.Range != nil {
				warnings++
			}
		}
		if warnings != 7 {
			t.Fatalf("expected seven source-ranged grouped-import warnings, got %d: %+v", warnings, result.Analysis.Diagnostics)
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		want              []nimGroupedImportExpected
	}{
		{
			name: "bu", env: "SCRIPTHOLD_REAL_NIM_GROUP_BU",
			marker: "import std/[math, random], memfiles as mf, cligen/sysUt",
			want: []nimGroupedImportExpected{
				{"std/math", "math", 11}, {"std/random", "random", 11},
			},
		},
		{
			name: "compiler", env: "SCRIPTHOLD_REAL_NIM_GROUP_COMPILER",
			marker: "import std / [os, strutils, sets]",
			want: []nimGroupedImportExpected{
				{"std/os", "os", 34}, {"std/strutils", "strutils", 34},
				{"std/sets", "sets", 34},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original Nim grouped-import source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 ||
				info.Size() > 16000 {
				t.Fatalf("original Nim source missing or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original grouped-import source oracle drift")
			}
			result := nimOriginalFromImportAnalysis(t, source)
			for _, expected := range tc.want {
				checkNimGroupedImport(t, result.Dependencies, source, expected)
			}
			for _, dep := range result.Dependencies {
				if dep.Range.Start.Line == tc.want[0].line &&
					(strings.Contains(dep.Value, "[") || strings.Contains(dep.Value, "]")) {
					t.Fatalf("original grouped import fabricated a module: %+v", dep)
				}
			}
		})
	}
}

func checkNimGroupedImport(t *testing.T, deps []StructuralDependency, source string, want nimGroupedImportExpected) {
	t.Helper()
	lines := strings.Split(source, "\n")
	if want.line <= 0 || want.line > len(lines) {
		t.Fatalf("invalid source oracle line: %+v", want)
	}
	column := strings.Index(lines[want.line-1], want.member)
	if column < 0 {
		t.Fatalf("original grouped-import member not present in source: %+v", want)
	}
	for _, dep := range deps {
		if dep.Value != want.module {
			continue
		}
		if dep.Kind != StructuralDependencyImport || dep.Alias != "" ||
			dep.Range.Start.Line != want.line || dep.Range.End.Line != want.line ||
			dep.Range.Start.Column != column+1 ||
			dep.Range.End.Column-dep.Range.Start.Column != len(want.member) {
			t.Fatalf("grouped module has wrong authored member source evidence: %+v; want %+v", dep, want)
		}
		return
	}
	t.Fatalf("missing Nim grouped module %+v; got %+v", want, deps)
}
