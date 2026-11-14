package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

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
