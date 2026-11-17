package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type dartMultiDeclaratorWant struct {
	qualified string
	kind      SymbolKind
	line      int
}

func TestDartMultipleDeclaratorsOriginalSourceNavigation(t *testing.T) {
	const portable = "const double alpha = 0.1,\n" +
		"    beta = 0.2, gamma = 0.3;\n" +
		"class Example {\n" +
		"  var x, y = 2, z = initial;\n" +
		"}\n" +
		"dynamic sink1, sink2;\n" +
		"final other = Holder(1, 2);\n" +
		"void after() {}\n"
	portableWant := []dartMultiDeclaratorWant{
		{"alpha", SymbolKindConstant, 1}, {"beta", SymbolKindConstant, 2},
		{"gamma", SymbolKindConstant, 2}, {"Example", SymbolKindClass, 3},
		{"Example.x", SymbolKindField, 4}, {"Example.y", SymbolKindField, 4},
		{"Example.z", SymbolKindField, 4}, {"sink1", SymbolKindVariable, 6},
		{"sink2", SymbolKindVariable, 6}, {"other", SymbolKindConstant, 7},
		{"after", SymbolKindFunction, 8},
	}
	for _, tc := range []struct{ name, src string }{
		{"LF", portable},
		{"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			checkDartMultiDeclaratorOriginal(t, tc.src, portableWant)
		})
	}
	for _, tc := range []struct {
		name, env, marker string
		want              []dartMultiDeclaratorWant
	}{
		{
			"flutter-cupertino", "SCRIPTHOLD_REAL_DART_MULTIDECL_FLUTTER",
			"const double kCupertinoFocusColorOpacity = 0.80,",
			[]dartMultiDeclaratorWant{
				{"kMinInteractiveDimensionCupertino", SymbolKindConstant, 29},
				{"kCupertinoFocusColorOpacity", SymbolKindConstant, 40},
				{"kCupertinoFocusColorBrightness", SymbolKindConstant, 41},
				{"kCupertinoFocusColorSaturation", SymbolKindConstant, 42},
				{"kCupertinoButtonTintedOpacityLight", SymbolKindConstant, 49},
				{"kCupertinoButtonTintedOpacityDark", SymbolKindConstant, 49},
				{"kCupertinoButtonTapMoveSlop", SymbolKindConstant, 94},
			},
		},
		{
			"dart-sdk-multiple-fields", "SCRIPTHOLD_REAL_DART_MULTIDECL_SDK",
			"var x = 2, y = 3, z, w = 2;",
			[]dartMultiDeclaratorWant{
				{"nullableString", SymbolKindVariable, 7},
				{"A", SymbolKindClass, 9},
				{"A.x", SymbolKindField, 10}, {"A.y", SymbolKindField, 10},
				{"A.z", SymbolKindField, 10},
				{"B", SymbolKindClass, 13},
				{"B.x", SymbolKindField, 14}, {"B.y", SymbolKindField, 14},
				{"B.z", SymbolKindField, 14}, {"B.w", SymbolKindField, 14},
				{"foo", SymbolKindFunction, 17}, {"main", SymbolKindFunction, 32},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 12000 {
				t.Fatalf("original source missing, nonregular or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("upstream original source drift")
			}
			checkDartMultiDeclaratorOriginal(t, string(body), tc.want)
		})
	}
}

func checkDartMultiDeclaratorOriginal(t *testing.T, source string, wants []dartMultiDeclaratorWant) {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("sample.dart", source), testAnalyzeOptions(true, 500))
	if err != nil {
		t.Fatal(err)
	}
	indexed := symbolsByQualifiedName(result.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, want := range wants {
		name := want.qualified
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			name = name[dot+1:]
		}
		if want.line < 1 || want.line > len(lines) ||
			!strings.Contains(lines[want.line-1], name) {
			t.Fatalf("independent original oracle line drift for %+v", want)
		}
		symbol, ok := indexed[want.qualified]
		if !ok || symbol.Kind != want.kind || symbol.NameRange.Start.Line != want.line {
			t.Errorf("Dart authored declaration %s at line %d not correctly indexed: %+v", want.qualified, want.line, symbol)
		}
	}
	for _, invented := range []string{"Holder", "Example.initial", "Example.2", "B.2"} {
		if symbol, ok := indexed[invented]; ok {
			t.Errorf("initializer invented Dart declaration %s: %+v", invented, symbol)
		}
	}
}

func TestDartMultipleDeclaratorBoundariesAndMalformedInputs(t *testing.T) {
	const portable = "const Map<String, int> counts = <String, int>{'a': 1, 'b': 2};\n" +
		"const double low = 0.1, high = 0.9;\n" +
		"final result = Pair(1, 2);\n" +
		"class C { var x = Pair(3, 4), y = 2; }\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", portable}, {"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("nested/"+tc.name, func(t *testing.T) {
			checkDartMultiDeclaratorOriginal(t, tc.source, []dartMultiDeclaratorWant{
				{"low", SymbolKindConstant, 2}, {"high", SymbolKindConstant, 2},
				{"result", SymbolKindConstant, 3}, {"C", SymbolKindClass, 4},
				{"C.x", SymbolKindField, 4}, {"C.y", SymbolKindField, 4},
			})
		})
	}
	for _, tc := range []struct{ name, source string }{
		{"empty-initializer", "const int alpha =, beta = 2;\nclass After {}\n"},
		{"empty-middle", "var good = 1, , phantom = 2;\nclass After {}\n"},
		{"too-many", "var " + strings.Join(func() []string {
			var names []string
			for i := 0; i < 66; i++ {
				names = append(names, "v"+string(rune('A'+i/26))+string(rune('a'+i%26)))
			}
			return names
		}(), ", ") + ";\nclass After {}\n"},
	} {
		t.Run("malformed/"+tc.name, func(t *testing.T) {
			result, err := (DartAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("sample.dart", tc.source), testAnalyzeOptions(true, 300))
			if err != nil {
				t.Fatal(err)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			if after, ok := symbols["After"]; !ok || after.Kind != SymbolKindClass {
				t.Errorf("real neighboring class lost after unsupported declaration: %+v", after)
			}
			for _, forbidden := range []string{"alpha", "beta", "good", "phantom", "vAa"} {
				if value, ok := symbols[forbidden]; ok {
					t.Errorf("invalid declaration invented %s: %+v", forbidden, value)
				}
			}
			if result.Analysis.CoverageComplete {
				t.Error("unsupported declaration claims complete coverage")
			}
			found := false
			for _, d := range result.Analysis.Diagnostics {
				if d.Code == "dart-unsupported-multiple-declarators" && d.Range != nil {
					found = true
				}
			}
			if !found {
				t.Error("missing source-ranged incomplete diagnostic")
			}
		})
	}
}
