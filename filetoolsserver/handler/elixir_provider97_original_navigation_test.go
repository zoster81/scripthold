package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Original Elixir and KDE sources are independently Git-verified.
// The assertions cover navigation to a maintenance target, not code execution.
func TestElixirProvider97OriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, excerpt, oracle, neighbor, neighborOwner string
		targetLine, neighborLine                                           int
	}{
		{
			name: "Elixir Atom charlist conversion", env: "SCRIPTHOLD_REAL_ELIXIR_ATOM97",
			target: "to_charlist", owner: "Atom", targetLine: 78,
			excerpt: "def to_charlist(atom) do", oracle: ":erlang.atom_to_list(atom)",
			neighbor: "to_char_list", neighborOwner: "Atom", neighborLine: 85,
		},
		{
			name: "KDE Geometry circle area vs rectangle", env: "SCRIPTHOLD_REAL_ELIXIR_KDE97",
			target: "area", owner: "Geometry", targetLine: 273,
			excerpt: "def area({:circle, r})", oracle: "3.14 * r * r",
			neighbor: "area", neighborOwner: "Geometry", neighborLine: 269,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_ELIXIR_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original Elixir project not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original Elixir project only partially configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("unbounded/nonregular original source: info=%v err=%v", info, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(original), tc.excerpt) || !strings.Contains(string(original), tc.oracle) {
				t.Fatal("source-first Elixir maintenance oracle mismatch")
			}
			h, ctx := NewHandler([]string{root}), context.Background()
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "elixir", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Elixir digest unusable: err=%v result=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "elixir",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated {
				t.Fatalf("Elixir original source outline unusable: err=%v result=%+v", err, outline)
			}
			selected, sibling := "", ""
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.targetLine {
					selected = sym.ID
				}
				if sym.Name == tc.neighbor && sym.ParentQualifiedName == tc.neighborOwner &&
					sym.NameRange.Start.Line == tc.neighborLine {
					sibling = sym.ID
				}
			}
			if selected == "" || sibling == "" || selected == sibling {
				t.Fatalf("edit target or distinct sibling/overload missing: target=%q sibling=%q symbols=%+v", selected, sibling, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "elixir",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Elixir source find failed: err=%v result=%+v", err, found)
			}
			foundSelected := false
			for _, sym := range found.Symbols {
				if sym.ID == selected && sym.NameRange.Start.Line == tc.targetLine {
					foundSelected = true
				}
			}
			file := found.Files[0]
			if !foundSelected || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Elixir selector identity inconsistent: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "elixir",
				Encoding: file.Encoding, SymbolID: selected,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Elixir original source excerpt unavailable: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "elixir", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 128,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("Elixir structural search failed: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == selected && hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("Elixir search cannot navigate exact overload: %+v", search.Search)
			}
		})
	}
}
