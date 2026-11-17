package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type dartEnumWant struct {
	name string
	kind SymbolKind
	line int
}

func TestDartEnumValuesOriginalSources(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		wants             []dartEnumWant
	}{
		{
			"flutter-material-card", "SCRIPTHOLD_REAL_DART_ENUM_FLUTTER",
			"enum _CardVariant { elevated, filled, outlined }",
			[]dartEnumWant{
				{"_CardVariant", SymbolKindEnum, 18},
				{"_CardVariant.elevated", SymbolKindConstant, 18},
				{"_CardVariant.filled", SymbolKindConstant, 18},
				{"_CardVariant.outlined", SymbolKindConstant, 18},
				{"Card", SymbolKindClass, 73},
				{"_CardDefaultsM2", SymbolKindClass, 275},
			},
		},
		{
			"dart-sdk-dot-shorthand", "SCRIPTHOLD_REAL_DART_ENUM_SDK",
			"enum Color { red, blue, green }",
			[]dartEnumWant{
				{"Color", SymbolKindEnum, 5},
				{"Color.red", SymbolKindConstant, 5},
				{"Color.blue", SymbolKindConstant, 5},
				{"Color.green", SymbolKindConstant, 5},
				{"main", SymbolKindFunction, 7},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent Dart enum original not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 20000 {
				t.Fatalf("invalid original: %v %v", fi, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("upstream authored enum marker drift")
			}
			checkDartEnumValues(t, string(body), tc.wants)
		})
	}
	const portable = "enum Direction {\n  north,\n  east,\n  south,\n  west,\n}\nclass After {}\n"
	wants := []dartEnumWant{
		{"Direction", SymbolKindEnum, 1},
		{"Direction.north", SymbolKindConstant, 2},
		{"Direction.east", SymbolKindConstant, 3},
		{"Direction.south", SymbolKindConstant, 4},
		{"Direction.west", SymbolKindConstant, 5},
		{"After", SymbolKindClass, 7},
	}
	for _, tc := range []struct{ name, src string }{{"LF", portable}, {"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) { checkDartEnumValues(t, tc.src, wants) })
	}
}

func TestDartEnumMalformedListsFailClosed(t *testing.T) {
	many := make([]string, 129)
	for i := range many {
		many[i] = "value" + strings.Repeat("x", i+1)
	}
	tooLarge := make([]string, 120)
	for i := range tooLarge {
		tooLarge[i] = "entry" + strings.Repeat("x", 130) + many[i]
	}
	cases := []struct{ name, src string }{
		{"duplicate", "enum Bad { one, one }\nclass After {}\n"},
		{"string-member", "enum Bad { one, 'notAnIdentifier' }\nclass After {}\n"},
		{"call-member", "enum Bad { one(1), two }\nclass After {}\n"},
		{"empty-between", "enum Bad { one,, two }\nclass After {}\n"},
		{"annotated", "enum Bad { @flag one, two }\nclass After {}\n"},
		{"too-many", "enum Bad { " + strings.Join(many, ", ") + " }\nclass After {}\n"},
		{"too-large", "enum Bad { " + strings.Join(tooLarge, ", ") + " }\nclass After {}\n"},
	}
	for _, tc := range cases {
		for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+nl.name, func(t *testing.T) {
				body := strings.ReplaceAll(tc.src, "\n", nl.value)
				result, err := (DartAnalyzer{}).Analyze(context.Background(),
					testSourceDocument("enum.dart", body), testAnalyzeOptions(true, 1000))
				if err != nil {
					t.Fatal(err)
				}
				nodes := symbolsByQualifiedName(result.Analysis.Symbols)
				for _, w := range []dartEnumWant{{"Bad", SymbolKindEnum, 1}, {"After", SymbolKindClass, 2}} {
					node, ok := nodes[w.name]
					if !ok || node.Kind != w.kind || node.NameRange.Start.Line != w.line {
						t.Errorf("valid neighboring type lost: %+v, got=%+v, exists=%v", w, node, ok)
					}
				}
				for q, node := range nodes {
					if strings.HasPrefix(q, "Bad.") {
						t.Errorf("malformed enum produced false member %s: %+v", q, node)
					}
				}
				ranged := false
				for _, d := range result.Analysis.Diagnostics {
					if d.Code == "dart-unsupported-enum-values" && d.Range != nil {
						ranged = true
					}
				}
				if !ranged || result.Analysis.CoverageComplete {
					t.Errorf("malformed enum must be source-ranged incomplete; diagnostics=%+v, complete=%v",
						result.Analysis.Diagnostics, result.Analysis.CoverageComplete)
				}
			})
		}
	}
}

func TestDartEnumPlainEntriesThenMemberBoundary(t *testing.T) {
	const body = "enum Stage { queued, active; int get count => 3; }\nclass After {}\n"
	for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(nl.name, func(t *testing.T) {
			checkDartEnumValues(t, strings.ReplaceAll(body, "\n", nl.value), []dartEnumWant{
				{"Stage", SymbolKindEnum, 1},
				{"Stage.queued", SymbolKindConstant, 1},
				{"Stage.active", SymbolKindConstant, 1},
				{"Stage.count", SymbolKindProperty, 1},
				{"After", SymbolKindClass, 2},
			})
		})
	}
}

func checkDartEnumValues(t *testing.T, source string, wants []dartEnumWant) {
	t.Helper()
	result, err := (DartAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("enum.dart", source), testAnalyzeOptions(true, 900))
	if err != nil {
		t.Fatal(err)
	}
	nodes := symbolsByQualifiedName(result.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, w := range wants {
		simple := w.name[strings.LastIndex(w.name, ".")+1:]
		if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], simple) {
			t.Fatalf("incorrect authored source line oracle: %+v", w)
		}
		got, exists := nodes[w.name]
		if !exists || got.Kind != w.kind || got.NameRange.Start.Line != w.line {
			t.Errorf("Dart original enum missing or wrong: want=%+v found=%v got=%+v", w, exists, got)
		}
		if exists && strings.Contains(w.name, ".") && got.ParentQualifiedName != strings.SplitN(w.name, ".", 2)[0] {
			t.Errorf("enum value %s has wrong lexical owner %+v", w.name, got)
		}
	}
	for _, falseName := range []string{"elevated", "outlined", "red", "blue", "north", "east"} {
		if node, ok := nodes[falseName]; ok {
			t.Errorf("enum value wrongly promoted to file scope: %+v", node)
		}
	}
}
