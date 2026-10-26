package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
		Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8", IncludeSignatures: true,
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

func TestSourceSymbolsDigestCompactOverview(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "sample.go")
	code := "package sample\ntype Router struct{}\nfunc NewRouter() *Router { return &Router{} }\nfunc (r *Router) ServeHTTP() {}\n"
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	call := func(presentation string) SourceSymbolsOutput {
		t.Helper()
		result, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: []string{path}, Language: "go", Presentation: presentation,
		})
		if err != nil || result != nil {
			t.Fatalf("digest %q: result=%+v err=%v", presentation, result, err)
		}
		return out
	}
	legacy, detailed := call(""), call("detailed")
	if !reflect.DeepEqual(legacy, detailed) || len(legacy.Digests) != 1 || len(legacy.Files) != 1 {
		t.Fatal("digest without presentation must preserve previous output")
	}
	compact := call("compact")
	if compact.Presentation != "compact" || len(compact.Files) != 0 || len(compact.Digests) != 0 ||
		len(compact.OverviewFiles) != 1 || compact.SymbolCount != legacy.SymbolCount ||
		compact.CoverageComplete != legacy.CoverageComplete || compact.FilesParsed != legacy.FilesParsed {
		t.Fatalf("compact digest lost coverage or changed shape: %+v", compact)
	}
	file := compact.OverviewFiles[0]
	if file.Path != path || file.SourceFingerprint != legacy.Files[0].SourceFingerprint ||
		file.Encoding != "utf-8" || len(file.DeclarationCounts) == 0 {
		t.Fatalf("compact digest lost file evidence: %+v", file)
	}
	found := false
	for _, entry := range file.EntryPoints {
		if entry.Name == "NewRouter" && entry.Kind == "function" && entry.Line > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing useful entry point: %+v", file.EntryPoints)
	}
	result, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Paths: []string{path}, Presentation: "tree",
	})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid digest presentation accepted: result=%+v err=%v", result, err)
	}
}

