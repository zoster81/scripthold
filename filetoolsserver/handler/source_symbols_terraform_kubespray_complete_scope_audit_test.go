package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Complete supported HCL block inventory transcribed from five independently
// read original Kubespray AWS source files BEFORE querying source_symbols.
// This does not claim validation of assignments, inline objects, providers'
// runtime semantics or Terraform plans.
func TestTerraformKubespraySelectedOriginalCompleteBlockInventory(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Terraform corpus not configured")
	}
	type selectedFile struct {
		file string
		// Manually verified source lines of every supported block header,
		// including nested blocks and inline empty data declarations.
		lines  []int
		owners map[int]string
	}
	cases := []selectedFile{
		{
			file:  "create-infrastructure.tf",
			lines: []int{1, 3, 11, 17, 24, 35, 47, 58, 81, 91, 105, 111, 121, 134, 144, 162, 177, 178},
			owners: map[int]string{
				3:   "terraform",
				91:  "aws_instance.k8s-master",
				121: "aws_instance.k8s-etcd",
				144: "aws_instance.k8s-worker",
				178: "null_resource.inventories",
			},
		},
		{
			file:   "variables.tf",
			lines:  []int{1, 5, 9, 13, 19, 23, 29, 35, 41, 44, 49, 59, 63, 68, 75, 84, 88, 92, 96, 100, 104, 108, 112, 116, 120, 128, 132, 136, 141},
			owners: map[int]string{44: "aws_ami.distro", 49: "aws_ami.distro"},
		},
		{file: "output.tf", lines: []int{1, 5, 9, 13, 17, 21, 25}},
		{
			file:  "modules/vpc/main.tf",
			lines: []int{1, 13, 18, 26, 39, 45, 62, 65, 75, 79, 89, 95, 103, 112, 121, 130},
			owners: map[int]string{
				65: "aws_route_table.kubernetes-public",
				79: "aws_route_table.kubernetes-private",
			},
		},
		{file: "modules/vpc/variables.tf", lines: []int{1, 5, 9, 14, 19, 24}},
	}
	type source struct {
		path  string
		lines []string
	}
	sources := make([]source, len(cases))
	for i, tc := range cases {
		path := filepath.Join(corpus, "terraform58-kubespray-audit-selected",
			"contrib", "terraform", "aws", filepath.FromSlash(tc.file))
		requireAdditionalOriginalFile(t, path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		sources[i] = source{path, strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")}
		for _, line := range tc.lines {
			if line < 1 || line > len(sources[i].lines) ||
				!strings.Contains(sources[i].lines[line-1], "{") {
				t.Fatalf("independent original block header incorrect %s:%d", tc.file, line)
			}
		}
	}
	for i, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			src := sources[i]
			h := NewHandler([]string{filepath.Dir(src.path)})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{src.path},
				Language: "terraform", Encoding: "utf-8",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 {
				t.Fatalf("original full-file HCL outline incomplete: %v %+v %+v", err, failed, out)
			}
			want := map[int]bool{}
			for _, n := range tc.lines {
				if want[n] {
					t.Fatalf("duplicate independent inventory line %d", n)
				}
				want[n] = true
			}
			seen := map[int]bool{}
			for _, sym := range out.Symbols {
				line := sym.NameRange.Start.Line
				if filepath.Clean(sym.Path) != filepath.Clean(src.path) {
					t.Errorf("wrong original file %+v", sym)
					continue
				}
				if !want[line] || seen[line] || line < 1 || line > len(src.lines) {
					t.Errorf("unexpected or duplicate original HCL symbol %s:%d %s", tc.file, line, sym.Name)
					continue
				}
				seen[line] = true
				header := strings.TrimSpace(src.lines[line-1])
				tokens := strings.Fields(header)
				if len(tokens) == 0 {
					t.Errorf("empty original header at %d", line)
					continue
				}
				word := tokens[0]
				name, kind, native := word, "section", "hcl-block"
				switch word {
				case "resource", "data":
					kind = "resource"
					native = word
					if len(tokens) < 4 {
						t.Fatalf("bad original resource header %q", header)
					}
					name = strings.Trim(tokens[1], "\"") + "." + strings.Trim(tokens[2], "\"")
				case "module", "variable", "output":
					native = word
					if word == "module" {
						kind = "module"
					} else {
						kind = "variable"
					}
					if len(tokens) < 3 {
						t.Fatalf("bad original block header %q", header)
					}
					name = strings.Trim(tokens[1], "\"")
				default:
					if len(tokens) > 2 && strings.HasPrefix(tokens[1], "\"") {
						name = word + "." + strings.Trim(tokens[1], "\"")
					}
				}
				if sym.Name != name || string(sym.Kind) != kind || sym.NativeKind != native ||
					sym.ParentQualifiedName != tc.owners[line] {
					t.Errorf("misidentified block at %d: got %s/%s %s owner=%q; original %s/%s %s owner=%q",
						line, sym.Kind, sym.NativeKind, sym.Name, sym.ParentQualifiedName,
						kind, native, name, tc.owners[line])
				}
			}
			for _, line := range tc.lines {
				if !seen[line] {
					t.Errorf("missing original HCL block at %d: %q", line, src.lines[line-1])
				}
			}
			if len(out.Symbols) != len(tc.lines) {
				t.Errorf("expected %d original blocks, got %d", len(tc.lines), len(out.Symbols))
			}
			t.Logf("independentCompleteFileBlocks=%d returned=%d", len(tc.lines), len(out.Symbols))
		})
	}
}
