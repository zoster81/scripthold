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
