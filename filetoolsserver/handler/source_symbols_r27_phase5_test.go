package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestR27Phase5SourceSymbolsRoutesPHPRubySwiftPascalAndDelphi(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	files := map[string]string{
		"sample.php":   "<?php\nclass PHPBox { public function get(): int { return 1; } }\n",
		"sample.rb":    "class RubyBox\n  def get\n    1\n  end\nend\n",
		"sample.swift": "struct SwiftBox { func get() -> Int { 1 } }\n",
		"sample.pp":    "program P;\ntype\n  PascalBox = class end;\n",
		"sample.dpr":   "unit D;\ninterface\ntype\n  DelphiBox = class end;\nimplementation\nend.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{root})
	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{root}, Encoding: "utf-8", IncludeSignatures: true,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("outline err=%v toolErr=%+v", err, toolErr)
	}
	if outline.FilesConsidered != 5 || outline.FilesParsed != 5 || outline.FilesSkipped != 0 || !outline.CoverageComplete {
		t.Fatalf("Phase 5 outline summary = %+v", outline)
	}
	parsedLanguages := map[string]bool{}
	for _, file := range outline.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 5 file routing = %+v", file)
		}
		parsedLanguages[file.Detection.Language] = true
	}
	for _, language := range []string{"php", "ruby", "swift", "pascal", "delphi"} {
		if !parsedLanguages[language] {
			t.Fatalf("missing routed language %s: %+v", language, outline.Files)
		}
	}

	wantSymbols := map[string]bool{
		"PHPBox": false, "RubyBox": false, "SwiftBox": false, "P.PascalBox": false, "D.DelphiBox": false,
	}
	for _, symbol := range outline.Symbols {
		if _, ok := wantSymbols[symbol.QualifiedName]; ok {
			wantSymbols[symbol.QualifiedName] = true
		}
	}
	for name, found := range wantSymbols {
		if !found {
			t.Fatalf("missing public Phase 5 symbol %s; symbols=%+v", name, outline.Symbols)
		}
	}

	for name, language := range map[string]string{
		"PHPBox": "php", "RubyBox": "ruby", "SwiftBox": "swift", "PascalBox": "pascal", "DelphiBox": "delphi",
	} {
		toolErr, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{root}, Query: name, Match: "exact", Encoding: "utf-8", MaxSymbols: 16,
		})
		if err != nil || toolErr != nil {
			t.Fatalf("find %s err=%v toolErr=%+v", name, err, toolErr)
		}
		if len(found.Symbols) != 1 || found.Symbols[0].Name != name || found.Symbols[0].Language != language {
			t.Fatalf("find %s = %+v, want one %s symbol", name, found.Symbols, language)
		}
	}
}
