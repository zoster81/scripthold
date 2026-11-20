package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type dartTypedMapWant struct {
	name string
	kind SymbolKind
	line int
}

func TestDartTypedMapLiteralOriginalSourceNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		wants             []dartTypedMapWant
	}{
		{
			name:   "flutter-cupertino",
			env:    "SCRIPTHOLD_REAL_DART_MAP_FLUTTER",
			marker: "const Map<CupertinoButtonSize, EdgeInsetsGeometry> kCupertinoButtonPadding =",
			wants: []dartTypedMapWant{
				{"kCupertinoFocusColorOpacity", SymbolKindConstant, 40},
				{"kCupertinoButtonPadding", SymbolKindConstant, 62},
				{"kCupertinoButtonSizeBorderRadius", SymbolKindConstant, 72},
				{"kCupertinoButtonMinSize", SymbolKindConstant, 82},
				{"kCupertinoButtonTapMoveSlop", SymbolKindConstant, 94},
			},
		},
		{
			name:   "dart-sdk-map-literal-const",
			env:    "SCRIPTHOLD_REAL_DART_MAP_SDK",
			marker: "const Map<int, String> intMap = <int, String>{",
			wants: []dartTypedMapWant{
				{"nullableIntMap1", SymbolKindConstant, 5},
				{"nullableIntMap2", SymbolKindConstant, 6},
				{"intList", SymbolKindConstant, 7},
				{"intMap", SymbolKindConstant, 8},
				{"numMap", SymbolKindConstant, 9},
				{"intIntList", SymbolKindConstant, 10},
				{"nullableNum1", SymbolKindConstant, 12},
				{"nullableString2", SymbolKindConstant, 16},
				{"main", SymbolKindFunction, 18},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 12000 {
				t.Fatalf("original source missing/nonregular/oversized: %v %v", info, err)
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(src), tc.marker) {
				t.Fatal("independent Dart original marker drift")
			}
			checkDartTypedMapLiteralOriginal(t, string(src), tc.wants)
		})
	}
	portable := "const Map<String, int> directMap = <String, int>{'a': 1, 'b': 2};\n" +
		"const List<int> directList = <int>[1, 2];\n" +
		"const Map<String, int> wrappedMap = const <String, int>{'c': 3};\n" +
		"const after = 1;\n" +
		"void next() {}\n"
	wants := []dartTypedMapWant{
		{"directMap", SymbolKindConstant, 1},
		{"directList", SymbolKindConstant, 2},
		{"wrappedMap", SymbolKindConstant, 3},
		{"after", SymbolKindConstant, 4},
		{"next", SymbolKindFunction, 5},
	}
	for _, tc := range []struct{ name, src string }{
		{"LF", portable}, {"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			checkDartTypedMapLiteralOriginal(t, tc.src, wants)
		})
	}
}

func checkDartTypedMapLiteralOriginal(t *testing.T, source string, wants []dartTypedMapWant) {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("sample.dart", source), testAnalyzeOptions(true, 500))
	if err != nil {
		t.Fatal(err)
	}
	nodes := symbolsByQualifiedName(result.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, want := range wants {
		if want.line < 1 || want.line > len(lines) || !strings.Contains(lines[want.line-1], want.name) {
			t.Fatalf("authored source oracle drift: %+v", want)
		}
		got, ok := nodes[want.name]
		if !ok || got.Kind != want.kind || got.NameRange.Start.Line != want.line {
			t.Errorf("missing/misclassified original Dart declaration %+v: found=%v got=%+v", want, ok, got)
		}
	}
}

func TestDartTypedMapLiteralCommaDoesNotConsumeComparisonsOrSiblings(t *testing.T) {
	const source = "const bool less = 1 < 2, greater = 3 > 2;\n" +
		"var first = 1, second = 2;\n" +
		"const Map<String, int> letters = <String, int>{'a': 1, 'b': 2};\n" +
		"const bool after = 5 < 6, last = 7 > 6;\n" +
		"void next() {}\n"
	wants := []dartTypedMapWant{
		{"less", SymbolKindConstant, 1}, {"greater", SymbolKindConstant, 1},
		{"first", SymbolKindVariable, 2}, {"second", SymbolKindVariable, 2},
		{"letters", SymbolKindConstant, 3},
		{"after", SymbolKindConstant, 4}, {"last", SymbolKindConstant, 4},
		{"next", SymbolKindFunction, 5},
	}
	for _, tc := range []struct{ name, src string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			checkDartTypedMapLiteralOriginal(t, tc.src, wants)
			result, err := (DartAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("sample.dart", tc.src), testAnalyzeOptions(true, 500))
			if err != nil {
				t.Fatal(err)
			}
			nodes := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, fake := range []string{"String", "int", "Map", "b", "2", "5"} {
				if s, ok := nodes[fake]; ok {
					t.Errorf("initializer or comparison invented %s: %+v", fake, s)
				}
			}
		})
	}
}
