package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func BenchmarkSourceQueryColdWarmAndLookup(b *testing.B) {
	root := b.TempDir()
	const fileCount = 80
	for index := 0; index < fileCount; index++ {
		name := fmt.Sprintf("Item%03d", index)
		content := fmt.Sprintf("package bench; public class %s { public int value() { return %d; } }\n", name, index)
		if err := os.WriteFile(filepath.Join(root, name+".java"), []byte(content), 0o600); err != nil {
			b.Fatal(err)
		}
	}
	ctx := context.Background()
	exact := SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Item079", Mode: "structural", Match: "exact",
		Language: "java", Encoding: "utf-8", MaxFiles: fileCount, MaxResults: 16,
	}
	prefix := exact
	prefix.Query = "Item07"
	prefix.Match = "prefix"

	b.Run("cold-many-small-exact", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			h := NewHandler([]string{root})
			result, output, err := h.SourceQuery(ctx, nil, exact)
			if err != nil || (result != nil && result.IsError) || output.Search == nil || len(output.Search.Matches) != 1 {
				b.Fatalf("cold exact result=%+v output=%+v err=%v", result, output, err)
			}
		}
	})

	h := NewHandler([]string{root})
	seedResult, seed, err := h.SourceQuery(ctx, nil, exact)
	if err != nil || (seedResult != nil && seedResult.IsError) || seed.Search == nil || len(seed.Search.Matches) != 1 {
		b.Fatalf("warm seed result=%+v output=%+v err=%v", seedResult, seed, err)
	}
	b.Run("warm-exact", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			result, output, err := h.SourceQuery(ctx, nil, exact)
			if err != nil || (result != nil && result.IsError) || output.Index != seed.Index || output.Search == nil || len(output.Search.Matches) != 1 {
				b.Fatalf("warm exact result=%+v output=%+v err=%v", result, output, err)
			}
		}
	})
	b.Run("warm-prefix", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			result, output, err := h.SourceQuery(ctx, nil, prefix)
			if err != nil || (result != nil && result.IsError) || output.Index != seed.Index || output.Search == nil || len(output.Search.Matches) == 0 {
				b.Fatalf("warm prefix result=%+v output=%+v err=%v", result, output, err)
			}
		}
	})
	b.Run("warm-incremental-one-file-change", func(b *testing.B) {
		path := filepath.Join(root, "Item079.java")
		previousGeneration := seed.Index.Generation
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			name := "Item079A"
			if iteration%2 != 0 {
				name = "Item079B"
			}
			content := fmt.Sprintf("package bench; public class %s { public int value() { return %d; } }\n", name, iteration)
			b.StopTimer()
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				b.Fatal(err)
			}
			changed := exact
			changed.Query = name
			b.StartTimer()
			result, output, err := h.SourceQuery(ctx, nil, changed)
			if err != nil || (result != nil && result.IsError) || output.Search == nil || len(output.Search.Matches) != 1 || output.Index.Generation <= previousGeneration {
				b.Fatalf("warm incremental result=%+v output=%+v err=%v", result, output, err)
			}
			previousGeneration = output.Index.Generation
		}
	})
}

func BenchmarkSourceQueryMixedLanguageMonorepo(b *testing.B) {
	root := b.TempDir()
	for index := 0; index < 10; index++ {
		fixtures := map[string]string{
			fmt.Sprintf("Java%02d.java", index):    fmt.Sprintf("package mixed; public class Java%02d {}\n", index),
			fmt.Sprintf("Ts%02d.ts", index):        fmt.Sprintf("export class Ts%02d {}\n", index),
			fmt.Sprintf("Scala%02d.scala", index):  fmt.Sprintf("package mixed\nclass Scala%02d:\n  def run(value: Int): Int = value\n", index),
			fmt.Sprintf("Flow%02d.js.flow", index): fmt.Sprintf("/* @flow */\nexport class Flow%02d { run(value: number): number { return value; } }\n", index),
		}
		for name, content := range fixtures {
			if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}
	input := SourceQueryInput{
		Operation: "search", Paths: []string{root}, Query: "Scala09", Mode: "structural", Match: "exact",
		Encoding: "utf-8", MaxFiles: 40, MaxResults: 8,
	}
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		h := NewHandler([]string{root})
		result, output, err := h.SourceQuery(context.Background(), nil, input)
		if err != nil || (result != nil && result.IsError) || output.Coverage.FilesParsed != 40 || !output.Coverage.CoverageComplete || output.Search == nil || len(output.Search.Matches) != 1 {
			b.Fatalf("mixed monorepo result=%+v output=%+v err=%v", result, output, err)
		}
	}
}

func benchmarkSourceFingerprint(b *testing.B, path string) string {
	b.Helper()
	document, err := sourceintelligence.OpenSourceDocument(context.Background(), path, sourceintelligence.OpenDocumentOptions{
		MaxFileBytes: 8 * 1024 * 1024, MaxDecodedCharacters: 8 * 1024 * 1024,
	})
	if err != nil {
		b.Fatal(err)
	}
	return document.SourceFingerprint
}

