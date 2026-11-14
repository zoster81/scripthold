package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These independently sourced TypeScript files distinguish a generic arrow
// starting after '=' from an unrelated constant string using the same layout.
func TestTypeScriptOriginalMultilineInitializers(t *testing.T) {
	cases := []struct {
		name, env, file, symbol, lineText, continuation string
		line                                            int
		kind, native                                    string
	}{
		{
			name:   "Zustand generic persistence helper",
			env:    "SCRIPTHOLD_REAL_TS_MULTILINE_ZUSTAND_ROOT",
			file:   "src/middleware/persist.ts",
			symbol: "toThenable", line: 157,
			lineText: "const toThenable =", continuation: "<Result, Input>(",
			kind: "function", native: "arrow-function",
		},
		{
			name:   "Quill multiline fixture string",
			env:    "SCRIPTHOLD_REAL_TS_MULTILINE_QUILL_ROOT",
			file:   "packages/quill/test/e2e/fixtures/index.ts",
			symbol: "P1", line: 35,
			lineText: "export const P1 =", continuation: "'Call me Ishmael.",
			kind: "constant", native: "const",
		},
	}
	configured := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("two original TypeScript project roots not configured")
	}
	if configured != len(cases) {
		t.Fatal("both independent original project roots are required")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source must be a bounded regular file: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			if tc.line > len(lines) || !strings.Contains(lines[tc.line-1], tc.lineText) ||
				!strings.Contains(string(raw), tc.continuation) {
				t.Fatalf("source-derived oracle changed: %s", tc.file)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "typescript", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original source digest unavailable: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "typescript", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original detailed outline unavailable: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "typescript",
				Query: tc.symbol, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			id := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.symbol && symbol.NativeKind == tc.native &&
					string(symbol.Kind) == tc.kind && symbol.NameRange.Start.Line == tc.line &&
					symbol.ParentQualifiedName == "" {
					id = symbol.ID
				}
			}
			inOutline := false
			for _, symbol := range outline.Symbols {
				if id != "" && symbol.ID == id {
					inOutline = true
				}
			}
			if err != nil || id == "" || !inOutline || len(found.Files) != 1 {
				t.Fatalf("source-derived kind/owner/line is missing: %v %+v", err, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("source fingerprint mismatch: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "typescript", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.lineText) ||
				!strings.Contains(shown.Show.Text, tc.continuation) {
				t.Fatalf("multiline source excerpt incomplete: %v %+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "typescript", Query: tc.symbol, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			linked := false
			if query.Search != nil {
				for _, hit := range query.Search.Matches {
					if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
						hit.Range.Start.Line == tc.line {
						linked = true
					}
				}
			}
			if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
				t.Fatalf("original structural search mismatch: %v %+v", err, query)
			}
		})
	}
}
