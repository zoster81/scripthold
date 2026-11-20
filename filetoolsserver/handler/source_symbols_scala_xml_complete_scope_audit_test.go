package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestScalaXMLSelectedOriginalFileMemberInventories inventories the selected
// real files from direct source reads. Top-level and type-member declarations,
// including overloads and primary-constructor val parameters, are in scope.
// Function-local vals/defs/objects and compiler resolution remain excluded;
// those must not be described as fully indexed language syntax.
func TestScalaXMLSelectedOriginalFileMemberInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original-project corpus not configured")
	}
	project := filepath.Join(root, "scala61-scala-xml-audit-selected")
	paths := []string{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", "shared/src/main/scala/scala/xml/Utility.scala", "shared/src/main/scala/scala/xml/Elem.scala", "jvm/src/test/scala/scala/xml/XMLSyntaxTest.scala"}
	type wantDecl struct {
		file                       string
		line                       int
		name, kind, native, parent string
	}
	expected := []wantDecl{
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 13, "scala", "package", "package", ""},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 14, "xml", "package", "package", "scala"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 13, "scala", "package", "package", ""},
		{"shared/src/main/scala/scala/xml/Utility.scala", 14, "xml", "package", "package", "scala"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 13, "scala", "package", "package", ""},
		{"shared/src/main/scala/scala/xml/Elem.scala", 14, "xml", "package", "package", "scala"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 32, "PrettyPrinter", "class", "class", "scala.xml"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 34, "this", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 36, "minimizeMode", "field", "val", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 37, "BrokenException", "class", "class", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 39, "Item", "class", "class", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 40, "Break", "module", "object", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 41, "toString", "method", "def", "scala.xml.PrettyPrinter.Break"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 43, "Box", "class", "class", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 44, "Para", "class", "class", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 46, "items", "field", "var", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 48, "cur", "field", "var", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 50, "reset", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 58, "cut", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 86, "makeBox", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 94, "makePara", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 100, "makeBreak", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 105, "leafTag", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 115, "startTag", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 128, "endTag", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 137, "childrenAreLeaves", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 145, "fits", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 148, "doPreserve", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 151, "traverse", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 210, "traverse", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 223, "format", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 225, "format", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 263, "format", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 272, "formatNodes", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/PrettyPrinter.scala", 283, "formatNodes", "method", "def", "scala.xml.PrettyPrinter"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 28, "Utility", "module", "object", "scala.xml"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 29, "SU", "field", "val", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 33, "implicitSbToString", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 37, "sbToString", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 42, "isAtomAndNotText", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 53, "trim", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 59, "combineAdjacentTextNodes", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 69, "trimProper", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 80, "sort", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 91, "sort", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 101, "escape", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 103, "Escapes", "module", "object", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 108, "pairs", "field", "val", "scala.xml.Utility.Escapes"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 115, "escMap", "field", "val", "scala.xml.Utility.Escapes"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 116, "unescMap", "field", "val", "scala.xml.Utility.Escapes"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 123, "escape", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 140, "unescape", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 147, "collectNamespaces", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 153, "collectNamespaces", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 185, "toXML", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 204, "serialize", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 217, "serializeImpl", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 272, "sequenceToXML", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 285, "splitName", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 294, "prefix", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 299, "hashCode", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 302, "appendQuoted", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 308, "appendQuoted", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 316, "appendEscapedQuoted", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 326, "getName", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 337, "checkAttributeValue", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 358, "parseAttributeValue", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Utility.scala", 412, "parseCharRef", "method", "def", "scala.xml.Utility"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 25, "Elem", "module", "object", "scala.xml"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 27, "apply", "method", "def", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 30, "unapplySeq", "method", "def", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 57, "Elem", "class", "class", "scala.xml"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 58, "prefix", "field", "val", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 59, "label", "field", "val", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 61, "scope", "field", "val", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 62, "minimizeEmpty", "field", "val", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 63, "child", "field", "val", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 66, "doCollectNamespaces", "method", "def", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 67, "doTransform", "method", "def", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 69, "attributes", "field", "val", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 81, "basisForHashCode", "method", "def", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 91, "%", "method", "def", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 100, "copy", "method", "def", "scala.xml.Elem"},
		{"shared/src/main/scala/scala/xml/Elem.scala", 112, "text", "method", "def", "scala.xml.Elem"},
		{"jvm/src/test/scala/scala/xml/XMLSyntaxTest.scala", 1, "scala.xml", "package", "package", ""},
		{"jvm/src/test/scala/scala/xml/XMLSyntaxTest.scala", 6, "XMLSyntaxTestJVM", "class", "class", "scala.xml"},
		{"jvm/src/test/scala/scala/xml/XMLSyntaxTest.scala", 9, "test3", "method", "def", "scala.xml.XMLSyntaxTestJVM"},
	}
	for _, rel := range paths {
		fullPath := filepath.Join(project, filepath.FromSlash(rel))
		requireAdditionalOriginalFile(t, fullPath)
		b, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatal(err)
		}
		if len(b) > 1<<20 {
			t.Fatalf("oversized original: %s", rel)
		}
		lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
		for _, w := range expected {
			if w.file != rel {
				continue
			}
			if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], w.name) {
				t.Fatalf("invalid independent source oracle %s:%d: %s", rel, w.line, w.name)
			}
		}
	}
	files := make([]string, 0, len(paths))
	for _, rel := range paths {
		files = append(files, filepath.Join(project, filepath.FromSlash(rel)))
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: files, Language: "scala", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: 4, MaxSymbols: 180,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 4 {
		t.Fatalf("incomplete original Scala file outline err=%v failed=%+v out=%+v", err, failed, out)
	}
	desired := map[string]wantDecl{}
	for _, d := range expected {
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, exists := desired[key]; exists {
			t.Fatalf("duplicate independent source oracle %s", key)
		}
		desired[key] = d
	}
	actual := map[string]bool{}
	for _, sym := range out.Symbols {
		rel, err := filepath.Rel(project, sym.Path)
		if err != nil {
			t.Errorf("unexpected symbol path %v", err)
			continue
		}
		rel = filepath.ToSlash(rel)
		key := rel + ":" + strconv.Itoa(sym.NameRange.Start.Line)
		if actual[key] {
			t.Errorf("duplicate output location %s", key)
		}
		actual[key] = true
		d, ok := desired[key]
		if !ok {
			t.Errorf("invented or local-scoped symbol %s: kind=%s name=%s owner=%s", key, sym.Kind, sym.Name, sym.ParentQualifiedName)
			continue
		}
		if d.name != sym.Name || d.kind != string(sym.Kind) || d.native != sym.NativeKind || d.parent != sym.ParentQualifiedName {
			t.Errorf("wrong source declaration %s expected=%+v got=%s %s native=%s owner=%s", key, d, sym.Kind, sym.Name, sym.NativeKind, sym.ParentQualifiedName)
		}
	}
	for key, d := range desired {
		if !actual[key] {
			t.Errorf("missing original %s name=%s", key, d.name)
		}
	}
	if len(out.Symbols) != len(expected) {
		t.Errorf("returned %d symbols for %d independently inventoried supported declarations", len(out.Symbols), len(expected))
	}
	t.Logf("Scala XML scope: four full original files, %d type/package/member declarations checked; nested function locals excluded", len(expected))
}
