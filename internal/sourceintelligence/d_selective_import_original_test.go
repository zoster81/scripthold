package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// A selective D import names a module before ':'; selected members and
// local binding aliases must not be appended to that module dependency.
func TestDSelectiveImportsOriginalSourceEvidence(t *testing.T) {
	const fixture = "module demo;\n" +
		"import std.stdio : writeln, writefln;\n" +
		"import std.array : join;\n" +
		"import std.conv, std.string : split;\n" +
		"import fs = std.file;\n" +
		"import std.process;\n" +
		"void outer() { import local.hidden : member; }\n" +
		"string decoy = \"import fake.literal : falseName;\";\n" +
		"/+ import fake.comment : falseName; +/\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", fixture}, {"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := dSelectiveAnalysis(t, tc.text)
			for _, expected := range []struct {
				target, alias string
				line          int
			}{
				{"std.stdio", "", 2},
				{"std.array", "", 3},
				{"std.conv", "", 4},
				{"std.string", "", 4},
				{"std.file", "fs", 5},
				{"std.process", "", 6},
			} {
				assertDModuleImport(t, result.Dependencies, expected.target, expected.alias, expected.line)
			}
			for _, dep := range result.Dependencies {
				if strings.ContainsAny(dep.Value, ":,=;") ||
					strings.HasPrefix(dep.Value, "local.") ||
					strings.HasPrefix(dep.Value, "fake.") {
					t.Fatalf("unsupported or invented D dependency %+v", dep)
				}
			}
		})
	}
	t.Run("unsupported-selector-and-module", func(t *testing.T) {
		result := dSelectiveAnalysis(t, "import std.good : member(nested);\n"+
			"import broken..module : item;\n"+
			"import good.path : name = original;\n")
		if result.Analysis.CoverageComplete {
			t.Fatalf("unsupported D import grammar incorrectly reported complete: %+v", result.Analysis)
		}
		foundWarning := false
		for _, diagnostic := range result.Analysis.Diagnostics {
			if diagnostic.Code == "d-unsupported-import-form" && diagnostic.Range != nil {
				foundWarning = true
			}
		}
		if !foundWarning {
			t.Fatalf("unsupported D import lacks diagnostic: %+v", result.Analysis.Diagnostics)
		}
		assertDModuleImport(t, result.Dependencies, "std.good", "", 1)
		assertDModuleImport(t, result.Dependencies, "good.path", "", 3)
		for _, dep := range result.Dependencies {
			if strings.Contains(dep.Value, "broken") || strings.ContainsAny(dep.Value, ":,=;") {
				t.Fatalf("unsupported D import fabricated module %+v", dep)
			}
		}
	})

	for _, tc := range []struct {
		name, env, marker string
		targets           []struct {
			module string
			line   int
		}
	}{
		{"yes", "SCRIPTHOLD_REAL_D_SELECTIVE_YES",
			"import std.stdio : writeln;", []struct {
				module string
				line   int
			}{
				{"std.stdio", 1}, {"std.array", 2},
			}},
		{"powernex", "SCRIPTHOLD_REAL_D_SELECTIVE_POWERNEX",
			"import core.time : msecs;", []struct {
				module string
				line   int
			}{
				{"core.time", 6}, {"std.stdio", 3},
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original D source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("original D source unavailable/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("D import source oracle drift")
			}
			result := dSelectiveAnalysis(t, string(body))
			for _, expected := range tc.targets {
				assertDModuleImport(t, result.Dependencies, expected.module, "", expected.line)
			}
			for _, dep := range result.Dependencies {
				if strings.ContainsAny(dep.Value, ":,=;") {
					t.Fatalf("original D selective dependency contains selector: %+v", dep)
				}
			}
		})
	}
	t.Run("function-local-original", func(t *testing.T) {
		path := os.Getenv("SCRIPTHOLD_REAL_D_SELECTIVE_TSV")
		if path == "" {
			t.Skip("independent original D local-scope control not configured")
		}
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 10000 {
			t.Fatalf("original D local source unavailable/unbounded: %v %v", info, err)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "import std.process : escapeShellCommand, executeShell;") {
			t.Fatal("original local-scope D import oracle drift")
		}
		result := dSelectiveAnalysis(t, string(body))
		if len(result.Dependencies) != 0 {
			t.Fatalf("function-local imports must not be asserted as top-level imports: %+v", result.Dependencies)
		}
	})
}

func dSelectiveAnalysis(t *testing.T, source string) AnalyzerResult {
	t.Helper()
	result, err := (DAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("selective.d", source), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("D original-source analysis unexpectedly truncated")
	}
	return result
}

func assertDModuleImport(t *testing.T, deps []StructuralDependency, target, alias string, line int) {
	t.Helper()
	for _, dep := range deps {
		if dep.Value != target {
			continue
		}
		if dep.Kind != StructuralDependencyImport || dep.Alias != alias ||
			dep.Range.Start.Line != line || dep.Range.End.Line != line {
			t.Fatalf("D import %q alias=%q line=%d got %+v", target, alias, line, dep)
		}
		return
	}
	t.Fatalf("missing D import %q alias=%q line=%d: %+v", target, alias, line, deps)
}
