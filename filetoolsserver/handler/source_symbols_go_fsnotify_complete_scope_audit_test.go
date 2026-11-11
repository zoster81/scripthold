package handler

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type fsnotifyDecl struct {
	file                       string
	line                       int
	name, kind, native, parent string
}

func fsnotifyEmbeddedName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return fsnotifyEmbeddedName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name
	case *ast.IndexExpr:
		return fsnotifyEmbeddedName(t.X)
	case *ast.IndexListExpr:
		return fsnotifyEmbeddedName(t.X)
	}
	return ""
}

// The independent syntax oracle is the Go standard-library parser over the
// complete original source bytes. It does not use sourceintelligence, runtime
// execution, build tags, type inference, or a projection-derived expected list.
func fsnotifyOriginalDeclarations(t *testing.T, full, relative string) []fsnotifyDecl {
	t.Helper()
	b, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > 1<<20 {
		t.Fatalf("original file too large: %s", relative)
	}
	fset := token.NewFileSet()
	tree, err := parser.ParseFile(fset, full, b, parser.AllErrors)
	if err != nil {
		t.Fatalf("Go original parser %s: %v", relative, err)
	}
	p := tree.Name.Name
	line := func(pos token.Pos) int { return fset.PositionFor(pos, false).Line }
	result := make([]fsnotifyDecl, 0, 160)
	add := func(pos token.Pos, name, kind, native, parent string) {
		if name == "" || name == "_" {
			return
		}
		result = append(result, fsnotifyDecl{relative, line(pos), name, kind, native, parent})
	}
	add(tree.Name.Pos(), p, "package", "package", "")
	for _, raw := range tree.Decls {
		switch decl := raw.(type) {
		case *ast.FuncDecl:
			if decl.Name == nil {
				continue
			}
			owner := p
			kind, native := "function", "function"
			if decl.Recv != nil && len(decl.Recv.List) > 0 {
				kind, native = "method", "method"
				if recv := fsnotifyEmbeddedName(decl.Recv.List[0].Type); recv != "" {
					owner = p + "." + recv
				}
			}
			add(decl.Name.Pos(), decl.Name.Name, kind, native, owner)
		case *ast.GenDecl:
			for _, rawSpec := range decl.Specs {
				switch spec := rawSpec.(type) {
				case *ast.TypeSpec:
					if decl.Tok != token.TYPE || spec.Name == nil {
						continue
					}
					kind, native := "type", "named-type"
					if spec.Assign.IsValid() {
						kind, native = "alias", "alias"
					} else {
						switch spec.Type.(type) {
						case *ast.StructType:
							kind, native = "struct", "struct"
						case *ast.InterfaceType:
							kind, native = "interface", "interface"
						}
					}
					add(spec.Name.Pos(), spec.Name.Name, kind, native, p)
					parent := p + "." + spec.Name.Name
					var fields *ast.FieldList
					isStruct := false
					switch body := spec.Type.(type) {
					case *ast.StructType:
						fields = body.Fields
						isStruct = true
					case *ast.InterfaceType:
						fields = body.Methods
					}
					if fields == nil {
						continue
					}
					for _, fld := range fields.List {
						if fld == nil {
							continue
						}
						if len(fld.Names) == 0 {
							name := fsnotifyEmbeddedName(fld.Type)
							if isStruct {
								add(fld.Type.Pos(), name, "field", "embedded-field", parent)
							} else {
								add(fld.Type.Pos(), name, "field", "embedded-interface", parent)
							}
							continue
						}
						for _, n := range fld.Names {
							if n == nil {
								continue
							}
							k, nativeKind := "field", "field"
							if !isStruct {
								k, nativeKind = "field", "interface-member"
								if _, ok := fld.Type.(*ast.FuncType); ok {
									k, nativeKind = "method", "interface-method"
								}
							}
							add(n.Pos(), n.Name, k, nativeKind, parent)
						}
					}
				case *ast.ValueSpec:
					k, native := "variable", "var"
					if decl.Tok == token.CONST {
						k, native = "constant", "const"
					} else if decl.Tok != token.VAR {
						continue
					}
					for _, n := range spec.Names {
						if n != nil {
							add(n.Pos(), n.Name, k, native, p)
						}
					}
				}
			}
		}
	}
	return result
}

func TestGoFsnotifyOriginalCompleteSyntaxDeclarationInventory(t *testing.T) {
	checkGoOriginalSyntaxDeclarations(t, "go-fsnotify", []string{"fsnotify.go", "backend_inotify.go", "backend_windows.go", "fsnotify_test.go"})
}

func TestGoGorillaMuxOriginalCompleteSyntaxDeclarationInventory(t *testing.T) {
	checkGoOriginalSyntaxDeclarations(t, "go-gorilla-mux", []string{"route.go", "mux.go", "mux_test.go", "middleware.go"})
}

func checkGoOriginalSyntaxDeclarations(t *testing.T, projectName string, files []string) {
	t.Helper()
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original Go project corpus not configured")
	}
	project := filepath.Join(root, projectName)
	paths := make([]string, 0, len(files))
	oracle := make(map[string]fsnotifyDecl)
	key := func(d fsnotifyDecl) string {
		return d.file + ":" + strconv.Itoa(d.line) + ":" + d.name + ":" + d.kind + ":" + d.parent
	}
	perFile := map[string]int{}
	for _, file := range files {
		full := filepath.Join(project, file)
		requireAdditionalOriginalFile(t, full)
		paths = append(paths, full)
		expected := fsnotifyOriginalDeclarations(t, full, file)
		perFile[file] = len(expected)
		for _, d := range expected {
			k := key(d)
			if _, exists := oracle[k]; exists {
				t.Fatalf("duplicate independent Go AST declaration: %+v", d)
			}
			oracle[k] = d
		}
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "go", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: 4, MaxSymbols: 256,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 4 {
		t.Fatalf("incomplete local handler Go source analysis err=%v failed=%+v out=%+v", err, failed, out)
	}
	observed := map[string]bool{}
	for _, sym := range out.Symbols {
		rel, err := filepath.Rel(project, sym.Path)
		if err != nil {
			t.Errorf("symbol outside original project: %v", err)
			continue
		}
		d := fsnotifyDecl{filepath.ToSlash(rel), sym.NameRange.Start.Line, sym.Name, string(sym.Kind), sym.NativeKind, sym.ParentQualifiedName}
		k := key(d)
		if observed[k] {
			t.Errorf("duplicate original Go result %s", k)
		}
		observed[k] = true
		expected, found := oracle[k]
		if !found {
			t.Errorf("invented/misowned declaration %+v", d)
			continue
		}
		if sym.NativeKind != expected.native {
			t.Errorf("native kind mismatch at %s: got %s want %s", k, sym.NativeKind, expected.native)
		}
	}
	for k := range oracle {
		if !observed[k] {
			t.Errorf("missing original Go declaration %s", k)
		}
	}
	if len(out.Symbols) != len(oracle) {
		t.Errorf("original Go AST %d vs handler symbols %d", len(oracle), len(out.Symbols))
	}
	t.Logf("Go %s complete selected files=4; independent Go parser declarations=%d; symbols=%d, files=%v", projectName, len(oracle), len(out.Symbols), perFile)
}
