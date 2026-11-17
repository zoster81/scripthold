package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestDartExternalGetterOriginalNavigation(t *testing.T) {
	const portable = "extension type Meter(int value) {\n" +
		"  external int get doubled;\n" +
		"}\n" +
		"external int get version;\n" +
		"class Neighbor {}\n"
	for _, tc := range []struct {
		name, source string
	}{
		{"LF", portable},
		{"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			checkDartExternalGetterSymbols(t, tc.source, map[string]int{
				"Meter.doubled": 2, "version": 4,
			})
		})
	}
	for _, tc := range []struct {
		name, env, marker string
		wanted            map[string]int
	}{
		{
			"dart-core", "SCRIPTHOLD_REAL_DART_EXTENSION_CORE",
			"external String? get userAgent;",
			map[string]int{
				"Navigator.userAgent":  13,
				"Navigator.appVersion": 15,
				"navigator":            19,
			},
		},
		{
			"flutter", "SCRIPTHOLD_REAL_DART_EXTENSION_FLUTTER_HOST",
			"external JSFunction get resume;",
			map[string]int{
				"testRunner":            23,
				"_JSApi.resume":         35,
				"_JSApi.restartCurrent": 40,
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent complete upstream source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 16000 {
				t.Fatalf("original Dart source missing, nonregular, or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("authored Dart original content marker drift")
			}
			checkDartExternalGetterSymbols(t, string(body), tc.wanted)
		})
	}
}

func checkDartExternalGetterSymbols(t *testing.T, source string, wanted map[string]int) {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("sample.dart", source), testAnalyzeOptions(true, 600))
	if err != nil {
		t.Fatal(err)
	}
	indexed := symbolsByQualifiedName(result.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for qualified, line := range wanted {
		symbol, exists := indexed[qualified]
		if !exists || symbol.Kind != SymbolKindProperty || symbol.NativeKind != "getter" ||
			symbol.NameRange.Start.Line != line {
			t.Errorf("authored external getter %s at line %d not indexed as property: %+v", qualified, line, symbol)
			continue
		}
		name := qualified
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			name = name[dot+1:]
		}
		if !strings.Contains(lines[line-1], "get "+name+";") {
			t.Fatalf("expected original external getter missing from line %d", line)
		}
	}
}
