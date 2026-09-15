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

var historicalMilestoneIdentifierPattern = regexp.MustCompile(`(?i)(?:phase|wave)[0-9]+|r(?:1[0-9]|2[0-9]|3[0-9])`)
var historicalMilestoneFilenamePattern = regexp.MustCompile(`(?i)(?:^|[_\-.])(?:phase|wave)[0-9]+(?:[_\-.]|$)|(?:^|[_\-.])r(?:1[0-9]|2[0-9]|3[0-9])(?:[_\-.]|$)`)
var historicalPhaseOperationPattern = regexp.MustCompile(`(?i)analyze_phase[0-9]+`)

func TestGoIdentifiersAndFilenamesHaveNoHistoricalMilestoneNames(t *testing.T) {
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
		if historicalMilestoneFilenamePattern.MatchString(name) {
			violations = append(violations, name+": historical milestone filename")
		}
		path := filepath.Join(sourceDir, name)
		file, err := parser.ParseFile(fileset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.Ident:
				if historicalMilestoneIdentifierPattern.MatchString(value.Name) {
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
					if historicalMilestoneIdentifierPattern.MatchString(comment.Text) {
						position := fileset.Position(comment.Pos())
						violations = append(violations, position.String()+": comment "+comment.Text)
					}
				}
			}
		}
	}

	if len(violations) > 0 {
		t.Fatalf("historical milestone naming remains in Go identifiers, filenames, or production metadata:\n%s", strings.Join(violations, "\n"))
	}
}
