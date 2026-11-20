package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKotlinPinnedWholeTimberMultilinePropertyRange(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_KOTLIN_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Kotlin originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "kotlin-timber-new-53e1559.kt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 893 {
		t.Fatalf("upstream Git-normalized Timber original changed: %d", len(b))
	}
	for _, tc := range []struct{ name, src string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("TimberIssueRegistry.kt", tc.src), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 6 {
				t.Fatalf("whole original expected six declarations: count=%d complete=%v truncated=%v",
					len(result.Analysis.Symbols), result.Analysis.CoverageComplete, result.Analysis.Truncated)
			}
			want := map[string]int{
				"timber.lint":                            1,
				"timber.lint.TimberIssueRegistry":        11,
				"timber.lint.TimberIssueRegistry.issues": 12,
				"timber.lint.TimberIssueRegistry.api":    15,
				"timber.lint.TimberIssueRegistry.minApi": 19,
				"timber.lint.TimberIssueRegistry.vendor": 22,
			}
			for _, s := range result.Analysis.Symbols {
				line, ok := want[s.QualifiedName]
				if !ok || s.NameRange.Start.Line != line {
					t.Errorf("unexpected original declaration: %+v", s)
				}
				if s.Name == "vendor" {
					if s.Kind != SymbolKindProperty || s.NativeKind != "val" ||
						s.ParentQualifiedName != "timber.lint.TimberIssueRegistry" ||
						s.DeclarationRange.Start.Line != 22 || s.DeclarationRange.End.Line != 27 {
						t.Errorf("vendor declaration must include complete Vendor(...) initializer through line 27: %+v", s)
					}
				}
			}
		})
	}
}

func TestKotlinPinnedIndependentWholeMoshiControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_KOTLIN_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Kotlin originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "kotlin-moshi-new-889013e.kt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2219 {
		t.Fatalf("independent whole Moshi original changed: %d", len(b))
	}
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("JsonString.kt", string(b)), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
		len(result.Analysis.Symbols) != 12 {
		t.Fatalf("independent Moshi 12 complete supported declarations: %d complete=%v",
			len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
	}
}

func TestKotlinMultilinePropertyInitializerBoundaries(t *testing.T) {
	cases := []struct {
		name, src string
		endLine   int
		count     int
	}{
		{"simple-constructor", "class C {\n val vendor =\n  Vendor(\n   name = \"value\",\n  )\n val after = 1\n}\n", 5, 3},
		{"plain-next-line", "class C {\n val vendor =\n  Vendor(7)\n val after = 1\n}\n", 3, 3},
		{"semicolon-next-declaration", "class C {\n val vendor =\n  Vendor(7)\n fun after() {}\n}\n", 3, 3},
		{"unsupported-continuation", "class C {\n val vendor =\n  Vendor(7)\n  .extra()\n val after = 1\n}\n", 2, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("property.kt", tc.src), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("symbol count %d expected %d: %+v", len(result.Analysis.Symbols), tc.count, result.Analysis.Symbols)
			}
			var found bool
			for _, s := range result.Analysis.Symbols {
				if s.Name == "vendor" {
					found = true
					if s.DeclarationRange.End.Line != tc.endLine {
						t.Errorf("vendor end line=%d expected %d", s.DeclarationRange.End.Line, tc.endLine)
					}
				}
			}
			if !found {
				t.Fatal("missing vendor property")
			}
		})
	}
}
