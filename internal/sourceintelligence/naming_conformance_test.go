package sourceintelligence

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

var historicalPhaseIdentifierPattern = regexp.MustCompile(`(?i)phase[0-9]+`)
var historicalPhaseOperationPattern = regexp.MustCompile(`(?i)analyze_phase[0-9]+`)

func TestGoIdentifiersAndProductionMetadataHaveNoHistoricalPhaseNames(t *testing.T) {
	t.Helper()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve naming conformance test path")
	}
	sourceDir := filepath.Dir(currentFile)
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		t.Fatalf("read source directory: %v", err)
	}

	var violations []string
	fileset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		production := !strings.HasSuffix(name, "_test.go")
		path := filepath.Join(sourceDir, name)
		file, err := parser.ParseFile(fileset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.Ident:
				if historicalPhaseIdentifierPattern.MatchString(value.Name) {
					position := fileset.Position(value.Pos())
					violations = append(violations, position.String()+": identifier "+value.Name)
				}
			case *ast.BasicLit:
				if !production || value.Kind != token.STRING {
					return true
				}
				decoded, err := strconv.Unquote(value.Value)
				if err == nil && historicalPhaseOperationPattern.MatchString(decoded) {
					position := fileset.Position(value.Pos())
					violations = append(violations, position.String()+": operation string "+decoded)
				}
			}
			return true
		})

		if production {
			for _, group := range file.Comments {
				for _, comment := range group.List {
					if historicalPhaseIdentifierPattern.MatchString(comment.Text) {
						position := fileset.Position(comment.Pos())
						violations = append(violations, position.String()+": comment "+comment.Text)
					}
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Fatalf("historical phase naming remains in Go identifiers or production metadata:\n%s", strings.Join(violations, "\n"))
	}
}
