package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Run local MCP navigation across two complete immutable Common Lisp originals.
func TestCommonLispPinnedWholeOriginalNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_COMMON_LISP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete Common Lisp source originals are not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, name, sourceSnippet string
		count, line, column       int
	}{
		{"quicklisp-config.lisp", "(setf config-value)", "(defun (setf config-value)", 3, 28, 8},
		{"slime-match.lisp", "select-match", "(defmacro select-match", 14, 92, 11},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("entire original unavailable or unsafe: %v %v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "common-lisp", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.count {
				t.Fatalf("whole-original outline: err=%v failed=%+v symbols=%+v", err, failed, outline.Symbols)
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("missing original fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "common-lisp",
				Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].Name != tc.name ||
				found.Symbols[0].NameRange.Start.Line != tc.line ||
				found.Symbols[0].NameRange.Start.Column != tc.column {
				t.Fatalf("whole-original exact symbol find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "common-lisp", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 16384,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.sourceSnippet) {
				t.Fatalf("fingerprint-verified original declaration show: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "common-lisp", Encoding: "utf-8", Query: tc.name, Match: "exact",
				MaxFiles: 1, MaxResults: 256,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 {
				t.Fatalf("whole-original exact structural search: err=%v failed=%+v", err, failed)
			}
		})
	}
}
