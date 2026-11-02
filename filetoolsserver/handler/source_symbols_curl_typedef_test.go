package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Original curl public header: CURLHcode is a typedef alias, not a variable.
func TestSourceSymbolsOriginalCurlAnonymousEnumTypedef(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	header := filepath.Join(root, "real-projects", "c-curl", "include", "curl", "header.h")
	h := NewHandler([]string{root})
	failed, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{header}, Query: "CURLHcode",
		Match: "exact", Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || !result.CoverageComplete || result.FilesParsed != 1 ||
		len(result.Symbols) != 1 {
		t.Fatalf("original curl header symbol lookup incomplete: err=%v failed=%+v symbols=%+v", err, failed, result.Symbols)
	}
	symbol := result.Symbols[0]
	if symbol.QualifiedName != "CURLHcode" || string(symbol.Kind) != "alias" ||
		symbol.NameRange.Start.Line != 56 || symbol.ID == "" {
		t.Errorf("original curl public enum typedef misclassified: %+v", symbol)
	}
}
