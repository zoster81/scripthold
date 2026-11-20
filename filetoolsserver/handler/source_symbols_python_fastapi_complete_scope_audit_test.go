package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// FastAPI original files were both read in full before this independent
// inventory was prepared. Every direct module function, class, class method
// and class-level assignment/annotation is listed. Parameter annotations in
// multiline signatures, docstring examples, and method-local assignments
// must not become source declarations.
func TestPythonFastAPIOriginalCompleteSupportedDeclarationInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original FastAPI project corpus not configured")
	}
	project := filepath.Join(root, "python-fastapi")
	type decl struct {
		file                       string
		line                       int
		name, kind, native, parent string
	}
	originalDeclarations := []decl{
		{"fastapi/param_functions.py", 13, "Path", "function", "function", ""},
		{"fastapi/param_functions.py", 357, "Query", "function", "function", ""},
		{"fastapi/param_functions.py", 701, "Header", "function", "function", ""},
		{"fastapi/param_functions.py", 1018, "Cookie", "function", "function", ""},
		{"fastapi/param_functions.py", 1323, "Body", "function", "function", ""},
		{"fastapi/param_functions.py", 1653, "Form", "function", "function", ""},
		{"fastapi/param_functions.py", 1968, "File", "function", "function", ""},
		{"fastapi/param_functions.py", 2283, "Depends", "function", "function", ""},
		{"fastapi/param_functions.py", 2372, "Security", "function", "function", ""},
		{"fastapi/params.py", 19, "ParamTypes", "class", "class", ""},
		{"fastapi/params.py", 26, "Param", "class", "class", ""},
		{"fastapi/params.py", 137, "Path", "class", "class", ""},
		{"fastapi/params.py", 221, "Query", "class", "class", ""},
		{"fastapi/params.py", 303, "Header", "class", "class", ""},
		{"fastapi/params.py", 387, "Cookie", "class", "class", ""},
		{"fastapi/params.py", 469, "Body", "class", "class", ""},
		{"fastapi/params.py", 581, "Form", "class", "class", ""},
		{"fastapi/params.py", 663, "File", "class", "class", ""},
		{"fastapi/params.py", 746, "Depends", "class", "class", ""},
		{"fastapi/params.py", 753, "Security", "class", "class", ""},
		{"fastapi/params.py", 20, "query", "field", "field", "ParamTypes"},
		{"fastapi/params.py", 21, "header", "field", "field", "ParamTypes"},
		{"fastapi/params.py", 22, "path", "field", "field", "ParamTypes"},
		{"fastapi/params.py", 23, "cookie", "field", "field", "ParamTypes"},
		{"fastapi/params.py", 27, "in_", "field", "field", "Param"},
		{"fastapi/params.py", 138, "in_", "field", "field", "Path"},
		{"fastapi/params.py", 222, "in_", "field", "field", "Query"},
		{"fastapi/params.py", 304, "in_", "field", "field", "Header"},
		{"fastapi/params.py", 388, "in_", "field", "field", "Cookie"},
		{"fastapi/params.py", 747, "dependency", "field", "field", "Depends"},
		{"fastapi/params.py", 748, "use_cache", "field", "field", "Depends"},
		{"fastapi/params.py", 749, "scope", "field", "field", "Depends"},
		{"fastapi/params.py", 754, "scopes", "field", "field", "Security"},
		{"fastapi/params.py", 29, "__init__", "method", "method", "Param"},
		{"fastapi/params.py", 133, "__repr__", "method", "method", "Param"},
		{"fastapi/params.py", 140, "__init__", "method", "method", "Path"},
		{"fastapi/params.py", 224, "__init__", "method", "method", "Query"},
		{"fastapi/params.py", 306, "__init__", "method", "method", "Header"},
		{"fastapi/params.py", 390, "__init__", "method", "method", "Cookie"},
		{"fastapi/params.py", 470, "__init__", "method", "method", "Body"},
		{"fastapi/params.py", 577, "__repr__", "method", "method", "Body"},
		{"fastapi/params.py", 582, "__init__", "method", "method", "Form"},
		{"fastapi/params.py", 664, "__init__", "method", "method", "File"},
	}
	files := []string{"fastapi/param_functions.py", "fastapi/params.py"}
	originals := map[string][]string{}
	paths := make([]string, 0, len(files))
	for _, rel := range files {
		full := filepath.Join(project, filepath.FromSlash(rel))
		requireAdditionalOriginalFile(t, full)
		data, err := os.ReadFile(full)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 1<<20 {
			t.Fatalf("oversized original %s", rel)
		}
		originals[rel] = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		paths = append(paths, full)
	}
	oracle := map[string]decl{}
	for _, d := range originalDeclarations {
		lines, present := originals[d.file]
		if !present || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], d.name) {
			t.Fatalf("original declaration absent at %s:%d: %s", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, found := oracle[key]; found {
			t.Fatalf("duplicate source position %s", key)
		}
		oracle[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "python", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: len(files), MaxSymbols: 128,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(files) {
		t.Fatalf("incomplete FastAPI source outline error=%v failed=%+v out=%+v", err, failed, out)
	}
	observed := map[string]bool{}
	for _, sym := range out.Symbols {
		relative, err := filepath.Rel(project, sym.Path)
		if err != nil {
			t.Errorf("symbol outside project: %v", err)
			continue
		}
		key := filepath.ToSlash(relative) + ":" + strconv.Itoa(sym.NameRange.Start.Line)
		if observed[key] {
			t.Errorf("duplicate symbol at %s", key)
		}
		observed[key] = true
		expected, found := oracle[key]
		if !found {
			t.Errorf("false declaration at %s: %s %s owner=%s", key, sym.Kind, sym.Name, sym.ParentQualifiedName)
			continue
		}
		if sym.Name != expected.name || string(sym.Kind) != expected.kind || sym.NativeKind != expected.native || sym.ParentQualifiedName != expected.parent {
			t.Errorf("incorrect %s: got %s/%s native=%s owner=%s; want %+v", key, sym.Kind, sym.Name, sym.NativeKind, sym.ParentQualifiedName, expected)
		}
	}
	for key, d := range oracle {
		if !observed[key] {
			t.Errorf("missing original declaration at %s: %s", key, d.name)
		}
	}
	if len(out.Symbols) != len(oracle) {
		t.Errorf("independent original declarations=%d indexed=%d", len(oracle), len(out.Symbols))
	}
	t.Logf("FastAPI two entire original files; %d independently inventoried declarations; %d indexed; complete=%v", len(oracle), len(out.Symbols), out.CoverageComplete)
}
