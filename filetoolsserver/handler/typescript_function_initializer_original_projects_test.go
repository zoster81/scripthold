package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source-first fixtures from independent production projects establish that a
// directly assigned function expression is a function, not a plain constant.
func TestTypeScriptOriginalDirectFunctionExpressions(t *testing.T) {
	cases := []struct {
		name, env, file, symbol, declaration, bodyExcerpt string
		neighbor, neighborKind, neighborNative            string
		line                                              int
	}{
		{
			name:   "Hexo theme rendering factory",
			env:    "SCRIPTHOLD_REAL_TS_FUNCTION_HEXO_ROOT",
			file:   "lib/hexo/index.ts",
			symbol: "createLoadThemeRoute", line: 62,
			declaration: "const createLoadThemeRoute = function(",
			bodyExcerpt: "const layout = [...new Set<string>(castArray(generatorResult.layout))]",
			neighbor:    "routeCache", neighborKind: "constant", neighborNative: "const",
		},
		{
			name:   "Ink Yoga text measurement",
			env:    "SCRIPTHOLD_REAL_TS_FUNCTION_INK_ROOT",
			file:   "src/dom.ts",
			symbol: "measureTextNode", line: 245,
			declaration: "const measureTextNode = function (",
			bodyExcerpt: "const dimensions = measureText(text)",
			neighbor:    "createNode", neighborKind: "function", neighborNative: "arrow-function",
		},
	}
	configured := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("two independent original TypeScript project roots not configured")
	}
	if configured != len(cases) {
		t.Fatal("both original project roots must be configured together")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := os.Getenv(tc.env)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source must be bounded and regular: %v %v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(raw), "\n")
			if tc.line > len(lines) || !strings.Contains(lines[tc.line-1], tc.declaration) ||
				!strings.Contains(string(raw), tc.bodyExcerpt) {
				t.Fatalf("original source oracle changed: %s", tc.file)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "typescript", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("digest unavailable: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "typescript", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("detailed outline unavailable: %v %+v", err, outline)
			}
			neighborFound := false
			for _, s := range outline.Symbols {
				if s.Name == tc.neighbor && string(s.Kind) == tc.neighborKind &&
					s.NativeKind == tc.neighborNative {
					neighborFound = true
				}
			}
			if !neighborFound {
				t.Errorf("neighbor declaration was incorrectly classified: %s", tc.neighbor)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "typescript",
				Query: tc.symbol, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
				FileDetails: "relevant",
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ""
			for _, s := range found.Symbols {
				if s.Name == tc.symbol && string(s.Kind) == "function" &&
					s.NativeKind == "function-expression" &&
					s.ParentQualifiedName == "" && s.NameRange.Start.Line == tc.line {
					id = s.ID
				}
			}
			existsInOutline := false
			for _, s := range outline.Symbols {
				if s.ID == id && id != "" {
					existsInOutline = true
				}
			}
			if id == "" || !existsInOutline || len(found.Files) != 1 {
				t.Fatalf("function symbol absent or incorrectly owned: %+v", found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("fingerprint mismatch: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "typescript", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.declaration) ||
				!strings.Contains(shown.Show.Text, tc.bodyExcerpt) {
				t.Fatalf("verified source excerpt incomplete: %v %+v", err, shown.Show)
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
				t.Fatalf("structural search did not return the verified function: %v %+v", err, query)
			}
		})
	}
}
