package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassicASPPinnedWholeBase64AuthoredOrder(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CLASSIC_ASP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Classic ASP originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "classic-asp-base64-1ca2387.asp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 1592 {
		t.Fatalf("pinned Base64 whole original size changed: %d", len(b))
	}
	for _, tc := range []struct{ name, src string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("base64.inc.asp", tc.src), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 14 {
				t.Fatalf("complete Base64 supported declarations: count=%d complete=%v truncated=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete, result.Analysis.Truncated)
			}
			want := []struct {
				name string
				line int
			}{
				{"Base64Encode", 2}, {"Base64Encode.oXML", 3}, {"Base64Encode.oNode", 3},
				{"Base64Decode", 13}, {"Base64Decode.oXML", 14}, {"Base64Decode.oNode", 14},
				{"Stream_StringToBinary", 24}, {"Stream_StringToBinary.adTypeText", 25},
				{"Stream_StringToBinary.adTypeBinary", 26}, {"Stream_StringToBinary.BinaryStream", 27},
				{"Stream_BinaryToString", 40}, {"Stream_BinaryToString.adTypeText", 41},
				{"Stream_BinaryToString.adTypeBinary", 42}, {"Stream_BinaryToString.BinaryStream", 43},
			}
			lastNameOffset := -1
			for i, s := range result.Analysis.Symbols {
				if s.QualifiedName != want[i].name || s.NameRange.Start.Line != want[i].line {
					t.Errorf("source order index %d actual %q line%d expected %q line%d", i, s.QualifiedName, s.NameRange.Start.Line, want[i].name, want[i].line)
				}
				if s.nameOffsets.Start <= lastNameOffset {
					t.Errorf("source name offsets not strictly ascending at %d: %s", i, s.QualifiedName)
				}
				lastNameOffset = s.nameOffsets.Start
			}
		})
	}
}

func TestClassicASPPinnedIndependentWholeDirectoryControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CLASSIC_ASP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Classic ASP originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "classic-asp-directory-86aefb1.asp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 7775 {
		t.Fatalf("pinned independent directory original changed: %d", len(b))
	}
	result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("list.asp", string(b)), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated || len(result.Analysis.Symbols) != 13 {
		t.Fatalf("whole independent directory 13 declarations count=%d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
	}
	want := []string{"sChevron", "bShowPath", "bShowRoot", "sFont", "sColFont", "TblHeader", "FileRow1", "FileRow2", "FolderRow1", "FolderRow2", "sError", "IsValidFile", "ByteConversion"}
	for i, s := range result.Analysis.Symbols {
		if s.Name != want[i] {
			t.Errorf("directory original index %d got %q expected %q", i, s.Name, want[i])
		}
	}
}

func TestClassicASPSameLineDimOrderAndVisibility(t *testing.T) {
	source := "<%\nFunction Test()\n  Dim zeta, alpha, omega\nEnd Function\nFunction NextOne()\n  Dim zeta, alpha\nEnd Function\n%>\n"
	result, err := (ClassicASPAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("ordered.asp", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Test", "Test.zeta", "Test.alpha", "Test.omega", "NextOne", "NextOne.zeta", "NextOne.alpha"}
	if len(result.Analysis.Symbols) != len(want) {
		t.Fatalf("got %d expected %d: %+v", len(result.Analysis.Symbols), len(want), result.Analysis.Symbols)
	}
	for i, s := range result.Analysis.Symbols {
		if s.QualifiedName != want[i] {
			t.Errorf("same-line source order %d got %q expected %q", i, s.QualifiedName, want[i])
		}
	}
}
