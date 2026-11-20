package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDartCollectionLiteralBeforeDeclarationsOriginalNavigation(t *testing.T) {
	const portable = "const names = <String>{\n" +
		"  'one',\n" +
		"  'two',\n" +
		"};\n" +
		"extension type Label(String text) {}\n" +
		"class Neighbor {}\n" +
		"const Set<String> other = <String>{'three'};\n" +
		"int following() => 1;\n"
	for _, tc := range []struct {
		name, source string
	}{
		{"LF", portable},
		{"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			checkDartCollectionLiteralOriginal(t, tc.source, []dartCollectionSymbolWant{
				{"names", SymbolKindConstant, 1},
				{"Label", SymbolKindType, 5},
				{"Neighbor", SymbolKindClass, 6},
				{"other", SymbolKindConstant, 7},
				{"following", SymbolKindFunction, 8},
			})
		})
	}
	for _, tc := range []struct {
		name, env, marker string
		want              []dartCollectionSymbolWant
	}{
		{
			"flutter-build-source", "SCRIPTHOLD_REAL_DART_COLLECTION_FLUTTER",
			"const _validLeadingDirectoryPlaceholders = <String>{",
			[]dartCollectionSymbolWant{
				{"_validLeadingDirectoryPlaceholders", SymbolKindConstant, 10},
				{"ExtensionArtifact", SymbolKindType, 27},
				{"ExtensionHostArtifact", SymbolKindType, 37},
				{"ExtensionSource", SymbolKindClass, 41},
				{"ExtensionPatternSource", SymbolKindClass, 146},
			},
		},
		{
			"dart-sdk-dds-validation", "SCRIPTHOLD_REAL_DART_COLLECTION_SDK",
			"const _allowedSecFetchSites = <String>{'same-origin', 'none'};",
			[]dartCollectionSymbolWant{
				{"isAllowedOrigin", SymbolKindFunction, 14},
				{"isAllowedHost", SymbolKindFunction, 60},
				{"secFetchSiteHeader", SymbolKindConstant, 78},
				{"_allowedSecFetchSites", SymbolKindConstant, 82},
				{"isAllowedSecFetchSite", SymbolKindFunction, 100},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent upstream Dart source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 16000 {
				t.Fatalf("original source missing/nonregular/oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("original Dart collection source marker drift")
			}
			checkDartCollectionLiteralOriginal(t, string(body), tc.want)
		})
	}
}

type dartCollectionSymbolWant struct {
	name string
	kind SymbolKind
	line int
}

func checkDartCollectionLiteralOriginal(t *testing.T, source string, wanted []dartCollectionSymbolWant) {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("sample.dart", source), testAnalyzeOptions(true, 600))
	if err != nil {
		t.Fatal(err)
	}
	indexed := symbolsByQualifiedName(result.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, want := range wanted {
		if want.line < 1 || want.line > len(lines) ||
			!strings.Contains(lines[want.line-1], want.name) {
			t.Fatalf("source oracle drift for %+v", want)
		}
		symbol, ok := indexed[want.name]
		if !ok || symbol.Kind != want.kind || symbol.NameRange.Start.Line != want.line {
			t.Errorf("Dart declaration %s missing/wrong after collection literal: %+v", want.name, symbol)
		}
	}
	for _, invented := range []string{"type", "const", "String", "BuiltInPlaceholders.projectDir"} {
		if symbol, ok := indexed[invented]; ok {
			t.Errorf("Dart collection contents invented declaration %s: %+v", invented, symbol)
		}
	}
}

func TestDartUnsupportedCollectionTailDoesNotInventCallables(t *testing.T) {
	for _, tc := range []struct {
		name, source string
	}{
		{"cascade", "final items = <String>{'one'}..add('two');\nclass After {}\n"},
		{"missing-semicolon", "const items = <String>{'one'}\nclass After {}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (DartAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("sample.dart", tc.source), testAnalyzeOptions(true, 200))
			if err != nil {
				t.Fatal(err)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			if symbol, ok := symbols["After"]; !ok || symbol.Kind != SymbolKindClass {
				t.Errorf("following real declaration lost: %+v", symbol)
			}
			for _, unsupported := range []string{"items", "add"} {
				if symbol, ok := symbols[unsupported]; ok {
					t.Errorf("unsupported initializer invented %s: %+v", unsupported, symbol)
				}
			}
			if result.Analysis.CoverageComplete {
				t.Error("unsupported initializer falsely claims complete coverage")
			}
			diagnosed := false
			for _, d := range result.Analysis.Diagnostics {
				if d.Code == "dart-unsupported-collection-initializer" && d.Range != nil {
					diagnosed = true
				}
			}
			if !diagnosed {
				t.Error("unsupported initializer missing source-ranged diagnostic")
			}
		})
	}
}
