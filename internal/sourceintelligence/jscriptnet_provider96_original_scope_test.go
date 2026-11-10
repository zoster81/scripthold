package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestJScriptNetProvider96MixedPackageAndGlobalScopes(t *testing.T) {
	text := `package CarePackage {
 class Helpers {
  static function Echo(value: Object): Object { return value; }
 }
}
class Handlers {
 static function onChange(source: Object): void { }
 static function onRename(source: Object): void { }
}
`
	result, err := (JScriptNetAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("watcher.js", text), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{
		"CarePackage":              1,
		"CarePackage.Helpers":      2,
		"CarePackage.Helpers.Echo": 3,
		"Handlers":                 6,
		"Handlers.onChange":        7,
		"Handlers.onRename":        8,
	}
	for _, symbol := range result.Analysis.Symbols {
		if want[symbol.QualifiedName] == symbol.NameRange.Start.Line {
			delete(want, symbol.QualifiedName)
		}
	}
	if len(want) != 0 {
		t.Errorf("JScript.NET cannot navigate declarations after closed package; missing=%v symbols=%v",
			want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Errorf("unexpected incomplete JScript.NET analysis: %+v", result.Analysis)
	}
}

func TestJScriptNetProvider96PackageAndGlobalsRespectSymbolCap(t *testing.T) {
	text := "package CarePackage { class Helpers { function Echo() {} } }\nclass Handlers { function onChange() {} }\n"
	for _, cap := range []int{1, 2, 3} {
		result, err := (JScriptNetAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("bounded.js", text), testAnalyzeOptions(true, cap))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Analysis.Symbols) > cap {
			t.Errorf("symbol cap %d exceeded: %+v", cap, result.Analysis.Symbols)
		}
		if !result.Analysis.Truncated || result.Analysis.CoverageComplete {
			t.Errorf("cap %d must report incomplete source retention: %+v", cap, result.Analysis)
		}
	}
}

func TestJScriptNetProvider96IndependentOriginalSources(t *testing.T) {
	cases := []struct {
		name, env, oracle string
		expected          map[string]int
		forbidden         []string
	}{
		{
			name:   "watcher event compiler maintenance",
			env:    "SCRIPTHOLD_REAL_JSNET_WATCHER96",
			oracle: "jscp.StartInfo.FileName = \"jsc.exe\";",
			expected: map[string]int{
				"CarePackage": 1, "CarePackage.Helpers": 2,
				"CarePackage.Helpers.Echo": 3, "CarePackage.Helpers.Pause": 6,
				"Handlers": 33, "Handlers.onChange": 34,
				"Handlers.onRename": 50, "Handlers.onError": 53,
			},
			forbidden: []string{"Process", "FileSystemWatcher", "jscp"},
		},
		{
			name:      "file reader maintenance",
			env:       "SCRIPTHOLD_REAL_JSNET_JSTEST96",
			oracle:    "var reader = new StreamReader(this.name)",
			expected:  map[string]int{"File": 7, "File.readFile": 9},
			forbidden: []string{"StreamReader", "MessageBox", "ReadToEnd"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original JScript.NET project not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source must be bounded regular file: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original JScript.NET maintenance oracle mismatch")
			}
			result, err := (JScriptNetAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(raw)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Errorf("original JScript.NET source was not fully processed: %+v", result.Analysis)
			}
			seen := map[string]int{}
			for _, symbol := range result.Analysis.Symbols {
				seen[symbol.QualifiedName] = symbol.NameRange.Start.Line
				for _, bad := range tc.forbidden {
					if symbol.Name == bad {
						t.Errorf("call or local fabricated as declaration: %+v", symbol)
					}
				}
			}
			for name, line := range tc.expected {
				if seen[name] != line {
					t.Errorf("missing source declaration %s:%d, got %d, symbols=%v", name, line, seen[name], seen)
				}
			}
		})
	}
}
