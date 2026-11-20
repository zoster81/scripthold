package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Complete independent declaration inventory from two Git-pinned upstream originals.
// Procedural statements, macro calls, and typed AXI interface ports are excluded.
func TestSystemVerilogTwoWholePinnedOriginalOutlines(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SYSTEMVERILOG_OUTLINE_ROOT")
	if root == "" {
		t.Skip("originals not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		records    string
	}{{file: "ibex_csr.sv", size: 1289, blob: "309a325f8b0bfd0bcf5e9ed219c4a2da24182b77", records: "ibex_csr|11|module|module||8|16\nWidth|12|constant|parameter|ibex_csr|29|34\nShadowCopy|13|constant|parameter|ibex_csr|29|39\nResetValue|14|constant|parameter|ibex_csr|29|39\nclk_i|16|signal|signal|ibex_csr|28|33\nrst_ni|17|signal|signal|ibex_csr|28|34\nwr_data_i|19|signal|signal|ibex_csr|28|37\nwr_en_i|20|signal|signal|ibex_csr|28|35\nrd_data_o|21|signal|signal|ibex_csr|28|37\nrd_error_o|23|signal|signal|ibex_csr|28|38\nrdata_q|26|signal|signal|ibex_csr|21|28\nshadow_q|39|signal|signal|ibex_csr|23|31"},
		{file: "axi_join.sv", size: 1264, blob: "f15648ebe19fb28fc61dd3ea0211a9e8c5ccdb8d", records: "axi_join_intf|19|module|module||8|21"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			stat, err := os.Lstat(path)
			if err != nil || !stat.Mode().IsRegular() || stat.Size() != tc.size {
				t.Fatalf("whole source changed: stat=%+v err=%v", stat, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(b), 0)
			_, _ = hash.Write(b)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("raw Git blob differs")
			}
			entries := strings.Split(tc.records, "\n")
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 80})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != len(entries) {
				t.Fatalf("full independently authored inventory want=%d got=%d err=%v failed=%+v", len(entries), len(out.Symbols), err, failed)
			}
			found := map[string]int{}
			ids := map[string]string{}
			for i, s := range out.Symbols {
				if _, ok := found[s.Name]; ok {
					t.Errorf("duplicate %s", s.Name)
				}
				found[s.Name] = i
				if s.ID == "" {
					t.Errorf("empty ID %+v", s)
				}
				ids[s.Name] = s.ID
			}
			for _, entry := range entries {
				f := strings.Split(entry, "|")
				if len(f) != 7 {
					t.Fatalf("invalid source-first row %s", entry)
				}
				line, _ := strconv.Atoi(f[1])
				col, _ := strconv.Atoi(f[5])
				end, _ := strconv.Atoi(f[6])
				i, ok := found[f[0]]
				if !ok {
					t.Errorf("missing original supported symbol %s", entry)
					continue
				}
				s := out.Symbols[i]
				qualified := f[0]
				if f[4] != "" {
					qualified = f[4] + "." + f[0]
				}
				if s.Name != f[0] || string(s.Kind) != f[2] || s.NativeKind != f[3] || s.QualifiedName != qualified || s.ParentQualifiedName != f[4] || s.NameRange.Start.Line != line || s.NameRange.Start.Column != col || s.NameRange.End.Line != line || s.NameRange.End.Column != end || s.DeclarationRange.Start.Line != line || s.DeclarationRange.End.Line < line {
					t.Errorf("source-first mismatch want=%s got=%+v", entry, s)
				}
				if f[4] == "" && s.ParentID != "" {
					t.Errorf("unexpected module parent %+v", s)
				}
				if f[4] != "" && s.ParentID != ids[f[4]] {
					t.Errorf("incorrect parent ID %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("original fingerprint missing")
			}
			module := strings.Split(entries[0], "|")[0]
			failed, exact, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8", Query: module, Match: "exact", MaxFiles: 1, MaxSymbols: 80})
			if err != nil || failed != nil || exact.Truncated || len(exact.Symbols) != 1 || exact.Symbols[0].ID != ids[module] {
				t.Fatalf("original exact find: err=%v failed=%+v result=%+v", err, failed, exact)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "systemverilog", Encoding: "utf-8", SymbolID: ids[module], SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, module) {
				t.Fatalf("original fingerprint show: err=%v failed=%+v show=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8", Query: module, Match: "exact", MaxFiles: 1, MaxResults: 80})
			if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) != 1 || query.Search.Matches[0].SymbolID != ids[module] {
				t.Fatalf("original structural query: err=%v failed=%+v result=%+v", err, failed, query)
			}
			for _, neg := range []string{"always_ff", "AXI_ASSIGN", "ASSERT_KNOWN", "gen_shadow"} {
				failed, x, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "systemverilog", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 80})
				if err != nil || failed != nil || len(x.Symbols) != 0 {
					t.Errorf("invented unsupported macro/statement %s: err=%v failed=%+v", neg, err, failed)
				}
			}
		})
	}
}
