package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A module's source is an attribute directly inside the module block. A
// nested object with the same key must not create a fabricated dependency.
func TestTerraformModuleSourceIgnoresNestedAttribute(t *testing.T) {
	check := func(t *testing.T, source string, want []string) {
		t.Helper()
		result, err := (TerraformAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("create-infrastructure.tf", source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		actual := make([]string, 0)
		for _, dependency := range result.Dependencies {
			if dependency.Kind == StructuralDependencyImport {
				actual = append(actual, dependency.Value)
			}
		}
		if !slices.Equal(actual, want) {
			t.Fatalf("module sources = %v, want %v; all dependencies=%+v", actual, want, result.Dependencies)
		}
	}
	cases := []struct {
		name, source string
		want         []string
	}{
		{
			name: "nested-before-direct",
			source: "module \"demo\" {\n" +
				"  config = {\n    source = \"./nested-decoy\"\n  }\n" +
				"  source = \"./correct\"\n}\n" +
				"module \"second\" {\n  source = \"./second\"\n}\n",
			want: []string{"./correct", "./second"},
		},
		{
			name: "nested-depth-and-quoted-braces",
			source: "module \"demo\" {\n  config = {\n" +
				"    label = \"braces } and { inside a string\"\n" +
				"    nested = {\n      source = \"./nested-decoy\"\n    }\n" +
				"  }\n  source = \"./correct\"\n}\n",
			want: []string{"./correct"},
		},
		{
			name:   "nested-without-direct-source",
			source: "module \"demo\" {\n  config = {\n    source = \"./nested-decoy\"\n  }\n}\n",
			want:   []string{},
		},
	}
	for _, tc := range cases {
		for _, ending := range []struct{ name, sep string }{
			{"LF", "\n"}, {"CRLF", "\r\n"},
		} {
			t.Run(tc.name+"/"+ending.name, func(t *testing.T) {
				check(t, strings.ReplaceAll(tc.source, "\n", ending.sep), tc.want)
			})
		}
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_TERRAFORM_KUBESPRAY_AWS_ROOT"); root != "" {
		t.Run("source-derived-original-Kubespray", func(t *testing.T) {
			path := filepath.Join(root, "create-infrastructure.tf")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Kubespray source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			const original = "module \"aws-vpc\" {\n  source = \"./modules/vpc\""
			if strings.Count(source, original) != 1 ||
				!strings.Contains(source, "module \"aws-nlb\" {\n  source = \"./modules/nlb\"") ||
				!strings.Contains(source, "module \"aws-iam\" {\n  source = \"./modules/iam\"") {
				t.Fatal("original Kubespray module source oracle changed")
			}
			want := []string{"./modules/vpc", "./modules/nlb", "./modules/iam"}
			check(t, source, want)
			derived := strings.Replace(source, original,
				"module \"aws-vpc\" {\n  options = {\n    source = \"./nested-decoy\"\n  }\n  source = \"./modules/vpc\"", 1)
			check(t, derived, want)
		})
	}
}

// An invalid outer block must not promote nested generic blocks to the root.
func TestTerraformMalformedOuterDoesNotInventNestedBlock(t *testing.T) {
	const source = "resource \"google_compute_firewall\" {\n" +
		"  allow {\n    ports = [\"22\"]\n  }\n}\n" +
		"variable \"name\" {\n  type = string\n}\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (TerraformAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 30))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete {
				t.Fatalf("invalid outer Terraform block claimed complete: %+v", result.Analysis)
			}
			validNeighbor := false
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == "allow" || sym.Name == "ports" {
					t.Fatalf("nested block promoted from malformed resource: %+v", sym)
				}
				if sym.Name == "name" && sym.NativeKind == "variable" {
					validNeighbor = true
				}
			}
			if !validNeighbor {
				t.Fatalf("independent valid variable missing: %+v", result.Analysis)
			}
		})
	}
}

// Terraform resource and data blocks require both a type and a local name.
// A missing local name must not silently disappear under complete coverage.
func TestTerraformMissingSecondBlockLabelMarksPartial(t *testing.T) {
	for _, word := range []string{"resource", "data"} {
		const suffix = "\nvariable \"name\" {\n  type = string\n}\nresource \"google_compute_firewall\" \"ssh\" {\n  name = \"valid\"\n}\n"
		base := word + " \"google_compute_firewall\" {\n  name = \"invalid\"\n}" + suffix
		for _, tc := range []struct{ name, source string }{
			{"LF", base}, {"CRLF", strings.ReplaceAll(base, "\n", "\r\n")},
		} {
			t.Run(word+"/"+tc.name, func(t *testing.T) {
				result, err := (TerraformAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 40))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.CoverageComplete || result.Analysis.Truncated {
					t.Fatalf("incomplete Terraform block header claimed complete: %+v", result.Analysis)
				}
				diagnostic, validVariable, validResource := false, false, false
				for _, d := range result.Analysis.Diagnostics {
					if d.Code == "terraform-missing-block-label" {
						diagnostic = true
					}
				}
				for _, symbol := range result.Analysis.Symbols {
					switch symbol.Name {
					case "name":
						validVariable = symbol.NativeKind == "variable" && symbol.NameRange.Start.Line == 4
					case "google_compute_firewall.ssh":
						validResource = symbol.NativeKind == "resource" && symbol.NameRange.Start.Line == 7
					case "google_compute_firewall.":
						t.Fatalf("malformed Terraform resource invented: %+v", symbol)
					}
				}
				if !diagnostic || !validVariable || !validResource {
					t.Fatalf("Terraform diagnostic or valid neighbors missing: diagnostic=%v variable=%v resource=%v symbols=%+v", diagnostic, validVariable, validResource, result.Analysis.Symbols)
				}
			})
		}
	}
}