func BenchmarkSourceQueryLargeGenerated(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "large.go")
	var source strings.Builder
	source.WriteString("package large\n")
	for index := 0; index < 5_000; index++ {
		fmt.Fprintf(&source, "func F%04d(value int) int { return value + %d }\n", index, index)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0o600); err != nil {
		b.Fatal(err)
	}
	input := SourceQueryInput{
		Operation: "search", Paths: []string{path}, Query: "F4999", Mode: "structural", Match: "exact",
		Language: "go", Encoding: "utf-8", MaxFiles: 1, MaxResults: 8,
	}
	b.SetBytes(int64(len(source.String())))
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		h := NewHandler([]string{root})
		result, output, err := h.SourceQuery(context.Background(), nil, input)
		if err != nil || (result != nil && result.IsError) || output.Search == nil || len(output.Search.Matches) != 1 {
			b.Fatalf("large generated result=%+v output=%+v err=%v", result, output, err)
		}
	}
}

func BenchmarkSourceQueryRelationsAndContext(b *testing.B) {
	b.Run("dependency-trace-8-files", func(b *testing.B) {
		root := b.TempDir()
		const count = 8
		for index := 0; index < count; index++ {
			name := fmt.Sprintf("N%02d", index)
			content := fmt.Sprintf("export class %s {}\n", name)
			if index+1 < count {
				next := fmt.Sprintf("N%02d", index+1)
				content = fmt.Sprintf("import { %s } from \"./%s\";\nexport class %s extends %s {}\n", next, next, name, next)
			}
			if err := os.WriteFile(filepath.Join(root, name+".ts"), []byte(content), 0o600); err != nil {
				b.Fatal(err)
			}
		}
		h := NewHandler([]string{root})
		startPath := filepath.Join(root, "N00.ts")
		endPath := filepath.Join(root, fmt.Sprintf("N%02d.ts", count-1))
		input := SourceQueryInput{
			Operation: "relations", Paths: []string{root}, Relation: "trace", Language: "typescript", Encoding: "utf-8",
			Subject:  &SourceSelectorInput{Kind: "path", Path: startPath, SourceFingerprint: benchmarkSourceFingerprint(b, startPath)},
			Target:   &SourceSelectorInput{Kind: "path", Path: endPath, SourceFingerprint: benchmarkSourceFingerprint(b, endPath)},
			MaxFiles: count, MaxResults: count, MaxNodes: count + 8, MaxEdges: count * 4, MaxDepth: count,
		}
		seedResult, seed, err := h.SourceQuery(context.Background(), nil, input)
		if err != nil || (seedResult != nil && seedResult.IsError) || seed.Relations == nil || len(seed.Relations.Relations) != count-1 {
			b.Fatalf("trace seed result=%+v output=%+v err=%v", seedResult, seed, err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			result, output, err := h.SourceQuery(context.Background(), nil, input)
			if err != nil || (result != nil && result.IsError) || output.Index != seed.Index || output.Relations == nil || len(output.Relations.Relations) != count-1 {
				b.Fatalf("trace result=%+v output=%+v err=%v", result, output, err)
			}
		}
	})

	b.Run("bounded-context", func(b *testing.B) {
		root := b.TempDir()
		path := filepath.Join(root, "Service.java")
		content := "package demo; public class Service { public int run(int value) { return value + 1; } }\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			b.Fatal(err)
		}
		h := NewHandler([]string{root})
		searchResult, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{root}, Query: "Service", Mode: "structural", Match: "exact", Language: "java", Encoding: "utf-8", MaxResults: 8,
		})
		if err != nil || (searchResult != nil && searchResult.IsError) || search.Search == nil || len(search.Search.Matches) != 1 {
			b.Fatalf("context seed search result=%+v output=%+v err=%v", searchResult, search, err)
		}
		match := search.Search.Matches[0]
		input := SourceQueryInput{
			Operation: "context", Paths: []string{root}, Targets: []SourceSelectorInput{{
				Kind: "symbol", Path: match.Path, SymbolID: match.SymbolID, SourceFingerprint: match.SourceFingerprint,
			}}, BudgetBytes: 4096, BodyPolicy: "prefer", Language: "java", Encoding: "utf-8", MaxItems: 8, MaxDepth: 2,
		}
		seedResult, seed, err := h.SourceQuery(context.Background(), nil, input)
		if err != nil || (seedResult != nil && seedResult.IsError) || seed.Context == nil || len(seed.Context.Items) == 0 {
			b.Fatalf("context seed result=%+v output=%+v err=%v", seedResult, seed, err)
		}
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			result, output, err := h.SourceQuery(context.Background(), nil, input)
			if err != nil || (result != nil && result.IsError) || output.Index != seed.Index || output.Context == nil || output.Context.UsedBytes > output.Context.BudgetBytes {
				b.Fatalf("context result=%+v output=%+v err=%v", result, output, err)
			}
		}
	})
}
