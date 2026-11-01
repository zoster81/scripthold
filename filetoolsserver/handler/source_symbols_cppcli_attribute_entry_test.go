package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A C++/CLI entry-point attribute must not hide the function or expose
// calls within its body as unrelated global declarations.
func TestSourceSymbolsCPPCLIAttributedEntryPoint(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "main.cpp")
	code := strings.Join([]string{
		"#include \"App.h\"",
		"using namespace System;",
		"[STAThreadAttribute]",
		"int main(array<String^>^ args) {",
		"  Application::EnableVisualStyles();",
		"  Application::Run(% form);",
		"  return 0;",
		"}",
		"int after = 1;",
		"",
	}, "\r\n")
	if err := os.WriteFile(path, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "compact", Paths: []string{path},
		Language: "cpp-cli", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !outline.CoverageComplete || len(outline.OutlineFiles) != 1 {
		t.Fatalf("outline incomplete: err=%v tool=%+v output=%+v", err, failed, outline)
	}
	byName := map[string]*SourceOutlineNode{}
	for _, symbol := range outline.OutlineFiles[0].Symbols {
		byName[symbol.QualifiedName] = symbol
	}
	if len(byName) != 2 || len(outline.OutlineFiles[0].Symbols) != 2 {
		t.Errorf("unexpected declarations (including calls/imports): %+v", outline.OutlineFiles[0].Symbols)
	}
	main := byName["main"]
	if main == nil || string(main.Kind) != "function" || main.Line != 4 {
		t.Fatalf("missing real attributed entry point: %+v", main)
	}
	if after := byName["after"]; after == nil || string(after.Kind) != "variable" || after.Line != 9 {
		t.Errorf("following true global variable missing: %+v", after)
	}
	failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, Language: "cpp-cli", Encoding: "utf-8",
		SymbolID: main.SymbolID, SourceFingerprint: outline.OutlineFiles[0].SourceFingerprint,
		MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "int main(array<String^>^ args)") ||
		strings.Contains(shown.Show.Text, "int after = 1;") {
		t.Fatalf("show omitted or leaked entry point: err=%v tool=%+v show=%+v", err, failed, shown.Show)
	}
	failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "cpp-cli",
		Query: "main", Mode: "structural", Match: "exact", MaxResults: 4,
	})
	if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) != 1 ||
		query.Search.Matches[0].Range.Start.Line != 4 {
		t.Fatalf("source_query main missing: err=%v tool=%+v query=%+v", err, failed, query.Search)
	}
}
