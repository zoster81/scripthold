package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// R currently indexes named '<- function' declarations, including direct
// assignments inside callback bodies, but does not resolve lexical ownership.
// The inventory below is source-first and complete only for this supported
// assigned-function syntax in the four pinned selected original files.
// Nonfunction globals, expressions, and R package execution are not certified.
func TestRSecondHalfOriginalAssignedFunctionInventory(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original R projects not configured")
	}
	type declaration struct {
		project, file string
		line          int
		name          string
	}
	sourceFirst := []declaration{
		{"r62-cli-audit-selected", "R/ansi.R", 4, "palette_idx"},
		{"r62-cli-audit-selected", "R/ansi.R", 20, "palette_color"},
		{"r62-cli-audit-selected", "R/ansi.R", 87, "is_builtin_style"},
		{"r62-cli-audit-selected", "R/ansi.R", 118, "ansi_style_str"},
		{"r62-cli-audit-selected", "R/ansi.R", 122, "create_ansi_style_tag"},
		{"r62-cli-audit-selected", "R/ansi.R", 129, "create_ansi_style_fun"},
		{"r62-cli-audit-selected", "R/ansi.R", 158, "create_ansi_style"},
		{"r62-cli-audit-selected", "R/ansi.R", 168, "print.cli_ansi_string"},
		{"r62-cli-audit-selected", "R/ansi.R", 178, "print.cli_ansi_style"},
		{"r62-cli-audit-selected", "R/ansi.R", 229, "make_ansi_style"},
		{"r62-cli-audit-selected", "R/ansi.R", 302, "is_r_color"},
		{"r62-cli-audit-selected", "R/ansi.R", 310, "is_rgb_matrix"},
		{"r62-cli-audit-selected", "R/ansi.R", 314, "ansi_style_from_r_color"},
		{"r62-cli-audit-selected", "R/ansi.R", 318, "ansi_style_8_from_rgb"},
		{"r62-cli-audit-selected", "R/ansi.R", 326, "ansi_style_from_rgb"},
		{"r62-cli-audit-selected", "R/ansi.R", 345, "ansi_scale"},
		{"r62-cli-audit-selected", "R/ansi.R", 358, "ansi256"},
		{"r62-cli-audit-selected", "R/ansi.R", 377, "ansi256_rgb_index"},
		{"r62-cli-audit-selected", "R/ansi.R", 398, "ansitrue"},
		{"r62-cli-audit-selected", "R/ansi.R", 446, "combine_ansi_styles"},
		{"r62-dplyr-audit-selected", "R/rename.R", 63, "rename"},
		{"r62-dplyr-audit-selected", "R/rename.R", 68, "rename.data.frame"},
		{"r62-dplyr-audit-selected", "R/rename.R", 83, "rename_with"},
		{"r62-dplyr-audit-selected", "R/rename.R", 88, "rename_with.data.frame"},
		{"r62-dplyr-audit-selected", "tests/testthat/test-rename.R", 80, "fn"},
		{"r62-dplyr-audit-selected", "tests/testthat/test-rename.R", 89, "fn"},
	}
	groups := []struct {
		project string
		files   []string
		count   int
	}{
		{"r62-cli-audit-selected", []string{"R/ansi.R", "tests/testthat/test-ansi.R"}, 20},
		{"r62-dplyr-audit-selected", []string{"R/rename.R", "tests/testthat/test-rename.R"}, 6},
	}
	for _, group := range groups {
		t.Run(group.project, func(t *testing.T) {
			project := filepath.Join(corpus, group.project)
			paths := make([]string, 0, len(group.files))
			sources := map[string][]string{}
			for _, rel := range group.files {
				full := filepath.Join(project, filepath.FromSlash(rel))
				requireAdditionalOriginalFile(t, full)
				b, err := os.ReadFile(full)
				if err != nil {
					t.Fatal(err)
				}
				if len(b) > 128<<10 {
					t.Fatalf("oversized selected R source %s", rel)
				}
				sources[rel] = strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
				paths = append(paths, full)
			}
			expected := map[string]declaration{}
			for _, d := range sourceFirst {
				if d.project != group.project {
					continue
				}
				lines, ok := sources[d.file]
				if !ok || d.line < 1 || d.line > len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[d.line-1]), d.name+" <- function(") {
					t.Fatalf("source-first R oracle invalid: %+v", d)
				}
				key := d.file + ":" + strconv.Itoa(d.line)
				if _, duplicate := expected[key]; duplicate {
					t.Fatalf("duplicate expected declaration at %s", key)
				}
				expected[key] = d
			}
			if len(expected) != group.count {
				t.Fatalf("incorrect original inventory %d expected=%d", len(expected), group.count)
			}
			handler := NewHandler([]string{project})
			failed, out, err := handler.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: paths, Language: "r", Encoding: "utf-8",
				Presentation: "detailed", MaxFiles: len(paths), MaxSymbols: 64,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(paths) {
				t.Fatalf("incomplete R source scan err=%v failed=%+v parsed=%d complete=%v truncated=%v", err, failed, out.FilesParsed, out.CoverageComplete, out.Truncated)
			}
			seen := map[string]bool{}
			for _, sym := range out.Symbols {
				relative, err := filepath.Rel(project, sym.Path)
				if err != nil {
					t.Errorf("outside project path %s", sym.Path)
					continue
				}
				key := filepath.ToSlash(relative) + ":" + strconv.Itoa(sym.NameRange.Start.Line)
				if seen[key] {
					t.Errorf("duplicate indexed R declaration at %s", key)
				}
				seen[key] = true
				d, ok := expected[key]
				if !ok {
					t.Errorf("unexpected R function %s %s", key, sym.Name)
					continue
				}
				if sym.Name != d.name || string(sym.Kind) != "function" || sym.NativeKind != "assigned-function" || sym.ParentQualifiedName != "" {
					t.Errorf("R source mismatch %s: got %s %s native=%s owner=%q; expected name %s with lexical owner unresolved", key, sym.Kind, sym.Name, sym.NativeKind, sym.ParentQualifiedName, d.name)
				}
			}
			for key, d := range expected {
				if !seen[key] {
					t.Errorf("missing original R function %s %s", key, d.name)
				}
			}
			if len(seen) != len(expected) {
				t.Errorf("R function count actual=%d expected=%d", len(seen), len(expected))
			}
			t.Logf("two entire R originals; independently inspected assigned-function declarations=%d matched=%d; no lexical ownership/runtime guarantee", len(expected), len(seen))
		})
	}
}
