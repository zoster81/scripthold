package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
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

func TestJScriptNetPinnedFullOriginalDeclarationIdentity(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JSNET_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete independent JScript.NET R30 originals are not configured")
	}
	type declaration struct {
		name, qualified string
		line            int
	}
	for _, tc := range []struct {
		file, oracle string
		expected     []declaration
	}{
		{
			file:   "codesanook-program.js",
			oracle: "package CodeSanook.Example.JScript.NET {",
			expected: []declaration{
				{"CodeSanook.Example.JScript.NET", "CodeSanook.Example.JScript.NET", 8},
				{"MainForm", "CodeSanook.Example.JScript.NET.MainForm", 10},
				{"label", "CodeSanook.Example.JScript.NET.MainForm.label", 13},
				{"textBox", "CodeSanook.Example.JScript.NET.MainForm.textBox", 14},
				{"button", "CodeSanook.Example.JScript.NET.MainForm.button", 15},
				{"panel", "CodeSanook.Example.JScript.NET.MainForm.panel", 16},
				{"MainForm", "CodeSanook.Example.JScript.NET.MainForm.MainForm", 19},
				{"buttonClicked", "CodeSanook.Example.JScript.NET.MainForm.buttonClicked", 62},
			},
		},
		{
			file:   "clearcanvas-jscript.js",
			oracle: "package ClearCanvas.Jscript\r\n{",
			expected: []declaration{
				{"ClearCanvas.Jscript", "ClearCanvas.Jscript", 31},
				{"Engine", "ClearCanvas.Jscript.Engine", 56},
				{"Run", "ClearCanvas.Jscript.Engine.Run", 58},
				{"CreateScript", "ClearCanvas.Jscript.Engine.CreateScript", 71},
				{"CreateScriptPrivate", "ClearCanvas.Jscript.Engine.CreateScriptPrivate", 76},
				{"ExecutableScript", "ClearCanvas.Jscript.ExecutableScript", 90},
				{"_ctx", "ClearCanvas.Jscript.ExecutableScript._ctx", 92},
				{"ExecutableScript", "ClearCanvas.Jscript.ExecutableScript.ExecutableScript", 95},
				{"Run", "ClearCanvas.Jscript.ExecutableScript.Run", 100},
			},
		},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("complete original must be a bounded regular file: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.oracle) {
				t.Fatalf("original upstream source marker missing in %s", tc.file)
			}
			result, err := (JScriptNetAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(raw)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Errorf("original source unexpectedly incomplete: %+v", result.Analysis)
			}
			symbols := result.Analysis.Symbols
			if len(symbols) != len(tc.expected) {
				t.Errorf("complete original declarations: got %d, want %d", len(symbols), len(tc.expected))
			}
			byLine := make(map[int][]NormalizedSymbol)
			for _, symbol := range symbols {
				line := symbol.NameRange.Start.Line
				byLine[line] = append(byLine[line], symbol)
			}
			for _, want := range tc.expected {
				found := byLine[want.line]
				if len(found) != 1 {
					t.Errorf("line %d: expected one authored %q, got %d: %+v", want.line, want.name, len(found), found)
					continue
				}
				if found[0].Name != want.name || found[0].QualifiedName != want.qualified {
					t.Errorf("line %d: authored %s/%s, got %s/%s",
						want.line, want.name, want.qualified, found[0].Name, found[0].QualifiedName)
				}
				if want.line == 13 || want.line == 14 || want.line == 15 || want.line == 16 || want.line == 92 {
					if found[0].Kind != SymbolKindField {
						t.Errorf("line %d: JScript.NET field classification missing: %+v", want.line, found[0])
					}
				}
				delete(byLine, want.line)
			}
			for line, extras := range byLine {
				t.Errorf("line %d: invented original class member(s): %+v", line, extras)
			}
		})
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
