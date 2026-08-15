package sourceintelligence

import (
	"context"
	"reflect"
	"testing"
)

func TestR27Phase10NormalizedKindsAreAccepted(t *testing.T) {
	for _, kind := range []SymbolKind{
		SymbolKindSchema,
		SymbolKindOperation,
		SymbolKindSignal,
		SymbolKindEntity,
		SymbolKindLabel,
		SymbolKindSelector,
		SymbolKindSection,
		SymbolKindKey,
		SymbolKindResource,
	} {
		if _, ok := normalizedSymbolKinds[kind]; !ok {
			t.Fatalf("Phase 10 normalized kind %q is not registered", kind)
		}
	}
}

func TestR27Phase10ProviderIdentityAndStructuralNavigation(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     map[string]SymbolKind
		deps     []string
	}{
		{
			language: "sql", analyzer: SQLAnalyzer{},
			text: "CREATE SCHEMA app;\nCREATE TABLE app.users (id INTEGER, name TEXT);\nCREATE VIEW app.active_users AS SELECT * FROM app.users;\nCREATE FUNCTION app.bump(x INTEGER) RETURNS INTEGER AS 'x + 1';\n",
			want: map[string]SymbolKind{"app": SymbolKindSchema, "app.users": SymbolKindType, "app.active_users": SymbolKindType, "app.bump": SymbolKindFunction},
		},
		{
			language: "plsql", analyzer: PLSQLAnalyzer{},
			text: "CREATE OR REPLACE PACKAGE demo AS\n  PROCEDURE run(value NUMBER);\n  FUNCTION calc(value NUMBER) RETURN NUMBER;\nEND demo;\n/\n",
			want: map[string]SymbolKind{"demo": SymbolKindPackage, "demo.run": SymbolKindFunction, "demo.calc": SymbolKindFunction},
		},
		{
			language: "graphql", analyzer: GraphQLAnalyzer{},
			text: "schema { query: Query }\ntype Query { user(id: ID!): User }\ntype User { id: ID! name: String! }\nquery GetUser($id: ID!) { user(id: $id) { id } }\n",
			want: map[string]SymbolKind{"schema": SymbolKindSchema, "Query": SymbolKindType, "Query.user": SymbolKindField, "User": SymbolKindType, "User.id": SymbolKindField, "User.name": SymbolKindField, "GetUser": SymbolKindOperation},
		},
		{
			language: "terraform", analyzer: TerraformAnalyzer{},
			text: "terraform { required_version = \">= 1.6\" }\nvariable \"region\" { type = string }\nresource \"aws_s3_bucket\" \"assets\" { bucket = \"demo\" }\nmodule \"network\" { source = \"./network\" }\noutput \"bucket_id\" { value = aws_s3_bucket.assets.id }\n",
			want: map[string]SymbolKind{"region": SymbolKindVariable, "aws_s3_bucket.assets": SymbolKindResource, "network": SymbolKindModule, "bucket_id": SymbolKindVariable},
			deps: []string{"./network"},
		},
		{
			language: "nix", analyzer: NixAnalyzer{},
			text: "{ pkgs ? import <nixpkgs> {} }:\nlet\n  answer = 42;\n  package = pkgs.stdenv.mkDerivation { pname = \"demo\"; };\nin { inherit answer package; }\n",
			want: map[string]SymbolKind{"answer": SymbolKindVariable, "package": SymbolKindVariable},
		},
		{
			language: "proto", analyzer: ProtoAnalyzer{},
			text: "syntax = \"proto3\";\npackage demo.v1;\nimport \"common.proto\";\nmessage User { string id = 1; }\nenum State { STATE_UNSPECIFIED = 0; }\nservice Users { rpc Get(User) returns (User); }\n",
			want: map[string]SymbolKind{"demo.v1": SymbolKindPackage, "demo.v1.User": SymbolKindType, "demo.v1.User.id": SymbolKindField, "demo.v1.State": SymbolKindEnum, "demo.v1.Users": SymbolKindInterface, "demo.v1.Users.Get": SymbolKindOperation},
			deps: []string{"common.proto"},
		},
		{
			language: "vhdl", analyzer: VHDLAnalyzer{},
			text: "library ieee;\nuse ieee.std_logic_1164.all;\nentity Counter is\n  port ( clk : in std_logic );\nend Counter;\narchitecture rtl of Counter is\n  signal count : integer;\nbegin\nend rtl;\n",
			want: map[string]SymbolKind{"Counter": SymbolKindEntity, "Counter.rtl": SymbolKindImplementation, "Counter.rtl.count": SymbolKindSignal},
			deps: []string{"ieee.std_logic_1164.all"},
		},
		{
			language: "verilog", analyzer: VerilogAnalyzer{},
			text: "module counter(input wire clk, output wire ready);\n  wire internal_ready;\n  reg [7:0] count;\nendmodule\n",
			want: map[string]SymbolKind{"counter": SymbolKindModule, "counter.internal_ready": SymbolKindSignal, "counter.count": SymbolKindSignal},
		},
		{
			language: "systemverilog", analyzer: SystemVerilogAnalyzer{},
			text: "package demo_pkg; typedef struct { logic valid; } payload_t; endpackage\ninterface bus_if; logic ready; endinterface\nmodule top; logic active; endmodule\n",
			want: map[string]SymbolKind{"demo_pkg": SymbolKindPackage, "demo_pkg.payload_t": SymbolKindType, "bus_if": SymbolKindInterface, "bus_if.ready": SymbolKindSignal, "top": SymbolKindModule, "top.active": SymbolKindSignal},
		},
		{
			language: "assembly", analyzer: AssemblyAnalyzer{},
			text: ".text\n.globl main\nmain:\n.loop:\n  nop\n  ret\n",
			want: map[string]SymbolKind{"main": SymbolKindLabel, ".loop": SymbolKindLabel},
		},
		{
			language: "html", analyzer: HTMLAnalyzer{},
			text: "<!doctype html><html><body><section id=\"hero\"><div id=\"card\"></div></section></body></html>",
			want: map[string]SymbolKind{"hero": SymbolKindEntity, "card": SymbolKindEntity},
		},
		{
			language: "xml", analyzer: XMLAnalyzer{},
			text: "<?xml version=\"1.0\"?><catalog><item id=\"first\"><name>Demo</name></item></catalog>",
			want: map[string]SymbolKind{"first": SymbolKindEntity},
		},
		{
			language: "css", analyzer: CSSAnalyzer{},
			text: ".card, #hero { display: block; }\n@media screen { .nested { color: red; } }\n",
			want: map[string]SymbolKind{".card": SymbolKindSelector, "#hero": SymbolKindSelector, ".nested": SymbolKindSelector},
		},
		{
			language: "scss", analyzer: SCSSAnalyzer{},
			text: "$gap: 1rem;\n.card { &__title { color: red; } }\n",
			want: map[string]SymbolKind{"gap": SymbolKindVariable, ".card": SymbolKindSelector, "&__title": SymbolKindSelector},
		},
		{
			language: "sass", analyzer: SassAnalyzer{},
			text: "$gap: 1rem\n.card\n  color: red\n  &__title\n    font-weight: bold\n",
			want: map[string]SymbolKind{"gap": SymbolKindVariable, ".card": SymbolKindSelector, "&__title": SymbolKindSelector},
		},
		{
			language: "less", analyzer: LessAnalyzer{},
			text: "@gap: 1rem;\n.card { .title { margin: @gap; } }\n",
			want: map[string]SymbolKind{"gap": SymbolKindVariable, ".card": SymbolKindSelector, ".title": SymbolKindSelector},
		},
		{
			language: "json", analyzer: JSONAnalyzer{},
			text: "{\"service\":{\"name\":\"api\",\"port\":8080},\"enabled\":true}",
			want: map[string]SymbolKind{"service": SymbolKindKey, "service.name": SymbolKindKey, "service.port": SymbolKindKey, "enabled": SymbolKindKey},
		},
		{
			language: "yaml", analyzer: YAMLAnalyzer{},
			text: "service:\n  name: api\n  port: 8080\nenabled: true\n",
			want: map[string]SymbolKind{"service": SymbolKindKey, "service.name": SymbolKindKey, "service.port": SymbolKindKey, "enabled": SymbolKindKey},
		},
		{
			language: "toml", analyzer: TOMLAnalyzer{},
			text: "title = \"demo\"\n[server]\nhost = \"localhost\"\n[server.tls]\nenabled = true\n",
			want: map[string]SymbolKind{"title": SymbolKindKey, "server": SymbolKindSection, "server.host": SymbolKindKey, "server.tls": SymbolKindSection, "server.tls.enabled": SymbolKindKey},
		},
		{
			language: "markdown", analyzer: MarkdownAnalyzer{},
			text: "# Project\n\n## Usage\nText\n\n### Examples\nMore\n",
			want: map[string]SymbolKind{"Project": SymbolKindSection, "Project.Usage": SymbolKindSection, "Project.Usage.Examples": SymbolKindSection},
		},
		{
			language: "openapi", analyzer: OpenAPIAnalyzer{},
			text: "openapi: 3.1.0\ninfo:\n  title: Demo\n  version: 1.0.0\npaths:\n  /users:\n    get:\n      operationId: listUsers\ncomponents:\n  schemas:\n    User:\n      type: object\n",
			want: map[string]SymbolKind{"listUsers": SymbolKindOperation, "User": SymbolKindType},
		},
		{
			language: "ansible-yaml", analyzer: AnsibleYAMLAnalyzer{},
			text: "- name: Configure web\n  hosts: web\n  tasks:\n    - name: Install nginx\n      ansible.builtin.package:\n        name: nginx\n        state: present\n    - name: Start nginx\n      ansible.builtin.service:\n        name: nginx\n        state: started\n",
			want: map[string]SymbolKind{"Configure web": SymbolKindSection, "Configure web.Install nginx": SymbolKindOperation, "Configure web.Start nginx": SymbolKindOperation},
		},
	}

	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			document := sourceDocumentForScanner(tc.text)
			document.Path = "fixture"
			result, err := tc.analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 0 {
				t.Fatalf("%s analysis unexpectedly partial: %+v", tc.language, result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for qualified, kind := range tc.want {
				if symbol, ok := byName[qualified]; !ok || symbol.Kind != kind || symbol.Language != tc.language || symbol.Analyzer != string(tc.analyzer.ID()) {
					t.Fatalf("%s %s = %+v exists=%v; symbols=%v", tc.language, qualified, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			gotDeps := dependencyValues(result.Dependencies)
			if len(gotDeps) == 0 && len(tc.deps) == 0 {
				return
			}
			if !reflect.DeepEqual(gotDeps, tc.deps) {
				t.Fatalf("%s dependencies=%v want %v", tc.language, gotDeps, tc.deps)
			}
		})
	}
}
