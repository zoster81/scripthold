package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Entire selected original MediatR files were independently inspected before
// comparing the local source_symbols detailed outline. The inventory includes
// namespaces, classes, fields, properties, constructors and methods, but does
// not imply full project coverage or runtime behavior.
func TestCSharpMediatROriginalCompleteDeclarationInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original MediatR source corpus not configured")
	}
	project := filepath.Join(root, "csharp-mediatr", "samples", "MediatR.Examples")
	type declaration struct {
		file               string
		line               int
		name, kind, parent string
	}
	independentlyInventoried := []declaration{
		{"Ping.cs", 1, "MediatR.Examples", "namespace", ""},
		{"Ping.cs", 3, "Ping", "class", "MediatR.Examples"},
		{"Ping.cs", 5, "Message", "property", "MediatR.Examples.Ping"},
		{"PingHandler.cs", 4, "MediatR.Examples", "namespace", ""},
		{"PingHandler.cs", 8, "PingHandler", "class", "MediatR.Examples"},
		{"PingHandler.cs", 10, "_writer", "field", "MediatR.Examples.PingHandler"},
		{"PingHandler.cs", 12, "PingHandler", "constructor", "MediatR.Examples.PingHandler"},
		{"PingHandler.cs", 17, "Handle", "method", "MediatR.Examples.PingHandler"},
		{"Pong.cs", 1, "MediatR.Examples", "namespace", ""},
		{"Pong.cs", 3, "Pong", "class", "MediatR.Examples"},
		{"Pong.cs", 5, "Message", "property", "MediatR.Examples.Pong"},
		{"Runner.cs", 5, "MediatR.Examples", "namespace", ""},
		{"Runner.cs", 11, "Runner", "class", "MediatR.Examples"},
		{"Runner.cs", 13, "Run", "method", "MediatR.Examples.Runner"},
		{"Runner.cs", 182, "IsHandlerForSameExceptionWorks", "method", "MediatR.Examples.Runner"},
		{"Runner.cs", 201, "IsHandlerForBaseExceptionWorks", "method", "MediatR.Examples.Runner"},
		{"Runner.cs", 220, "IsHandlerForLessSpecificExceptionWorks", "method", "MediatR.Examples.Runner"},
		{"Runner.cs", 239, "IsPreferredHandlerForBaseExceptionWorks", "method", "MediatR.Examples.Runner"},
		{"Runner.cs", 259, "IsOverriddenHandlerForBaseExceptionWorks", "method", "MediatR.Examples.Runner"},
		{"Runner.cs", 279, "IsExceptionHandledBy", "method", "MediatR.Examples.Runner"},
		{"Runner.cs", 293, "RunResults", "class", "MediatR.Examples"},
		{"Runner.cs", 295, "RequestHandlers", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 296, "VoidRequestsHandlers", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 297, "PipelineBehaviors", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 298, "RequestPreProcessors", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 299, "RequestPostProcessors", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 300, "OrderedPipelineBehaviors", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 301, "ConstrainedGenericBehaviors", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 302, "NotificationHandler", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 303, "MultipleNotificationHandlers", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 304, "CovariantNotificationHandler", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 305, "ConstrainedGenericNotificationHandler", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 306, "HandlerForSameException", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 307, "HandlerForBaseException", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 308, "HandlerForLessSpecificException", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 309, "PreferredHandlerForBaseException", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 310, "OverriddenHandlerForBaseException", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 313, "StreamRequestHandlers", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 314, "StreamPipelineBehaviors", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 315, "StreamOrderedPipelineBehaviors", "property", "MediatR.Examples.RunResults"},
		{"Runner.cs", 318, "WrappingWriter", "class", "MediatR.Examples"},
		{"Runner.cs", 320, "_innerWriter", "field", "MediatR.Examples.WrappingWriter"},
		{"Runner.cs", 321, "_stringWriter", "field", "MediatR.Examples.WrappingWriter"},
		{"Runner.cs", 323, "WrappingWriter", "constructor", "MediatR.Examples.WrappingWriter"},
		{"Runner.cs", 328, "Write", "method", "MediatR.Examples.WrappingWriter"},
		{"Runner.cs", 334, "WriteLineAsync", "method", "MediatR.Examples.WrappingWriter"},
		{"Runner.cs", 340, "Encoding", "property", "MediatR.Examples.WrappingWriter"},
		{"Runner.cs", 342, "Contents", "property", "MediatR.Examples.WrappingWriter"},
	}
	files := []string{"Ping.cs", "PingHandler.cs", "Pong.cs", "Runner.cs"}
	originals := map[string][]string{}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		path := filepath.Join(project, file)
		requireAdditionalOriginalFile(t, path)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(bytes) > 1<<20 {
			t.Fatalf("oversized selected original: %s", file)
		}
		originals[file] = strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")
		paths = append(paths, path)
	}
	expected := map[string]declaration{}
	for _, d := range independentlyInventoried {
		lines, exists := originals[d.file]
		if !exists || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], d.name) {
			t.Fatalf("invalid original declaration %s:%d %s", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, duplicated := expected[key]; duplicated {
			t.Fatalf("duplicate original declaration location: %s", key)
		}
		expected[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "csharp", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: 4, MaxSymbols: 128,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 4 {
		t.Fatalf("incomplete original-file outline: err=%v failed=%+v result=%+v", err, failed, out)
	}
	actual := map[string]bool{}
	for _, s := range out.Symbols {
		file := filepath.Base(s.Path)
		key := file + ":" + strconv.Itoa(s.NameRange.Start.Line)
		if filepath.Clean(s.Path) != filepath.Join(project, file) {
			t.Errorf("symbol from unexpected file: %+v", s)
		}
		if actual[key] {
			t.Errorf("duplicate declared position: %s", key)
		}
		actual[key] = true
		d, exists := expected[key]
		if !exists {
			t.Errorf("invented source declaration: %s, %s %s", key, s.Kind, s.Name)
			continue
		}
		if s.Name != d.name || string(s.Kind) != d.kind || s.NativeKind != d.kind || s.ParentQualifiedName != d.parent {
			t.Errorf("misidentified original declaration at %s: got %s %s native=%q owner=%q; want %+v", key, s.Kind, s.Name, s.NativeKind, s.ParentQualifiedName, d)
		}
	}
	for key, d := range expected {
		if !actual[key] {
			t.Errorf("missing original declaration at %s: %s", key, d.name)
		}
	}
	if len(out.Symbols) != len(independentlyInventoried) {
		t.Errorf("complete original files expected %d declarations; tool returned %d", len(independentlyInventoried), len(out.Symbols))
	}
	t.Logf("original complete selected files=%d independently inventoried=%d returned=%d coverageComplete=%v", len(files), len(independentlyInventoried), len(out.Symbols), out.CoverageComplete)
}
