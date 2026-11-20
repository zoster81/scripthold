package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Opt-in source snapshots match the original projects byte-for-byte.
// Their wider project tests and neighboring modules are inspected independently.
func TestGleamOriginalFunctionBodyNavigation(t *testing.T) {
	cases := []struct {
		name, env, query, body, neighbor string
		start, end                       int
	}{
		{
			name:     "http-method-token-validation",
			env:      "SCRIPTHOLD_REAL_GLEAM_HTTP",
			query:    "parse_method",
			body:     "is_valid_token(method)",
			neighbor: "is_valid_token_loop",
			start:    31, end: 48,
		},
		{
			name:     "otp-actor-startup-timeout",
			env:      "SCRIPTHOLD_REAL_GLEAM_OTP",
			query:    "start",
			body:     "Error(InitTimeout)",
			neighbor: "loop",
			start:    592, end: 636,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("original project fixture not configured")
			}
			h := NewHandler([]string{os.Getenv("SCRIPTHOLD_REAL_GLEAM_ROOT")})
			// The supplied file must be inside the explicitly authorized original corpus.
			tool, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "gleam", Query: tc.query,
				Match: "exact", MaxFiles: 1, MaxSymbols: 24, FileDetails: "relevant",
			})
			if err != nil || tool != nil || found.FilesParsed != 1 || !found.CoverageComplete || found.Truncated {
				t.Fatalf("Gleam find incomplete: tool=%v err=%v files=%d", tool, err, found.FilesParsed)
			}
			var id string
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.query && symbol.NameRange.Start.Line == tc.start &&
					symbol.DeclarationRange.End.Line == tc.end && symbol.BodyRange != nil {
					id = symbol.ID
				}
			}
			if id == "" || len(found.Files) != 1 {
				t.Fatalf("missing complete original body %s; found=%+v", tc.query, found.Symbols)
			}
			fingerprint, encoding := found.Files[0].SourceFingerprint, found.Files[0].Encoding
			if fingerprint == "" || encoding == "" {
				t.Fatal("missing fingerprint/encoding for exact source")
			}
			showTool, show, showErr := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "gleam", Encoding: encoding,
				SymbolID: id, SourceFingerprint: fingerprint, MaxBytes: 20000,
			})
			if showErr != nil || showTool != nil || show.Show == nil || !strings.Contains(show.Show.Text, tc.body) {
				t.Fatalf("source show omitted task body: tool=%v err=%v", showTool, showErr)
			}
			queryTool, query, queryErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "gleam",
				Query: tc.query, Match: "exact", MaxFiles: 1, MaxResults: 20,
			})
			if queryErr != nil || (queryTool != nil && queryTool.IsError) || query.Search == nil ||
				query.Coverage.FilesParsed != 1 || query.Coverage.ResultsTruncated {
				t.Fatalf("source_query search incomplete: tool=%v err=%v", queryTool, queryErr)
			}
			selected := false
			for _, match := range query.Search.Matches {
				if match.Name == tc.query && match.SymbolID == id && match.SourceFingerprint == fingerprint {
					selected = true
				}
			}
			if !selected {
				t.Fatalf("source_query search lost %s", tc.query)
			}
			otherTool, other, otherErr := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "gleam", Query: tc.neighbor,
				Match: "exact", MaxFiles: 1, MaxSymbols: 20,
			})
			if otherErr != nil || otherTool != nil || len(other.Symbols) != 1 ||
				other.Symbols[0].Name != tc.neighbor {
				t.Fatalf("neighbor declaration missing %s: tool=%v err=%v", tc.neighbor, otherTool, otherErr)
			}
		})
	}
}
