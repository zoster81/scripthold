package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The module target and the locally bound alias are distinct source facts.
// Never append "as Alias" to the imported path or its source range.
func TestGroovyAliasImportOriginalSourceEvidence(t *testing.T) {
	const fixture = "import java.sql.Date as SqlDate\n" +
		"import static java.lang.Math.abs as mabs\n" +
		"import java.time.Instant\n" +
		"class Example {}\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", fixture}, {"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := groovyAliasAnalysis(t, tc.text)
			if len(result.Dependencies) != 3 {
				t.Fatalf("dependencies=%+v", result.Dependencies)
			}
			groovyCheckAliasImport(t, result.Dependencies, "java.sql.Date", "SqlDate", 1)
			groovyCheckAliasImport(t, result.Dependencies, "java.lang.Math.abs", "mabs", 2)
			groovyCheckAliasImport(t, result.Dependencies, "java.time.Instant", "", 3)
		})
	}
	for _, tc := range []struct {
		name, env, marker, target, alias string
		line                             int
	}{
		{"jyreport", "SCRIPTHOLD_REAL_GROOVY_JYREPORT",
			"import java.sql.Date as SqlDate", "java.sql.Date", "SqlDate", 3},
		{"matrix", "SCRIPTHOLD_REAL_GROOVY_MATRIX",
			"import java.sql.Date as SqlDate", "java.sql.Date", "SqlDate", 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent Groovy original not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("Groovy original unavailable/unbounded: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.marker) {
				t.Fatal("Groovy original oracle drift")
			}
			result := groovyAliasAnalysis(t, string(data))
			groovyCheckAliasImport(t, result.Dependencies, tc.target, tc.alias, tc.line)
		})
	}
}
func groovyAliasAnalysis(t *testing.T, text string) AnalyzerResult {
	t.Helper()
	result, err := (GroovyAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("original.groovy", text), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("Groovy alias original analysis truncated")
	}
	return result
}
func groovyCheckAliasImport(t *testing.T, deps []StructuralDependency, target, alias string, line int) {
	t.Helper()
	for _, d := range deps {
		if d.Value != target {
			continue
		}
		if d.Kind != StructuralDependencyImport || d.Alias != alias ||
			d.Range.Start.Line != line || d.Range.End.Line != line {
			t.Fatalf("import %s alias=%q line=%d got %+v", target, alias, line, d)
		}
		return
	}
	t.Fatalf("missing Groovy authored import %q alias=%q line=%d; got=%+v", target, alias, line, deps)
}
