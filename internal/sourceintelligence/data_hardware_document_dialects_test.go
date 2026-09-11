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
			result, err := (SQLAnalyzer{}).Analyze(context.Background(), testSourceDocument("dialect.sql", tc.text), testAnalyzeOptions(true, 64))
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

func TestSQLQuotedDDLHierarchyAndReferenceDependencies(t *testing.T) {
	text := "CREATE SCHEMA IF NOT EXISTS `lemans24` DEFAULT CHARACTER SET utf8mb4;\n" +
		"CREATE TABLE IF NOT EXISTS `lemans24`.`drivers` (`id` INT PRIMARY KEY);\n" +
		"CREATE TABLE IF NOT EXISTS `lemans24`.`results` (`driver_id` INT, CONSTRAINT `fk_driver` FOREIGN KEY (`driver_id`) REFERENCES `lemans24`.`drivers` (`id`));\n" +
		"CREATE TABLE [reporting].[runs] ([job_id] INT REFERENCES [reporting].[jobs]([id]));\n" +
		"CREATE TABLE \"odd.name\" (id INT);\n" +
		"GRANT CREATE TABLE TO chinook;\n" +
		"REVOKE CREATE VIEW FROM chinook;\n" +
		"-- REFERENCES ignored_table(id)\n" +
		"SELECT 'REFERENCES ignored_string(id)';\n"
	result, err := (SQLAnalyzer{}).Analyze(context.Background(), testSourceDocument("schema.sql", text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	schema, ok := byName["lemans24"]
	if !ok || schema.Kind != SymbolKindSchema {
		t.Fatalf("SQL schema = %+v exists=%v; symbols=%v", schema, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	for _, name := range []string{"lemans24.drivers", "lemans24.results"} {
		symbol, exists := byName[name]
		if !exists || symbol.Kind != SymbolKindType || symbol.ParentID != schema.ID || symbol.ParentQualifiedName != schema.QualifiedName {
			t.Fatalf("SQL child %s = %+v exists=%v; schema=%+v symbols=%v", name, symbol, exists, schema, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if _, ok := byName["reporting.runs"]; !ok {
		t.Fatalf("SQL bracket-qualified table missing; symbols=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if _, ok := byName[`"odd.name"`]; !ok {
		t.Fatalf("SQL quoted identifier containing a dot was split; symbols=%v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	dependencies := make(map[string]StructuralDependencyKind, len(result.Dependencies))
	for _, dependency := range result.Dependencies {
		dependencies[dependency.Value] = dependency.Kind
	}
	for _, value := range []string{"lemans24.drivers", "reporting.jobs"} {
		if kind := dependencies[value]; string(kind) != "reference" {
			t.Fatalf("SQL dependency %q kind=%q; all=%+v", value, kind, result.Dependencies)
		}
	}
	for _, forbidden := range []string{"ignored_table", "ignored_string"} {
		if _, ok := dependencies[forbidden]; ok {
			t.Fatalf("opaque SQL text produced dependency %q: %+v", forbidden, result.Dependencies)
		}
	}
	for _, forbidden := range []string{"TO", "FROM"} {
		if _, ok := byName[forbidden]; ok {
			t.Fatalf("SQL DCL phrase produced phantom declaration %q: %v", forbidden, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
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
	result, err := (TerraformAnalyzer{}).Analyze(context.Background(), testSourceDocument("service.hcl", "service \"api\" { port = 8080 }\nprovider \"aws\" {}\nlocals { answer = 42 }\n"), testAnalyzeOptions(true, 64))
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

func TestTerraformStructuralBlocksExposeHierarchy(t *testing.T) {
	text := `resource "aws_instance" "web" {
  lifecycle {
    create_before_destroy = true
  }
  provisioner "local-exec" {
    command = "echo ready"
  }
  tags = {
    lifecycle = "metadata"
  }
}
service "api" {
  route "health" {
    path = "/health"
  }
  settings = {
    route = "metadata"
  }
}
`
	result, err := (TerraformAnalyzer{}).Analyze(context.Background(), testSourceDocument("hierarchy.tf", text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, tc := range []struct {
		name       string
		parentName string
		kind       SymbolKind
	}{
		{name: "aws_instance.web.lifecycle", parentName: "aws_instance.web", kind: SymbolKindSection},
		{name: "aws_instance.web.provisioner.local-exec", parentName: "aws_instance.web", kind: SymbolKindSection},
		{name: "service.api.route.health", parentName: "service.api", kind: SymbolKindSection},
	} {
		symbol, ok := byName[tc.name]
		if !ok || symbol.Kind != tc.kind {
			t.Fatalf("Terraform nested block %s = %+v exists=%v; symbols=%v", tc.name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		parent, ok := byName[tc.parentName]
		if !ok || symbol.ParentID != parent.ID || symbol.ParentQualifiedName != parent.QualifiedName {
			t.Fatalf("Terraform nested block %s parent = %q/%q, want %q/%q", tc.name, symbol.ParentID, symbol.ParentQualifiedName, parent.ID, parent.QualifiedName)
		}
	}
	for _, forbidden := range []string{"aws_instance.web.tags", "aws_instance.web.tags.lifecycle", "service.api.settings", "service.api.settings.route"} {
		if _, ok := byName[forbidden]; ok {
			t.Fatalf("Terraform object expression unexpectedly became a structural block %s; symbols=%v", forbidden, sortedSymbolQualifiedNames(result.Analysis.Symbols))
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
			result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), testSourceDocument("fixture.asm", tc.text), testAnalyzeOptions(true, 64))
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