func TestSourceSymbolsOutlinePresentations(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "sample.go")
	code := "package sample\ntype Box struct { Value int }\nfunc (b *Box) Get() int { return b.Value }\n"
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	call := func(view string, kinds []string) SourceSymbolsOutput {
		t.Helper()
		result, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Paths: []string{path}, Language: "go",
			Presentation: view, Kinds: kinds, IncludeSignatures: true,
		})
		if result != nil || err != nil {
			t.Fatalf("outline %q: %+v %v", view, result, err)
		}
		return out
	}
	defaultTree, explicitTree := call("", nil), call("tree", nil)
	if !reflect.DeepEqual(defaultTree, explicitTree) || len(defaultTree.OutlineFiles) != 1 || len(defaultTree.Symbols) != 0 {
		t.Fatal("omitted presentation must return the tree view")
	}
	legacy := call("detailed", nil)
	if len(legacy.Symbols) == 0 || len(legacy.Files) != 1 || len(legacy.OutlineFiles) != 0 {
		t.Fatal("explicit detailed must retain the legacy output contract")
	}
	for _, view := range []string{"compact", "tree"} {
		out := call(view, nil)
		if out.SymbolCount != legacy.SymbolCount || out.CoverageComplete != legacy.CoverageComplete ||
			len(out.Symbols) != 0 || len(out.Files) != 0 || len(out.OutlineFiles) != 1 {
			t.Fatalf("%s changed counts or output shape: %+v", view, out)
		}
		file := out.OutlineFiles[0]
		if file.Path != path || file.SourceFingerprint != legacy.Files[0].SourceFingerprint ||
			file.Encoding != "utf-8" {
			t.Fatalf("%s missing file identity: %+v", view, file)
		}
		seen := make(map[string]bool)
		var walk func([]*SourceOutlineNode)
		walk = func(nodes []*SourceOutlineNode) {
			for _, n := range nodes {
				if n.SymbolID == "" || seen[n.SymbolID] || n.Line <= 0 {
					t.Fatalf("%s duplicate or invalid node: %+v", view, n)
				}
				seen[n.SymbolID] = true
				if n.Name == "Get" && n.Signature == "" {
					t.Fatalf("%s method signature missing", view)
				}
				walk(n.Children)
			}
		}
		walk(file.Symbols)
		if len(seen) != legacy.SymbolCount {
			t.Fatalf("%s retained %d/%d symbols", view, len(seen), legacy.SymbolCount)
		}
	}
	filtered := call("tree", []string{"method"})
	if len(filtered.OutlineFiles) != 1 || len(filtered.OutlineFiles[0].Symbols) != 1 ||
		filtered.OutlineFiles[0].Symbols[0].ParentID == "" {
		t.Fatalf("lost filtered ancestor: %+v", filtered.OutlineFiles)
	}
	result, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "unsupported",
	})
	if result == nil || !result.IsError || err != nil || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid view: %+v %v", result, err)
	}
}
func TestSourceSymbolsOutlineKindsLimitAfterAnalysis(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	source := "package sample\ntype Box struct { First int; Second int; Third int }\nfunc (b *Box) One() {}\nfunc (b *Box) Two() {}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	call := func(limit int) SourceSymbolsOutput {
		t.Helper()
		result, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: "tree", Paths: []string{path}, Language: "go",
			Kinds: []string{"method"}, MaxFiles: 1, MaxSymbols: limit,
		})
		if err != nil || result != nil {
			t.Fatalf("outline filtered limit %d: result=%+v err=%v", limit, result, err)
		}
		return out
	}
	both := call(2)
	if both.SymbolCount != 2 || !both.CoverageComplete || both.Truncated ||
		len(both.OutlineFiles) != 1 || !both.OutlineFiles[0].CoverageComplete ||
		len(both.OutlineFiles[0].Symbols) != 2 {
		t.Fatalf("matching symbols must not be lost behind unmatched declarations: %+v", both)
	}
	if both.OutlineFiles[0].Symbols[0].Name != "One" ||
		both.OutlineFiles[0].Symbols[1].Name != "Two" {
		t.Fatalf("unexpected filtered ordering: %+v", both.OutlineFiles[0].Symbols)
	}
	first := call(1)
	if first.SymbolCount != 1 || !first.Truncated || first.CoverageComplete ||
		len(first.OutlineFiles) != 1 || !first.OutlineFiles[0].CoverageComplete {
		t.Fatalf("matching output truncation must not be confused with analyzer coverage: %+v", first)
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
	_, first, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, IncludeSignatures: false})
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, IncludeSignatures: false})
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

	_, filtered, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, Includes: []string{"*.go"}, Kinds: []string{"function"}})
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
	input := SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, Encoding: "utf-8"}
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

