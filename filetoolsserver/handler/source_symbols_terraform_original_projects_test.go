package handler

import (
	"os"
	"testing"
)

// These checks inspect independent VPC Terraform projects without running
// Terraform, connecting to cloud services, or executing the originals' tests.
func TestSourceSymbolsOriginalTerraformInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Terraform corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "terraform-aws-vpc", language: "terraform",
			search: "aws_nat_gateway.this", primary: "main.tf",
			negativeFile: "outputs.tf", negativeQuery: "aws_nat_gateway.this",
			paths: []string{"variables.tf", "main.tf", "outputs.tf", "examples/complete/main.tf"},
			oracles: []extraAuditEvidence{
				{"variables.tf", "variable \"enable_nat_gateway\" {"},
				{"variables.tf", "variable \"single_nat_gateway\" {"},
				{"variables.tf", "variable \"one_nat_gateway_per_az\" {"},
				{"variables.tf", "variable \"reuse_nat_ips\" {"},
				{"variables.tf", "default     = false"},
				{"main.tf", "nat_gateway_count = var.single_nat_gateway ? 1 : var.one_nat_gateway_per_az ? length(var.azs) : local.max_subnet_length"},
				{"main.tf", "resource \"aws_eip\" \"nat\" {"},
				{"main.tf", "count = local.create_vpc && var.enable_nat_gateway && !var.reuse_nat_ips ? local.nat_gateway_count : 0"},
				{"main.tf", "resource \"aws_nat_gateway\" \"this\" {"},
				{"main.tf", "count = local.create_vpc && var.enable_nat_gateway ? local.nat_gateway_count : 0"},
				{"main.tf", "var.single_nat_gateway ? 0 : count.index"},
				{"main.tf", "resource \"aws_route\" \"private_nat_gateway\" {"},
				{"main.tf", "nat_gateway_id         = element(aws_nat_gateway.this[*].id, count.index)"},
				{"outputs.tf", "output \"natgw_ids\" {"},
				{"outputs.tf", "value       = aws_nat_gateway.this[*].id"},
				{"examples/complete/main.tf", "module \"vpc\" {"},
				{"examples/complete/main.tf", "enable_nat_gateway = true"},
				{"examples/complete/main.tf", "single_nat_gateway = true"},
				{"README.md", "If both `single_nat_gateway` and `one_nat_gateway_per_az` are set to `true`, then `single_nat_gateway` takes precedence."},
			},
			showAsWritten: map[string]string{
				"aws_eip.nat":                   `resource "aws_eip" "nat"`,
				"aws_nat_gateway.this":          `resource "aws_nat_gateway" "this"`,
				"aws_route.private_nat_gateway": `resource "aws_route" "private_nat_gateway"`,
			},
			targets: []extraAuditSymbol{
				{"variables.tf", "enable_nat_gateway", 1213},
				{"variables.tf", "single_nat_gateway", 1225},
				{"variables.tf", "one_nat_gateway_per_az", 1231},
				{"main.tf", "aws_eip.nat", 1207},
				{"main.tf", "aws_nat_gateway.this", 1228},
				{"main.tf", "aws_route.private_nat_gateway", 1256},
				{"outputs.tf", "natgw_ids", 522},
				{"examples/complete/main.tf", "vpc", 25},
			},
		},
		{
			project: "terraform-cloudposse-vpc", language: "terraform",
			search: "dns_hostnames_enabled", primary: "variables.tf",
			negativeFile: "outputs.tf", negativeQuery: "dns_hostnames_enabled",
			paths: []string{"variables.tf", "main.tf", "outputs.tf", "examples/complete/main.tf"},
			oracles: []extraAuditEvidence{
				{"variables.tf", "variable \"dns_hostnames_enabled\" {"},
				{"variables.tf", "variable \"dns_support_enabled\" {"},
				{"variables.tf", "Set `true` to enable DNS resolution in the VPC"},
				{"variables.tf", "default     = true"},
				{"main.tf", "dns_hostnames_enabled            = var.dns_hostnames_enabled"},
				{"main.tf", "dns_support_enabled              = var.dns_support_enabled"},
				{"main.tf", "resource \"aws_vpc\" \"default\" {"},
				{"main.tf", "enable_dns_hostnames                 = local.dns_hostnames_enabled"},
				{"main.tf", "enable_dns_support                   = local.dns_support_enabled"},
				{"main.tf", "resource \"aws_internet_gateway\" \"default\" {"},
				{"outputs.tf", "output \"vpc_id\" {"},
				{"outputs.tf", "value       = join(\"\", aws_vpc.default.*.id)"},
				{"examples/complete/main.tf", "module \"vpc\" {"},
				{"examples/complete/main.tf", "source                  = \"../../\""},
				{"examples/complete/main.tf", "module \"vpc_disabled\" {"},
				{"test/src/examples_complete_test.go", "func TestExamplesComplete(t *testing.T) {"},
				{"test/src/examples_complete_test.go", "terraform.InitAndApply(t, terraformOptions)"},
				{"test/src/examples_complete_test.go", "func TestExamplesCompleteDisabled(t *testing.T) {"},
				{"README.md", "input_dns_hostnames_enabled"},
			},
			showAsWritten: map[string]string{
				"aws_vpc.default":              `resource "aws_vpc" "default"`,
				"aws_internet_gateway.default": `resource "aws_internet_gateway" "default"`,
			},
			targets: []extraAuditSymbol{
				{"variables.tf", "dns_hostnames_enabled", 107},
				{"variables.tf", "dns_support_enabled", 113},
				{"main.tf", "aws_vpc.default", 48},
				{"main.tf", "aws_internet_gateway.default", 94},
				{"outputs.tf", "vpc_id", 1},
				{"examples/complete/main.tf", "vpc", 6},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
