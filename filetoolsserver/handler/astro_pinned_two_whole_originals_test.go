package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type astroPinnedDeclaration struct {
	name, kind, native, language     string
	line, nameCol, nameEnd, scopeEnd int
}

// Complete original upstream Astro sources independently inventory declarations
// in frontmatter, embedded script, host markup and CSS. The second original
// contains no supported authored named symbols, despite imported components.
func TestAstroTwoWholePinnedOriginalPositiveAndEmptyInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ASTRO_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete original Astro sources not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		expected   []astroPinnedDeclaration
	}{
		{
			file: "tsparticles-index.astro", blob: "1568e71ca009576a7dda4816119e05b3b44dcc9b", size: 4259,
			expected: []astroPinnedDeclaration{
				{"options", "constant", "const", "typescript", 8, 7, 14, 31},
				{"configs", "constant", "const", "javascript", 48, 19, 26, 51},
				{"currentConfig", "variable", "let", "javascript", 52, 17, 30, 52},
				{"tsparticles", "entity", "id-element", "html", 62, 24, 35, 62},
				{"main", "selector", "selector", "css", 95, 5, 9, 99},
				{"h1", "selector", "selector", "css", 101, 5, 7, 105},
				{".text-gradient", "selector", "selector", "css", 107, 5, 19, 113},
				{".instructions", "selector", "selector", "css", 115, 5, 18, 122},
				{".instructions code", "selector", "selector", "css", 124, 5, 23, 131},
				{".instructions strong", "selector", "selector", "css", 133, 5, 25, 135},
				{".link-card-grid", "selector", "selector", "css", 137, 5, 20, 142},
			},
		},
		{file: "withastro-index.astro", blob: "c04f3602b5521c56580c70dd0846b2c559be7193", size: 387},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("complete pinned Astro original missing or changed: info=%+v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(data), 0)
			_, _ = hash.Write(data)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("whole original Git blob changed")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "astro", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != len(tc.expected) {
				t.Fatalf("entire authored Astro original expected %d entries: err=%v failed=%+v result=%+v", len(tc.expected), err, failed, outline)
			}
			byName := make(map[string]int, len(outline.Symbols))
			for i, sym := range outline.Symbols {
				if _, duplicate := byName[sym.Name]; duplicate {
					t.Errorf("duplicate authored Astro declaration %s", sym.Name)
				}
				byName[sym.Name] = i
				if sym.ID == "" || sym.RegionID == "" {
					t.Errorf("missing original identity or embedded region for %+v", sym)
				}
			}
			for _, exp := range tc.expected {
				i, ok := byName[exp.name]
				if !ok {
					t.Errorf("authored original declaration missing %+v", exp)
					continue
				}
				sym := outline.Symbols[i]
				if sym.Name != exp.name || sym.QualifiedName != exp.name ||
					string(sym.Kind) != exp.kind || sym.NativeKind != exp.native ||
					sym.Language != exp.language || sym.ParentID != "" ||
					sym.ParentQualifiedName != "" ||
					sym.NameRange.Start.Line != exp.line ||
					sym.NameRange.Start.Column != exp.nameCol ||
					sym.NameRange.End.Line != exp.line ||
					sym.NameRange.End.Column != exp.nameEnd ||
					sym.DeclarationRange.Start.Line != exp.line ||
					sym.DeclarationRange.End.Line != exp.scopeEnd {
					t.Errorf("authored Astro embedded declaration mismatch want=%+v got=%+v", exp, sym)
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != tc.size {
				t.Fatal("complete original source identity/fingerprint missing")
			}
			if len(tc.expected) == 0 {
				failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "astro", Encoding: "utf-8",
					Query: "Welcome", Match: "exact", MaxFiles: 1, MaxSymbols: 32,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 0 {
					t.Fatalf("zero-declaration Astro original invented imported component symbol: err=%v failed=%+v found=%+v", err, failed, found)
				}
				return
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "astro", Encoding: "utf-8",
				Query: "tsparticles", Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 ||
				found.Symbols[0].ID != outline.Symbols[byName["tsparticles"]].ID {
				t.Fatalf("whole original Astro HTML-id find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "astro", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, "tsparticles") {
				t.Fatalf("fingerprint-bound original Astro HTML-id show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "astro", Encoding: "utf-8", Query: "tsparticles", Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || failed != nil || query.Search == nil ||
				len(query.Search.Matches) != 1 ||
				query.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original Astro structural search: err=%v failed=%+v matches=%+v", err, failed, query)
			}
		})
	}
}
