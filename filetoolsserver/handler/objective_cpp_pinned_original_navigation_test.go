package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent complete upstream Objective-C++ originals are read, never run.
func TestObjectiveCPPPinnedWholeOriginalSymbolNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OBJECTIVE_CPP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete immutable upstream Objective-C++ originals not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file, target, excerpt string
		want                  int
	}{
		{"fastbuild-osmenu.mm", "MenuOSX_Create", "MenuOSX_Create(", 6},
		{"llvm-ivar-objects.mm", "~X", "~X()", 37},
		{"llvm-ivar-objects.mm", "~Y", "~Y()", 37},
		{"llvm-ivar-objects.mm", "~S", "~S()", 37},
	} {
		t.Run(tc.file+"/"+tc.target, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original fixture invalid: %v %v", info, err)
			}
			failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "objective-cpp", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 4096,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != tc.want {
				t.Fatalf("whole upstream Objective-C++ source must parse fully: err=%v failed=%+v returned=%d want=%d",
					err, failed, len(outline.Symbols), tc.want)
			}
			fp := outline.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("missing original source fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "objective-cpp",
				Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxSymbols: 4096,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].Name != tc.target {
				t.Fatalf("original exact find failed: err=%v failed=%+v count=%d",
					err, failed, len(found.Symbols))
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "objective-cpp", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fp, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("fingerprint-confirmed original show failed: err=%v failed=%+v", err, failed)
			}
			failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "objective-cpp", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 4096,
			})
			if err != nil || failed != nil || searched.Search == nil ||
				len(searched.Search.Matches) != 1 {
				t.Fatalf("original Objective-C++ structural search failed: err=%v failed=%+v", err, failed)
			}
		})
	}
}
