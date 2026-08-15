package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestR27Phase10ConformanceAcrossEncodingsAndDeterminism(t *testing.T) {
	tests := []struct {
		name, extension, encoding, text string
		bom                             bool
		analyzer                        SourceAnalyzer
		want                            []string
	}{
		{name: "sql-windows1252-crlf", extension: ".sql", encoding: "windows-1252", analyzer: SQLAnalyzer{}, text: "-- café\r\nCREATE TABLE users(id INTEGER);\r\n", want: []string{"users"}},
		{name: "plsql-utf16le", extension: ".pkb", encoding: "utf-16le", bom: true, analyzer: PLSQLAnalyzer{}, text: "-- résumé\nCREATE OR REPLACE PACKAGE Demo AS\n  PROCEDURE Run;\nEND Demo;\n/\n", want: []string{"Demo", "Demo.Run"}},
		{name: "graphql-utf16be", extension: ".graphql", encoding: "utf-16be", bom: true, analyzer: GraphQLAnalyzer{}, text: "# résumé\nschema { query: Query }\ntype Query { ping: String }\n", want: []string{"schema", "Query", "Query.ping"}},
		{name: "terraform-utf32le", extension: ".tf", encoding: "utf-32le", bom: true, analyzer: TerraformAnalyzer{}, text: "# café\nterraform { required_version = \">= 1.6\" }\nresource \"demo_resource\" \"main\" {}\n", want: []string{"demo_resource.main"}},
		{name: "nix-windows1252", extension: ".nix", encoding: "windows-1252", analyzer: NixAnalyzer{}, text: "# café\nlet\n  answer = 42;\nin { inherit answer; }\n", want: []string{"answer"}},
		{name: "proto-utf16le", extension: ".proto", encoding: "utf-16le", bom: true, analyzer: ProtoAnalyzer{}, text: "// résumé\nsyntax = \"proto3\";\npackage demo;\nmessage Item { string id = 1; }\n", want: []string{"demo", "demo.Item", "demo.Item.id"}},
		{name: "vhdl-windows1252-crlf", extension: ".vhd", encoding: "windows-1252", analyzer: VHDLAnalyzer{}, text: "-- café\r\nentity Counter is\r\nend Counter;\r\narchitecture rtl of Counter is\r\n  signal count : integer;\r\nbegin\r\nend rtl;\r\n", want: []string{"Counter", "Counter.rtl", "Counter.rtl.count"}},
		{name: "verilog-utf16be", extension: ".v", encoding: "utf-16be", bom: true, analyzer: VerilogAnalyzer{}, text: "// résumé\nmodule counter;\n  wire ready;\nendmodule\n", want: []string{"counter", "counter.ready"}},
		{name: "systemverilog-utf32le", extension: ".sv", encoding: "utf-32le", bom: true, analyzer: SystemVerilogAnalyzer{}, text: "// café\ninterface bus_if; logic ready; endinterface\n", want: []string{"bus_if", "bus_if.ready"}},
		{name: "assembly-windows1252", extension: ".asm", encoding: "windows-1252", analyzer: AssemblyAnalyzer{}, text: "; café\nmain:\n  nop\n", want: []string{"main"}},
		{name: "html-utf16le", extension: ".html", encoding: "utf-16le", bom: true, analyzer: HTMLAnalyzer{}, text: "<!-- résumé --><main id=\"hero\">café</main>\n", want: []string{"hero"}},
		{name: "xml-utf16be", extension: ".xml", encoding: "utf-16be", bom: true, analyzer: XMLAnalyzer{}, text: "<?xml version=\"1.0\"?><!-- résumé --><item id=\"root\">café</item>\n", want: []string{"root"}},
		{name: "css-windows1252", extension: ".css", encoding: "windows-1252", analyzer: CSSAnalyzer{}, text: "/* café */\n.card { display: block; }\n", want: []string{".card"}},
		{name: "scss-utf16le", extension: ".scss", encoding: "utf-16le", bom: true, analyzer: SCSSAnalyzer{}, text: "/* résumé */\n$gap: 1rem;\n.card { color: red; }\n", want: []string{"gap", ".card"}},
		{name: "sass-windows1252", extension: ".sass", encoding: "windows-1252", analyzer: SassAnalyzer{}, text: "// café\n$gap: 1rem\n.card\n  color: red\n", want: []string{"gap", ".card"}},
		{name: "less-utf16be", extension: ".less", encoding: "utf-16be", bom: true, analyzer: LessAnalyzer{}, text: "/* résumé */\n@gap: 1rem;\n.card { margin: @gap; }\n", want: []string{"gap", ".card"}},
		{name: "json-utf32le", extension: ".json", encoding: "utf-32le", bom: true, analyzer: JSONAnalyzer{}, text: "{\"service\":{\"name\":\"café\"}}\n", want: []string{"service", "service.name"}},
		{name: "yaml-windows1252-crlf", extension: ".yaml", encoding: "windows-1252", analyzer: YAMLAnalyzer{}, text: "# café\r\nservice:\r\n  name: api\r\n", want: []string{"service", "service.name"}},
		{name: "toml-utf16le", extension: ".toml", encoding: "utf-16le", bom: true, analyzer: TOMLAnalyzer{}, text: "# résumé\n[server]\nhost = \"localhost\"\n", want: []string{"server", "server.host"}},
		{name: "markdown-utf16be", extension: ".md", encoding: "utf-16be", bom: true, analyzer: MarkdownAnalyzer{}, text: "# Project\n\nRésumé\n\n## Usage\n", want: []string{"Project", "Project.Usage"}},
		{name: "openapi-utf32le", extension: ".yaml", encoding: "utf-32le", bom: true, analyzer: OpenAPIAnalyzer{}, text: "openapi: 3.1.0\ninfo:\n  title: Café\n  version: 1.0.0\npaths:\n  /users:\n    get:\n      operationId: listUsers\n", want: []string{"listUsers"}},
		{name: "ansible-windows1252", extension: ".yml", encoding: "windows-1252", analyzer: AnsibleYAMLAnalyzer{}, text: "# café\n- name: Configure web\n  hosts: web\n  tasks:\n    - name: Ping\n      ansible.builtin.ping:\n", want: []string{"Configure web", "Configure web.Ping"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture"+tc.extension)
			if err := os.WriteFile(path, encodeSourceFixture(t, tc.encoding, tc.text, tc.bom), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{
				RequestedEncoding: tc.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000,
			})
			if err != nil {
				t.Fatal(err)
			}
			first, err := tc.analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			second, err := tc.analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("%s output is nondeterministic", tc.analyzer.Language())
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("%s conformance partial: %+v", tc.analyzer.Language(), first.Analysis)
			}
			names := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("%s missing %s; symbols=%v", tc.analyzer.Language(), want, names)
				}
			}
		})
	}
}
