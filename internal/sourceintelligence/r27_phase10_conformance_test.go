package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase10OpaqueRegionsDoNotEmitDeclarations(t *testing.T) {
	tests := []struct {
		name      string
		analyzer  SourceAnalyzer
		text      string
		want      string
		forbidden []string
	}{
		{
			name: "sql-comments-and-strings", analyzer: SQLAnalyzer{},
			text: "-- CREATE TABLE fake_comment(id int);\nSELECT 'CREATE VIEW fake_string AS SELECT 1';\n/* CREATE FUNCTION fake_block() RETURNS int; */\nCREATE TABLE real_table(id int);\n",
			want: "real_table", forbidden: []string{"fake_comment", "fake_string", "fake_block"},
		},
		{
			name: "plsql-comments-and-strings", analyzer: PLSQLAnalyzer{},
			text: "CREATE OR REPLACE PACKAGE demo AS\n  -- PROCEDURE fake_comment;\n  value VARCHAR2(100) := 'FUNCTION fake_string RETURN NUMBER';\n  /* PROCEDURE fake_block; */\n  PROCEDURE real_run;\nEND demo;\n/\n",
			want: "demo.real_run", forbidden: []string{"demo.fake_comment", "demo.fake_string", "demo.fake_block"},
		},
		{
			name: "graphql-descriptions", analyzer: GraphQLAnalyzer{},
			text: "\"\"\"type Fake { hidden: String }\"\"\"\n# type Commented { x: Int }\ntype Real { value: String }\n",
			want: "Real", forbidden: []string{"Fake", "Commented"},
		},
		{
			name: "terraform-heredoc", analyzer: TerraformAnalyzer{},
			text: "locals { text = <<EOF\nresource \"fake\" \"inside_text\" {}\nEOF\n}\nresource \"real\" \"bucket\" {}\n",
			want: "real.bucket", forbidden: []string{"fake.inside_text"},
		},
		{
			name: "proto-block-comment", analyzer: ProtoAnalyzer{},
			text: "/*\nmessage Fake { string hidden = 1; }\n*/\nmessage Real { string value = 1; }\n",
			want: "Real", forbidden: []string{"Fake"},
		},
		{
			name: "verilog-block-comment", analyzer: VerilogAnalyzer{},
			text: "/*\nmodule fake; wire hidden; endmodule\n*/\nmodule real;\n  // wire comment_signal;\n  wire actual;\n  string text = \"wire fake_string;\";\nendmodule\n",
			want: "real.actual", forbidden: []string{"fake", "real.comment_signal", "real.fake_string"},
		},
		{
			name: "systemverilog-block-comment", analyzer: SystemVerilogAnalyzer{},
			text: "/* interface fake_if; logic hidden; endinterface */\ninterface real_if;\n  logic actual; // logic commented;\nendinterface\n",
			want: "real_if.actual", forbidden: []string{"fake_if", "real_if.commented"},
		},
		{
			name: "html-comment", analyzer: HTMLAnalyzer{},
			text: "<!-- <div id=\"fake\"></div> --><main id=\"real\"></main>",
			want: "real", forbidden: []string{"fake"},
		},
		{
			name: "xml-comment-cdata", analyzer: XMLAnalyzer{},
			text: "<root><!-- <item id=\"fake-comment\"/> --><![CDATA[<item id=\"fake-cdata\"/>]]><item id=\"real\"/></root>",
			want: "real", forbidden: []string{"fake-comment", "fake-cdata"},
		},
		{
			name: "css-comment-and-string", analyzer: CSSAnalyzer{},
			text: "/* .fake { color: red; } */\n.real { content: \".also-fake { x: y }\"; display: block; }\n",
			want: ".real", forbidden: []string{".fake", ".also-fake"},
		},
		{
			name: "markdown-fenced-code", analyzer: MarkdownAnalyzer{},
			text: "# Real\n\n```markdown\n# Fake\n```\n\n## Child\n",
			want: "Real.Child", forbidden: []string{"Real.Fake"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(tc.text)
			doc.Path = "fixture"
			result, err := tc.analyzer.Analyze(context.Background(), doc, phase3AnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			if _, ok := byName[tc.want]; !ok {
				t.Fatalf("wanted symbol %q missing; symbols=%v diagnostics=%+v", tc.want, sortedSymbolQualifiedNames(result.Analysis.Symbols), result.Analysis.Diagnostics)
			}
			for _, forbidden := range tc.forbidden {
				if symbol, ok := byName[forbidden]; ok {
					t.Fatalf("opaque region leaked %q: %+v", forbidden, symbol)
				}
			}
		})
	}
}

func TestR27Phase10MalformedStructuredDocumentsAreIncomplete(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{name: "json", analyzer: JSONAnalyzer{}, text: "{\"service\": {\"name\": \"api\"}"},
		{name: "toml", analyzer: TOMLAnalyzer{}, text: "[server\nhost = \"localhost\"\n"},
		{name: "yaml-tabs", analyzer: YAMLAnalyzer{}, text: "service:\n\tname: api\n"},
		{name: "graphql", analyzer: GraphQLAnalyzer{}, text: "type Broken { value: String\n"},
		{name: "terraform", analyzer: TerraformAnalyzer{}, text: "resource \"x\" \"broken\" {\n"},
		{name: "proto", analyzer: ProtoAnalyzer{}, text: "message Broken { string value = 1;\n"},
		{name: "vhdl", analyzer: VHDLAnalyzer{}, text: "entity Broken is\n  port (clk : in bit);\n"},
		{name: "verilog", analyzer: VerilogAnalyzer{}, text: "module broken; wire x;\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			doc := sourceDocumentForScanner(tc.text)
			result, err := tc.analyzer.Analyze(context.Background(), doc, phase3AnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("malformed %s overclaimed complete coverage: %+v", tc.name, result.Analysis)
			}
		})
	}
}
