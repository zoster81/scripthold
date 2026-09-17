package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/config"
)

func TestHandleMarkdownReadExercisesMarkspliceBackedActions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	source := "# Project\r\n\r\nSee [child](#child).\r\n\r\n## Child\r\n\r\nBody.\r\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	result, inspected, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "inspect", Path: path, Encoding: "utf-8", Limit: 16,
	})
	if err != nil || result.IsError {
		t.Fatalf("inspect result=%+v output=%+v err=%v", result, inspected, err)
	}
	if len(inspected.SourceFingerprint) != 64 || inspected.Encoding != "utf-8" || inspected.LineEndings.Style != LineEndingCRLF || inspected.Inspect == nil || len(inspected.Inspect.Sections) != 2 {
		t.Fatalf("inspect output=%+v", inspected)
	}

	result, queried, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "query", Path: path, Encoding: "utf-8", Query: "nodes", Kinds: []string{"heading"}, Limit: 8,
	})
	if err != nil || result.IsError || len(queried.Nodes) != 2 {
		t.Fatalf("query result=%+v output=%+v err=%v", result, queried, err)
	}
	targetID := queried.Nodes[0].TargetID
	if len(targetID) != 64 {
		t.Fatalf("target id=%q", targetID)
	}

	result, got, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "get", Path: path, Encoding: "utf-8", TargetID: targetID, IncludeSource: true,
	})
	if err != nil || result.IsError || got.Target == nil || got.Target.Kind != "heading" || !strings.Contains(got.Source, "Project") {
		t.Fatalf("get result=%+v output=%+v err=%v", result, got, err)
	}

	result, resolved, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "resolve", Path: path, Encoding: "utf-8", Fragment: "#child",
	})
	if err != nil || result.IsError || resolved.Fragment == nil || resolved.Fragment.TargetID == "" {
		t.Fatalf("resolve result=%+v output=%+v err=%v", result, resolved, err)
	}

	result, validated, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "validate", Path: path, Encoding: "utf-8", Fragment: "child",
	})
	if err != nil || result.IsError || validated.Valid == nil || !*validated.Valid {
		t.Fatalf("validate result=%+v output=%+v err=%v", result, validated, err)
	}

	result, generated, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "generate", Path: path, Encoding: "utf-8", Generate: "toc",
	})
	if err != nil || result.IsError || !strings.Contains(generated.Generated, "Project") || !strings.Contains(generated.Generated, "Child") {
		t.Fatalf("generate result=%+v output=%+v err=%v", result, generated, err)
	}
}

func TestHandleMarkdownReadRejectsInvalidUnionLimitsTargetsAndPaths(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	if err := os.WriteFile(path, []byte("# Project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	invalid := []MarkdownReadInput{
		{},
		{Action: "unknown", Path: path},
		{Action: "inspect", Path: path},
		{Action: "inspect", Path: path, Limit: -1},
		{Action: "query", Path: path, Query: "nodes", Limit: 1, TargetID: strings.Repeat("a", 64)},
		{Action: "get", Path: path},
		{Action: "resolve", Path: path},
		{Action: "generate", Path: path, Generate: "html"},
	}
	for _, input := range invalid {
		result, _, err := h.HandleMarkdownRead(context.Background(), nil, input)
		if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
			t.Fatalf("invalid input=%+v result=%+v err=%v", input, result, err)
		}
	}

	missing, _, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{
		Action: "get", Path: path, TargetID: strings.Repeat("a", 64),
	})
	if err != nil || missing == nil || !missing.IsError || missing.Meta[ErrorCodeMetaKey] != ErrCodeNotFound || missing.Meta[MarkdownErrorCodeMetaKey] != MarkdownErrTargetNotFound {
		t.Fatalf("missing target result=%+v err=%v", missing, err)
	}

	outside := filepath.Join(filepath.Dir(root), "outside.md")
	if err := os.WriteFile(outside, []byte("# Outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	denied, _, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "inspect", Path: outside, Limit: 1})
	if err != nil || denied == nil || !denied.IsError || denied.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("denied result=%+v err=%v", denied, err)
	}
}

func TestHandleMarkdownReadEnforcesConfiguredOutputBudget(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "guide.md")
	if err := os.WriteFile(path, []byte("# Project\n\n## Child\n\nBody.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load()
	cfg.Limits.MaxOutputBytes = 1
	h := NewHandler([]string{root}, WithConfig(cfg))
	result, _, err := h.HandleMarkdownRead(context.Background(), nil, MarkdownReadInput{Action: "inspect", Path: path, Limit: 8})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("limited result=%+v err=%v", result, err)
	}
}
