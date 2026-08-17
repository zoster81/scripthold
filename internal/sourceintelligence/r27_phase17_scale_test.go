package sourceintelligence

import (
	"context"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase17SourceIntelligenceHasNoHiddenExecutionOrNetworkImports(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate sourceintelligence package")
	}
	directory := filepath.Dir(currentFile)
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := map[string]bool{
		"os/exec":  true,
		"log/slog": true,
		"net":      true,
		"net/http": true,
		"net/rpc":  true,
		"net/smtp": true,
		"net/url":  true,
	}
	fileset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(directory, name)
		file, err := parser.ParseFile(fileset, path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse imports %s: %v", name, err)
		}
		for _, imported := range file.Imports {
			value := strings.Trim(imported.Path.Value, "\"")
			if forbidden[value] {
				t.Errorf("production source-intelligence file %s imports forbidden external-execution/network package %q", name, value)
			}
		}
	}
}

func TestR27Phase17CancelledRefreshPublishesNoGeneration(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("project", "phase17", "cancel.ts")
	fact := phase15Facts(t, registry, path, "typescript", "export class Cancelled {}\n")
	manager, err := NewProjectIndexManager(ProjectIndexManagerLimits{MaxProjects: 1, MaxGenerations: 2})
	if err != nil {
		t.Fatal(err)
	}
	options := ProjectIndexRefreshOptions{
		ScopeFingerprint:    phase15Digest("phase17-cancel-scope"),
		AnalysisFingerprint: phase15Digest("phase17-analysis"),
		Snapshots:           []ProjectIndexFileSnapshot{{Path: path, SourceFingerprint: fact.SourceFingerprint}},
		ResolverLimits:      phase15ResolverLimits(),
	}
	started := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	options.Analyze = func(analyzeCtx context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		close(started)
		<-analyzeCtx.Done()
		return ProjectIndexAnalysisResult{}, operation.Wrap(operation.KindCancelled, "phase17_analyze", snapshot.Path, analyzeCtx.Err())
	}
	result := make(chan error, 1)
	go func() {
		_, refreshErr := manager.Refresh(ctx, registry, options)
		result <- refreshErr
	}()
	<-started
	cancel()
	if refreshErr := <-result; operation.KindOf(refreshErr) != operation.KindCancelled {
		t.Fatalf("cancelled refresh error=%v kind=%v", refreshErr, operation.KindOf(refreshErr))
	}

	options.Analyze = func(_ context.Context, snapshot ProjectIndexFileSnapshot) (ProjectIndexAnalysisResult, error) {
		copy := fact
		return ProjectIndexAnalysisResult{ObservedFingerprint: snapshot.SourceFingerprint, Facts: &copy}, nil
	}
	selection, err := manager.Refresh(context.Background(), registry, options)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Evidence.Generation != 1 || selection.Evidence.Staleness != IndexCurrent || selection.Coverage.FilesParsed != 1 {
		t.Fatalf("cancelled refresh leaked a generation: selection=%+v coverage=%+v", selection.Evidence, selection.Coverage)
	}
}

func BenchmarkR27Phase17AmbiguousDetection(b *testing.B) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		b.Fatal(err)
	}
	cases := []DetectionInput{
		{Path: "shared.m", Text: "function y = run(x)\n  y = x;\nend\n"},
		{Path: "shared.h", Text: "struct Item { int value; };\n"},
		{Path: "module.bas", Text: "Public Sub Run()\r\nEnd Sub\r\n"},
		{Path: "shared.inc", Text: "type TPoint = record X: Integer; end;\n"},
		{Path: "config.yaml", Text: "name: demo\nitems:\n  - one\n"},
		{Path: "schema.sql", Text: "CREATE TABLE items (id INTEGER);\n"},
		{Path: "app.js", Text: "export class Service { run() {} }\n"},
	}
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for _, input := range cases {
			if _, err := DetectLanguage(context.Background(), registry, input); err != nil {
				b.Fatal(err)
			}
		}
	}
}
