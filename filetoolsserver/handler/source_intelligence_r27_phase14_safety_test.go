package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestR27Phase14ContextMaterializationRejectsPostPlanMutation(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "current.java")
	content := "public class Current {}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	document, err := sourceintelligence.OpenSourceDocument(context.Background(), path, sourceintelligence.OpenDocumentOptions{
		RequestedEncoding: "utf-8", MaxFileBytes: int64(len(content) + 64), MaxDecodedCharacters: len(content) + 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := sourceintelligence.ProjectContextPlan{Candidates: []sourceintelligence.ProjectContextCandidate{{
		Entity: sourceintelligence.RelationEntity{Path: path, Language: "java", SymbolID: strings.Repeat("1", 64), SourceFingerprint: document.SourceFingerprint},
		Reason: sourceintelligence.ContextTarget, Representation: sourceintelligence.ContextSignature, Priority: 1,
		Offsets: sourceintelligence.OffsetRange{Start: 0, End: len(content)}, Evidence: sourceintelligence.SymbolEvidenceStructural, Resolution: sourceintelligence.ResolutionResolved,
	}}, UsedBytes: len(content)}
	if err := os.WriteFile(path, []byte(content+"// changed after plan\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	if _, _, err := h.materializeSourceContext(context.Background(), plan, "utf-8", h.sourceLimits()); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("post-plan mutation error = %v kind=%v", err, operation.KindOf(err))
	}
}

func TestR27Phase14SourceQueryContextPreservesDecodedUTF16CoordinatesAndBudget(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "Unicode.java")
	content := "package demo;\r\npublic class Unicode {\r\n    public int café(int value) { return value + 1; }\r\n}\r\n"
	if err := os.WriteFile(path, encodeGrepFixture(t, "utf-16-le", content, true), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Language: "java", Encoding: "utf-16-le", IncludeSignatures: true, MaxSymbols: 64,
	})
	if err != nil || toolErr != nil || len(outline.Files) != 1 {
		t.Fatalf("UTF-16 outline result=%+v output=%+v err=%v", toolErr, outline, err)
	}
	var target sourceintelligence.NormalizedSymbol
	for _, symbol := range outline.Symbols {
		if symbol.Name == "café" {
			target = symbol
			break
		}
	}
	if target.ID == "" {
		t.Fatalf("UTF-16 target missing: %+v", outline.Symbols)
	}
	result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "context", Paths: []string{root}, Language: "java", Encoding: "utf-16-le",
		Targets:     []SourceSelectorInput{{Kind: "symbol", Path: path, SymbolID: target.ID, SourceFingerprint: outline.Files[0].SourceFingerprint}},
		BudgetBytes: 4096, BodyPolicy: "prefer", MaxItems: 4, MaxDepth: 2,
	})
	if err != nil || (result != nil && result.IsError) || output.Context == nil || len(output.Context.Items) == 0 {
		t.Fatalf("UTF-16 context result=%+v output=%+v err=%v", result, output, err)
	}
	item := output.Context.Items[0]
	if !strings.Contains(item.Text, "café") || item.Entity.Range == nil || item.Entity.Range.Start.Line != 3 {
		t.Fatalf("UTF-16 context item = %+v", item)
	}
	if output.Context.UsedBytes != phase14SafetyContextBytes(output.Context.Items) {
		t.Fatalf("UTF-16 context budget = %+v", output.Context)
	}
}

func phase14SafetyContextBytes(items []sourceintelligence.ContextItem) int {
	total := 0
	for _, item := range items {
		total += len(item.Text)
	}
	return total
}
