package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Two independent original JSON projects exercise exact navigation while
// honestly reporting unsupported object keys inside arrays.
func TestJSONOriginalProjectNavigationAndHonestArrayCoverage(t *testing.T) {
	tests := []struct {
		name, env, target, owner, sibling, siblingOwner, excerpt, oracle, absent string
		line, siblingLine                                                        int
	}{
		{
			name:   "GitHub check-run webhook annotations URL",
			env:    "SCRIPTHOLD_REAL_JSON_WEBHOOK",
			target: "annotations_url", owner: "check_run.output", line: 20,
			sibling: "annotations_count", siblingOwner: "check_run.output", siblingLine: 19,
			excerpt: "\"annotations_url\": \"https://api.github.com/",
			oracle:  "\"pull_requests\": [", absent: "_enableSearch",
		},
		{
			name:   "Polly DocFX search configuration",
			env:    "SCRIPTHOLD_REAL_JSON_DOCFX",
			target: "_enableSearch", owner: "build.globalMetadata", line: 36,
			sibling: "_lang", siblingOwner: "build.globalMetadata", siblingLine: 37,
			excerpt: "\"_enableSearch\": true",
			oracle:  "\"metadata\": [", absent: "annotations_url",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_JSON_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original JSON project not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original JSON fixture partially configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("original JSON source oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "json", MaxFiles: 1,
			})
			if err != nil || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("JSON digest unavailable: err=%v result=%+v", err, digest)
			}
			if digest.CoverageComplete {
				t.Fatalf("JSON object arrays were falsely declared fully indexed: %+v", digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "json",
				MaxFiles: 1, MaxSymbols: 256, Presentation: "detailed",
			})
			if err != nil || outline.FilesParsed != 1 || outline.Truncated || outline.CoverageComplete {
				t.Fatalf("JSON outline misleading or unusable: err=%v result=%+v", err, outline)
			}
			targetOK, siblingOK := false, false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.line {
					targetOK = true
				}
				if sym.Name == tc.sibling && sym.ParentQualifiedName == tc.siblingOwner &&
					sym.NameRange.Start.Line == tc.siblingLine {
					siblingOK = true
				}
			}
			if !targetOK || !siblingOK {
				t.Fatalf("JSON original target or related key missing: target=%v sibling=%v", targetOK, siblingOK)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "json",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || len(found.Files) != 1 || found.Truncated || found.CoverageComplete {
				t.Fatalf("JSON find missing honest partial status: err=%v found=%+v", err, found)
			}
			targetID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.ParentQualifiedName == tc.owner &&
					sym.NameRange.Start.Line == tc.line {
					targetID = sym.ID
				}
			}
			file := found.Files[0]
			if targetID == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("JSON exact selector or source fingerprint unavailable: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "json",
				Encoding: file.Encoding, SymbolID: targetID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("JSON exact source excerpt incorrect: err=%v show=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "json", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated {
				t.Fatalf("JSON structural source search failed: err=%v search=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == targetID && hit.SourceFingerprint == file.SourceFingerprint &&
					hit.Range.Start.Line == tc.line {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("JSON source query is not linked to safe edit target: %+v", search.Search)
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "json",
				Query: tc.absent, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || len(unrelated.Symbols) != 0 {
				t.Fatalf("unrelated JSON key fabricated: err=%v symbols=%+v", err, unrelated.Symbols)
			}
		})
	}
}
