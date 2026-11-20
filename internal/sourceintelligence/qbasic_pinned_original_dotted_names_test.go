package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

// Test complete independently pinned QBasic sources against their directly
// authored dotted identifiers. Original files remain immutable.
func TestQBasicPinnedWholeOriginalDottedNames(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_QBASIC_OUTLINE_ROOT")
	if root == "" {
		t.Skip("four complete independent QBasic originals are not configured")
	}
	cases := []struct {
		file       string
		count      int
		legacy1252 bool
	}{
		{"advgen-AG.QBI", 116, false},
		{"advgen-DRAWBOX.BAS", 12, true},
		{"qb64pe-logging.bas", 1, false},
		{"qb64pe-settings.bas", 1, false},
	}
	names := map[string][]struct {
		line int
		raw  string
		name string
	}{
		"advgen-AG.QBI": {
			{140, "C.IFLOC", "C.IFLOC"}, {141, "C.MOVE", "C.MOVE"},
			{142, "C.DISPLAY", "C.DISPLAY"}, {143, "C.LOCK", "C.LOCK"},
			{144, "C.UNLOCK", "C.UNLOCK"}, {145, "C.OPEN", "C.OPEN"},
			{146, "C.CLOSE", "C.CLOSE"}, {147, "C.DEFAULT", "C.DEFAULT"},
			{148, "C.CREATURE", "C.CREATURE"}, {149, "C.POINTS", "C.POINTS"},
			{150, "C.STOP", "C.STOP"}, {151, "C.IFLOCNOT", "C.IFLOCNOT"},
			{152, "C.IFABOUT", "C.IFABOUT"}, {153, "C.STARTEVENT", "C.STARTEVENT"},
			{154, "C.KILLEVENT", "C.KILLEVENT"}, {155, "C.ADDEXIT", "C.ADDEXIT"},
		},
		"advgen-DRAWBOX.BAS": {
			{2, "Find.Help!", "Find.Help"}, {3, "Get.KeyCode!", "Get.KeyCode"},
			{15, "Draw.Box", "Draw.Box"}, {35, "Draw.Rect", "Draw.Rect"},
			{202, "Restore.Screen", "Restore.Screen"}, {210, "Save.Screen", "Save.Screen"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 1<<20 {
				t.Fatalf("unavailable or unsafe entire QBasic original: info=%v error=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if tc.legacy1252 {
				data, err = charmap.Windows1252.NewDecoder().Bytes(data)
				if err != nil {
					t.Fatalf("original Windows-1252 decoding: %v", err)
				}
			}
			text := string(data)
			result, err := (QBasicAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, text), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != tc.count {
				t.Fatalf("complete QBasic original outline: %+v", result.Analysis)
			}
			byLine := make(map[int]NormalizedSymbol)
			for _, symbol := range result.Analysis.Symbols {
				byLine[symbol.NameRange.Start.Line] = symbol
			}
			lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
			for _, expected := range names[tc.file] {
				symbol, exists := byLine[expected.line]
				raw := lines[expected.line-1]
				column := strings.Index(raw, expected.raw) + 1
				if column < 1 {
					t.Fatalf("original line %d missing %q", expected.line, expected.raw)
				}
				kind := SymbolKindConstant
				if tc.file == "advgen-DRAWBOX.BAS" {
					kind = SymbolKindFunction
				}
				if !exists || symbol.Name != expected.name || symbol.Kind != kind ||
					symbol.QualifiedName != expected.name ||
					symbol.NameRange.Start.Column != column ||
					symbol.NameRange.End.Column != column+len(expected.raw) {
					t.Errorf("directly authored dotted declaration %s:%d: %+v exists=%v",
						tc.file, expected.line, symbol, exists)
				}
			}
			if tc.file == "advgen-DRAWBOX.BAS" {
				static, ok := byLine[51]
				if !ok || static.Name != "Insert" || static.Kind != SymbolKindVariable ||
					static.QualifiedName != "Editor.Insert" {
					t.Errorf("valid local STATIC variable lost: %+v exists=%v", static, ok)
				}
			}
		})
	}
}

func TestQBasicDottedNameLFCRLFAndNegativeControls(t *testing.T) {
	for _, eol := range []string{"\n", "\r\n"} {
		lines := []string{
			"CONST C.IFLOC = 1",
			"CONST Plain = 2",
			"DECLARE FUNCTION Find.Help! ()",
			"SUB Draw.Box ()",
			"    ' CONST C.COMMENT = 1",
			"    dummy$ = \"a.b\"",
			"END SUB",
			"",
		}
		source := strings.Join(lines, eol)
		result, err := (QBasicAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("dotted.bas", source), testAnalyzeOptions(true, 32))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 4 {
			t.Fatalf("valid QBasic LF/CRLF direct declarations and negatives: %+v", result.Analysis)
		}
		for i, want := range []string{"C.IFLOC", "Plain", "Find.Help", "Draw.Box"} {
			s := result.Analysis.Symbols[i]
			if s.Name != want || s.QualifiedName != want {
				t.Errorf("source-authored direct name %q: %+v", want, s)
			}
		}
	}
}
