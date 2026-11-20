package handler

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Opt-in fixtures must be byte-identical with two independent original
// projects. This tests the current local handler, not an installed MCP server.
func TestJinjaOriginalProjectMacroNavigationAndConciseJSON(t *testing.T) {
	cases := []struct {
		name, env, target, first, later, unrelated string
		line, endLine                              int
	}{
		{
			name: "Kubespray local volume provisioning configuration",
			env:  "SCRIPTHOLD_REAL_JINJA_KUBESPRAY", target: "convert_keys",
			first:     "mydict.__setitem__(new_key, value)",
			later:     "convert_keys(value) if value is mapping",
			unrelated: "storageClassMap", line: 2, endLine: 10,
		},
		{
			name: "DebOps environment facts case conversion",
			env:  "SCRIPTHOLD_REAL_JINJA_DEBOPS", target: "print_environment",
			first:     "environment__case == 'lower'",
			later:     "print_environment(widget)",
			unrelated: "environment__case", line: 5, endLine: 43,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_JINJA_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original Jinja source fixtures not configured")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "jinja", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || !digest.CoverageComplete ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Jinja source digest incomplete: error=%v output=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "jinja",
				MaxFiles: 1, MaxSymbols: 256, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("Jinja source outline incomplete: error=%v output=%+v", err, outline)
			}
			inOutline := false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("Jinja outline omitted real macro %s: %+v", tc.target, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "jinja",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 256, FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || len(found.Files) != 1 || found.Truncated {
				t.Fatalf("Jinja find incomplete: error=%v output=%+v", err, found)
			}
			symbolID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == "function" &&
					sym.NameRange.Start.Line == tc.line &&
					sym.DeclarationRange.End.Line == tc.endLine &&
					sym.BodyRange != nil {
					symbolID = sym.ID
				}
			}
			if symbolID == "" {
				t.Fatalf("Jinja macro missing full source body: %+v", found.Symbols)
			}
			identity := found.Files[0]
			if identity.SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				identity.Encoding == "" {
				t.Fatal("Jinja file fingerprint/encoding selector unavailable")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "jinja", Encoding: identity.Encoding,
				SymbolID: symbolID, SourceFingerprint: identity.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.first) ||
				!strings.Contains(shown.Show.Text, tc.later) {
				t.Fatalf("Jinja original body not shown exactly: error=%v show=%+v", err, shown.Show)
			}
			_, concise, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Presentation: "concise", Paths: []string{path},
				Language: "jinja", Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || len(concise.ConciseFiles) != 1 ||
				concise.ConciseFiles[0].SourceFingerprint != identity.SourceFingerprint ||
				concise.ConciseFiles[0].Encoding != identity.Encoding {
				t.Fatalf("concise JSON lost safe Jinja file selector: err=%v output=%+v", err, concise)
			}
			compactMatch := false
			for _, match := range concise.ConciseFiles[0].Matches {
				if match.Name == tc.target && match.SymbolID == symbolID {
					compactMatch = true
				}
			}
			if !compactMatch {
				t.Fatalf("concise JSON omitted original macro identity %s", tc.target)
			}
			_, conciseShown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Presentation: "concise", Part: "declaration",
				Path: path, Language: "jinja", Encoding: identity.Encoding,
				SymbolID: symbolID, SourceFingerprint: identity.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || conciseShown.ConciseShow == nil ||
				!conciseShown.ConciseShow.Complete ||
				!strings.Contains(conciseShown.ConciseShow.Code, tc.first) ||
				!strings.Contains(conciseShown.ConciseShow.Code, tc.later) {
				t.Fatalf("concise Jinja show lost source body: err=%v output=%+v", err, conciseShown.ConciseShow)
			}
			_, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Presentation: "concise",
				Paths: []string{path}, Language: "jinja", Query: tc.target,
				Match: "exact", MaxFiles: 1, MaxResults: 20,
			})
			if err != nil || queried.Search == nil || !queried.Coverage.CoverageComplete {
				t.Fatalf("Jinja concise structural search failed: error=%v result=%+v", err, queried)
			}
			queryMatched := false
			for _, match := range queried.Search.Matches {
				if match.Name == tc.target && match.SymbolID == symbolID &&
					match.SourceFingerprint == identity.SourceFingerprint {
					queryMatched = true
				}
			}
			if !queryMatched {
				t.Fatalf("Jinja concise structural search omitted source identity")
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "jinja",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated value invented as Jinja declaration: error=%v result=%+v", err, negative.Symbols)
			}
			conciseJSON, err := json.Marshal(concise)
			if err != nil {
				t.Fatal(err)
			}
			legacyJSON, err := json.Marshal(found)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("Jinja task %s: safe find JSON bytes concise=%d legacy=%d", tc.target, len(conciseJSON), len(legacyJSON))
		})
	}
}
