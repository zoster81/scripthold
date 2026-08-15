package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestR27Phase4SourceSymbolsRoutesJavaScriptTypeScriptAndRust(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"sample.jsx": "export class JSBox extends Base { run() { return 1; } }\nexport const jswork = () => 1;\n",
		"sample.tsx": "export interface TSBox { value: string; }\nexport const tswork = <T,>(value: T): T => value;\n",
		"lib.rs":     "pub struct RustBox { pub value: i32 }\npub fn rustwork() {}\n",
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
	if outline.FilesConsidered != 3 || outline.FilesParsed != 3 || outline.FilesSkipped != 0 || !outline.CoverageComplete {
		t.Fatalf("Phase 4 outline summary = %+v", outline)
	}
	parsedLanguages := map[string]bool{}
	for _, file := range outline.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 4 file routing = %+v", file)
		}
		parsedLanguages[file.Detection.Language] = true
	}
	for _, language := range []string{"javascript", "typescript", "rust"} {
		if !parsedLanguages[language] {
			t.Fatalf("missing routed language %s: %+v", language, outline.Files)
		}
	}

	wantSymbols := map[string]bool{"JSBox": false, "TSBox": false, "RustBox": false}
	for _, symbol := range outline.Symbols {
		if _, ok := wantSymbols[symbol.QualifiedName]; ok {
			wantSymbols[symbol.QualifiedName] = true
		}
	}
	for name, found := range wantSymbols {
		if !found {
			t.Fatalf("missing public Phase 4 symbol %s; symbols=%+v", name, outline.Symbols)
		}
	}

	for name, language := range map[string]string{"JSBox": "javascript", "TSBox": "typescript", "RustBox": "rust"} {
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
