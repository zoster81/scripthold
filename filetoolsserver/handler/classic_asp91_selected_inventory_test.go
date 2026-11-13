package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independently annotated supported declarations in two fully read original
// Classic ASP files. This is selected-file structural coverage, not a complete
// VBScript grammar, a whole-project inventory, or runtime behavior validation.
func TestClassicASP91SelectedOriginalSupportedDeclarationInventory(t *testing.T) {
	base64 := os.Getenv("SCRIPTHOLD_REAL_ASP91_BASE64_ROOT")
	browser := os.Getenv("SCRIPTHOLD_REAL_ASP91_BROWSER_ROOT")
	if base64 == "" && browser == "" {
		t.Skip("selected pinned Classic ASP source scopes not configured")
	}
	if base64 == "" || browser == "" {
		t.Fatal("both independent pinned Classic ASP originals must be configured")
	}
	type declaration struct {
		name, kind, native, parent string
		line                       int
	}
	m := func(name, kind, native, parent string, line int) declaration {
		return declaration{name, kind, native, parent, line}
	}
	cases := []struct {
		name, root, file string
		want             []declaration
		negative         []string
	}{
		{
			name: "Base64 nested VBScript declarations", root: base64, file: "base64.inc.asp",
			want: []declaration{
				m("Base64Encode", "method", "function", "", 2),
				m("oXML", "variable", "variable", "Base64Encode", 3),
				m("oNode", "variable", "variable", "Base64Encode", 3),
				m("Base64Decode", "method", "function", "", 13),
				m("oXML", "variable", "variable", "Base64Decode", 14),
				m("oNode", "variable", "variable", "Base64Decode", 14),
				m("Stream_StringToBinary", "method", "function", "", 24),
				m("adTypeText", "constant", "const", "Stream_StringToBinary", 25),
				m("adTypeBinary", "constant", "const", "Stream_StringToBinary", 26),
				m("BinaryStream", "variable", "variable", "Stream_StringToBinary", 27),
				m("Stream_BinaryToString", "method", "function", "", 40),
				m("adTypeText", "constant", "const", "Stream_BinaryToString", 41),
				m("adTypeBinary", "constant", "const", "Stream_BinaryToString", 42),
				m("BinaryStream", "variable", "variable", "Stream_BinaryToString", 43),
			},
			negative: []string{"CreateObject", "ReadText", "WriteText"},
		},
		{
			name: "Directory filter versus output constants", root: browser, file: "list.asp",
			want: []declaration{
				m("sChevron", "constant", "const", "", 17),
				m("bShowPath", "constant", "const", "", 20),
				m("bShowRoot", "constant", "const", "", 22),
				m("sFont", "constant", "const", "", 25),
				m("sColFont", "constant", "const", "", 27),
				m("TblHeader", "constant", "const", "", 29),
				m("FileRow1", "constant", "const", "", 32),
				m("FileRow2", "constant", "const", "", 33),
				m("FolderRow1", "constant", "const", "", 34),
				m("FolderRow2", "constant", "const", "", 35),
				m("sError", "variable", "variable", "", 52),
				m("IsValidFile", "method", "function", "", 177),
				m("ByteConversion", "method", "function", "", 196),
			},
			// Assignments and method calls are not declarations.
			negative: []string{"AllowExt", "DenyExt", "MapPath", "Server", "response"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tc.root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("pinned source is not a bounded regular file: info=%v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			// Every declaration was separately located from the complete
			// original text before querying the development handler.
			originalLines := strings.Split(string(data), "\n")
			for _, expected := range tc.want {
				if expected.line <= 0 || expected.line > len(originalLines) ||
					!strings.Contains(originalLines[expected.line-1], expected.name) {
					t.Fatalf("independent original-source line oracle changed: %+v", expected)
				}
			}
			_, outline, err := NewHandler([]string{tc.root}).SourceSymbols(
				context.Background(), nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed",
					Paths: []string{path}, Language: "classic-asp",
					MaxFiles: 1, MaxSymbols: len(tc.want) + 16,
				})
			if err != nil || outline.FilesParsed != 1 || !outline.CoverageComplete ||
				outline.Truncated || len(outline.Symbols) != len(tc.want) {
				t.Fatalf("selected-file supported inventory count/coverage wrong: err=%v want=%d got=%d complete=%v truncated=%v symbols=%+v",
					err, len(tc.want), len(outline.Symbols), outline.CoverageComplete, outline.Truncated, outline.Symbols)
			}
			expected := make(map[string]bool, len(tc.want))
			key := func(name, kind, native, parent string, line int) string {
				return fmt.Sprintf("%s|%s|%s|%s|%d", name, kind, native, parent, line)
			}
			for _, d := range tc.want {
				k := key(d.name, d.kind, d.native, d.parent, d.line)
				if _, duplicate := expected[k]; duplicate {
					t.Fatalf("duplicate independently annotated declaration: %s", k)
				}
				expected[k] = false
			}
			for _, symbol := range outline.Symbols {
				if symbol.Path != path || symbol.ID == "" {
					t.Errorf("unsafe returned source identity: %+v", symbol)
				}
				for _, forbidden := range tc.negative {
					if symbol.Name == forbidden {
						t.Errorf("call/assignment fabricated as declaration: %+v", symbol)
					}
				}
				k := key(symbol.Name, string(symbol.Kind), symbol.NativeKind,
					symbol.ParentQualifiedName, symbol.NameRange.Start.Line)
				if _, ok := expected[k]; !ok {
					t.Errorf("original supported declaration invented or mislocated: %s", k)
				} else if expected[k] {
					t.Errorf("original declaration duplicated: %s", k)
				} else {
					expected[k] = true
				}
			}
			for k, found := range expected {
				if !found {
					t.Errorf("original supported declaration omitted: %s", k)
				}
			}
			t.Logf("independently inventoried supported declarations: %d", len(tc.want))
		})
	}
}
