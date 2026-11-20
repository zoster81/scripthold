package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTwigPinnedWholeOriginalInlineBlockScope(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_TWIG_OUTLINE_ROOT")
	if root == "" {
		t.Skip("three complete pinned upstream Twig originals not configured")
	}
	for _, tc := range []struct {
		file, marker string
		want         map[string]int
	}{
		{"symfony-admin-layout.html.twig", "{% block importmap %}", map[string]int{
			"importmap": 10, "header_navigation_links": 15,
		}},
		{"easyadmin-menu.html.twig", "{% block main_menu %}", map[string]int{
			"main_menu_before": 3, "main_menu": 5, "main_menu_after": 113,
			"render_menu_badge": 120,
		}},
		{"symfony-blog-index.html.twig", "{% block body_id 'blog_index' %}", map[string]int{
			"body_id": 3, "main": 5, "sidebar": 54,
		}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
				t.Fatalf("original source must be a bounded regular file: stat=%v err=%v", stat, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.marker) {
				t.Fatal("original source marker mismatch")
			}
			result, err := (TwigAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(raw)), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("complete authored Twig source must have complete coverage: %+v", result.Analysis)
			}
			got := result.Analysis.Symbols
			if len(got) != len(tc.want) {
				t.Errorf("full source symbol count got %d want %d", len(got), len(tc.want))
			}
			seen := make(map[string]bool, len(got))
			for _, sym := range got {
				if sym.QualifiedName != sym.Name || sym.ParentID != "" ||
					sym.ParentQualifiedName != "" {
					t.Errorf("top-level block incorrectly nested: %+v", sym)
				}
				if expectedLine, exists := tc.want[sym.Name]; !exists ||
					expectedLine != sym.NameRange.Start.Line {
					t.Errorf("unexpected or misplaced Twig declaration: %+v", sym)
				}
				if sym.Name == "body_id" {
					if sym.DeclarationRange.End.Line != 3 ||
						sym.DeclarationRange.End.Column <= sym.NameRange.End.Column ||
						sym.BodyRange != nil {
						t.Errorf("inline Twig block must finish at its authored tag, with no body: %+v", sym)
					}
				}
				if seen[sym.Name] {
					t.Errorf("duplicate Twig declaration: %+v", sym)
				}
				seen[sym.Name] = true
			}
			for name := range tc.want {
				if !seen[name] {
					t.Errorf("missing authored Twig block or macro: %s", name)
				}
			}
		})
	}
}

func TestTwigInlineBlocksDoNotBorrowNeighboringOrNestedClosers(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(eol, "\r", "CR"), func(t *testing.T) {
			text := strings.Join([]string{
				"{% block title 'hello' %}",
				"{% block main %}",
				"{% block badge 'inside' -%}",
				"{% block child %}value{% endblock %}",
				"{% endblock %}",
				"{% block sidebar %}ok{% endblock %}",
				"",
			}, eol)
			result, err := (TwigAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("inline.twig", text), testAnalyzeOptions(true, 50))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
				len(result.Analysis.Symbols) != 5 {
				t.Fatalf("all inline and paired Twig scopes must be retained: %+v", result.Analysis)
			}
			want := map[string]int{"title": 1, "main": 2, "main.badge": 3,
				"main.child": 4, "sidebar": 6}
			for _, sym := range result.Analysis.Symbols {
				if want[sym.QualifiedName] != sym.NameRange.Start.Line {
					t.Errorf("authored lexical owner lost: %+v", sym)
				}
				if sym.Name == "title" || sym.Name == "badge" {
					if sym.BodyRange != nil || sym.DeclarationRange.End.Line != sym.NameRange.Start.Line {
						t.Errorf("inline block incorrectly acquired a paired body: %+v", sym)
					}
				}
				delete(want, sym.QualifiedName)
			}
			if len(want) != 0 {
				t.Errorf("missing original authored blocks: %v", want)
			}
		})
	}
}

func TestTwigUnterminatedPairedBlockStillSignalsPartialCoverage(t *testing.T) {
	result, err := (TwigAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("incomplete.twig", "{% block title 'inline' %}\n{% block main %}"),
		testAnalyzeOptions(true, 10))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatal("unclosed paired block must not be silently marked complete")
	}
}
