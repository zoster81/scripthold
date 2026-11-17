package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type terraformLocalWant struct {
	name string
	line int
}

func TestTerraformDirectLocalsOriginalSources(t *testing.T) {
	const portable = "locals {\n" +
		"  name = \"service\"\n" +
		"  tags = {\n" +
		"    Example = local.name\n" +
		"  }\n" +
		"  region = \"us-east-1\"\n" +
		"}\nmodule \"next\" {\n  source = \"./next\"\n}\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", portable},
		{"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			assertTerraformDirectLocals(t, tc.source,
				[]terraformLocalWant{{"name", 2}, {"tags", 3}, {"region", 6}},
				[]string{"next"})
		})
	}
	for _, tc := range []struct {
		name, env, marker string
		wants             []terraformLocalWant
		modules           []string
	}{
		{
			name:   "aws-vpc",
			env:    "SCRIPTHOLD_REAL_TERRAFORM_LOCALS_AWS",
			marker: "  name   = \"ex-${basename(path.cwd)}\"",
			wants: []terraformLocalWant{
				{"name", 8}, {"region", 9}, {"vpc_cidr", 11},
				{"azs", 12}, {"tags", 14},
			},
			modules: []string{"vpc", "disabled"},
		},
		{
			name:    "terraform-core",
			env:     "SCRIPTHOLD_REAL_TERRAFORM_LOCALS_CORE",
			marker:  "  foo = 3",
			wants:   []terraformLocalWant{{"foo", 2}},
			modules: []string{"child", "count_child"},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independently authored Terraform original not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 60 || info.Size() > 10000 {
				t.Fatalf("original Terraform source missing, nonregular or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.marker) {
				t.Fatal("independent Terraform original marker drift")
			}
			assertTerraformDirectLocals(t, string(body), tc.wants, tc.modules)
		})
	}
	t.Run("unsupported-locals-fail-closed", func(t *testing.T) {
		for _, body := range []string{
			"  broken =\n",
			"  broken =  # missing value\n",
			"  broken == true\n",
			strings.Repeat("  item = 1\n", 257),
			"  too_big = \"" + strings.Repeat("x", 66000) + "\"\n",
		} {
			source := "locals {\n" + body + "}\nmodule \"safe\" { source = \"./safe\" }\n"
			result, err := (TerraformAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("main.tf", source), testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name := range byName {
				if strings.HasPrefix(name, "locals.") {
					t.Errorf("unsupported locals invented %q", name)
				}
			}
			if _, ok := byName["safe"]; !ok {
				t.Error("valid neighboring module lost")
			}
			diagnosed := false
			for _, d := range result.Analysis.Diagnostics {
				if d.Code == "terraform-unsupported-locals" && d.Range != nil {
					diagnosed = true
				}
			}
			if result.Analysis.CoverageComplete || !diagnosed {
				t.Error("unsupported locals not diagnosed as source-ranged incomplete")
			}
		}
	})
	t.Run("quoted-braces-and-nested-keys", func(t *testing.T) {
		const source = "locals {\n  label = \"braces { and } in string\"\n" +
			"  tags = {\n    Fake = \"not a local\"\n  }\n  count = 3\n}\n"
		assertTerraformDirectLocals(t, source,
			[]terraformLocalWant{{"label", 2}, {"tags", 3}, {"count", 6}}, nil)
	})
	t.Run("generic-hcl-preserves-sections-only", func(t *testing.T) {
		result, err := (TerraformAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("service.hcl", "locals { answer = 42 }\nservice \"api\" { port = 8080 }\n"),
			testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if byName["locals"].Kind != SymbolKindSection {
			t.Errorf("generic HCL locals section lost: %+v", byName["locals"])
		}
		if _, found := byName["locals.answer"]; found {
			t.Errorf("generic HCL attribute promoted to Terraform local: %+v", byName["locals.answer"])
		}
	})
}

func assertTerraformDirectLocals(t *testing.T, source string, wants []terraformLocalWant, modules []string) {
	t.Helper()
	res, err := (TerraformAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("main.tf", source), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(res.Analysis.Symbols)
	parent, exists := byName["locals"]
	if !exists || parent.Kind != SymbolKindSection {
		t.Fatalf("authored locals block missing: %+v", parent)
	}
	lines := strings.Split(source, "\n")
	for _, w := range wants {
		if w.line < 1 || w.line > len(lines) {
			t.Fatalf("oracle line out of source: %+v", w)
		}
		column := strings.Index(lines[w.line-1], w.name) + 1
		if column == 0 {
			t.Fatalf("authored Terraform local %q missing at line %d", w.name, w.line)
		}
		name := "locals." + w.name
		symbol, ok := byName[name]
		if !ok || symbol.Kind != SymbolKindVariable ||
			symbol.NativeKind != "local-binding" ||
			symbol.ParentQualifiedName != "locals" ||
			symbol.ParentID != parent.ID ||
			symbol.NameRange.Start.Line != w.line ||
			symbol.NameRange.Start.Column != column ||
			symbol.DeclarationRange.Start.Line != w.line {
			t.Errorf("Terraform original local %q missing/misclassified at %d:%d: %+v",
				name, w.line, column, symbol)
		}
	}
	for _, module := range modules {
		if symbol, ok := byName[module]; !ok || symbol.Kind != SymbolKindModule {
			t.Errorf("neighboring original Terraform module %q lost: %+v", module, symbol)
		}
	}
	for _, fake := range []string{
		"locals.Example", "locals.GithubRepo", "locals.GithubOrg",
		"locals.tags.Example", "locals.tags.GithubRepo",
	} {
		if _, exists := byName[fake]; exists {
			t.Errorf("nested HCL object attribute falsely indexed as local %q", fake)
		}
	}
}
