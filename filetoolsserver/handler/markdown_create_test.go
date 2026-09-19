package handler

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/markdownintelligence"
)

func TestMarkdownCreateTransportMapsClosedModelToConstructionAdapter(t *testing.T) {
	raw := []byte(`{
		"path":"doc.md",
		"encoding":"utf-8",
		"bom":"never",
		"frontMatter":{"format":"toml","fields":[{"key":"title","value":"Docs"}]},
		"blocks":[
			{"type":"heading","level":1,"content":[{"type":"text","text":"Title"}]},
			{"type":"blockquote","depth":1,"blocks":[
				{"type":"paragraph","content":[{"type":"strong","children":[{"type":"text","text":"Nested"}]}]}
			]},
			{"type":"fenced_code","code":"fmt.Println(1)","info":"go"},
			{"type":"table","header":["A"],"rows":[["1"]],"alignments":["center"]}
		]
	}`)
	var input MarkdownCreateInput
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if input.Path != "doc.md" || input.Encoding != "utf-8" || input.BOM != "never" {
		t.Fatalf("transport metadata = %+v", input)
	}
	if got := input.Blocks[2].Code; got != "fmt.Println(1)" {
		t.Fatalf("fenced code = %q", got)
	}

	document := markdownCreateDocument(input)
	result, err := markdownintelligence.BuildDocument(document, markdownintelligence.CreateLimits{MaxTextBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	got := string(result)
	for _, want := range []string{
		"+++\n",
		"title = \"Docs\"\n",
		"# Title\n",
		"> **Nested**\n",
		"\x60\x60\x60go\n",
		"| A |",
		"| :---: |",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated Markdown missing %q:\n%s", want, got)
		}
	}
}

func TestMarkdownCreateTransportConversionOwnsNestedSlices(t *testing.T) {
	input := MarkdownCreateInput{Blocks: []MarkdownCreateBlock{{
		Type:       "table",
		Header:     []string{"A"},
		Rows:       [][]string{{"one"}},
		Alignments: []string{"left"},
	}}}
	document := markdownCreateDocument(input)

	input.Blocks[0].Header[0] = "changed"
	input.Blocks[0].Rows[0][0] = "changed"
	input.Blocks[0].Alignments[0] = "right"

	block := document.Blocks[0]
	if block.Header[0] != "A" || block.Rows[0][0] != "one" || block.Alignments[0] != "left" {
		t.Fatalf("construction model aliases transport input: %+v", block)
	}
}
