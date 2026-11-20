package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Local MCP navigation is qualified against two entire, independently pinned
// VHDL originals; no compiled artifacts or connected server are involved.
func TestVHDLPinnedWholeOriginalSignalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VHDL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete VHDL original projects are not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, name, declaration string
		count, line, column     int
	}{
		{"full-adder.vhd", "sig_tmp_0", "signal sig_tmp_0 : std_logic;", 4, 35, 12},
		{"tt09-counter.vhdl", "second_counter", "signal second_counter : std_logic_vector(23 downto 0) := (others => '0');", 9, 28, 12},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("whole VHDL original unavailable or unsafe: %v %v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "vhdl", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.count {
				t.Fatalf("original full detailed VHDL outline: err=%v failed=%+v symbols=%+v", err, failed, outline.Symbols)
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing original source fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "vhdl", Encoding: "utf-8",
				Query: tc.name, Match: "exact", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 {
				t.Fatalf("original source exact signal find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
			}
			sym := found.Symbols[0]
			if sym.Name != tc.name || sym.Kind != "signal" || sym.NativeKind != "signal" ||
				sym.NameRange.Start.Line != tc.line || sym.NameRange.Start.Column != tc.column ||
				sym.SignatureRange == nil || !strings.Contains(sym.Signature, tc.declaration) {
				t.Fatalf("original complete signal signature and name: %+v", sym)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vhdl", Encoding: "utf-8",
				SymbolID: sym.ID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.declaration) {
				t.Fatalf("fingerprint-bound original source show: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "vhdl", Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxResults: 256,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 {
				t.Fatalf("original VHDL structural search: err=%v failed=%+v", err, failed)
			}
		})
	}
}
