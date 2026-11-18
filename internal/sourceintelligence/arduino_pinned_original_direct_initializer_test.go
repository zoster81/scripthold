package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original Arduino sketches are read in full but never executed.
func TestArduinoPinnedWholeOriginalConstructorVariable(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ARDUINO_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two original Arduino sketches are not configured")
	}
	for _, tc := range []struct {
		file      string
		wantCount int
	}{
		{"servo-sweep.ino", 4},
		{"neopixel-simple.ino", 3},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Arduino sketch unavailable or unsafe: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (ArduinoAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(data)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
				len(result.Analysis.Symbols) != tc.wantCount {
				t.Fatalf("complete original direct symbols: %+v", result.Analysis)
			}
			wanted := "myservo"
			if tc.file == "neopixel-simple.ino" {
				wanted = "pixels"
			}
			found := false
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name != wanted {
					continue
				}
				found = true
				if symbol.Kind != SymbolKindVariable || symbol.NativeKind != "variable" ||
					symbol.SignatureRange != nil || symbol.Signature != "" {
					t.Errorf("authored global object must not be a function: %+v", symbol)
				}
			}
			if !found {
				t.Errorf("original global object %q missing", wanted)
			}
		})
	}
}

func TestCPPArduinoExpressionInitializerPrototypeGuards(t *testing.T) {
	for _, analyzer := range []SourceAnalyzer{CPPAnalyzer{}, ArduinoAnalyzer{}} {
		for _, tc := range []struct {
			name, source, symbol string
			kind                 SymbolKind
		}{
			{"ordinary function prototype", "int adjust(int mask);", "adjust", SymbolKindFunction},
			{"default expression remains a prototype", "void render(int mask = FLAGS + OTHER);", "render", SymbolKindFunction},
			{"most vexing parse is a function", "Widget vexing(Widget());", "vexing", SymbolKindFunction},
			{"array extent expression is a parameter", "void write(char data[COUNT + 1]);", "write", SymbolKindFunction},
			{"function type template non-type argument", "void map(std::integral_constant<int, COUNT + 1> value);", "map", SymbolKindFunction},
		} {
			t.Run(analyzer.Language()+"/"+tc.name, func(t *testing.T) {
				result, err := analyzer.Analyze(context.Background(),
					testSourceDocument("prototype.cpp", tc.source+"\n"), testAnalyzeOptions(true, 128))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 1 {
					t.Fatalf("prototype should be exactly one source declaration: %+v", result.Analysis)
				}
				symbol := result.Analysis.Symbols[0]
				if symbol.Name != tc.symbol || symbol.Kind != tc.kind || symbol.NativeKind != "function-declaration" {
					t.Errorf("valid prototype must not be reclassified: %+v", symbol)
				}
			})
		}
	}
}

func TestArduinoConstructorExpressionNotFunctionLFCRLF(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			source := strings.Join([]string{
				"#define COUNT 16",
				"#define PIN 6",
				"Widget pixels(COUNT, PIN, FLAGS + OTHER);",
				"void setup() {}",
				"void loop() {}",
				"",
			}, eol)
			result, err := (ArduinoAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("object.ino", source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 3 || !result.Analysis.CoverageComplete {
				t.Fatalf("unexpected direct sketch symbols: %+v", result.Analysis)
			}
			got := result.Analysis.Symbols[0]
			if got.Name != "pixels" || got.Kind != SymbolKindVariable ||
				got.NativeKind != "variable" || got.SignatureRange != nil {
				t.Errorf("macro-expression constructor must be variable: %+v", got)
			}
		})
	}
}
