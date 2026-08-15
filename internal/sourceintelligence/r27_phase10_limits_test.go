package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase10CancellationAndSymbolLimits(t *testing.T) {
	for _, analyzer := range phase10Analyzers() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := analyzer.Analyze(ctx, phase9TestDocument("cancel.fixture", "placeholder\n"), phase3AnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}

	for _, tc := range []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"sql", SQLAnalyzer{}, generatedPhase10SQL(1200)},
		{"plsql", PLSQLAnalyzer{}, generatedPhase10PLSQL(1200)},
		{"graphql", GraphQLAnalyzer{}, generatedPhase10GraphQL(1200)},
		{"terraform", TerraformAnalyzer{}, generatedPhase10Terraform(1200)},
		{"nix", NixAnalyzer{}, generatedPhase10Nix(1200)},
		{"proto", ProtoAnalyzer{}, generatedPhase10Proto(1200)},
		{"vhdl", VHDLAnalyzer{}, generatedPhase10VHDL(1200)},
		{"verilog", VerilogAnalyzer{}, generatedPhase10Verilog(1200)},
		{"systemverilog", SystemVerilogAnalyzer{}, generatedPhase10SystemVerilog(1200)},
		{"assembly", AssemblyAnalyzer{}, generatedPhase10Assembly(1200)},
		{"html", HTMLAnalyzer{}, generatedPhase10HTML(1200)},
		{"xml", XMLAnalyzer{}, generatedPhase10XML(1200)},
		{"css", CSSAnalyzer{}, generatedPhase10CSS(1200)},
		{"scss", SCSSAnalyzer{}, generatedPhase10SCSS(1200)},
		{"sass", SassAnalyzer{}, generatedPhase10Sass(1200)},
		{"less", LessAnalyzer{}, generatedPhase10Less(1200)},
		{"json", JSONAnalyzer{}, generatedPhase10JSON(1200)},
		{"yaml", YAMLAnalyzer{}, generatedPhase10YAML(1200)},
		{"toml", TOMLAnalyzer{}, generatedPhase10TOML(1200)},
		{"markdown", MarkdownAnalyzer{}, generatedPhase10Markdown(1200)},
		{"openapi", OpenAPIAnalyzer{}, generatedPhase10OpenAPI(1200)},
		{"ansible-yaml", AnsibleYAMLAnalyzer{}, generatedPhase10Ansible(1200)},
	} {
		t.Run("limit-"+tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), phase9TestDocument("limit.fixture", tc.text), phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result symbols=%d truncated=%v complete=%v diagnostics=%+v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
		})
	}
}

func phase10Analyzers() []SourceAnalyzer {
	return []SourceAnalyzer{
		SQLAnalyzer{}, PLSQLAnalyzer{}, GraphQLAnalyzer{}, TerraformAnalyzer{}, NixAnalyzer{}, ProtoAnalyzer{},
		VHDLAnalyzer{}, VerilogAnalyzer{}, SystemVerilogAnalyzer{}, AssemblyAnalyzer{},
		HTMLAnalyzer{}, XMLAnalyzer{}, CSSAnalyzer{}, SCSSAnalyzer{}, SassAnalyzer{}, LessAnalyzer{},
		JSONAnalyzer{}, YAMLAnalyzer{}, TOMLAnalyzer{}, MarkdownAnalyzer{}, OpenAPIAnalyzer{}, AnsibleYAMLAnalyzer{},
	}
}

func generatedPhase10SQL(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "CREATE TABLE t%04d(id INTEGER);\n", i)
	}
	return b.String()
}

func generatedPhase10PLSQL(count int) string {
	var b strings.Builder
	b.WriteString("CREATE OR REPLACE PACKAGE demo AS\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "  PROCEDURE p%04d;\n", i)
	}
	b.WriteString("END demo;\n/\n")
	return b.String()
}

func generatedPhase10GraphQL(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "type T%04d { value: String }\n", i)
	}
	return b.String()
}

func generatedPhase10Terraform(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "variable \"v%04d\" {}\n", i)
	}
	return b.String()
}

func generatedPhase10Nix(count int) string {
	var b strings.Builder
	b.WriteString("let\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "  v%04d = %d;\n", i, i)
	}
	b.WriteString("in {}\n")
	return b.String()
}

func generatedPhase10Proto(count int) string {
	var b strings.Builder
	b.WriteString("syntax = \"proto3\";\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "message M%04d { string value = 1; }\n", i)
	}
	return b.String()
}

func generatedPhase10VHDL(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "entity E%04d is\nend E%04d;\n", i, i)
	}
	return b.String()
}

func generatedPhase10Verilog(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "module m%04d; endmodule\n", i)
	}
	return b.String()
}

func generatedPhase10SystemVerilog(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "interface i%04d; endinterface\n", i)
	}
	return b.String()
}

func generatedPhase10Assembly(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "label_%04d:\n  nop\n", i)
	}
	return b.String()
}

func generatedPhase10HTML(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "<div id=\"id%04d\"></div>\n", i)
	}
	return b.String()
}

func generatedPhase10XML(count int) string {
	var b strings.Builder
	b.WriteString("<root>\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "<item id=\"id%04d\"/>\n", i)
	}
	b.WriteString("</root>\n")
	return b.String()
}

func generatedPhase10CSS(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, ".s%04d { display: block; }\n", i)
	}
	return b.String()
}

func generatedPhase10SCSS(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "$v%04d: %dpx;\n", i, i)
	}
	return b.String()
}

func generatedPhase10Sass(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "$v%04d: %dpx\n", i, i)
	}
	return b.String()
}

func generatedPhase10Less(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "@v%04d: %dpx;\n", i, i)
	}
	return b.String()
}

func generatedPhase10JSON(count int) string {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i < count; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "\"k%04d\":%d", i, i)
	}
	b.WriteString("}\n")
	return b.String()
}

func generatedPhase10YAML(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "k%04d: %d\n", i, i)
	}
	return b.String()
}

func generatedPhase10TOML(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "k%04d = %d\n", i, i)
	}
	return b.String()
}

func generatedPhase10Markdown(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "# Heading %04d\n", i)
	}
	return b.String()
}

func generatedPhase10OpenAPI(count int) string {
	var b strings.Builder
	b.WriteString("openapi: 3.1.0\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "operationId: op%04d\n", i)
	}
	return b.String()
}

func generatedPhase10Ansible(count int) string {
	var b strings.Builder
	b.WriteString("- name: Play\n  hosts: all\n  tasks:\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "    - name: Task %04d\n      debug:\n        msg: ok\n", i)
	}
	return b.String()
}
