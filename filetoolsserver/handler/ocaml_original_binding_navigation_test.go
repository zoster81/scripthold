package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// The opt-in OCaml files are exact tracked originals from independent projects.
// Their neighboring interfaces and tests are checked separately against Git;
// source navigation does not claim complete expression or call semantics.
func TestOCamlOriginalProjectBindingNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, kind, excerpt, sibling, unrelated string
		line, siblingLine                                    int
	}{
		{
			name:   "OCaml standard-library Option syntax operators",
			env:    "SCRIPTHOLD_REAL_OCAML_STDLIB",
			target: "let*", kind: "variable", line: 65,
			excerpt: "let ( let* ) = bind",
			sibling: "get", siblingLine: 21,
			unrelated: "go",
		},
		{
			name:   "Exercism binary search bounds and empty array",
			env:    "SCRIPTHOLD_REAL_OCAML_EXERCISM",
			target: "find", kind: "function", line: 3,
			excerpt: "let find xs value =",
			sibling: "go", siblingLine: 4,
			unrelated: "bind",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_OCAML_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original OCaml file fixtures not configured")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "ocaml", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("OCaml original digest incomplete: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "ocaml",
				MaxFiles: 1, MaxSymbols: 128, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("OCaml original outline incomplete: err=%v outline=%+v", err, outline)
			}
			present := false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line {
					present = true
				}
			}
			if !present {
				t.Fatalf("original OCaml binding %q absent from outline", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ocaml",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("OCaml original find incomplete: err=%v found=%+v", err, found)
			}
			selected := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == tc.kind &&
					sym.NameRange.Start.Line == tc.line {
					selected = sym.ID
				}
			}
			if selected == "" {
				t.Fatalf("OCaml original target %q missing or wrong kind: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				file.Encoding == "" {
				t.Fatal("OCaml safe exact-source selector missing")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ocaml", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("OCaml exact source not retrievable: err=%v shown=%+v", err, shown.Show)
			}
			_, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "ocaml", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 30,
			})
			if err != nil || queried.Search == nil || !queried.Coverage.CoverageComplete ||
				queried.Coverage.ResultsTruncated {
				t.Fatalf("OCaml search incomplete: err=%v query=%+v", err, queried)
			}
			searchHit := false
			for _, match := range queried.Search.Matches {
				if match.Name == tc.target && match.SymbolID == selected &&
					match.SourceFingerprint == file.SourceFingerprint {
					searchHit = true
				}
			}
			if !searchHit {
				t.Fatalf("OCaml structural query omitted exact source identity %q", tc.target)
			}
			if tc.target == "let*" {
				_, types, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "ocaml",
					Query: "t", Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				})
				if err != nil || !types.CoverageComplete || len(types.Symbols) != 1 ||
					types.Symbols[0].NameRange.Start.Line != 16 ||
					string(types.Symbols[0].Kind) != "type" {
					t.Fatalf("actual Option type t hidden by type variable: err=%v symbols=%+v", err, types.Symbols)
				}
				_, fakeType, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "ocaml",
					Query: "a", Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				})
				if err != nil || !fakeType.CoverageComplete || len(fakeType.Symbols) != 0 {
					t.Fatalf("OCaml type parameter fabricated as declaration: err=%v symbols=%+v", err, fakeType.Symbols)
				}
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ocaml",
				Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			related := false
			if err == nil && sibling.CoverageComplete {
				for _, sym := range sibling.Symbols {
					if sym.Name == tc.sibling && sym.NameRange.Start.Line == tc.siblingLine {
						related = true
					}
				}
			}
			if !related {
				t.Fatalf("OCaml real adjacent binding %s missing: err=%v found=%+v", tc.sibling, err, sibling.Symbols)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ocaml",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated OCaml binding fabricated: err=%v symbols=%+v", err, negative.Symbols)
			}
		})
	}
}
