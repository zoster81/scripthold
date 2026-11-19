package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJavaScriptPinnedWholeExpressRegexASIRange(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JAVASCRIPT_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole JavaScript originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "express-generator-test-utils.js"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1794 {
		t.Fatalf("Express upstream original changed: %d", len(b))
	}
	for _, tc := range []struct{ name, src string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (JavaScriptAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("utils.js", tc.src), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 11 {
				t.Fatalf("complete Express original eleven supported declarations: count=%d complete=%v truncated=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete, result.Analysis.Truncated)
			}
			expected := map[string]int{
				"fs": 3, "os": 4, "path": 5, "uid": 6, "WARNINGS_REGEXP": 8,
				"childEnvironment": 17, "parseCreatedFiles": 30, "parseWarnings": 52,
				"stripColors": 66, "stripWarnings": 71, "tmpDir": 75,
			}
			for _, symbol := range result.Analysis.Symbols {
				line, ok := expected[symbol.Name]
				if !ok || symbol.NameRange.Start.Line != line {
					t.Errorf("wrong whole source symbol: %+v", symbol)
				}
				if symbol.Name == "WARNINGS_REGEXP" {
					if symbol.Kind != SymbolKindVariable || symbol.NativeKind != "var" ||
						symbol.DeclarationRange.Start.Line != 8 || symbol.DeclarationRange.End.Line != 8 ||
						symbol.DeclarationRange.End.Column != 73 {
						t.Errorf("regexp initializer must stop at original line8 col73, never include unrelated module.exports: %+v", symbol)
					}
				}
			}
		})
	}
}

func TestJavaScriptPinnedIndependentWholeGeekmagicControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JAVASCRIPT_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole JavaScript originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "geekmagic-utils.js"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1583 {
		t.Fatalf("Geekmagic upstream original changed: %d", len(b))
	}
	result, err := (JavaScriptAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("utils.js", string(b)), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 4 {
		t.Fatalf("independent four complete supported top-level declarations: count=%d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
	}
	for _, name := range []string{"humanFileSize", "apiFetch", "includeHTML", "setHeaderTitle"} {
		symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)[name]
		if !ok || symbol.Kind != SymbolKindFunction {
			t.Errorf("independent authored function missing: %s", name)
		}
	}
}

func TestJavaScriptRegexMaskPreservesExpressionBoundaryAndOffsets(t *testing.T) {
	tests := []struct{ name, source, expression string }{
		{"regex-literal", "var pattern = /[\\r\\n]+/g\nmodule.exports.x = 1\n", "/[\\r\\n]+/g"},
		{"brace-regex", "var pattern = /[{}]/\nmodule.exports.x = 1\n", "/[{}]/"},
		{"division", "var pattern = x / y\nmodule.exports.x = 1\n", "x / y"},
		{"regex-followed-function", "var pattern = /a\\/b/\nfunction after() {}\n", "/a\\/b/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			masked, err := maskECMAScriptRegexLiterals(context.Background(), tc.source, 128)
			if err != nil {
				t.Fatal(err)
			}
			if len(masked) != len(tc.source) {
				t.Errorf("masked source offsets changed")
			}
			if tc.name != "division" && strings.Contains(masked, tc.expression) {
				t.Errorf("regex not masked: %q", masked)
			}
			if tc.name == "division" && masked != tc.source {
				t.Errorf("division unexpectedly masked")
			}
			result, err := (JavaScriptAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("regexp.js", tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Errorf("complete lexical source became partial")
			}
			var pattern bool
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Name == "pattern" {
					pattern = true
					if symbol.DeclarationRange.End.Line != 1 {
						t.Errorf("regex/division variable must end at own source line: %+v", symbol)
					}
				}
			}
			if !pattern {
				t.Error("missing authored pattern declaration")
			}
		})
	}
}
