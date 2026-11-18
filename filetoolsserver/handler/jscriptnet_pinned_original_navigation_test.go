package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are complete pinned original sources, not copied or executed projects.
// An opt-in fixture root keeps the local MCP navigation test reproducible.
func TestJScriptNetPinnedFullOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JSNET_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete JScript.NET R30 originals not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, target, excerpt string
		count                 int
	}{
		{"codesanook-program.js", "buttonClicked", "MessageBox.Show(String.Format", 8},
		{"clearcanvas-jscript.js", "CreateScriptPrivate", "return new ExecutableScript(ctx);", 9},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("full original missing or invalid: info=%v err=%v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "jscript-net", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.count {
				t.Fatalf("complete original outline inconsistent: err=%v failure=%+v symbols=%d want=%d", err, failed, len(outline.Symbols), tc.count)
			}
			for _, sym := range outline.Symbols {
				if sym.Name == "var" || sym.Name == "return" || sym.Name == "for" {
					t.Errorf("JScript.NET body keyword fabricated declaration: %+v", sym)
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing original source fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "jscript-net",
				Encoding: "utf-8", Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].Name != tc.target {
				t.Fatalf("authored method not findable: err=%v failure=%+v matches=%d", err, failed, len(found.Symbols))
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "jscript-net",
				Encoding: "utf-8", SymbolID: found.Symbols[0].ID,
				SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("original method body not shown: err=%v failure=%+v", err, failed)
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "jscript-net", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 1 {
				t.Fatalf("original structural search failed: err=%v failure=%+v", err, failed)
			}
		})
	}
}
