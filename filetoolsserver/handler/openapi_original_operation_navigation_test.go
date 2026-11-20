package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Two independent original OpenAPI documents are compared with their tracked
// server implementation files; operationId locates a route contract, not code.
func TestOpenAPIOriginalOperationTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, sibling, absent string
		line, siblingLine                  int
	}{
		{
			name:   "Connexion API key protected secret operation",
			env:    "SCRIPTHOLD_REAL_OPENAPI_CONNEXION",
			target: "app.get_secret", line: 11,
			absent: "app.apikey_auth",
		},
		{
			name:   "Swagger Petstore pet update operation",
			env:    "SCRIPTHOLD_REAL_OPENAPI_PETSTORE",
			target: "updatePet", line: 45,
			sibling: "addPet", siblingLine: 86,
			absent: "app.get_secret",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_OPENAPI_ROOT"), os.Getenv(tc.env)
			if root == "" || path == "" {
				t.Skip("original OpenAPI fixture not configured")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "openapi", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original OpenAPI overview incomplete: err=%v result=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "openapi",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("original OpenAPI outline incomplete: err=%v result=%+v", err, outline)
			}
			inOutline := false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("OpenAPI operation %q missing from outline", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "openapi",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				len(found.Files) != 1 {
				t.Fatalf("OpenAPI find incomplete: err=%v result=%+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == "operation" &&
					sym.NameRange.Start.Line == tc.line {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("original OpenAPI operation missing: %+v", found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("OpenAPI safe source selector mismatch")
			}
			_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "openapi", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || show.Show == nil ||
				!strings.Contains(show.Show.Text, "operationId: "+tc.target) {
				t.Fatalf("OpenAPI source declaration unavailable: err=%v show=%+v", err, show.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "openapi", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 20,
			})
			if err != nil || search.Search == nil || !search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated {
				t.Fatalf("OpenAPI structural search incomplete: err=%v result=%+v", err, search)
			}
			matched := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == id && hit.Name == tc.target &&
					hit.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("OpenAPI structural search omitted %q", tc.target)
			}
			if tc.sibling != "" {
				_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "openapi",
					Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
				})
				if err != nil || len(sibling.Symbols) != 1 ||
					sibling.Symbols[0].NameRange.Start.Line != tc.siblingLine ||
					string(sibling.Symbols[0].Kind) != "operation" {
					t.Fatalf("OpenAPI related operation %s missing: err=%v found=%+v", tc.sibling, err, sibling)
				}
			}
			_, absent, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "openapi",
				Query: tc.absent, Match: "exact", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || !absent.CoverageComplete || len(absent.Symbols) != 0 {
				t.Fatalf("OpenAPI unrelated implementation falsely declared: err=%v found=%+v", err, absent.Symbols)
			}
		})
	}
}
