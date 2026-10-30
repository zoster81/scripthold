package sourceintelligence

import (
	"context"
	"strings"
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

// Unsupported table headers must not become proven declarations. The scanner
// still reports partial coverage when SQL dialect syntax is not recognized.
func TestSQLUnsupportedTableHeaderDoesNotInventDeclarations(t *testing.T) {
	text := "CREATE TABLE broken COLUMNS (id INT);\n" +
		"CREATE TABLE valid (id INTEGER);\n" +
		"CREATE TABLE copied AS SELECT id FROM valid;\n" +
		"CREATE TABLE cloned LIKE valid;\n" +
		"CREATE TABLE no_body;\n" +
		"CREATE VIEW active AS SELECT id FROM valid;\n"
	result, err := (SQLAnalyzer{}).Analyze(context.Background(), testSourceDocument("unsupported.sql", text), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"valid", "copied", "cloned", "active"} {
		if _, ok := symbols[name]; !ok {
			t.Errorf("valid declaration %q missing: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, name := range []string{"broken", "no_body"} {
		if _, ok := symbols[name]; ok {
			t.Errorf("unsupported SQL table header invented declaration %q", name)
		}
	}
	if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) != 2 {
		t.Fatalf("unsupported headers require two explicit incomplete-coverage diagnostics: %+v", result.Analysis)
	}
	for _, diagnostic := range result.Analysis.Diagnostics {
		if diagnostic.Code != "sql-unrecognized-table-header" {
			t.Errorf("unexpected SQL header diagnostic: %+v", diagnostic)
		}
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

func TestAssemblyNamedDataExcludesStructureAndMacroFields(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		want      map[string]int
		forbidden []string
	}{
		{
			name: "masm-fields-and-macro",
			text: ".data\nMsg DB 'hello'\nWORD_BUFFER DW 512 dup (?)\n" +
				"VIOinfo struc\nlevel DB ?\nflags DW ?\nVIOinfo ends\n" +
				"after_struct DB 8\nmymacro macro args\ninner DB 9\nendm\n" +
				"after_macro DW 10\n",
			want:      map[string]int{"Msg": 2, "WORD_BUFFER": 3, "after_struct": 8, "after_macro": 12},
			forbidden: []string{"level", "flags", "inner", "mymacro", "VIOinfo"},
		},
		{
			name: "nasm-and-negative-syntax",
			text: "section .data\ngreeting db 'hello'\ncount dw 4\n" +
				"; Ghost DB 1\ninvalid db ; no operand\nlabel: db 1\n" +
				"struc Packet\nfield db 1\nendstruc\n" +
				"%macro make 0\nmacro_value db 3\n%endmacro\n" +
				"tail dw 2\n",
			want:      map[string]int{"greeting": 2, "count": 3, "tail": 13},
			forbidden: []string{"Ghost", "invalid", "field", "macro_value"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), testSourceDocument("data.asm", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, line := range tc.want {
				symbol, ok := got[name]
				if !ok || symbol.Kind != SymbolKindVariable || symbol.DeclarationRange.Start.Line != line {
					t.Errorf("Assembly data variable %q: got=%+v present=%v, want variable at line %d", name, symbol, ok, line)
				}
			}
			for _, name := range tc.forbidden {
				if _, ok := got[name]; ok {
					t.Errorf("Assembly macro/structure/invalid data became a variable: %q", name)
				}
			}
		})
	}
}

func TestAssemblyMotorolaFCBAndFCCNamedData(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		want      map[string]int
		forbidden []string
		complete  bool
	}{
		{
			name: "68hc11-declarations-and-invalid-operands",
			text: "INTERRUPTS:\nTRAPP FCB CR,LF\n  FCB CR,LF\n  FCC 'unlabelled'\n" +
				"PROMPT fcb 0\nBANNER FCC \"HELLO\"\nSLASH FCC /a;b/ ; real character data\n" +
				"COLD LDS #STACK\n; Ghost FCB 2\ninvalidByte FCB ; no value\n" +
				"invalidStart FCB ,7\ninvalidText FCC unquoted\nemptyText FCC ; no string\n" +
				"namedLabel: FCB 9\n",
			want:      map[string]int{"TRAPP": 2, "PROMPT": 5, "BANNER": 6, "SLASH": 7},
			forbidden: []string{"COLD", "Ghost", "invalidByte", "invalidStart", "invalidText", "emptyText"},
			complete:  true,
		},
		{
			name: "macro-and-structure-scopes",
			text: "before FCB 1\nGenerator MACRO\nmacroByte FCB 2\nmacroText FCC 'ghost'\nENDM\n" +
				"Packet STRUCT\nstructByte FCB 3\nstructText FCC 'hidden'\nPacket ENDS\n" +
				"after FCC 'done'\n",
			want:      map[string]int{"before": 1, "after": 10},
			forbidden: []string{"macroByte", "macroText", "structByte", "structText"},
			complete:  true,
		},
		{
			name:      "unterminated-macro",
			text:      "before FCB 1\nGenerator MACRO\nmacroData FCB 2\nmacroText FCC 'hidden'\n",
			want:      map[string]int{"before": 1},
			forbidden: []string{"macroData", "macroText"},
			complete:  false,
		},
	}
	for _, tc := range tests {
		for _, eol := range []struct {
			name, value string
		}{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				source := strings.ReplaceAll(tc.text, "\n", eol.value)
				result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), testSourceDocument("motorola.asm", source), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				byName := symbolsByQualifiedName(result.Analysis.Symbols)
				for name, line := range tc.want {
					sym, ok := byName[name]
					if !ok || sym.Kind != SymbolKindVariable || sym.DeclarationRange.Start.Line != line {
						t.Errorf("named Motorola data %q: got=%+v exists=%v, want variable line %d", name, sym, ok, line)
					}
				}
				for _, name := range tc.forbidden {
					if sym, ok := byName[name]; ok {
						t.Errorf("invalid, instruction or unexpanded data exported: %+v", sym)
					}
				}
				if symbol, ok := byName["namedLabel"]; tc.name == "68hc11-declarations-and-invalid-operands" && (!ok || symbol.Kind != SymbolKindLabel) {
					t.Errorf("colon-terminated Motorola label changed kind: %+v, exists=%v", symbol, ok)
				}
				if result.Analysis.CoverageComplete != tc.complete {
					t.Errorf("coverageComplete=%v, want %v, diagnostics=%+v", result.Analysis.CoverageComplete, tc.complete, result.Analysis.Diagnostics)
				}
			})
		}
	}
}

