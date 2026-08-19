package diagnostics

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionLogMessagesAreStatic(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	for _, relativeRoot := range []string{"cmd", "filetoolsserver", "internal"} {
		root := filepath.Join(repositoryRoot, relativeRoot)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if entry.Name() == "testdata" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			fileSet := token.NewFileSet()
			parsed, err := parser.ParseFile(fileSet, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !isLogMethod(selector.Sel.Name) || !isDiagnosticLoggerReceiver(selector.X) {
					return true
				}
				literal, ok := call.Args[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					position := fileSet.Position(call.Pos())
					t.Errorf("%s:%d: diagnostic log message must be a string literal", filepath.ToSlash(path), position.Line)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("scan production logging under %s: %v", root, err)
		}
	}
}

func isLogMethod(name string) bool {
	switch name {
	case "Debug", "Info", "Warn", "Error":
		return true
	default:
		return false
	}
}

func isDiagnosticLoggerReceiver(expression ast.Expr) bool {
	switch receiver := expression.(type) {
	case *ast.Ident:
		return receiver.Name == "slog" || receiver.Name == "logger"
	case *ast.SelectorExpr:
		return receiver.Sel.Name == "logger" || receiver.Sel.Name == "Logger"
	default:
		return false
	}
}
