package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These line inventories were independently transcribed from complete Git
// original TOML source files before comparing source_symbols results. They
// cover every table and assignment in the seven selected TOML files, rather
// than only the few maintenance-task targets from the navigation audit.
//
// A full declaration inventory for selected files does not claim to validate
// TOML values, Cargo execution, or any unselected project files.
func TestTOMLSelectedOriginalCompleteDeclarationInventories(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original TOML source corpus not configured")
	}
	cases := []struct {
		project, file string
		// One source declaration per line: section/array-table or assignment.
		// Array values, multiline string contents, comments and blank lines
		// are deliberately excluded, including array elements beginning '['.
		lines []int
	}{
		{"toml60-zoxide-audit-selected", "Cargo.toml", []int{
			1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13,
			15, 16, 18, 19, 20, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33,
			35, 36, 38, 39, 40, 41, 42, 43, 45, 46, 47, 48, 49,
			51, 52, 53, 55, 56, 57, 58, 59, 61, 62, 104, 108, 109,
		}},
		{"toml60-zoxide-audit-selected", ".cargo/config.toml", []int{1, 2, 6, 7}},
		{"toml60-zoxide-audit-selected", "Cross.toml", []int{1, 2}},
		{"toml60-ripgrep-audit-selected", "Cargo.toml", []int{
			1, 2, 3, 4, 5, 10, 11, 12, 13, 14, 15, 16, 25, 26, 27, 28,
			30, 31, 32, 33, 35, 36, 37, 39, 40, 53, 54, 55, 57,
			58, 59, 60, 61, 62, 63, 64, 65, 66, 67, 69, 70,
			72, 73, 74, 75, 77, 78, 81, 83, 84, 86, 87,
			88, 89, 90, 91, 92, 93, 94, 95, 96, 98, 99,
			101, 102, 103, 104, 122,
		}},
		{"toml60-ripgrep-audit-selected", "crates/grep/Cargo.toml", []int{
			1, 2, 3, 4, 5, 8, 9, 10, 11, 12, 13, 14, 15,
			17, 18, 19, 20, 21, 22, 23, 25, 26, 27, 29, 30, 33, 34,
		}},
		{"toml60-ripgrep-audit-selected", "crates/pcre2/Cargo.toml", []int{
			1, 2, 3, 4, 5, 8, 9, 10, 11, 12, 13, 14, 15, 17, 18, 19, 20,
		}},
		{"toml60-ripgrep-audit-selected", "crates/printer/Cargo.toml", []int{
			1, 2, 3, 4, 5, 9, 10, 11, 12, 13, 14, 15, 16,
			18, 19, 20, 22, 23, 24, 25, 26, 27, 28, 29,
			31, 32, 34, 36, 44,
		}},
	}
	// Read every complete input before the first source_symbols invocation.
	// This is a test-harness full-byte read, not a claim of human full-project
	// review. The independent inventories above were authored from the source.
	type original struct {
		path  string
		lines []string
	}
	originals := make([]original, len(cases))
	for i, tc := range cases {
		p := filepath.Join(corpus, tc.project, filepath.FromSlash(tc.file))
		requireAdditionalOriginalFile(t, p)
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		originals[i] = original{path: p, lines: strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")}
	}
	for i, tc := range cases {
		t.Run(tc.project+"/"+tc.file, func(t *testing.T) {
			src := originals[i]
			expected := make(map[int]struct{}, len(tc.lines))
			for _, line := range tc.lines {
				if line < 1 || line > len(src.lines) {
					t.Fatalf("independent source line %d out of bounds", line)
				}
				if _, dup := expected[line]; dup {
					t.Fatalf("duplicate independent inventory line %d", line)
				}
				expected[line] = struct{}{}
				text := strings.TrimSpace(src.lines[line-1])
				if !strings.HasPrefix(text, "[") && !strings.Contains(text, "=") {
					t.Fatalf("independent inventory line %d is not a TOML declaration: %q", line, text)
				}
			}
			h := NewHandler([]string{filepath.Dir(src.path)})
			failed, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{src.path}, Language: "toml",
				Encoding: "utf-8", Presentation: "detailed", MaxFiles: 1, MaxSymbols: 500,
			})
			if err != nil || failed != nil || result.Truncated || !result.CoverageComplete ||
				result.FilesParsed != 1 {
				t.Fatalf("full original file outline not reliable: err=%v failed=%+v result=%+v", err, failed, result)
			}
			actual := make(map[int]struct{}, len(result.Symbols))
			for _, sym := range result.Symbols {
				line := sym.NameRange.Start.Line
				if filepath.Clean(sym.Path) != filepath.Clean(src.path) ||
					line < 1 || line > len(src.lines) {
					t.Errorf("invented or misplaced declaration: %+v", sym)
					continue
				}
				if _, dup := actual[line]; dup {
					t.Errorf("multiple declarations reported on one source line: %d", line)
					continue
				}
				actual[line] = struct{}{}
				raw := src.lines[line-1]
				if !strings.Contains(raw, sym.Name) {
					t.Errorf("symbol name %q absent at source line %d: %q", sym.Name, line, raw)
				}
				if !strings.HasPrefix(strings.TrimSpace(raw), "[") {
					// All assignments in these seven complete original files
					// use bare keys, including dotted Cargo keys.
					key, _, ok := strings.Cut(raw, "=")
					if !ok || sym.Name != strings.TrimSpace(key) {
						t.Errorf("incorrect source key at line %d: got %q, original %q", line, sym.Name, key)
					}
				}
				if _, ok := expected[line]; !ok {
					t.Errorf("extra declaration %s %q at line %d: %q", sym.Kind, sym.Name, line, raw)
				}
				wantKind := "key"
				if strings.HasPrefix(strings.TrimSpace(raw), "[") {
					wantKind = "section"
				}
				if string(sym.Kind) != wantKind {
					t.Errorf("unexpected declaration kind %s, want %s, at line %d", sym.Kind, wantKind, line)
				}
			}
			for line := range expected {
				if _, ok := actual[line]; !ok {
					t.Errorf("missing source declaration at line %d: %q", line, src.lines[line-1])
				}
			}
			t.Logf("independentCompleteFileInventory=%d indexed=%d coverageComplete=%v",
				len(expected), len(result.Symbols), result.CoverageComplete)
		})
	}
}
