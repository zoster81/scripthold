package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestR27Phase14SourceQueryContextBodySignatureBudgetAndStale(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "Service.java")
	content := "package demo;\npublic class Service {\n    public int run(int value) { return value + 1; }\n}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Language: "java", Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 64,
	})
	if err != nil || toolErr != nil || len(outline.Files) != 1 {
		t.Fatalf("outline result=%+v output=%+v err=%v", toolErr, outline, err)
	}
	var run sourceintelligence.NormalizedSymbol
	for _, symbol := range outline.Symbols {
		if symbol.QualifiedName == "demo.Service.run" {
			run = symbol
			break
		}
	}
	if run.ID == "" {
		t.Fatalf("run symbol missing: %+v", outline.Symbols)
	}
	selector := SourceSelectorInput{Kind: "symbol", Path: path, SymbolID: run.ID, SourceFingerprint: outline.Files[0].SourceFingerprint}

	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "context", Paths: []string{root}, Targets: []SourceSelectorInput{selector}, BudgetBytes: 4096,
		BodyPolicy: "prefer", Language: "java", Encoding: "utf-8", MaxItems: 8, MaxDepth: 2,
	})
	if err != nil || (result != nil && result.IsError) {
		t.Fatalf("context result=%+v output=%+v err=%v", result, output, err)
	}
	if output.Context == nil || len(output.Context.Items) < 2 {
		t.Fatalf("context output = %+v", output)
	}
	first := output.Context.Items[0]
	if first.Entity.SymbolID != run.ID || first.Reason != sourceintelligence.ContextTarget || first.Representation != sourceintelligence.ContextBody || !strings.Contains(first.Text, "public int run(int value)") {
		t.Fatalf("target context item = %+v", first)
	}
	used := 0
	for _, item := range output.Context.Items {
		used += len(item.Text)
	}
	if output.Context.UsedBytes != used || used > output.Context.BudgetBytes || output.Context.BudgetBytes != 4096 {
		t.Fatalf("context budget = %+v calculated=%d", output.Context, used)
	}

	signatureResult, signatureOutput, signatureErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "context", Paths: []string{root}, Targets: []SourceSelectorInput{selector}, BudgetBytes: 4096,
		BodyPolicy: "signatures-only", Language: "java", Encoding: "utf-8", MaxItems: 1, MaxDepth: 2,
	})
	if signatureErr != nil || (signatureResult != nil && signatureResult.IsError) || signatureOutput.Context == nil || len(signatureOutput.Context.Items) != 1 {
		t.Fatalf("signature context result=%+v output=%+v err=%v", signatureResult, signatureOutput, signatureErr)
	}
	if item := signatureOutput.Context.Items[0]; item.Representation != sourceintelligence.ContextSignature || strings.Contains(item.Text, "return value + 1") {
		t.Fatalf("signature-only item = %+v", item)
	}
	if !signatureOutput.Coverage.Truncated || signatureOutput.Coverage.CoverageComplete {
		t.Fatalf("signature-only coverage = %+v", signatureOutput.Coverage)
	}

	if err := os.WriteFile(path, []byte(content+"// changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	staleResult, _, staleErr := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "context", Paths: []string{root}, Targets: []SourceSelectorInput{selector}, BudgetBytes: 4096,
		BodyPolicy: "prefer", Language: "java", Encoding: "utf-8", MaxItems: 8, MaxDepth: 2,
	})
	if staleErr != nil {
		t.Fatalf("stale context transport error: %v", staleErr)
	}
	if staleResult == nil || !staleResult.IsError || staleResult.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale context result = %+v", staleResult)
	}
}