func TestAssemblyLabelsInsideMacroAndStructureAreNotGlobals(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		want      map[string]int
		forbidden []string
		complete  bool
	}{
		{
			name: "masm",
			text: "entry:\nMake MACRO\ninside_label:\nInner PROC\nInner ENDP\nENDM\n" +
				"Packet STRUCT\nfield_label:\nPacket ENDS\nexit_label:\nExport PROC\nExport ENDP\n",
			want:      map[string]int{"entry": 1, "exit_label": 10, "Export": 11},
			forbidden: []string{"inside_label", "Inner", "field_label"},
			complete:  true,
		},
		{
			name:      "nasm",
			text:      "start:\n%macro define 0\nmacro_label:\n%endmacro\nstruc Record\nstructure_label:\nendstruc\ndone:\n",
			want:      map[string]int{"start": 1, "done": 8},
			forbidden: []string{"macro_label", "structure_label"},
			complete:  true,
		},
		{
			name:      "unterminated-masm-macro",
			text:      "before:\nGen MACRO\nshould_not_escape:\nGhost PROC\n",
			want:      map[string]int{"before": 1},
			forbidden: []string{"should_not_escape", "Ghost"},
			complete:  false,
		},
	}
	for _, tc := range tests {
		for _, eol := range []struct {
			name  string
			value string
		}{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				source := strings.ReplaceAll(tc.text, "\n", eol.value)
				result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), testSourceDocument("scopes.asm", source), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				got := symbolsByQualifiedName(result.Analysis.Symbols)
				for name, line := range tc.want {
					symbol, ok := got[name]
					if !ok || symbol.Kind != SymbolKindLabel || symbol.DeclarationRange.Start.Line != line {
						t.Errorf("global Assembly label/procedure %q: got=%+v present=%v, want label at line %d", name, symbol, ok, line)
					}
				}
				for _, name := range tc.forbidden {
					if symbol, ok := got[name]; ok {
						t.Errorf("unexpanded macro/structure exported a global: %+v", symbol)
					}
				}
				if result.Analysis.CoverageComplete != tc.complete {
					t.Errorf("coverage complete=%v, want %v; diagnostics=%+v", result.Analysis.CoverageComplete, tc.complete, result.Analysis.Diagnostics)
				}
			})
		}
	}
}

