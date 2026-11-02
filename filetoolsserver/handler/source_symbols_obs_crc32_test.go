package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A single declaration in the original OBS Studio public header must not
// become two identical symbols through preprocessing or linkage branches.
func TestSourceSymbolsOriginalOBSCRC32SingleDeclaration(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	path := filepath.Join(root, "real-projects", "c-obsproject-obs-studio", "libobs", "util", "crc32.h")
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || failed != nil || !out.CoverageComplete || len(out.Symbols) != 1 ||
		out.Symbols[0].QualifiedName != "calc_crc32" ||
		string(out.Symbols[0].Kind) != "function" ||
		out.Symbols[0].NameRange.Start.Line != 25 {
		t.Fatalf("original OBS crc32.h has one declaration at line 25, got: err=%v failed=%+v symbols=%+v", err, failed, out.Symbols)
	}
}
