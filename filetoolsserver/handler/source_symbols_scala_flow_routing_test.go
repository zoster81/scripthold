package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

func TestPublicRoutingAcrossScalaFlowMixedRepository(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	files := map[string]string{
		"Service.scala": `package demo
trait Worker:
  def run(value: Int): Int
class ScalaBox extends Worker:
  def run(value: Int): Int = value
`,
		"model.js.flow": `/* @flow */
export type ID = string;
export class FlowBox extends Base { run(value: ID): ID { return value; } }
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	h := NewHandler([]string{root})
	toolErr, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{root}, Encoding: "utf-8", IncludeSignatures: true, MaxSymbols: 128,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("Phase 16 outline err=%v toolErr=%+v", err, toolErr)
	}
	if outline.FilesConsidered != 2 || outline.FilesParsed != 2 || outline.FilesSkipped != 0 || !outline.CoverageComplete {
		t.Fatalf("Phase 16 outline summary=%+v", outline)
	}
	languages := map[string]bool{}
	for _, file := range outline.Files {
		if file.ErrorCode != "" || file.Detection.Language == "" {
			t.Fatalf("Phase 16 file routing=%+v", file)
		}
		languages[file.Detection.Language] = true
	}
	if !languages["scala"] || !languages["flow"] {
		t.Fatalf("Phase 16 routed languages=%v files=%+v", languages, outline.Files)
	}
	for name, language := range map[string]string{"ScalaBox": "scala", "FlowBox": "flow"} {
		toolErr, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{root}, Query: name, Match: "exact", Encoding: "utf-8", MaxSymbols: 16,
		})
		if err != nil || toolErr != nil {
			t.Fatalf("Phase 16 find %s err=%v toolErr=%+v", name, err, toolErr)
		}
		if len(found.Symbols) != 1 || found.Symbols[0].Name != name || found.Symbols[0].Language != language {
			t.Fatalf("Phase 16 find %s=%+v want one %s symbol", name, found.Symbols, language)
		}
	}

	for name, language := range map[string]string{"ScalaBox": "scala", "FlowBox": "flow"} {
		result, output, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{root}, Query: name, Mode: "structural", Match: "exact", MaxResults: 16,
		})
		if err != nil || (result != nil && result.IsError) {
			t.Fatalf("Phase 16 source_query %s result=%+v output=%+v err=%v", name, result, output, err)
		}
		if output.Search == nil || len(output.Search.Matches) != 1 {
			t.Fatalf("Phase 16 source_query %s output=%+v", name, output)
		}
		match := output.Search.Matches[0]
		if match.SymbolID == "" || match.Language != language || match.Evidence != sourceintelligence.SymbolEvidenceStructural {
			t.Fatalf("Phase 16 source_query %s match=%+v", name, match)
		}
		if output.Index.Staleness != sourceintelligence.IndexCurrent || output.Index.Generation == 0 || output.Index.Fingerprint == "" {
			t.Fatalf("Phase 16 source_query %s index=%+v", name, output.Index)
		}
	}
}
