package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTerraformDirectLocalsOriginalMCPNavigation(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		members           []struct {
			name string
			line int
		}
	}{
		{
			name: "aws-vpc", env: "SCRIPTHOLD_REAL_TERRAFORM_LOCALS_AWS",
			marker: "locals {",
			members: []struct {
				name string
				line int
			}{{"name", 8}, {"region", 9}, {"vpc_cidr", 11}, {"azs", 12}, {"tags", 14}},
		},
		{
			name: "terraform-core", env: "SCRIPTHOLD_REAL_TERRAFORM_LOCALS_CORE",
			marker: "  foo = 3",
			members: []struct {
				name string
				line int
			}{{"foo", 2}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent Terraform original not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 60 || info.Size() > 10000 {
				t.Fatalf("original Terraform unavailable/nonregular/oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("authored upstream Terraform marker drift")
			}
			lines := strings.Split(source, "\n")
			h := NewHandler([]string{filepath.Dir(path)})
			failure, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "terraform",
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failure != nil || result.Truncated || len(result.OutlineFiles) != 1 ||
				result.OutlineFiles[0].SourceFingerprint == "" {
				t.Fatalf("Terraform original outline failed: %v %+v %+v", err, failure, result)
			}
			for _, want := range tc.members {
				col := strings.Index(lines[want.line-1], want.name) + 1
				if col < 1 {
					t.Fatalf("local %q not authored at line %d", want.name, want.line)
				}
				qualified := "locals." + want.name
				node := findTerraformLocalOriginalOutline(result.OutlineFiles[0].Symbols, qualified)
				if node == nil || node.Kind != "variable" || node.Line != want.line || node.Column != col {
					t.Errorf("Terraform local %q not safely navigable at %d:%d: %+v", qualified, want.line, col, node)
				}
			}
			for _, forbidden := range []string{"locals.Example", "locals.GithubRepo", "locals.GithubOrg", "locals.tags.Example"} {
				if node := findTerraformLocalOriginalOutline(result.OutlineFiles[0].Symbols, forbidden); node != nil {
					t.Errorf("nested object key incorrectly promoted to local: %+v", node)
				}
			}
			failure, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "terraform",
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failure != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Terraform locals digest failed: %v %+v %+v", err, failure, digest)
			}
			found := 0
			for _, count := range digest.Digests[0].DeclarationCounts {
				if count.Kind == "variable" {
					found += count.Count
				}
			}
			if found < len(tc.members) {
				t.Fatalf("original Terraform locals digest missing variable inventory: got %d need %d", found, len(tc.members))
			}
		})
	}
}

func findTerraformLocalOriginalOutline(nodes []*SourceOutlineNode, qualified string) *SourceOutlineNode {
	for _, n := range nodes {
		if n.QualifiedName == qualified {
			return n
		}
		if found := findTerraformLocalOriginalOutline(n.Children, qualified); found != nil {
			return found
		}
	}
	return nil
}
