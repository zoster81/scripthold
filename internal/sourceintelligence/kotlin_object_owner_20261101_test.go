package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestKotlinObjectOwnsNestedDeclarations(t *testing.T) {
	source := "package demo\nobject Engine {\n    fun interface Factory {\n        fun make(): String\n    }\n}\n"
	for _, tc := range []struct{ name, eol string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(strings.ReplaceAll(source, "\n", tc.eol)), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("partial: %+v", result.Analysis.Diagnostics)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			owner, ok := symbols["demo.Engine"]
			if !ok || owner.NativeKind != "object" || owner.NameRange.Start.Line != 2 {
				t.Fatalf("Kotlin object missing: %+v, all=%v", owner, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			child, ok := symbols["demo.Engine.Factory"]
			if !ok || child.Kind != SymbolKindInterface || child.ParentID != owner.ID || child.NameRange.Start.Line != 3 {
				t.Fatalf("nested fun interface has wrong owner: %+v, all=%v", child, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			if _, wrong := symbols["demo.Factory"]; wrong {
				t.Fatal("nested interface escaped object owner")
			}
		})
	}
}

func TestKotlinOriginalCoilObjectOwnership(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_KOTLIN_COIL_SINGLETON")
	if path == "" {
		t.Skip("qualified original fixture not configured")
	}
	stat, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() > 1<<20 {
		t.Fatal("original fixture exceeds 1MiB")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(body))
	doc.Path = path
	result, err := (KotlinAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 20000))
	if err != nil {
		t.Fatal(err)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	owner, ok := symbols["coil3.SingletonImageLoader"]
	if !ok || owner.NativeKind != "object" || owner.NameRange.Start.Line != 12 {
		t.Fatalf("missing original object: %+v, all=%v", owner, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	child, ok := symbols["coil3.SingletonImageLoader.Factory"]
	if !ok || child.Kind != SymbolKindInterface || child.ParentID != owner.ID || child.NameRange.Start.Line != 114 {
		t.Fatalf("original Factory wrong parent: %+v, all=%v", child, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestKotlinValueClassNestedOwners(t *testing.T) {
	const source = "package demo\n" +
		"interface Base {\n" +
		"  val key: String\n" +
		"  @JvmInline\n" +
		"  value class First(val value: String) : Base {\n" +
		"    override val key: String\n" +
		"      get() = value\n" +
		"    fun read(): String = key\n" +
		"  }\n" +
		"  @JvmInline\n" +
		"  value class Second(val value: String) : Base {\n" +
		"    override val key: String\n" +
		"      get() = value\n" +
		"  }\n" +
		"}\n" +
		"@JvmInline\n" +
		"value class Top(val value: Int) {\n" +
		"  fun plusOne(): Int = value + 1\n" +
		"}\n"
	for _, variant := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(variant.name, func(t *testing.T) {
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", variant.eol)),
				testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("supported Kotlin declarations reported partial: %+v", result.Analysis)
			}
			symbols := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, tc := range []struct {
				name, parent, native string
				kind                 SymbolKind
				line                 int
			}{
				{"demo.Base.First", "demo.Base", "value-class", SymbolKindClass, 5},
				{"demo.Base.Second", "demo.Base", "value-class", SymbolKindClass, 11},
				{"demo.Top", "demo", "value-class", SymbolKindClass, 17},
				{"demo.Base.First.key", "demo.Base.First", "val", SymbolKindProperty, 6},
				{"demo.Base.Second.key", "demo.Base.Second", "val", SymbolKindProperty, 12},
				{"demo.Base.First.read", "demo.Base.First", "method", SymbolKindMethod, 8},
				{"demo.Top.plusOne", "demo.Top", "method", SymbolKindMethod, 18},
			} {
				sym, ok := symbols[tc.name]
				if !ok || sym.Kind != tc.kind || sym.NativeKind != tc.native ||
					sym.ParentQualifiedName != tc.parent || sym.NameRange.Start.Line != tc.line {
					t.Errorf("wrong Kotlin value-class symbol %s: %+v (exists=%t)", tc.name, sym, ok)
				}
			}
			if _, wrong := symbols["demo.Base.read"]; wrong {
				t.Error("nested value-class method escaped to interface")
			}
		})
	}
}
