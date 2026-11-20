package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type terraformPinnedDeclaration struct {
	name, kind, native, qualified, owner                         string
	line, nameStart, nameEnd, declStart, declEndLine, declEndCol int
	authoredHeader                                               string
}

// All three complete upstream originals were independently inventoried before
// invoking the development handler. HCL canonical names can include block
// kinds and paired labels, not just a lexical name token.
func TestTerraformThreeWholePinnedOriginalOutlines(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_TERRAFORM_OUTLINE_ROOT")
	if root == "" {
		t.Skip("three independently Git-pinned Terraform originals not configured")
	}
	cases := []struct {
		file, blob, target string
		size               int64
		want               []terraformPinnedDeclaration
	}{
		{
			file: "aws-simple-main.tf", size: 1011, blob: "733622cc601fba4c86c1bb4232b7778938f70b0d",
			target: "tags",
			want: []terraformPinnedDeclaration{
				{"provider.aws", "section", "hcl-block", "provider.aws", "", 1, 1, 14, 1, 3, 2, "provider \"aws\" {"},
				{"aws_availability_zones.available", "resource", "data", "aws_availability_zones.available", "", 5, 7, 41, 1, 5, 45, "data \"aws_availability_zones\" \"available\" {}"},
				{"locals", "section", "hcl-block", "locals", "", 7, 1, 7, 1, 19, 2, "locals {"},
				{"name", "variable", "local-binding", "locals.name", "locals", 8, 3, 7, 3, 8, 38, "  name   = \"ex-${basename(path.cwd)}\""},
				{"region", "variable", "local-binding", "locals.region", "locals", 9, 3, 9, 3, 9, 23, "  region = \"eu-west-1\""},
				{"vpc_cidr", "variable", "local-binding", "locals.vpc_cidr", "locals", 11, 3, 11, 3, 11, 27, "  vpc_cidr = \"10.0.0.0/16\""},
				{"azs", "variable", "local-binding", "locals.azs", "locals", 12, 3, 6, 3, 12, 70, "  azs      = slice(data.aws_availability_zones.available.names, 0, 3)"},
				{"tags", "variable", "local-binding", "locals.tags", "locals", 14, 3, 7, 3, 18, 4, "  tags = {"},
				{"vpc", "module", "module", "vpc", "", 25, 9, 12, 1, 35, 2, "module \"vpc\" {"},
				{"disabled", "module", "module", "disabled", "", 41, 9, 17, 1, 45, 2, "module \"disabled\" {"},
			},
		},
		{
			file: "aws-flow-outputs.tf", size: 1506, blob: "38bf2119074844bb0827a002a48abe50b5f564e7",
			target: "id",
			want: []terraformPinnedDeclaration{
				{"id", "variable", "output", "id", "", 5, 9, 11, 1, 8, 2, "output \"id\" {"},
				{"arn", "variable", "output", "arn", "", 10, 9, 12, 1, 13, 2, "output \"arn\" {"},
				{"cloudwatch_log_group_name", "variable", "output", "cloudwatch_log_group_name", "", 19, 9, 34, 1, 22, 2, "output \"cloudwatch_log_group_name\" {"},
				{"cloudwatch_log_group_arn", "variable", "output", "cloudwatch_log_group_arn", "", 24, 9, 33, 1, 27, 2, "output \"cloudwatch_log_group_arn\" {"},
				{"iam_role_name", "variable", "output", "iam_role_name", "", 33, 9, 22, 1, 36, 2, "output \"iam_role_name\" {"},
				{"iam_role_arn", "variable", "output", "iam_role_arn", "", 38, 9, 21, 1, 41, 2, "output \"iam_role_arn\" {"},
				{"iam_role_unique_id", "variable", "output", "iam_role_unique_id", "", 43, 9, 27, 1, 46, 2, "output \"iam_role_unique_id\" {"},
			},
		},
		{
			file: "cole-main.tf", size: 290, blob: "4ab6b4aaeb7998c2840bc162726273e5694d82a8",
			target: "required_providers",
			want: []terraformPinnedDeclaration{
				{"terraform", "section", "hcl-block", "terraform", "", 4, 1, 10, 1, 11, 2, "terraform {"},
				{"required_providers", "section", "hcl-block", "terraform.required_providers", "terraform", 5, 3, 21, 1, 10, 4, "  required_providers {"},
			},
		},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("original Terraform source absent/changed: info=%+v err=%v", info, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			blob := sha1.New()
			_, _ = fmt.Fprintf(blob, "blob %d%c", len(original), 0)
			_, _ = blob.Write(original)
			if hex.EncodeToString(blob.Sum(nil)) != tc.blob {
				t.Fatal("full pinned Git original changed")
			}
			failed, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "terraform", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || outlined.Truncated || !outlined.CoverageComplete ||
				outlined.FilesParsed != 1 || len(outlined.Files) != 1 || len(outlined.Symbols) != len(tc.want) {
				t.Fatalf("complete authored HCL inventory: err=%v failed=%+v result=%+v", err, failed, outlined)
			}
			byName := make(map[string]int, len(outlined.Symbols))
			ids := make(map[string]string, len(outlined.Symbols))
			for i, sym := range outlined.Symbols {
				if _, exists := byName[sym.QualifiedName]; exists {
					t.Errorf("duplicate HCL declaration: %s", sym.QualifiedName)
				}
				byName[sym.QualifiedName] = i
				ids[sym.QualifiedName] = sym.ID
				if sym.ID == "" {
					t.Errorf("empty HCL identity %s", sym.QualifiedName)
				}
			}
			for _, exp := range tc.want {
				idx, exists := byName[exp.qualified]
				if !exists {
					t.Errorf("original supported declaration missing %+v", exp)
					continue
				}
				sym := outlined.Symbols[idx]
				if sym.Name != exp.name || sym.QualifiedName != exp.qualified ||
					string(sym.Kind) != exp.kind || sym.NativeKind != exp.native ||
					sym.ParentQualifiedName != exp.owner ||
					sym.NameRange.Start.Line != exp.line || sym.NameRange.Start.Column != exp.nameStart ||
					sym.NameRange.End.Line != exp.line || sym.NameRange.End.Column != exp.nameEnd ||
					sym.DeclarationRange.Start.Line != exp.line ||
					sym.DeclarationRange.Start.Column != exp.declStart ||
					sym.DeclarationRange.End.Line != exp.declEndLine ||
					sym.DeclarationRange.End.Column != exp.declEndCol ||
					!strings.Contains(sym.Signature, strings.TrimSpace(exp.authoredHeader)) {
					t.Errorf("original Terraform symbol mismatch: want=%+v got=%+v", exp, sym)
				}
				if exp.owner == "" && sym.ParentID != "" {
					t.Errorf("unexpected HCL parent %+v", sym)
				}
				if exp.owner != "" && sym.ParentID != ids[exp.owner] {
					t.Errorf("broken HCL owner link %+v", sym)
				}
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if fingerprint == "" || outlined.Files[0].SourceBytes != tc.size {
				t.Fatal("HCL fingerprint or bytes missing")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "terraform", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 {
				t.Fatalf("original HCL find: err=%v failed=%+v result=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "terraform", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.target) {
				t.Fatalf("original HCL fingerprint show: err=%v failed=%+v result=%+v", err, failed, shown.Show)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "terraform",
				Encoding: "utf-8", Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original HCL structural search: err=%v failed=%+v result=%+v", err, failed, queried)
			}
		})
	}
}
