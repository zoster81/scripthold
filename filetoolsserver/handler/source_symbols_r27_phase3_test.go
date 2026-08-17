package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestR27Phase3SourceSymbolsRoutesC_CPP_Java_Kotlin(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	files := map[string]string{
		"sample.c":    "struct CBox { int value; };\nint cwork(int value) { return value; }\n",
		"sample.cpp":  "namespace cppdemo { class Box { public: int get() const; }; }\n",
		"Sample.java": "package jdemo; public class Box { public int get() { return 1; } }\n",
		"sample.kt":   "package kdemo\nclass Box(val value: Int) { fun get(): Int = value }\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Paths: []string{root}, Encoding: "utf-8", IncludeSignatures: true})
	if err != nil || toolErr != nil {
		t.Fatalf("outline err=%v toolErr=%+v", err, toolErr)
	}
	if outline.FilesConsidered != 4 || outline.FilesParsed != 4 || outline.FilesSkipped != 0 || !outline.CoverageComplete {
		t.Fatalf("Phase 3 outline summary = %+v", outline)
	}
	parsedLanguages := map[string]bool{}
	for _, file := range outline.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 3 file routing = %+v", file)
		}
		parsedLanguages[file.Detection.Language] = true
	}
	for _, language := range []string{"c", "cpp", "java", "kotlin"} {
		if !parsedLanguages[language] {
			t.Fatalf("missing routed language %s: %+v", language, outline.Files)
		}
	}
	wantSymbols := map[string]bool{"CBox": false, "cppdemo.Box": false, "jdemo.Box": false, "kdemo.Box": false}
	for _, symbol := range outline.Symbols {
		if _, ok := wantSymbols[symbol.QualifiedName]; ok {
			wantSymbols[symbol.QualifiedName] = true
		}
	}
	for name, found := range wantSymbols {
		if !found {
			t.Fatalf("missing public Phase 3 symbol %s; symbols=%+v", name, outline.Symbols)
		}
	}

	toolErr, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "find", Paths: []string{root}, Query: "Box", Match: "exact", Encoding: "utf-8", MaxSymbols: 16})
	if err != nil || toolErr != nil {
		t.Fatalf("find err=%v toolErr=%+v", err, toolErr)
	}
	languages := map[string]bool{}
	for _, symbol := range found.Symbols {
		if symbol.Name == "Box" {
			languages[symbol.Language] = true
		}
	}
	for _, language := range []string{"cpp", "java", "kotlin"} {
		if !languages[language] {
			t.Fatalf("find omitted %s Box: %+v", language, found.Symbols)
		}
	}
}