func TestSourceSymbolsFindRelevantFileDetails(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("file-%02d.go", i)
		symbol := fmt.Sprintf("Other%02d", i)
		if i == 4 {
			symbol = "Wanted"
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("package sample\nfunc "+symbol+"() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "extra.txt"), []byte("unrecognized"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	input := SourceSymbolsInput{Operation: "find", Paths: []string{root}, Query: "Wanted", Match: "exact", MaxSymbols: 2}
	toolErr, full, err := h.SourceSymbols(context.Background(), nil, input)
	if err != nil || toolErr != nil || len(full.Files) != 9 {
		t.Fatalf("full find: %+v %+v %v", toolErr, full, err)
	}
	input.FileDetails = "relevant"
	toolErr, compact, err := h.SourceSymbols(context.Background(), nil, input)
	if err != nil || toolErr != nil {
		t.Fatalf("compact find: %+v %v", toolErr, err)
	}
	if len(compact.Files) != 2 || filepath.Base(compact.Files[0].Path) != "extra.txt" ||
		filepath.Base(compact.Files[1].Path) != "file-04.go" {
		t.Fatalf("compact file evidence: %+v", compact.Files)
	}
	if !reflect.DeepEqual(compact.Symbols, full.Symbols) || compact.FilesConsidered != full.FilesConsidered ||
		compact.FilesParsed != full.FilesParsed || compact.FilesSkipped != full.FilesSkipped ||
		compact.CoverageComplete != full.CoverageComplete || compact.Truncated != full.Truncated {
		t.Fatalf("compact changed result or coverage: %+v vs %+v", compact, full)
	}
	input.Query = "Missing"
	_, missing, err := h.SourceSymbols(context.Background(), nil, input)
	if err != nil || len(missing.Files) != 1 || missing.Files[0].Status == "parsed" {
		t.Fatalf("missing must retain unsupported file evidence: %+v %v", missing, err)
	}
	input.FileDetails = "unknown"
	toolErr, _, err = h.SourceSymbols(context.Background(), nil, input)
	if err != nil || toolErr == nil || !toolErr.IsError || toolErr.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("unknown file detail: %+v %v", toolErr, err)
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

func TestSourceSymbolsValidationOrderAndMessages(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	tooManyKinds := make([]string, 33)
	tests := []struct {
		name    string
		input   SourceSymbolsInput
		code    string
		message string
	}{
		{
			name:    "show dispatch precedes batch validation",
			input:   SourceSymbolsInput{Operation: " SHOW "},
			code:    ErrCodeInvalidInput,
			message: "show requires path, symbolId, sourceFingerprint, language, and encoding",
		},
		{
			name:    "operation precedes paths",
			input:   SourceSymbolsInput{Operation: "unknown"},
			code:    ErrCodeInvalidInput,
			message: "operation must be outline, digest, find, or show",
		},
		{
			name:    "paths required",
			input:   SourceSymbolsInput{Operation: "outline"},
			code:    ErrCodeInvalidInput,
			message: "paths must contain at least one path",
		},
		{
			name:    "max files precedes max symbols",
			input:   SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{"unused"}, MaxFiles: -1, MaxSymbols: -1},
			code:    ErrCodeInvalidInput,
			message: "maxFiles must be positive",
		},
		{
			name:    "max symbols precedes find query",
			input:   SourceSymbolsInput{Operation: "find", Paths: []string{"unused"}, MaxSymbols: -1},
			code:    ErrCodeInvalidInput,
			message: "maxSymbols must be positive",
		},
		{
			name:    "find query validation",
			input:   SourceSymbolsInput{Operation: "find", Paths: []string{"unused"}},
			code:    ErrCodeInvalidInput,
			message: "query must be a non-empty string up to 512 Unicode scalar values",
		},
		{
			name:    "find match validation",
			input:   SourceSymbolsInput{Operation: "find", Paths: []string{"unused"}, Query: "name", Match: "suffix"},
			code:    ErrCodeInvalidInput,
			message: "match must be exact, prefix, or qualified",
		},
		{
			name:    "kinds limit",
			input:   SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{"unused"}, Kinds: tooManyKinds},
			code:    ErrCodeLimit,
			message: "kinds exceeds the 32-item limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _, err := h.SourceSymbols(context.Background(), nil, test.input)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != test.code {
				t.Fatalf("result = %+v, want code %s", result, test.code)
			}
			if len(result.Content) != 1 {
				t.Fatalf("content = %+v, want one text item", result.Content)
			}
			content, ok := result.Content[0].(*mcp.TextContent)
			if !ok || content.Text != test.message {
				t.Fatalf("message = %#v, want %q", content, test.message)
			}
		})
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

	limitResult, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if limitResult == nil || !limitResult.IsError || limitResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("aggregate limit result = %+v", limitResult)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cancelResult, _, err := NewHandler([]string{root}).SourceSymbols(cancelled, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
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
	denied, _, err := NewHandler([]string{root}).SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{outside}, Language: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if denied == nil || !denied.IsError || denied.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("denied root result = %+v", denied)
	}
}

func TestSourceFileSelectionKeepsLexicographicPrefixWithoutRetainingAllPaths(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z.go", "a.go", "middle.go", "nested/b.go", "nested/a.go", "note.txt"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte("package demo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	paths := []string{filepath.Join(root, "z.go"), filepath.Join(root, "nested"), root, filepath.Join(root, "a.go")}
	want := []string{
		filepath.Join(root, "a.go"), filepath.Join(root, "middle.go"),
		filepath.Join(root, "nested", "a.go"), filepath.Join(root, "nested", "b.go"), filepath.Join(root, "z.go"),
	}
	for limit := 1; limit <= len(want)+1; limit++ {
		selected, truncated, err := h.collectBoundedSourceFiles(context.Background(), paths, []string{"*.go"}, nil, false, limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		expectedCount := len(want)
		if limit < expectedCount {
			expectedCount = limit
		}
		if !reflect.DeepEqual(selected, want[:expectedCount]) || truncated != (len(want) > limit) {
			t.Fatalf("limit %d: selected=%v truncated=%t; want=%v truncated=%t", limit, selected, truncated, want[:expectedCount], len(want) > limit)
		}
	}
}