func TestAssemblyEQUInsideMacroDefinitionIsNotGlobal(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		expected  string
		forbidden string
	}{
		{
			name:      "masm",
			text:      "makeConst MACRO\ninner EQU 2\nENDM\noutside EQU 3\n",
			expected:  "outside",
			forbidden: "inner",
		},
		{
			name:      "nasm",
			text:      "%macro makeConst 0\nlocalValue EQU 4\n%endmacro\npublicValue EQU 5\n",
			expected:  "publicValue",
			forbidden: "localValue",
		},
	}
	for _, tc := range tests {
		for _, eol := range []struct {
			name  string
			value string
		}{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"-"+eol.name, func(t *testing.T) {
				source := strings.ReplaceAll(tc.text, "\n", eol.value)
				result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), testSourceDocument("macro.asm", source), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				if symbol, exists := symbols[tc.expected]; !exists || symbol.Kind != SymbolKindConstant {
					t.Errorf("real constant %q missing: %+v", tc.expected, symbol)
				}
				if symbol, exists := symbols[tc.forbidden]; exists {
					t.Errorf("unexpanded macro body became global constant: %+v", symbol)
				}
				if !result.Analysis.CoverageComplete {
					t.Errorf("closed macro has partial coverage: %+v", result.Analysis.Diagnostics)
				}
			})
		}
	}
}

func TestAssemblyEQUConstantsAreNotLabelsOrInstructions(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		want   map[string]int
		labels []string
	}{
		{
			name:   "masm-equ",
			text:   ".data\nMsg DB 'hello'\nmyequ EQU 3\nMain PROC\n  ret\nMain ENDP\n",
			want:   map[string]int{"myequ": 3},
			labels: []string{"Main"},
		},
		{
			name:   "68hc11-equ",
			text:   "INTERRUPTS:\nPORTD EQU $1008\nSP EQU $20\nsym1 EQU 1\nsym2: EQU 2\n",
			want:   map[string]int{"PORTD": 2, "SP": 3, "sym1": 4},
			labels: []string{"sym2", "INTERRUPTS"},
		},
		{
			name: "forbidden-pseudo-declarations",
			text: "; Ghost EQU 5\nFake EQU ; missing operand\nmov eax, EQU\nnope EQU\n_valid EQU $10 ; comment\n",
			want: map[string]int{"_valid": 5},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (AssemblyAnalyzer{}).Analyze(context.Background(), testSourceDocument("fixture.asm", tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, line := range tc.want {
				sym, ok := got[name]
				if !ok || sym.Kind != SymbolKindConstant || sym.DeclarationRange.Start.Line != line {
					t.Errorf("Assembly EQU %q: got=%+v present=%v, want constant at line %d", name, sym, ok, line)
				}
			}
			for _, name := range tc.labels {
				if sym, ok := got[name]; !ok || sym.Kind != SymbolKindLabel {
					t.Errorf("Assembly label %q: got=%+v present=%v", name, sym, ok)
				}
			}
			for _, name := range []string{"Ghost", "Fake", "nope", "mov", "EQU", "DB"} {
				if _, ok := got[name]; ok {
					t.Errorf("unsupported or malformed assembly text became symbol %q", name)
				}
			}
		})
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
