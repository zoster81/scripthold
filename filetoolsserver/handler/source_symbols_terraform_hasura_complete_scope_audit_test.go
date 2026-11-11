package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independently inventoried from four complete original Hasura AlloyDB HCL
// files. This is a selected-file declaration audit, not validation of cloud
// resources or all HCL expressions; no Terraform code is executed.
func TestTerraformHasuraSelectedOriginalCompleteBlockInventory(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Terraform corpus not configured")
	}
	type declaration struct {
		name, kind, native, owner string
		line                      int
	}
	cases := []struct {
		file string
		want []declaration
	}{
		{"alloydb.tf", []declaration{
			{"password", "variable", "variable", "", 4},
			{"google_alloydb_cluster.default", "resource", "resource", "", 9},
			{"initial_user", "section", "hcl-block", "google_alloydb_cluster.default", 15},
			{"google_alloydb_instance.primary", "resource", "resource", "", 21},
			{"url", "variable", "output", "", 31},
		}},
		{"network.tf", []declaration{
			{"google_compute_network.default", "resource", "resource", "", 3},
			{"google_compute_global_address.private_ip_alloc", "resource", "resource", "", 7},
			{"google_service_networking_connection.default", "resource", "resource", "", 15},
			{"google_compute_firewall.ssh", "resource", "resource", "", 22},
			{"allow", "section", "hcl-block", "google_compute_firewall.ssh", 24},
			{"google_compute_firewall.postgres", "resource", "resource", "", 36},
			{"allow", "section", "hcl-block", "google_compute_firewall.postgres", 38},
		}},
		{"service_account.tf", []declaration{
			{"google_service_account.service_account", "resource", "resource", "", 4},
			{"google_project_iam_binding.service_account_alloydb_client", "resource", "resource", "", 9},
		}},
		{"provider.tf", []declaration{
			{"locals", "section", "hcl-block", "", 1},
			{"provider.google", "section", "hcl-block", "", 5},
			{"name", "variable", "variable", "", 10},
			{"google_project.project", "resource", "data", "", 14},
		}},
	}
	type original struct {
		path  string
		lines []string
	}
	sources := make([]original, len(cases))
	for i, tc := range cases {
		path := filepath.Join(corpus, "terraform58-hasura-audit-selected",
			"server", "test-manual", "alloydb", tc.file)
		requireAdditionalOriginalFile(t, path)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources[i] = original{path, strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")}
		for _, item := range tc.want {
			if item.line < 1 || item.line > len(sources[i].lines) ||
				!strings.HasSuffix(strings.TrimSpace(sources[i].lines[item.line-1]), "{") {
				t.Fatalf("incorrect independent HCL block location: %s:%d", tc.file, item.line)
			}
		}
	}
	for i, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			source := sources[i]
			h := NewHandler([]string{filepath.Dir(source.path)})
			failed, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{source.path},
				Language: "terraform", Encoding: "utf-8",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || result.Truncated ||
				!result.CoverageComplete || result.FilesParsed != 1 {
				t.Fatalf("complete source HCL outline unavailable: err=%v failed=%+v result=%+v",
					err, failed, result)
			}
			expected := make(map[int]declaration, len(tc.want))
			for _, item := range tc.want {
				if _, exists := expected[item.line]; exists {
					t.Fatalf("duplicate independent declaration at %d", item.line)
				}
				expected[item.line] = item
			}
			seen := map[int]bool{}
			for _, symbol := range result.Symbols {
				line := symbol.NameRange.Start.Line
				if filepath.Clean(symbol.Path) != filepath.Clean(source.path) {
					t.Errorf("incorrect source file for declaration %+v", symbol)
					continue
				}
				if seen[line] {
					t.Errorf("fabricated duplicate HCL declaration at line %d", line)
				}
				seen[line] = true
				want, ok := expected[line]
				if !ok {
					t.Errorf("unlisted HCL block %s %s at %d", symbol.Kind, symbol.Name, line)
					continue
				}
				if symbol.Name != want.name || string(symbol.Kind) != want.kind ||
					symbol.NativeKind != want.native ||
					symbol.ParentQualifiedName != want.owner {
					t.Errorf("misidentified HCL block line=%d: got %s/%s %s parent=%q want %+v",
						line, symbol.Kind, symbol.NativeKind, symbol.Name,
						symbol.ParentQualifiedName, want)
				}
			}
			for _, item := range tc.want {
				if !seen[item.line] {
					t.Errorf("missing HCL declaration %s at %d", item.name, item.line)
				}
			}
			if len(result.Symbols) != len(tc.want) {
				t.Errorf("expected %d complete source blocks, got %d", len(tc.want), len(result.Symbols))
			}
			t.Logf("independentCompleteFileBlocks=%d indexed=%d", len(tc.want), len(result.Symbols))
		})
	}
}
