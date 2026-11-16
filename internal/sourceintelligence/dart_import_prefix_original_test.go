package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// A Dart import prefix is a local binding, not part of the URI. Keep both
// source facts independently, including deferred and show/hide directives.
func TestDartImportPrefixOriginalSources(t *testing.T) {
	const source = "import 'package:sample/a.dart' as a;\n" +
		"import 'package:sample/b.dart' deferred as b;\n" +
		"import 'package:sample/c.dart' as c show Item;\n" +
		"import 'package:sample/plain.dart';\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := dartImportPrefixAnalysis(t, tc.text)
			for _, expected := range []struct {
				path, alias string
				line        int
			}{
				{"package:sample/a.dart", "a", 1},
				{"package:sample/b.dart", "b", 2},
				{"package:sample/c.dart", "c", 3},
				{"package:sample/plain.dart", "", 4},
			} {
				assertDartImportPrefix(t, result.Dependencies, expected.path, expected.alias, expected.line)
			}
		})
	}
	for _, tc := range []struct {
		name, env, marker, path, prefix string
		line                            int
	}{
		{"google-photos", "SCRIPTHOLD_REAL_DART_GPH",
			"import 'package:gpth/interactive.dart' as interactive;",
			"package:gpth/interactive.dart", "interactive", 4},
		{"dartx", "SCRIPTHOLD_REAL_DART_DARTX",
			"import 'package:collection/collection.dart' as collection;",
			"package:collection/collection.dart", "collection", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 20000 {
				t.Fatalf("original Dart source missing/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original Dart import oracle drift")
			}
			result := dartImportPrefixAnalysis(t, string(body))
			assertDartImportPrefix(t, result.Dependencies, tc.path, tc.prefix, tc.line)
		})
	}
}

func dartImportPrefixAnalysis(t *testing.T, source string) AnalyzerResult {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("original.dart", source), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("Dart import prefix original analysis truncated")
	}
	return result
}

func assertDartImportPrefix(t *testing.T, dependencies []StructuralDependency, path, alias string, line int) {
	t.Helper()
	for _, dep := range dependencies {
		if dep.Value != path {
			continue
		}
		if dep.Kind != StructuralDependencyImport || dep.Alias != alias ||
			dep.Range.Start.Line != line || dep.Range.End.Line != line {
			t.Fatalf("Dart authored import=%q prefix=%q line=%d, got %+v", path, alias, line, dep)
		}
		return
	}
	t.Fatalf("missing Dart authored import=%q prefix=%q line=%d: %+v", path, alias, line, dependencies)
}
