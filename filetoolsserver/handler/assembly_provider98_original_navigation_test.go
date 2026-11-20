package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Original Git repository sources have been verified independently; these
// checks exercise only their selected source scopes through local MCP handlers.
func TestAssemblyProvider98OriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, targetKind, excerpt, evidence, neighbor, neighborKind string
		targetLine, neighborLine                                                 int
	}{
		{
			name: "FASM command-line argument parsing", env: "SCRIPTHOLD_REAL_ASM98_FASM",
			target: "get_params", targetKind: "label", targetLine: 78,
			excerpt: "get_params:", evidence: "call\tget_params",
			neighbor: "find_param", neighborKind: "label", neighborLine: 107,
		},
		{
			name: "NASM message length calculation", env: "SCRIPTHOLD_REAL_ASM98_KDE",
			target: "MessageLength", targetKind: "constant", targetLine: 39,
			excerpt: "MessageLength  EQU $-Message", evidence: "mov   R8, MessageLength",
			neighbor: "Message", neighborKind: "variable", neighborLine: 38,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_ASM98_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("two original Assembly source projects not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original Assembly file and root must both be configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("selected original Assembly source must be bounded and regular: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.evidence) {
				t.Fatal("source-first original Assembly maintenance oracle differs")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "assembly", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Assembly digest unusable: err=%v result=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "assembly",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated || !outline.CoverageComplete {
				t.Fatalf("selected Assembly scope incomplete: err=%v result=%+v", err, outline)
			}
			selected, neighbor := "", ""
			startLabels := map[int]string{}
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == tc.targetKind &&
					sym.NameRange.Start.Line == tc.targetLine {
					selected = sym.ID
				}
				if sym.Name == tc.neighbor && string(sym.Kind) == tc.neighborKind &&
					sym.NameRange.Start.Line == tc.neighborLine {
					neighbor = sym.ID
				}
				if sym.Name == "Start" && sym.Kind == "label" {
					startLabels[sym.NameRange.Start.Line] = sym.ID
				}
				if sym.Name == "GetTickCount" || sym.Name == "include" || sym.Name == "mov" {
					t.Errorf("instruction, external call or include fabricated as symbol: %+v", sym)
				}
			}
			if selected == "" || neighbor == "" || selected == neighbor {
				t.Fatalf("original edit target or distinct neighbor missing: target=%q neighbor=%q symbols=%+v", selected, neighbor, outline.Symbols)
			}
			if tc.target == "MessageLength" && (startLabels[46] == "" || startLabels[71] == "" || startLabels[46] == startLabels[71]) {
				t.Errorf("ambiguous original Start labels lack distinct source identities: %+v", startLabels)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "assembly",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 512, FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("targeted Assembly lookup failed: err=%v found=%+v", err, found)
			}
			seen := false
			for _, sym := range found.Symbols {
				if sym.ID == selected && sym.NameRange.Start.Line == tc.targetLine {
					seen = true
				}
			}
			file := found.Files[0]
			if !seen || file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Assembly exact selector missing: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "assembly", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("original Assembly source excerpt missing: err=%v show=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "assembly", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 256,
			})
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated {
				t.Fatalf("Assembly structural lookup unavailable: err=%v result=%+v", err, query)
			}
			linked := false
			for _, hit := range query.Search.Matches {
				if hit.SymbolID == selected && hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("structural lookup not linked to original Assembly source: %+v", query.Search)
			}
		})
	}
}
