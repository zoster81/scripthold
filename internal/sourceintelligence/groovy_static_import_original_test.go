package sourceintelligence

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
)

// Each import is an authored module reference, not a keyword, annotation,
// interpolation or unrelated string. Inspect originals without running them.
func TestGroovyStaticImportsOriginalSourceEvidence(t *testing.T) {
	const fixture = "import static java.lang.Math.min\n" +
		"import static java.lang.Math.*\n" +
		"import groovy.transform.TailRecursive\n" +
		"@Grab('should.not.be.imported')\n" +
		"class Example { String text = 'import fake.Module' }\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkGroovySourceImports(t, tc.source, []string{
				"java.lang.Math.min", "java.lang.Math.*", "groovy.transform.TailRecursive",
			}, []int{1, 2, 3})
		})
	}
	for _, tc := range []struct {
		name, env, marker string
		expected          []string
		lines             []int
	}{
		{"rosetta", "SCRIPTHOLD_REAL_GROOVY_ROSETTA",
			"import static java.lang.Math.min",
			[]string{"groovy.transform.TailRecursive", "java.lang.Math.min"}, []int{1, 2}},
		{"making-java-groovy", "SCRIPTHOLD_REAL_GROOVY_MJG",
			"import static java.lang.Math.*",
			[]string{"java.lang.Math.*", "org.apache.commons.math3.complex.Complex",
				"org.apache.commons.math3.complex.ComplexUtils", "groovy.sql.Sql"},
			[]int{9, 10, 11, 12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Groovy source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("original Groovy source unavailable/unbounded: %v %v", info, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(bytes)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original Groovy source oracle drift")
			}
			checkGroovySourceImports(t, source, tc.expected, tc.lines)
		})
	}
}
func checkGroovySourceImports(t *testing.T, source string, want []string, lines []int) {
	t.Helper()
	result, err := (GroovyAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("original.groovy", source), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatalf("Groovy analysis unexpectedly truncated: %+v", result.Analysis)
	}
	got := make([]string, 0, len(result.Dependencies))
	gotLines := make([]int, 0, len(result.Dependencies))
	for _, d := range result.Dependencies {
		if d.Kind == StructuralDependencyImport {
			got = append(got, d.Value)
			gotLines = append(gotLines, d.Range.Start.Line)
		}
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotLines, lines) {
		t.Fatalf("Groovy import facts=%q lines=%v, want %q lines %v", got, gotLines, want, lines)
	}
}
