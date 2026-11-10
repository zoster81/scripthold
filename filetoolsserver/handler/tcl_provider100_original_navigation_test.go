package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Two independent Git original projects validate local MCP source navigation.
// Their source bytes are read only; the Tcl samples are never executed.
func TestTclProvider100OriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, anchor, oracle, neighbor string
		targetLine, neighborLine                    int
		sameName                                    bool
	}{
		{
			name: "KDE optional argument procedure versus varargs shadow",
			env:  "SCRIPTHOLD_REAL_TCL100_KDE", target: "test",
			anchor: "proc test {arg1 {arg2 {}} {arg3 {fr fq r}}}",
			oracle: "set String \\{;", neighbor: "test",
			targetLine: 22, neighborLine: 28, sameName: true,
		},
		{
			name:     "highlightjs namespaced recursive counter versus prime test",
			env:      "SCRIPTHOLD_REAL_TCL100_HIGHLIGHT",
			target:   "give::recursive::count",
			anchor:   "proc give::recursive::count {base p}",
			oracle:   "set result [expr $result * $base]",
			neighbor: "isprime", targetLine: 7, neighborLine: 24,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_TCL100_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independently verified Tcl original not configured")
			}
			if root == "" || path == "" {
				t.Fatal("Tcl original path and allowed root must both be configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Tcl source must be a bounded regular file: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.anchor) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original Tcl source maintenance oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "tcl", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Tcl digest unavailable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "tcl",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated || !outline.CoverageComplete {
				t.Fatalf("Tcl source scope not fully navigable: err=%v result=%+v", err, outline)
			}
			targetID, siblingID := "", ""
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == "function" &&
					sym.NameRange.Start.Line == tc.targetLine {
					targetID = sym.ID
				}
				if sym.Name == tc.neighbor && string(sym.Kind) == "function" &&
					sym.NameRange.Start.Line == tc.neighborLine {
					siblingID = sym.ID
				}
				if sym.Name == "String" || sym.Name == "set" ||
					sym.Name == "helper.tcl" || sym.Name == "rand" {
					t.Errorf("data, command or dependency misidentified as Tcl proc: %+v", sym)
				}
			}
			if targetID == "" || siblingID == "" || targetID == siblingID {
				t.Fatalf("source target or competing procedure missing: target=%q sibling=%q symbols=%+v", targetID, siblingID, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "tcl",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("exact Tcl find failed: err=%v result=%+v", err, found)
			}
			seenTarget, seenSibling := false, false
			for _, sym := range found.Symbols {
				if sym.ID == targetID && sym.NameRange.Start.Line == tc.targetLine {
					seenTarget = true
				}
				if sym.ID == siblingID && sym.NameRange.Start.Line == tc.neighborLine {
					seenSibling = true
				}
			}
			file := found.Files[0]
			if !seenTarget || (tc.sameName && !seenSibling) ||
				file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Tcl source identity failed exact name disambiguation: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "tcl",
				Encoding: file.Encoding, SymbolID: targetID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.anchor) {
				t.Fatalf("original Tcl fingerprint-protected excerpt unavailable: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "tcl", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 128,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("Tcl structural search failed: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == targetID && hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Tcl structural search did not link exact source: %+v", search.Search)
			}
		})
	}
}
