package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent original TypeScript projects distinguish a direct asserted
// arrow from an immediately invoked closure that stores its result.
func TestTypeScriptOriginalAssertedArrowAndImmediateCall(t *testing.T) {
	cases := []struct {
		name, env, file, symbol, declaration, adjacent, native, kind string
		line                                                         int
	}{
		{
			name: "Zustand exported store factory",
			env:  "SCRIPTHOLD_REAL_TS_ASSERTED_ARROW_ZUSTAND_ROOT",
			file: "src/vanilla.ts", symbol: "createStore", line: 99,
			declaration: "export const createStore = ((createState) =>",
			adjacent:    "createState ? createStoreImpl(createState) : createStoreImpl) as CreateStore",
			native:      "arrow-function", kind: "function",
		},
		{
			name: "Pretext computed browser test port",
			env:  "SCRIPTHOLD_REAL_TS_ASSERTED_ARROW_PRETEXT_ROOT",
			file: "scripts/accuracy-check.ts", symbol: "requestedPort", line: 181,
			declaration: "const requestedPort = (() => {",
			adjacent:    "const parsed = Number.parseInt(requestedPortRaw, 10)",
			native:      "const", kind: "constant",
		},
	}
	configured := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("original Zustand and Pretext TypeScript roots not configured")
	}
	if configured != len(cases) {
		t.Fatal("both independent original TypeScript project roots must be configured")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source must be a bounded regular file: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			if tc.line > len(lines) || !strings.Contains(lines[tc.line-1], tc.declaration) ||
				!strings.Contains(string(raw), tc.adjacent) {
				t.Fatalf("independent original declaration oracle has changed in %s", tc.file)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "typescript", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original source digest missing: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "typescript", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original detailed outline missing: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "typescript",
				Query: tc.symbol, Match: "exact", MaxFiles: 1, MaxSymbols: 64, FileDetails: "relevant",
			})
			id := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.symbol && string(symbol.Kind) == tc.kind &&
					symbol.NativeKind == tc.native && symbol.NameRange.Start.Line == tc.line &&
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
				t.Fatalf("original declaration kind/line/owner mismatch: %v %+v", err, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("source identity mismatch: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "typescript",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.declaration) {
				t.Fatalf("fingerprint-bound source excerpt unavailable: %v %+v", err, shown.Show)
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
				t.Fatalf("original structural search not linked: %v %+v", err, query)
			}
		})
	}
}
