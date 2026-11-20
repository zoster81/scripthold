package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// A conditional Dart directive has multiple authored URI alternatives. The
// analyzer may keep the authored default URI but cannot claim it indexed the
// platform-guarded alternatives as unconditional imports.
func TestDartConditionalURICoverageOriginalSources(t *testing.T) {
	for _, tc := range []struct{ name, text string }{
		{"LF", "import 'plain_if.dart';\nexport 'public.dart';\npart 'fragment.dart';\n"},
		{"CRLF", "import 'plain_if.dart';\r\nexport 'public.dart';\r\npart 'fragment.dart';\r\n"},
	} {
		t.Run("ordinary-"+tc.name, func(t *testing.T) {
			result := dartConditionalAnalysis(t, tc.text)
			if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
				t.Fatalf("ordinary Dart directives lost complete coverage: %+v", result.Analysis)
			}
			assertDartConditionalDefault(t, result.Dependencies, "plain_if.dart", 1)
		})
	}
	const fixture = "import 'default.dart' if (dart.library.io) 'native.dart' if (dart.library.html) 'web.dart';\n" +
		"export 'api.dart' if (dart.library.io) 'api_native.dart';\n" +
		"import 'plain.dart';\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := dartConditionalAnalysis(t, tc.text)
			assertDartConditionalPartial(t, result, 2)
			for _, expected := range []struct {
				value string
				line  int
			}{
				{"default.dart", 1},
				{"api.dart", 2},
				{"plain.dart", 3},
			} {
				assertDartConditionalDefault(t, result.Dependencies, expected.value, expected.line)
			}
			for _, forbidden := range []string{"native.dart", "web.dart", "api_native.dart"} {
				for _, dep := range result.Dependencies {
					if dep.Value == forbidden {
						t.Fatalf("guarded URI %q presented without platform guard: %+v", forbidden, dep)
					}
				}
			}
		})
	}
	for _, tc := range []struct {
		name, env, marker string
		minDiagnostics    int
		defaultURI        string
		defaultLine       int
	}{
		{"hive", "SCRIPTHOLD_REAL_DART_COND_HIVE",
			"if (dart.library.html) 'package:hive/src/impl/isolate_stub.dart'",
			1, "dart:isolate", 6},
		{"dart-sass", "SCRIPTHOLD_REAL_DART_COND_SASS",
			"if (dart.library.io) 'io/vm.dart'",
			2, "io/interface.dart", 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 10000 {
				t.Fatalf("unavailable/unbounded original Dart source: %v %v", info, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(b), tc.marker) {
				t.Fatal("original conditional directive oracle drift")
			}
			result := dartConditionalAnalysis(t, string(b))
			assertDartConditionalPartial(t, result, tc.minDiagnostics)
			assertDartConditionalDefault(t, result.Dependencies, tc.defaultURI, tc.defaultLine)
		})
	}
}

func dartConditionalAnalysis(t *testing.T, text string) AnalyzerResult {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("conditional.dart", text), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("Dart conditional analysis unexpectedly truncated")
	}
	return result
}

func assertDartConditionalPartial(t *testing.T, result AnalyzerResult, minimum int) {
	t.Helper()
	count := 0
	for _, diagnostic := range result.Analysis.Diagnostics {
		if diagnostic.Code == "dart-conditional-uri-alternatives" {
			count++
			if diagnostic.Range == nil || diagnostic.Range.Start.Line <= 0 {
				t.Fatalf("conditional diagnostic missing authored range: %+v", diagnostic)
			}
		}
	}
	if result.Analysis.CoverageComplete || count < minimum {
		t.Fatalf("Dart conditional URI coverage incorrectly complete or undiagnosed: complete=%v diagnostics=%+v", result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
	}
}

func assertDartConditionalDefault(t *testing.T, deps []StructuralDependency, uri string, line int) {
	t.Helper()
	for _, dep := range deps {
		if dep.Kind == StructuralDependencyImport && dep.Value == uri &&
			dep.Range.Start.Line == line && dep.Range.End.Line == line {
			return
		}
	}
	t.Fatalf("authored default URI %q line=%d missing from structural dependencies %+v", uri, line, deps)
}
