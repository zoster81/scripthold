package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The three source files come from independent upstream TypeScript projects.
// Each constant/variable stores an object; its arrow callbacks belong to its
// initializer, not to the variable's declared kind.
func TestTypeScriptOriginalNestedArrowObjectVariables(t *testing.T) {
	cases := []struct {
		name, env, file, symbol, declaration, callback, unrelated string
		line                                                      int
	}{
		{
			name: "Pretext masonry state", env: "SCRIPTHOLD_REAL_TS_ARROW_PRETEXT_ROOT",
			file: "pages/demos/masonry/index.ts", symbol: "st", line: 35,
			declaration: "const st: State = {", callback: "cards: rawThoughts.map(text => ({",
			unrelated: "scheduleRender",
		},
		{
			name: "Ink app lifecycle defaults", env: "SCRIPTHOLD_REAL_TS_ARROW_INK_ROOT",
			file: "src/components/AppContext.ts", symbol: "defaultValue", line: 95,
			declaration: "const defaultValue: Props = {",
			callback:    "suspendTerminal: (async (callback?: () => void | Promise<void>) => {",
			unrelated:   "suspendTerminal",
		},
		{
			name: "Radar database verification counters", env: "SCRIPTHOLD_REAL_TS_ARROW_RADAR_ROOT",
			file: "scripts/verify-db.ts", symbol: "summary", line: 60,
			declaration: "const summary = {",
			callback:    "pass: checks.filter((check) => check.status === \"pass\").length,",
			unrelated:   "filter",
		},
	}
	configured := 0
	for _, tc := range cases {
		if os.Getenv(tc.env) != "" {
			configured++
		}
	}
	if configured == 0 {
		t.Skip("three independent original TypeScript project roots not configured")
	}
	if configured != len(cases) {
		t.Fatal("all three original TypeScript project roots must be supplied together")
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
			source := string(raw)
			lines := strings.Split(source, "\n")
			if tc.line > len(lines) || !strings.Contains(lines[tc.line-1], tc.declaration) ||
				!strings.Contains(source, tc.callback) {
				t.Fatalf("independent original TypeScript declaration/callback oracle changed in %s", tc.file)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path},
				Language: "typescript", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("TypeScript original digest unavailable: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "typescript", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("TypeScript original detailed outline unavailable: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "typescript",
				Query: tc.symbol, Match: "exact", MaxFiles: 1,
				MaxSymbols: 80, FileDetails: "relevant",
			})
			id := ""
			for _, symbol := range found.Symbols {
				if symbol.Name == tc.symbol && string(symbol.Kind) == "constant" &&
					symbol.NativeKind == "const" && symbol.NameRange.Start.Line == tc.line &&
					symbol.ParentQualifiedName == "" {
					id = symbol.ID
				}
			}
			present := false
			for _, symbol := range outline.Symbols {
				if symbol.ID == id && id != "" {
					present = true
				}
			}
			if err != nil || !present || len(found.Files) != 1 {
				t.Fatalf("original object containing nested arrows falsely classified or missing: %s line %d err=%v symbols=%+v", tc.symbol, tc.line, err, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("TypeScript source identity mismatch: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "typescript",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.declaration) {
				t.Fatalf("original typed object variable excerpt missing: %v %+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "typescript", Query: tc.symbol, Match: "exact",
				MaxFiles: 1, MaxResults: 80,
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
				t.Fatalf("original object variable not linked to structural search: %v %+v", err, query)
			}
		})
	}
}
