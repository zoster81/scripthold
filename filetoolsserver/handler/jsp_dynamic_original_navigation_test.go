package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Complete independent upstream originals verify JSP static-ID navigation
// without evaluating dynamic server expressions or running the projects.
func TestJSPPinnedStaticIDsAndDynamicSourceNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JSP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete pinned upstream JSP originals not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, target, excerpt string
		symbolCount           int
	}{
		{"bookstore-index.jsp", "header", "<div id=\"header\">", 6},
		{"exam-system-exam.jsp", "myform", "<form id=\"myform\" action=\"controller.jsp\">", 3},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("whole upstream file missing or unsafe: %v %v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "jsp", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 4096,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				len(outline.Files) != 1 || outline.FilesParsed != 1 ||
				len(outline.Symbols) != tc.symbolCount {
				t.Fatalf("JSP original outline incomplete: err=%v failed=%+v symbols=%d want=%d",
					err, failed, len(outline.Symbols), tc.symbolCount)
			}
			for _, sym := range outline.Symbols {
				if sym.Name == "c1" || sym.Name == "c2" || sym.Name == "c3" || sym.Name == "c4" {
					t.Errorf("dynamically generated JSP attribute falsely indexed as a stable ID: %+v", sym)
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("original source fingerprint missing")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "jsp",
				Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxSymbols: 4096,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].Name != tc.target {
				t.Fatalf("JSP original exact find failed: err=%v failed=%+v", err, failed)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "jsp",
				Encoding: "utf-8", SymbolID: found.Symbols[0].ID,
				SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("JSP original fingerprint-bound show failed: err=%v failed=%+v", err, failed)
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "jsp", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 4096,
			})
			if err != nil || failed != nil || search.Search == nil ||
				len(search.Search.Matches) != 1 {
				t.Fatalf("JSP original structural search failed: err=%v failed=%+v", err, failed)
			}
		})
	}
}
