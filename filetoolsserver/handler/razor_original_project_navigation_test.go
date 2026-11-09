package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Selected originals are byte-identical with independent tracked Razor projects.
// Related models, controllers, tests and alternative templates are checked via
// original source; these opt-in tests do not execute third-party applications.
func TestRazorOriginalProjectTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, kind, excerpt, sibling, siblingKind, absent string
		line, siblingLine                                              int
	}{
		{
			name:    "ABP multitenancy error page model",
			env:     "SCRIPTHOLD_REAL_RAZOR_ABP",
			target:  "MultiTenancyMiddlewareErrorPage",
			kind:    "method",
			excerpt: "Model = model;",
			sibling: "Model", siblingKind: "property", absent: "BasicAuthLoginModel",
			line: 11, siblingLine: 16,
		},
		{
			name:    "Umbraco login error presentation",
			env:     "SCRIPTHOLD_REAL_RAZOR_UMBRACO",
			target:  ".error-message",
			kind:    "selector",
			excerpt: ".error-message",
			sibling: "login-error", siblingKind: "entity",
			absent: "MultiTenancyMiddlewareErrorPage",
			line:   95, siblingLine: 128,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, root := os.Getenv(tc.env), os.Getenv("SCRIPTHOLD_REAL_RAZOR_ROOT")
			if path == "" || root == "" {
				t.Skip("original Razor fixture not configured")
			}
			h := NewHandler([]string{root})
			ctx := context.Background()
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "razor", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("Razor source overview incomplete: error=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "razor",
				MaxFiles: 1, MaxSymbols: 65, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("Razor source outline incomplete: error=%v outline=%+v", err, outline)
			}
			inOutline := false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line {
					inOutline = true
				}
			}
			if !inOutline {
				t.Fatalf("Razor outline omitted %s", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "razor",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 25, FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				found.FilesParsed != 1 || len(found.Files) != 1 {
				t.Fatalf("Razor find incomplete: error=%v result=%+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == tc.kind &&
					sym.NameRange.Start.Line == tc.line {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("Razor structural target %q not found: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.SourceFingerprint == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Razor fingerprint/encoding mismatch")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "razor", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("Razor exact source unavailable: error=%v show=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "razor",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 25,
			})
			if err != nil || query.Search == nil || !query.Coverage.CoverageComplete ||
				query.Coverage.ResultsTruncated {
				t.Fatalf("Razor structural search incomplete: error=%v result=%+v", err, query)
			}
			matched := false
			for _, match := range query.Search.Matches {
				if match.Name == tc.target && match.SymbolID == id &&
					match.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("Razor structural query omitted %q", tc.target)
			}
			_, sibling, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "razor",
				Query: tc.sibling, Match: "exact", MaxFiles: 1, MaxSymbols: 25,
			})
			if err != nil || len(sibling.Symbols) != 1 ||
				sibling.Symbols[0].NameRange.Start.Line != tc.siblingLine ||
				string(sibling.Symbols[0].Kind) != tc.siblingKind {
				t.Fatalf("Razor related member %q missing or wrong: error=%v result=%+v", tc.sibling, err, sibling)
			}
			_, other, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "razor",
				Query: tc.absent, Match: "exact", MaxFiles: 1, MaxSymbols: 25,
			})
			if err != nil || !other.CoverageComplete || len(other.Symbols) != 0 {
				t.Fatalf("Razor unrelated symbol %q invented: error=%v result=%+v", tc.absent, err, other)
			}
		})
	}
}
