package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestLuauPinnedWholeOriginalTypeFieldsAreNotDeclarations(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_LUAU_OUTLINE_FILE")
	if path == "" {
		t.Skip("pinned whole Luau original not configured")
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(source) != 3347 {
		t.Fatalf("pinned raw Git original changed: %d bytes", len(source))
	}
	want := []struct {
		name string
		line int
	}{
		{"Node", 3}, {"createNode", 17}, {"addChild", 33}, {"createDocument", 37},
		{"createParagraph", 41}, {"createHeading", 47}, {"createCodeBlock", 54},
		{"createBlockquote", 61}, {"createList", 65}, {"createListItem", 71},
		{"createThematicBreak", 75}, {"createText", 79}, {"createEmphasis", 85},
		{"createStrong", 89}, {"createCode", 93}, {"createLink", 99},
		{"createImage", 106}, {"createHtmlBlock", 113}, {"createSoftBreak", 119},
		{"createHardBreak", 123},
	}
	for _, original := range []struct{ name, value string }{
		{"git-lf", string(source)}, {"worktree-crlf", strings.ReplaceAll(string(source), "\n", "\r\n")},
	} {
		t.Run(original.name, func(t *testing.T) {
			result, err := (LuauAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("original.luau", original.value), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatal("unexpected incomplete analysis")
			}
			if len(result.Analysis.Symbols) != len(want) {
				t.Errorf("complete pinned source has %d supported declarations; got %d", len(want), len(result.Analysis.Symbols))
			}
			byLine := make(map[int]string)
			for _, s := range result.Analysis.Symbols {
				if s.Name == "string" || s.Name == "nodeType" {
					t.Errorf("invented type from record/table field: %s line %d", s.Name, s.NameRange.Start.Line)
				}
				if prev, exists := byLine[s.NameRange.Start.Line]; exists {
					t.Errorf("duplicate source declaration at %d: %s, %s", s.NameRange.Start.Line, prev, s.Name)
				}
				byLine[s.NameRange.Start.Line] = s.Name
			}
			for _, w := range want {
				if byLine[w.line] != w.name {
					t.Errorf("source line %d: expected %q, got %q", w.line, w.name, byLine[w.line])
				}
			}
		})
	}
}

func TestLuauTypeHeaderDoesNotScanPastPunctuation(t *testing.T) {
	source := "export type Data<T> = { type: string }\nlocal function factory(): Data<string>\n    return { type = value }\nend\ntype Alias = string\n"
	result, err := (LuauAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("type.luau", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range result.Analysis.Symbols {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "Data,factory,Alias" {
		t.Fatalf("types and function only, got %v", names)
	}
}
