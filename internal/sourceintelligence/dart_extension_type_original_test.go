package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type dartExtensionTypeWant struct {
	name string
	line int
}

func TestDartExtensionTypeOriginalSourceNavigation(t *testing.T) {
	const portable = "extension type Meter(int value) {\n" +
		"  int get doubled => value * 2;\n" +
		"}\n" +
		"extension Extra on int {\n" +
		"  int get triple => this * 3;\n" +
		"}\n" +
		"class Other {}\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", portable},
		{"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			checkDartExtensionTypeOriginal(t, tc.source,
				[]dartExtensionTypeWant{{"Meter", 1}},
				map[string]SymbolKind{
					"Meter.doubled": SymbolKindProperty,
					"Extra":         SymbolKindType,
					"Extra.triple":  SymbolKindProperty,
					"Other":         SymbolKindClass,
				})
		})
	}
	t.Run("unsupported-extension-type-names-fail-closed", func(t *testing.T) {
		for _, header := range []string{
			"extension type { void fake() {} }\n",
			"extension type const { void fake() {} }\n",
			"extension type implements Base { void fake() {} }\n",
			"extension type const const Bad(int value) {}\n",
			"extension type Token { int fake; }\n",
		} {
			source := header + "class After {}\n"
			result, err := (DartAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("sample.dart", source), testAnalyzeOptions(true, 200))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, invalid := range []string{"type", "const", "Token", "Bad", "type.fake",
				"const.Bad", "Token.fake", "fake"} {
				if symbol, ok := byName[invalid]; ok {
					t.Errorf("unsupported extension type %q invented %s: %+v", header, invalid, symbol)
				}
			}
			if _, ok := byName["After"]; !ok {
				t.Errorf("valid adjacent Dart class lost after %q", header)
			}
			ranged := false
			for _, d := range result.Analysis.Diagnostics {
				if d.Code == "dart-unsupported-extension-type" && d.Range != nil {
					ranged = true
				}
			}
			if !ranged || result.Analysis.CoverageComplete {
				t.Errorf("malformed Dart extension type %q lacks source-ranged incomplete diagnostic", header)
			}
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		wants             []dartExtensionTypeWant
		neighbors         map[string]SymbolKind
	}{
		{
			"dart-core", "SCRIPTHOLD_REAL_DART_EXTENSION_CORE",
			"extension type Navigator(JSObject _) {",
			[]dartExtensionTypeWant{{"Navigator", 11}},
			map[string]SymbolKind{"Navigator.userAgent": SymbolKindProperty, "Navigator.appVersion": SymbolKindProperty},
		},
		{
			"flutter-web-host", "SCRIPTHOLD_REAL_DART_EXTENSION_FLUTTER_HOST",
			"extension type _TestRunner(JSObject _) implements JSObject {",
			[]dartExtensionTypeWant{{"_TestRunner", 17}, {"_JSApi", 29}},
			map[string]SymbolKind{"_TestRunner.waitUntilDone": SymbolKindMethod},
		},
		{
			"dart-sdk-inline-const", "SCRIPTHOLD_REAL_DART_EXTENSION_SDK_INLINE",
			"extension type const ExtensionType1(int it) {}",
			[]dartExtensionTypeWant{
				{"ExtensionType1", 1}, {"ExtensionType2", 2},
				{"ExtensionType3", 3}, {"ExtensionType4", 4},
				{"ExtensionType5", 22}, {"ExtensionType6", 23}, {"ExtensionType7", 24},
			},
			map[string]SymbolKind{"ExtensionType4.getter": SymbolKindProperty, "ExtensionType4.method": SymbolKindMethod},
		},
		{
			"dart-sdk-generic-const", "SCRIPTHOLD_REAL_DART_EXTENSION_SDK_GENERIC",
			"extension type const ExtensionType<T>(T id) {}",
			[]dartExtensionTypeWant{{"ExtensionType", 9}},
			map[string]SymbolKind{"Class": SymbolKindClass},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored upstream Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 16000 {
				t.Fatalf("original Dart source missing, nonregular or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("pinned Dart original content marker drift")
			}
			checkDartExtensionTypeOriginal(t, string(body), tc.wants, tc.neighbors)
		})
	}
}

func checkDartExtensionTypeOriginal(t *testing.T, source string, wants []dartExtensionTypeWant, neighbors map[string]SymbolKind) {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("sample.dart", source), testAnalyzeOptions(true, 600))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, want := range wants {
		if want.line < 1 || want.line > len(lines) {
			t.Fatalf("original Dart oracle line out of range: %+v", want)
		}
		column := strings.Index(lines[want.line-1], want.name) + 1
		if column < 1 {
			t.Fatalf("authored extension type %q missing from line %d", want.name, want.line)
		}
		symbol, ok := byName[want.name]
		if !ok || symbol.Kind != SymbolKindType || symbol.NativeKind != "extension-type" ||
			symbol.NameRange.Start.Line != want.line ||
			symbol.NameRange.Start.Column != column ||
			symbol.DeclarationRange.Start.Line != want.line {
			t.Errorf("authored Dart extension type %q missing or misclassified at %d:%d: %+v",
				want.name, want.line, column, symbol)
		}
	}
	for name, kind := range neighbors {
		if sym, ok := byName[name]; !ok || sym.Kind != kind {
			t.Errorf("neighboring authored Dart symbol %q missing or wrong kind %s: %+v", name, kind, sym)
		}
	}
	for _, bogus := range []string{"type", "const", "type.userAgent", "type.waitUntilDone",
		"type.doubled", "type.triple", "type.ExtensionSource", "const.ExtensionArtifact"} {
		if symbol, exists := byName[bogus]; exists {
			t.Errorf("Dart keyword %q incorrectly indexed as a type or member: %+v", bogus, symbol)
		}
	}
}
