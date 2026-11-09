package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Source snapshots are independently checked against tracked, unexecuted EJS
// projects. Related server routes, client validation, and alternative templates
// are inspected in their original Git trees rather than inferred from markup.
func TestEJSOriginalProjectTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, sibling, excerpt, unrelated string
		line, siblingLine                              int
	}{
		{
			name:   "Todo removal form and route",
			env:    "SCRIPTHOLD_REAL_EJS_TODO",
			target: "top", line: 24,
			excerpt:   "formaction=\"/removetask\"",
			unrelated: "newProductForm",
		},
		{
			name:   "Ecommerce tag field and form binding",
			env:    "SCRIPTHOLD_REAL_EJS_ECOMMERCE",
			target: "productTags", line: 289,
			sibling: "newProductForm", siblingLine: 123,
			excerpt:   "name=\"tags\"",
			unrelated: "addProductCopy",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_EJS_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original EJS file fixture not configured")
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "ejs", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("EJS digest incomplete: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "ejs",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 100,
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("EJS outline incomplete: err=%v outline=%+v", err, outline)
			}
			inOutline := false
			for _, symbol := range outline.Symbols {
				if symbol.Name == tc.target && symbol.NameRange.Start.Line == tc.line {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("EJS relevant form entity %q omitted from outline", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ejs",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 30, FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				len(found.Files) != 1 {
				t.Fatalf("EJS find incomplete: err=%v found=%+v", err, found)
			}
			selected := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.target && string(symbol.Kind) == "entity" &&
					symbol.NameRange.Start.Line == tc.line {
					selected = symbol.ID
				}
			}
			if selected == "" {
				t.Fatalf("EJS exact entity %q missing: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				file.Encoding == "" {
				t.Fatal("EJS missing exact source file identity")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ejs", Encoding: file.Encoding,
				SymbolID: selected, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("EJS source excerpt does not contain %q: err=%v show=%+v",
					tc.excerpt, err, shown.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "ejs", Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 30,
			})
			if err != nil || searched.Search == nil || !searched.Coverage.CoverageComplete ||
				searched.Coverage.ResultsTruncated {
				t.Fatalf("EJS structural search incomplete: err=%v result=%+v", err, searched)
			}
			matched := false
			for _, match := range searched.Search.Matches {
				if match.Name == tc.target && match.SymbolID == selected &&
					match.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("EJS structural search omitted exact entity %s", tc.target)
			}
			if tc.sibling != "" {
				_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "ejs",
					Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
				})
				if err != nil || len(sibling.Symbols) != 1 ||
					sibling.Symbols[0].NameRange.Start.Line != tc.siblingLine ||
					string(sibling.Symbols[0].Kind) != "entity" {
					t.Fatalf("EJS related form entity %s missing: err=%v found=%+v",
						tc.sibling, err, sibling)
				}
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ejs",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 30,
			})
			if err != nil || !unrelated.CoverageComplete || len(unrelated.Symbols) != 0 {
				t.Fatalf("unrelated EJS project symbol promoted: err=%v found=%+v", err, unrelated.Symbols)
			}
		})
	}
}
