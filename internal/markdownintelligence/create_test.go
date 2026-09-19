package markdownintelligence

import (
	"errors"
	"strings"
	"testing"

	"github.com/zoster81/marksplice"
	"github.com/zoster81/scripthold/internal/operation"
)

func TestBuildDocumentUsesTypedMarkspliceConstruction(t *testing.T) {
	result, err := BuildDocument(CreateDocument{
		FrontMatter: &CreateFrontMatter{
			Format: "yaml",
			Fields: []CreateFrontMatterField{{Key: "title", Value: "Docs"}},
		},
		Blocks: []CreateBlock{
			{
				Type:  "heading",
				Level: 1,
				Content: []CreateInline{
					{Type: "text", Text: "Hello "},
					{Type: "emphasis", Children: []CreateInline{{Type: "text", Text: "world"}}},
				},
			},
			{Type: "paragraph", Content: []CreateInline{
				{Type: "text", Text: "See "},
				{Type: "forward_reference_link", Reference: "docs", Children: []CreateInline{{Type: "text", Text: "documentation"}}},
				{Type: "text", Text: "."},
			}},
			{Type: "blockquote", Depth: 1, Blocks: []CreateBlock{
				{Type: "paragraph", Content: []CreateInline{{Type: "strong", Children: []CreateInline{{Type: "text", Text: "Nested"}}}}},
			}},
			{Type: "reference_definition", Label: "docs", Destination: "https://example.com", Deferred: true},
		},
	}, CreateLimits{MaxElements: MaxCreateElements, MaxDepth: MaxCreateDepth, MaxTextBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	got := string(result)
	for _, want := range []string{
		"---\n",
		"title: \"Docs\"\n",
		"# Hello *world*\n",
		"See [documentation][docs]\\.\n",
		"> **Nested**\n",
		"[docs]: <https://example.com>\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated Markdown missing %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(got, "\n") {
		t.Fatalf("generated Markdown lacks final LF: %q", got)
	}
}

func TestBuildDocumentSupportsListsTasksCodeMathAndTables(t *testing.T) {
	result, err := BuildDocument(CreateDocument{Blocks: []CreateBlock{
		{Type: "list", Items: []CreateListItem{{Markdown: "parent", Depth: 0}, {Markdown: "child", Depth: 1}}},
		{Type: "task_list", Ordered: true, Items: []CreateListItem{{Markdown: "done", Checked: true, Depth: 0}}},
		{Type: "fenced_code", Code: "fmt.Println(\"ok\")", Info: "go"},
		{Type: "math_block", Payload: "x+y"},
		{Type: "table", Header: []string{"A", "B"}, Alignments: []string{"left", "right"}, Rows: [][]string{{"1", "2"}}},
	}}, CreateLimits{MaxTextBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	got := string(result)
	for _, want := range []string{"- parent\n", "  - child\n", "1. [x] done\n", "\x60\x60\x60go\n", "$$x+y$$\n", "| A | B |"} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated Markdown missing %q:\n%s", want, got)
		}
	}
}

func TestBuildDocumentDelegatesInvalidSyntaxToMarksplice(t *testing.T) {
	_, err := BuildDocument(CreateDocument{Blocks: []CreateBlock{{
		Type:        "paragraph",
		RawMarkdown: "# becomes a heading",
	}}}, CreateLimits{MaxTextBytes: 1 << 20})
	if !errors.Is(err, marksplice.ErrInvalidConstruction) {
		t.Fatalf("error = %v, want ErrInvalidConstruction", err)
	}
}

func TestBuildDocumentEnforcesHostConstructionLimitsBeforeBuilderWork(t *testing.T) {
	deep := CreateInline{Type: "text", Text: "leaf"}
	for range 5 {
		deep = CreateInline{Type: "emphasis", Children: []CreateInline{deep}}
	}
	_, err := BuildDocument(CreateDocument{Blocks: []CreateBlock{{
		Type: "paragraph", Content: []CreateInline{deep},
	}}}, CreateLimits{MaxElements: 100, MaxDepth: 3, MaxTextBytes: 1 << 20})
	if operation.KindOf(err) != operation.KindLimit || !strings.Contains(err.Error(), "depth exceeds") {
		t.Fatalf("depth error = %v", err)
	}

	_, err = BuildDocument(CreateDocument{Blocks: []CreateBlock{{
		Type: "paragraph", Content: []CreateInline{{Type: "text", Text: "0123456789"}},
	}}}, CreateLimits{MaxElements: 100, MaxDepth: 10, MaxTextBytes: 4})
	if operation.KindOf(err) != operation.KindLimit || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("byte-limit error = %v", err)
	}
}

func TestBuildDocumentRejectsUnknownClosedModelValues(t *testing.T) {
	_, err := BuildDocument(CreateDocument{Blocks: []CreateBlock{{Type: "unknown"}}}, CreateLimits{MaxTextBytes: 1 << 20})
	if !errors.Is(err, marksplice.ErrInvalidConstruction) {
		t.Fatalf("unknown block error = %v", err)
	}

	_, err = BuildDocument(CreateDocument{Blocks: []CreateBlock{{
		Type: "paragraph", Content: []CreateInline{{Type: "unknown"}},
	}}}, CreateLimits{MaxTextBytes: 1 << 20})
	if !errors.Is(err, marksplice.ErrInvalidConstruction) {
		t.Fatalf("unknown inline error = %v", err)
	}
}
