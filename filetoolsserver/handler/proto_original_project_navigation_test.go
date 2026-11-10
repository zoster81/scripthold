package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Two independently checked-out original Protocol Buffers schemas validate
// maintenance navigation without running protobuf generators.
func TestProtoOriginalProjectNestedMessageNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, owner, excerpt, oracle, sibling, siblingOwner, unrelated string
		line, siblingLine                                                           int
		forbidden                                                                   []string
	}{
		{
			name:   "Hex package release publish metadata",
			env:    "SCRIPTHOLD_REAL_PROTO_GLEAM",
			target: "published_at", owner: "package.Release", line: 37,
			excerpt: "optional Timestamp published_at = 7",
			oracle:  "repeated uint32 advisory_indexes = 6",
			sibling: "outer_checksum", siblingOwner: "package.Release", siblingLine: 32,
			unrelated: "token_type",
			forbidden: []string{"package.published_at", "package.Package.published_at"},
		},
		{
			name:   "Sonar nested token type within its enclosing message",
			env:    "SCRIPTHOLD_REAL_PROTO_SONAR",
			target: "token_type", owner: "sonaranalyzer.TokenTypeInfo.TokenInfo", line: 53,
			excerpt: "TokenType token_type = 1",
			oracle:  "repeated TokenInfo token_info = 2",
			sibling: "token_info", siblingOwner: "sonaranalyzer.TokenTypeInfo", siblingLine: 57,
			unrelated: "published_at",
			forbidden: []string{
				"sonaranalyzer.TokenInfo", "sonaranalyzer.TokenInfo.token_type",
				"sonaranalyzer.TokenTypeInfo.token_type",
				"sonaranalyzer.CopyPasteTokenInfo.token_value",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_PROTO_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("original Protocol Buffers project is not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original Protocol Buffers project is only partially configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("cannot read original Protocol Buffers source: %v", err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) {
				t.Fatal("Protocol Buffers original source oracle mismatch")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "proto", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Protocol Buffers digest unusable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "proto",
				MaxFiles: 1, MaxSymbols: 120, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original Protocol Buffers outline incomplete: err=%v outline=%+v", err, outline)
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
				for _, forbidden := range tc.forbidden {
					if sym.QualifiedName == forbidden {
						t.Fatalf("fabricated original Protocol Buffers owner: %+v", sym)
					}
				}
			}
			if !targetOK || !siblingOK {
				t.Fatalf("original schema navigation target/sibling absent: target=%v sibling=%v", targetOK, siblingOK)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "proto",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 60,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original protobuf find incomplete: err=%v result=%+v", err, found)
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
				t.Fatalf("original protobuf exact source selector unavailable: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "proto",
				Encoding: file.Encoding, SymbolID: targetID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("original protobuf source excerpt inaccurate: err=%v shown=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "proto", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 60,
			})
			if err != nil || search.Search == nil || !search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated {
				t.Fatalf("original protobuf structural query incomplete: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.SymbolID == targetID && hit.SourceFingerprint == file.SourceFingerprint &&
					hit.Range.Start.Line == tc.line {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("protobuf structural query lacks target identity: %+v", search.Search)
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "proto",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 40,
			})
			if err != nil || !unrelated.CoverageComplete || len(unrelated.Symbols) != 0 {
				t.Fatalf("protobuf unrelated cross-project name falsely indexed: err=%v symbols=%+v", err, unrelated.Symbols)
			}
		})
	}
}
