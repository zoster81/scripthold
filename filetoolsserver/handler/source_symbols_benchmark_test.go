package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkSourceSymbolsOperations(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "sample.go")
	var source string
	source = "package sample\n"
	for index := 0; index < 200; index++ {
		source += fmt.Sprintf("func F%03d(value int) int { return value }\n", index)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		b.Fatal(err)
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	_, seed, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
	if err != nil || len(seed.Symbols) == 0 {
		b.Fatalf("seed outline failed: %v %+v", err, seed)
	}
	showInput := SourceSymbolsInput{Operation: "show", Path: path, SymbolID: seed.Symbols[len(seed.Symbols)-1].ID, SourceFingerprint: seed.Files[0].SourceFingerprint, Language: "go", Encoding: "utf-8", MaxBytes: 4096}
	cases := []struct {
		name  string
		input SourceSymbolsInput
	}{
		{name: "outline", input: SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Language: "go", Encoding: "utf-8", MaxSymbols: 1000}},
		{name: "digest", input: SourceSymbolsInput{Operation: "digest", Paths: []string{path}, Language: "go", Encoding: "utf-8"}},
		{name: "find", input: SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "go", Encoding: "utf-8", Query: "F199", Match: "exact", MaxSymbols: 16}},
		{name: "show", input: showInput},
	}
	for _, testCase := range cases {
		b.Run(testCase.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				toolErr, _, err := h.SourceSymbols(ctx, nil, testCase.input)
				if err != nil || toolErr != nil {
					b.Fatalf("source_symbols %s failed: err=%v toolErr=%+v", testCase.name, err, toolErr)
				}
			}
		})
	}
}

func BenchmarkSourceSymbolsMixedManySmallFiles(b *testing.B) {
	root := b.TempDir()
	for index := 0; index < 80; index++ {
		var name, content string
		switch index % 4 {
		case 0:
			name = fmt.Sprintf("file-%03d.go", index)
			content = fmt.Sprintf("package p%03d\nfunc Work%03d() {}\n", index, index)
		case 1:
			name = fmt.Sprintf("file-%03d.cs", index)
			content = fmt.Sprintf("namespace N%d { public class C%d { public void Work%d() {} } }\n", index, index, index)
		case 2:
			name = fmt.Sprintf("file-%03d.vb", index)
			content = fmt.Sprintf("Class C%d\nSub Work%d()\nEnd Sub\nEnd Class\n", index, index)
		default:
			name = fmt.Sprintf("file-%03d.py", index)
			content = fmt.Sprintf("def work%d():\n    return %d\n", index, index)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	input := SourceSymbolsInput{Operation: "digest", Paths: []string{root}, Encoding: "utf-8"}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		toolErr, output, err := h.SourceSymbols(context.Background(), nil, input)
		if err != nil || toolErr != nil {
			b.Fatalf("mixed source_symbols failed: err=%v toolErr=%+v", err, toolErr)
		}
		if output.FilesParsed != 80 {
			b.Fatalf("parsed files=%d, want 80", output.FilesParsed)
		}
	}
}
