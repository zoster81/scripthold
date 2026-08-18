package sourceintelligence

import (
	"context"
	"testing"
)

func TestSQLDialectProfiles(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "postgresql-materialized-view",
			text: "CREATE MATERIALIZED VIEW reporting.active_users AS SELECT 1;\nCREATE OR REPLACE FUNCTION reporting.bump(x integer) RETURNS integer AS 'select x + 1';\n",
			want: []string{"reporting.active_users", "reporting.bump"},
		},
		{
			name: "sqlserver-create-or-alter",
			text: "CREATE OR ALTER PROCEDURE dbo.RunReport AS SELECT 1;\nCREATE OR ALTER VIEW dbo.CurrentReport AS SELECT 1 AS value;\n",
			want: []string{"dbo.RunReport", "dbo.CurrentReport"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (SQLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("dialect.sql", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("SQL dialect %s missing %s: %v", tc.name, want, names)
				}
			}
		})
	}
}

func TestGenericHCLUsesTerraformProviderWithoutInventingTerraformSemantics(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	detection, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "service.hcl", Text: "service \"api\" { port = 8080 }\n"})
	if err != nil {
		t.Fatal(err)
	}
	if detection.State != DetectionProbable || detection.Language != "terraform" {
		t.Fatalf("generic HCL detection = %+v, want terraform canonical provider", detection)
	}
	result, err := (TerraformAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("service.hcl", "service \"api\" { port = 8080 }\nprovider \"aws\" {}\nlocals { answer = 42 }\n"), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"service.api", "provider.aws", "locals"} {
		if symbol, ok := byName[name]; !ok || symbol.Kind != SymbolKindSection {
			t.Fatalf("generic HCL section %s = %+v exists=%v; symbols=%v", name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestAssemblyDialectLabels(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{name: "gas", text: ".globl _start\n_start:\n.Lretry:\n1:\n  nop\n", want: []string{"_start", ".Lretry", "1"}},
		{name: "nasm", text: "global _start\nsection .text\n_start:\n.loop:\n  nop\n", want: []string{"_start", ".loop"}},
		{name: "masm", text: ".code\nMain PROC\n  ret\nMain ENDP\n", want: []string{"Main"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixture.asm", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("assembly dialect %s missing %s: %v", tc.name, want, names)
				}
			}
		})
	}
}

func TestDistinctiveMarkersIgnoreOpaqueText(t *testing.T) {
	registry, err := NewLanguageRegistry(defaultLanguageDescriptors())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		path      string
		text      string
		want      string
		forbidden string
	}{
		{name: "java-comment-systemverilog", path: "Demo.java", text: "/* interface fake; logic x; endinterface */\nclass Demo {}\n", want: "java", forbidden: "systemverilog"},
		{name: "java-string-graphql", path: "Demo.java", text: "class Demo { String s = \"schema { query: Query }\"; }\n", want: "java", forbidden: "graphql"},
		{name: "cpp-comment-verilog", path: "demo.cpp", text: "/*\nmodule fake;\nendmodule\n*/\nclass Demo {};\n", forbidden: "verilog"},
		{name: "sql-string-plsql", path: "demo.sql", text: "SELECT 'CREATE OR REPLACE PACKAGE fake AS';\nCREATE TABLE real_table(id int);\n", want: "sql", forbidden: "plsql"},
		{name: "yaml-block-scalar-openapi", path: "config.yaml", text: "description: |\n  openapi: 3.1.0\nservice: demo\n", want: "yaml", forbidden: "openapi"},
		{name: "yaml-nested-ansible-example", path: "config.yml", text: "example:\n  - name: Demo\n    hosts: web\n    tasks:\n      - name: Ping\n", want: "yaml", forbidden: "ansible-yaml"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
			if err != nil {
				t.Fatal(err)
			}
			if tc.want != "" && (result.State != DetectionProbable || result.Language != tc.want) {
				t.Fatalf("opaque distinctive marker detection = %+v, want probable %s", result, tc.want)
			}
			if result.Language == tc.forbidden {
				t.Fatalf("opaque distinctive marker selected forbidden language %s: %+v", tc.forbidden, result)
			}
			for _, candidate := range result.Candidates {
				if candidate.Language == tc.forbidden {
					t.Fatalf("opaque distinctive marker leaked forbidden candidate %s: %+v", tc.forbidden, result)
				}
			}
		})
	}
}
