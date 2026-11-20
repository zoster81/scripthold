package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independent upstream Git projects, three whole original sources.
// The supported declaration inventory was independently frozen before local analysis.
func TestVerilogThreeWholePinnedOriginalOutlines(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VERILOG_OUTLINE_ROOT")
	if root == "" {
		t.Skip("original Verilog corpus not configured")
	}
	type decl struct {
		name, native, owner              string
		line, column, endLine, endColumn int
	}
	cases := []struct {
		file string
		size int64
		want []decl
	}{
		{"yosys-aes-kexp128.v", 492, []decl{
			{"aes_key_expand_128", "module", "", 4, 8, 23, 10},
			{"w", "signal", "aes_key_expand_128", 9, 11, 9, 12},
		}},
		{"ctags-verilog-basic.v", 894, []decl{
			{"mod", "module", "", 8, 8, 64, 10},
			{"a", "signal", "mod", 33, 6, 33, 7},
			{"f", "signal", "mod", 34, 5, 34, 6},
			{"g", "signal", "mod", 35, 6, 35, 7},
			{"mynet", "signal", "mod", 55, 19, 55, 24},
			{"PARAM", "parameter", "mod", 18, 11, 18, 21},
			{"STATE1", "parameter", "mod", 20, 11, 20, 25},
		}},
		{"ctags-verilog-2001.v", 831, []decl{
			{"mod", "module", "", 8, 8, 51, 10},
			{"a", "signal", "mod", 12, 16, 12, 17},
			{"e", "signal", "mod", 15, 17, 15, 18},
			{"f", "signal", "mod", 16, 16, 16, 17},
			{"g", "signal", "mod", 17, 16, 17, 17},
			{"scounter", "signal", "mod", 32, 18, 32, 26},
			{"PARAM1", "parameter", "mod", 9, 15, 9, 26},
			{"PARAM2", "parameter", "mod", 10, 15, 11, 1},
			{"LOCALPARAM", "localparam", "mod", 20, 12, 20, 30},
			{"STATE1", "localparam", "mod", 22, 12, 22, 26},
		}},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("original bytes/type changed: %v", info)
			}
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "verilog", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || fail != nil || !out.CoverageComplete || out.Truncated ||
				out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != len(tc.want) {
				t.Fatalf("whole original inventory mismatch: %v %+v", err, out)
			}
			ids := map[string]string{}
			for _, sym := range out.Symbols {
				if ids[sym.Name] != "" || sym.ID == "" {
					t.Errorf("duplicate/empty identity %+v", sym)
				}
				ids[sym.Name] = sym.ID
			}
			for _, expected := range tc.want {
				var ok bool
				for _, sym := range out.Symbols {
					if sym.Name != expected.name {
						continue
					}
					ok = true
					kind := "signal"
					if expected.native == "module" {
						kind = "module"
					}
					if expected.native == "parameter" || expected.native == "localparam" {
						kind = "constant"
					}
					if string(sym.Kind) != kind || sym.NativeKind != expected.native ||
						sym.ParentQualifiedName != expected.owner ||
						sym.NameRange.Start.Line != expected.line ||
						sym.NameRange.Start.Column != expected.column ||
						sym.DeclarationRange.End.Line != expected.endLine ||
						sym.DeclarationRange.End.Column != expected.endColumn ||
						sym.SignatureRange == nil ||
						sym.SignatureRange.End.Line != expected.endLine ||
						sym.SignatureRange.End.Column != expected.endColumn ||
						!strings.Contains(sym.Signature, expected.name) {
						t.Errorf("source declaration differs: want %+v got %+v", expected, sym)
					}
					if expected.owner != "" && sym.ParentID != ids[expected.owner] {
						t.Errorf("invalid original owner identity: %+v", sym)
					}
				}
				if !ok {
					t.Errorf("missing authored declaration %+v", expected)
				}
			}
			target := tc.want[0].name
			fingerprint := out.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing original fingerprint")
			}
			fail, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "verilog", Query: target,
				Match: "exact", MaxFiles: 1, MaxSymbols: 8,
			})
			if err != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != ids[target] {
				t.Fatalf("original find mismatch %v %+v", err, found)
			}
			fail, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "verilog", Encoding: "utf-8",
				SymbolID: ids[target], SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target) {
				t.Fatalf("original fingerprint-bound show failed %v", err)
			}
			fail, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "verilog", Query: target, Match: "exact", MaxFiles: 1, MaxResults: 8,
			})
			if err != nil || fail != nil || query.Search == nil ||
				len(query.Search.Matches) != 1 || query.Search.Matches[0].SymbolID != ids[target] {
				t.Fatalf("original structural query mismatch %v %+v", err, query)
			}
		})
	}
}
