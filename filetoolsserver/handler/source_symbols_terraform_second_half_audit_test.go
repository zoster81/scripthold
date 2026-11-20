package handler

import (
	"os"
	"testing"
)

// Provider 58: two further independent real Terraform source projects,
// Kubespray (AWS/VPC) and Hasura (AlloyDB), distinct from initial AWS-VPC
// modules. No cloud APIs, Terraform plans or original project code are run.
func TestSourceSymbolsTerraformSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Terraform projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "terraform58-kubespray-audit-selected", language: "terraform",
			search:        "aws_nat_gateway.cluster-nat-gateway",
			primary:       "contrib/terraform/aws/modules/vpc/main.tf",
			negativeFile:  "contrib/terraform/aws/output.tf",
			negativeQuery: "aws_nat_gateway.cluster-nat-gateway",
			paths: []string{
				"contrib/terraform/aws/create-infrastructure.tf",
				"contrib/terraform/aws/variables.tf",
				"contrib/terraform/aws/modules/vpc/main.tf",
				"contrib/terraform/aws/output.tf",
			},
			oracles: []extraAuditEvidence{
				{"contrib/terraform/aws/create-infrastructure.tf", "module \"aws-vpc\" {"},
				{"contrib/terraform/aws/create-infrastructure.tf", "source = \"./modules/vpc\""},
				{"contrib/terraform/aws/create-infrastructure.tf", "resource \"aws_instance\" \"k8s-master\" {"},
				{"contrib/terraform/aws/variables.tf", "variable \"aws_vpc_cidr_block\" {"},
				{"contrib/terraform/aws/modules/vpc/main.tf", "resource \"aws_vpc\" \"cluster-vpc\" {"},
				{"contrib/terraform/aws/modules/vpc/main.tf", "resource \"aws_nat_gateway\" \"cluster-nat-gateway\" {"},
				{"contrib/terraform/aws/modules/vpc/main.tf", "resource \"aws_route_table\" \"kubernetes-private\" {"},
				{"contrib/terraform/aws/modules/vpc/variables.tf", "variable \"aws_cidr_subnets_private\" {"},
				{"contrib/terraform/aws/output.tf", "output \"masters\" {"},
			},
			showAsWritten: map[string]string{
				"aws-vpc":                             "module \"aws-vpc\"",
				"aws_nat_gateway.cluster-nat-gateway": "resource \"aws_nat_gateway\" \"cluster-nat-gateway\"",
				"aws_vpc.cluster-vpc":                 "resource \"aws_vpc\" \"cluster-vpc\"",
				"aws_route_table.kubernetes-private":  "resource \"aws_route_table\" \"kubernetes-private\"",
				"aws_instance.k8s-master":             "resource \"aws_instance\" \"k8s-master\"",
			},
			targets: []extraAuditSymbol{
				{"contrib/terraform/aws/create-infrastructure.tf", "aws-vpc", 24},
				{"contrib/terraform/aws/create-infrastructure.tf", "aws_instance.k8s-master", 81},
				{"contrib/terraform/aws/variables.tf", "aws_vpc_cidr_block", 59},
				{"contrib/terraform/aws/modules/vpc/main.tf", "aws_vpc.cluster-vpc", 1},
				{"contrib/terraform/aws/modules/vpc/main.tf", "aws_nat_gateway.cluster-nat-gateway", 39},
				{"contrib/terraform/aws/modules/vpc/main.tf", "aws_route_table.kubernetes-private", 75},
			},
		},
		{
			project: "terraform58-hasura-audit-selected", language: "terraform",
			search:        "google_alloydb_instance.primary",
			primary:       "server/test-manual/alloydb/alloydb.tf",
			negativeFile:  "server/test-manual/alloydb/service_account.tf",
			negativeQuery: "google_alloydb_instance.primary",
			paths: []string{
				"server/test-manual/alloydb/alloydb.tf",
				"server/test-manual/alloydb/network.tf",
				"server/test-manual/alloydb/service_account.tf",
				"server/test-manual/alloydb/provider.tf",
			},
			oracles: []extraAuditEvidence{
				{"server/test-manual/alloydb/alloydb.tf", "resource \"google_alloydb_cluster\" \"default\" {"},
				{"server/test-manual/alloydb/alloydb.tf", "resource \"google_alloydb_instance\" \"primary\" {"},
				{"server/test-manual/alloydb/alloydb.tf", "output \"url\" {"},
				{"server/test-manual/alloydb/network.tf", "resource \"google_compute_network\" \"default\" {"},
				{"server/test-manual/alloydb/network.tf", "resource \"google_service_networking_connection\" \"default\" {"},
				{"server/test-manual/alloydb/service_account.tf", "resource \"google_service_account\" \"service_account\" {"},
				{"server/test-manual/alloydb/provider.tf", "provider \"google\" {"},
				{"server/test-manual/alloydb/bastion.tf", "resource \"google_compute_instance\" \"bastion\" {"},
			},
			showAsWritten: map[string]string{
				"google_alloydb_cluster.default":         "resource \"google_alloydb_cluster\" \"default\"",
				"google_alloydb_instance.primary":        "resource \"google_alloydb_instance\" \"primary\"",
				"google_compute_network.default":         "resource \"google_compute_network\" \"default\"",
				"google_service_account.service_account": "resource \"google_service_account\" \"service_account\"",
			},
			targets: []extraAuditSymbol{
				{"server/test-manual/alloydb/alloydb.tf", "google_alloydb_cluster.default", 9},
				{"server/test-manual/alloydb/alloydb.tf", "google_alloydb_instance.primary", 21},
				{"server/test-manual/alloydb/network.tf", "google_compute_network.default", 3},
				{"server/test-manual/alloydb/service_account.tf", "google_service_account.service_account", 4},
				{"server/test-manual/alloydb/provider.tf", "name", 10},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
