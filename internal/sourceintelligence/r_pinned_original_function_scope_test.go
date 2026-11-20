package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both entire original upstream R files are read, not executed. Function
// declarations must span their actual authored bodies; signatures end at
// the opening body brace, even when formal arguments use multiple lines.
func TestRPinnedWholeOriginalFunctionScopes(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_R_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete immutable R upstream originals not configured")
	}
	for _, tc := range []struct {
		file      string
		functions []struct {
			name string
			line int
			end  int
		}
	}{
		{"cli-ansi.R", []struct {
			name string
			line int
			end  int
		}{
			{"palette_idx", 4, 18}, {"palette_color", 20, 22},
			{"is_builtin_style", 87, 89}, {"ansi_style_str", 118, 120},
			{"create_ansi_style_tag", 122, 127}, {"create_ansi_style_fun", 129, 156},
			{"create_ansi_style", 158, 164}, {"print.cli_ansi_string", 168, 174},
			{"print.cli_ansi_style", 178, 183}, {"make_ansi_style", 229, 298},
			{"is_r_color", 302, 308}, {"is_rgb_matrix", 310, 312},
			{"ansi_style_from_r_color", 314, 316}, {"ansi_style_8_from_rgb", 318, 324},
			{"ansi_style_from_rgb", 326, 334}, {"ansi_scale", 345, 356},
			{"ansi256", 358, 373}, {"ansi256_rgb_index", 377, 396},
			{"ansitrue", 398, 410}, {"combine_ansi_styles", 446, 453},
		}},
		{"dplyr-rename.R", []struct {
			name string
			line int
			end  int
		}{
			{"rename", 63, 65}, {"rename.data.frame", 68, 75},
			{"rename_with", 83, 85}, {"rename_with.data.frame", 88, 112},
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original upstream R file unavailable: %v %v", info, err)
			}
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := (RAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(src)), testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("complete original unexpectedly marked incomplete: %+v", result.Analysis)
			}
			if len(result.Analysis.Symbols) != len(tc.functions) {
				t.Fatalf("whole authored R functions: got=%d want=%d", len(result.Analysis.Symbols), len(tc.functions))
			}
			for i, expected := range tc.functions {
				s := result.Analysis.Symbols[i]
				if s.Name != expected.name || s.QualifiedName != expected.name ||
					s.Kind != SymbolKindFunction || s.NativeKind != "assigned-function" ||
					s.NameRange.Start.Line != expected.line {
					t.Errorf("original authored function %d mismatch: %+v, expected=%+v", i, s, expected)
				}
				if s.DeclarationRange.End.Line != expected.end ||
					s.DeclarationRange.End.Column != 2 {
					t.Errorf("original authored body for %s ends at %d:2, got %+v",
						expected.name, expected.end, s.DeclarationRange.End)
				}
				if expected.name == "make_ansi_style" {
					if s.SignatureRange == nil || s.SignatureRange.End.Line != 234 ||
						s.SignatureRange.End.Column != 4 ||
						!strings.Contains(s.Signature, "colors = num_ansi_colors()\n) {") {
						t.Errorf("authored multiline formal argument signature lost: %+v", s)
					}
				}
			}
		})
	}
}

func TestRBracedFunctionLexicalBoundaries(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			text := strings.Join([]string{
				"outer <- function(x = list(a = (1))) {",
				"  text <- \"brace } # string\"",
				"  if (length(x) > 0) { x } # }",
				"}",
				"after <- function(y) { y }",
				"",
			}, eol)
			result, err := (RAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("nested.R", text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 2 || !result.Analysis.CoverageComplete {
				t.Fatalf("R lexical balance produced unexpected symbols/coverage: %+v", result.Analysis)
			}
			outer, after := result.Analysis.Symbols[0], result.Analysis.Symbols[1]
			if outer.DeclarationRange.End.Line != 4 || outer.DeclarationRange.End.Column != 2 ||
				after.DeclarationRange.End.Line != 5 ||
				!strings.HasSuffix(outer.Signature, "{") ||
				!strings.Contains(outer.Signature, "list(a = (1))") {
				t.Fatalf("nested parentheses/comments/quoted braces changed source spans: outer=%+v after=%+v", outer, after)
			}
		})
	}
	for _, tc := range []struct {
		name, source string
		incomplete   bool
	}{
		{"nonbraced-expression", "plain <- function(x) x + 1\n", false},
		{"missing-closing-brace", "broken <- function(x) {\n  x\n", true},
		{"bounded-oversized-brace", "large <- function(x) {\n" + strings.Repeat("  value <- 1\n", 7000) + "}\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (RAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("bounded.R", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 1 || result.Analysis.CoverageComplete == tc.incomplete {
				t.Fatalf("unexpected R fallback or incomplete coverage: %+v", result.Analysis)
			}
			if tc.incomplete && result.Analysis.Symbols[0].DeclarationRange.End.Line != 1 {
				t.Fatalf("unproven R function range must not span unrelated source: %+v", result.Analysis.Symbols[0])
			}
		})
	}
}
