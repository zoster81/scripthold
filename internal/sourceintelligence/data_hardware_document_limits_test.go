package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestDataHardwareDocumentCancellationAndSymbolLimits(t *testing.T) {
	for _, analyzer := range dataHardwareDocumentAnalyzers() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := analyzer.Analyze(ctx, scientificLegacyFunctionalTestDocument("cancel.fixture", "placeholder\n"), testAnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}

	for _, tc := range []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"sql", SQLAnalyzer{}, generatedSQL(1200)},
		{"plsql", PLSQLAnalyzer{}, generatedPLSQL(1200)},
		{"graphql", GraphQLAnalyzer{}, generatedGraphQL(1200)},
		{"terraform", TerraformAnalyzer{}, generatedTerraform(1200)},
		{"nix", NixAnalyzer{}, generatedNix(1200)},
		{"proto", ProtoAnalyzer{}, generatedProto(1200)},
		{"vhdl", VHDLAnalyzer{}, generatedVHDL(1200)},
		{"verilog", VerilogAnalyzer{}, generatedVerilog(1200)},
		{"systemverilog", SystemVerilogAnalyzer{}, generatedSystemVerilog(1200)},
		{"assembly", AssemblyAnalyzer{}, generatedAssembly(1200)},
		{"html", HTMLAnalyzer{}, generatedHTML(1200)},
		{"xml", XMLAnalyzer{}, generatedXML(1200)},
		{"css", CSSAnalyzer{}, generatedCSS(1200)},
		{"scss", SCSSAnalyzer{}, generatedSCSS(1200)},
		{"sass", SassAnalyzer{}, generatedSass(1200)},
		{"less", LessAnalyzer{}, generatedLess(1200)},
		{"json", JSONAnalyzer{}, generatedJSON(1200)},
		{"yaml", YAMLAnalyzer{}, generatedYAML(1200)},
		{"toml", TOMLAnalyzer{}, generatedTOML(1200)},
		{"markdown", MarkdownAnalyzer{}, generatedMarkdown(1200)},
		{"openapi", OpenAPIAnalyzer{}, generatedOpenAPI(1200)},
		{"ansible-yaml", AnsibleYAMLAnalyzer{}, generatedAnsible(1200)},
	} {
		t.Run("limit-"+tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("limit.fixture", tc.text), testAnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result symbols=%d truncated=%v complete=%v diagnostics=%+v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
		})
	}
}

func dataHardwareDocumentAnalyzers() []SourceAnalyzer {
	return []SourceAnalyzer{
		SQLAnalyzer{}, PLSQLAnalyzer{}, GraphQLAnalyzer{}, TerraformAnalyzer{}, NixAnalyzer{}, ProtoAnalyzer{},
		VHDLAnalyzer{}, VerilogAnalyzer{}, SystemVerilogAnalyzer{}, AssemblyAnalyzer{},
		HTMLAnalyzer{}, XMLAnalyzer{}, CSSAnalyzer{}, SCSSAnalyzer{}, SassAnalyzer{}, LessAnalyzer{},
		JSONAnalyzer{}, YAMLAnalyzer{}, TOMLAnalyzer{}, MarkdownAnalyzer{}, OpenAPIAnalyzer{}, AnsibleYAMLAnalyzer{},
	}
}

func generatedSQL(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "CREATE TABLE t%04d(id INTEGER);\n", i)
	}
	return b.String()
}

func generatedPLSQL(count int) string {
	var b strings.Builder
	b.WriteString("CREATE OR REPLACE PACKAGE demo AS\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "  PROCEDURE p%04d;\n", i)
	}
	b.WriteString("END demo;\n/\n")
	return b.String()
}

func generatedGraphQL(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "type T%04d { value: String }\n", i)
	}
	return b.String()
}

func generatedTerraform(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "variable \"v%04d\" {}\n", i)
	}
	return b.String()
}

func generatedNix(count int) string {
	var b strings.Builder
	b.WriteString("let\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "  v%04d = %d;\n", i, i)
	}
	b.WriteString("in {}\n")
	return b.String()
}

func generatedProto(count int) string {
	var b strings.Builder
	b.WriteString("syntax = \"proto3\";\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "message M%04d { string value = 1; }\n", i)
	}
	return b.String()
}

func generatedVHDL(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "entity E%04d is\nend E%04d;\n", i, i)
	}
	return b.String()
}

func generatedVerilog(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "module m%04d; endmodule\n", i)
	}
	return b.String()
}

func generatedSystemVerilog(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "interface i%04d; endinterface\n", i)
	}
	return b.String()
}

func generatedAssembly(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "label_%04d:\n  nop\n", i)
	}
	return b.String()
}

func generatedHTML(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "<div id=\"id%04d\"></div>\n", i)
	}
	return b.String()
}

func generatedXML(count int) string {
	var b strings.Builder
	b.WriteString("<root>\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "<item id=\"id%04d\"/>\n", i)
	}
	b.WriteString("</root>\n")
	return b.String()
}

func generatedCSS(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, ".s%04d { display: block; }\n", i)
	}
	return b.String()
}

func generatedSCSS(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "$v%04d: %dpx;\n", i, i)
	}
	return b.String()
}

func generatedSass(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "$v%04d: %dpx\n", i, i)
	}
	return b.String()
}

func generatedLess(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "@v%04d: %dpx;\n", i, i)
	}
	return b.String()
}

func generatedJSON(count int) string {
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

func generatedYAML(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "k%04d: %d\n", i, i)
	}
	return b.String()
}

func generatedTOML(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "k%04d = %d\n", i, i)
	}
	return b.String()
}

func generatedMarkdown(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "# Heading %04d\n", i)
	}
	return b.String()
}

func generatedOpenAPI(count int) string {
	var b strings.Builder
	b.WriteString("openapi: 3.1.0\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "operationId: op%04d\n", i)
	}
	return b.String()
}

func generatedAnsible(count int) string {
	var b strings.Builder
	b.WriteString("- name: Play\n  hosts: all\n  tasks:\n")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "    - name: Task %04d\n      debug:\n        msg: ok\n", i)
	}
	return b.String()
}
