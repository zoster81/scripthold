package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
)

func TestSourceSymbolsOutlineFindShowAndStaleFingerprint(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	content := "package sample\n\ntype Box struct { Value int }\nfunc Work(value int) int { return value }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Language: "go", Encoding: "utf-8", IncludeSignatures: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outline.CoordinateSystem != sourceCoordinateSystem || outline.FilesConsidered != 1 || outline.FilesParsed != 1 || !outline.CoverageComplete {
		t.Fatalf("outline summary = %+v", outline)
	}
	if len(outline.Files) != 1 || outline.Files[0].Detection.State != "exact" || outline.Files[0].SourceFingerprint == "" {
		t.Fatalf("outline file evidence = %+v", outline.Files)
	}
	var workID string
	for _, symbol := range outline.Symbols {
		if symbol.Name == "Work" {
			workID = symbol.ID
			if symbol.Signature == "" {
				t.Fatal("requested outline signature is empty")
			}
		}
	}
	if workID == "" {
		t.Fatalf("Work missing from outline: %+v", outline.Symbols)
	}

	_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: "Work", Match: "exact", Language: "go", Encoding: "utf-8", MaxSymbols: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Symbols) != 1 || found.Symbols[0].ID != workID {
		t.Fatalf("find result = %+v", found.Symbols)
	}

	_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: workID, SourceFingerprint: outline.Files[0].SourceFingerprint,
		Language: "go", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if shown.Show == nil || !strings.Contains(shown.Show.Text, "func Work(value int) int") || shown.Show.SymbolID != workID {
		t.Fatalf("show output = %+v", shown.Show)
	}

	if err := os.WriteFile(path, []byte(content+"\nvar Changed = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, _, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: workID, SourceFingerprint: outline.Files[0].SourceFingerprint,
		Language: "go", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale == nil || !stale.IsError || stale.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale show result = %+v", stale)
	}
}

func TestSourceSymbolsDirectoryOrderingFiltersDigestAndUnsupportedCoverage(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"b.cs":  "namespace Demo { public class B { public void Run() {} } }\n",
		"a.go":  "package alpha\nfunc Run() {}\n",
		"z.txt": "ordinary prose that is not source code\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	_, first, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{root}, IncludeSignatures: false})
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{root}, IncludeSignatures: false})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("directory outline is nondeterministic")
	}
	if len(first.Files) != 3 || filepath.Base(first.Files[0].Path) != "a.go" || filepath.Base(first.Files[1].Path) != "b.cs" || filepath.Base(first.Files[2].Path) != "z.txt" {
		t.Fatalf("file ordering = %+v", first.Files)
	}
	if first.CoverageComplete || first.FilesSkipped != 1 || first.Files[2].ErrorCode != ErrCodeUnsupported {
		t.Fatalf("unsupported coverage = %+v", first)
	}

	_, filtered, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{root}, Includes: []string{"*.go"}, Kinds: []string{"function"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Files) != 1 || filepath.Base(filtered.Files[0].Path) != "a.go" || len(filtered.Symbols) != 1 || filtered.Symbols[0].Name != "Run" {
		t.Fatalf("filtered outline = %+v", filtered)
	}

	_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "digest", Paths: []string{root}, Includes: []string{"*.cs"}, Language: "csharp", Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if len(digest.Digests) != 1 || digest.Digests[0].SourceBytes == 0 || len(digest.Digests[0].DeclarationCounts) == 0 || digest.Symbols != nil {
		t.Fatalf("digest = %+v", digest)
	}
}

func TestSourceSymbolsConcurrencyDoesNotChangeDeterministicOutput(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 24; index++ {
		path := filepath.Join(root, fmt.Sprintf("file-%02d.go", index))
		content := fmt.Sprintf("package p%02d\nfunc F%02d() {}\n", index, index)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := config.Load()
	serialConfig := *base
	serialConfig.Source.MaxConcurrency = 1
	parallelConfig := *base
	parallelConfig.Source.MaxConcurrency = 4
	input := SourceSymbolsInput{Operation: "outline", Paths: []string{root}, Encoding: "utf-8"}
	_, serial, err := NewHandler([]string{root}, WithConfig(&serialConfig)).SourceSymbols(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	_, parallel, err := NewHandler([]string{root}, WithConfig(&parallelConfig)).SourceSymbols(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(serial, parallel) {
		t.Fatalf("source output changed with concurrency:\nserial=%+v\nparallel=%+v", serial, parallel)
	}
}

func TestSourceSymbolsFindResultLimitDoesNotTruncateAnalysis(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "late.go")
	var source strings.Builder
	source.WriteString("package sample\n")
	for index := 0; index < 40; index++ {
		fmt.Fprintf(&source, "func F%02d() {}\n", index)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	toolErr, found, err := NewHandler([]string{root}).SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: "F39", Match: "exact", Language: "go", Encoding: "utf-8", MaxSymbols: 1,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("find late symbol err=%v toolErr=%+v", err, toolErr)
	}
	if len(found.Symbols) != 1 || found.Symbols[0].Name != "F39" || !found.CoverageComplete || found.Truncated {
		t.Fatalf("late find result = %+v", found)
	}
}

func TestSourceSymbolsFindQueryLimitCountsUnicodeScalars(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Work() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	accepted, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: strings.Repeat("é", 512), Language: "go", Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted != nil && accepted.IsError {
		t.Fatalf("512-scalar query was rejected: %+v", accepted)
	}

	rejected, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: strings.Repeat("é", 513), Language: "go", Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rejected == nil || !rejected.IsError || rejected.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("513-scalar query result = %+v, want INVALID_INPUT", rejected)
	}
}

func TestSourceSymbolsLimitsCancellationAndDeniedRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc A() {}\nfunc B() {}\nfunc C() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load()
	cfg.Source.MaxSymbols = 2
	cfg.Source.MaxAggregateBytes = 16
	h := NewHandler([]string{root}, WithConfig(cfg))

	limitResult, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if limitResult == nil || !limitResult.IsError || limitResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("aggregate limit result = %+v", limitResult)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cancelResult, _, err := NewHandler([]string{root}).SourceSymbols(cancelled, nil, SourceSymbolsInput{Operation: "outline", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelResult == nil || !cancelResult.IsError || cancelResult.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancel result = %+v", cancelResult)
	}

	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	denied, _, err := NewHandler([]string{root}).SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Paths: []string{outside}, Language: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if denied == nil || !denied.IsError || denied.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("denied root result = %+v", denied)
	}
}
