package sourceintelligence

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPrivateSymbolOffsetSnapshotsRemainOwnerOnly(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve symbol offset immutability test path")
	}
	sourceDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatalf("read source directory: %v", err)
	}

	allowed := map[string]struct{}{
		"normalizeSymbol":   {},
		"SourceOffsets":     {},
		"sameSourceOffsets": {},
	}
	privateFields := map[string]struct{}{
		"signatureOffsets": {},
		"bodyOffsets":      {},
	}
	fileset := token.NewFileSet()
	var violations []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		path := filepath.Join(sourceDir, name)
		file, err := parser.ParseFile(fileset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			ast.Inspect(function.Body, func(node ast.Node) bool {
				fieldName := ""
				fieldPos := token.NoPos
				switch value := node.(type) {
				case *ast.SelectorExpr:
					fieldName = value.Sel.Name
					fieldPos = value.Sel.Pos()
				case *ast.KeyValueExpr:
					if identifier, ok := value.Key.(*ast.Ident); ok {
						fieldName = identifier.Name
						fieldPos = identifier.Pos()
					}
				}
				if _, private := privateFields[fieldName]; !private {
					return true
				}
				if name == "symbol_builder.go" {
					if _, owner := allowed[function.Name.Name]; owner {
						return true
					}
				}
				position := fileset.Position(fieldPos)
				violations = append(violations, position.String()+": "+function.Name.Name+" accesses "+fieldName)
				return true
			})
		}
	}
	if len(violations) > 0 {
		t.Fatalf("private symbol offset snapshots escaped their immutable owners:\n%s", strings.Join(violations, "\n"))
	}
}
