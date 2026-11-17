package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type dartTypedefWant struct {
	name string
	kind SymbolKind
	line int
}

func TestDartTypedefOriginalSourceNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		wants             []dartTypedefWant
	}{
		{
			"flutter-foundation", "SCRIPTHOLD_REAL_DART_TYPEDEF_FLUTTER",
			"typedef ValueChanged<T> = void Function(T value);",
			[]dartTypedefWant{
				{"ValueChanged", SymbolKindAlias, 17},
				{"ValueSetter", SymbolKindAlias, 31},
				{"ValueGetter", SymbolKindAlias, 39},
				{"IterableFilter", SymbolKindAlias, 42},
				{"AsyncCallback", SymbolKindAlias, 52},
				{"AsyncValueSetter", SymbolKindAlias, 61},
				{"AsyncValueGetter", SymbolKindAlias, 69},
				{"CachingIterable", SymbolKindClass, 107},
				{"Factory", SymbolKindClass, 245},
				{"lerpDuration", SymbolKindFunction, 262},
			},
		},
		{
			"dart-sdk-annotated", "SCRIPTHOLD_REAL_DART_TYPEDEF_SDK",
			"typedef void G();",
			[]dartTypedefWant{
				{"A", SymbolKindClass, 5},
				{"F", SymbolKindAlias, 10},
				{"G", SymbolKindAlias, 13},
				{"main", SymbolKindFunction, 15},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("full independent Dart original not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 15000 {
				t.Fatalf("source missing/nonregular/oversized: %v %v", info, err)
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(src), tc.marker) {
				t.Fatal("pinned Dart source marker drift")
			}
			checkDartTypedefSymbols(t, string(src), tc.wants)
		})
	}
	portable := "typedef Mapper<T> = T Function(T value);\n" +
		"typedef void Legacy(int value);\n" +
		"typedef Pair = Map<String, int>;\n" +
		"void after() {}\n"
	wants := []dartTypedefWant{
		{"Mapper", SymbolKindAlias, 1},
		{"Legacy", SymbolKindAlias, 2},
		{"Pair", SymbolKindAlias, 3},
		{"after", SymbolKindFunction, 4},
	}
	for _, tc := range []struct{ name, src string }{
		{"LF", portable}, {"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			checkDartTypedefSymbols(t, tc.src, wants)
		})
	}
}

func TestDartTypedefMalformedDoesNotInventNamesOrHideNeighbors(t *testing.T) {
	const src = "typedef = void Function();\n" +
		"class RealOne {}\n" +
		"typedef Missing =;\n" +
		"typedef Broken<T = int;\n" +
		"typedef Bogus = 42;\n" +
		"void after() {}\n" +
		"typedef Incomplete =\n" +
		"class RealTwo {}\n" +
		"typedef Valid = String;\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", src}, {"CRLF", strings.ReplaceAll(src, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (DartAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("sample.dart", tc.text), testAnalyzeOptions(true, 400))
			if err != nil {
				t.Fatal(err)
			}
			nodes := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []dartTypedefWant{
				{"RealOne", SymbolKindClass, 2},
				{"after", SymbolKindFunction, 6},
				{"RealTwo", SymbolKindClass, 8},
				{"Valid", SymbolKindAlias, 9},
			} {
				got, ok := nodes[want.name]
				if !ok || got.Kind != want.kind || got.NameRange.Start.Line != want.line {
					t.Errorf("neighbor hidden/misclassified %+v: %+v (found=%v)", want, got, ok)
				}
			}
			for _, fake := range []string{"Missing", "Broken", "Bogus", "Incomplete", "Function"} {
				if got, ok := nodes[fake]; ok {
					t.Errorf("unsupported Dart typedef invented %s: %+v", fake, got)
				}
			}
			diagnostics := 0
			for _, d := range result.Analysis.Diagnostics {
				if d.Code == "dart-unsupported-typedef" && d.Range != nil {
					diagnostics++
				}
			}
			if diagnostics < 5 || result.Analysis.CoverageComplete {
				t.Errorf("malformed typedefs require source-ranged incomplete coverage; diagnostics=%d complete=%v",
					diagnostics, result.Analysis.CoverageComplete)
			}
		})
	}
}

func checkDartTypedefSymbols(t *testing.T, source string, wants []dartTypedefWant) {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("sample.dart", source), testAnalyzeOptions(true, 600))
	if err != nil {
		t.Fatal(err)
	}
	nodes := symbolsByQualifiedName(result.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, w := range wants {
		if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], w.name) {
			t.Fatalf("authored source oracle drift: %+v", w)
		}
		got, found := nodes[w.name]
		if !found || got.Kind != w.kind || got.NameRange.Start.Line != w.line {
			t.Errorf("missing/misclassified Dart original typedef or adjacent declaration: %+v found=%v got=%+v", w, found, got)
		}
	}
	for _, fake := range []string{"Function", "typedef", "T", "String", "int"} {
		if got, ok := nodes[fake]; ok {
			t.Errorf("Dart typedef RHS invented declaration %s: %+v", fake, got)
		}
	}
}
