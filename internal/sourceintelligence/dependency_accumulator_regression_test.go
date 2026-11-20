package sourceintelligence

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func protoImportsFixture(count int) *SourceDocument {
	var source strings.Builder
	source.WriteString("syntax = \"proto3\";\npackage demo;\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&source, "import \"shared/%04d.proto\";\n", index)
	}
	source.WriteString("import \"shared/0000.proto\";\nmessage Ready {}\n")
	return testSourceDocument("fixture.proto", source.String())
}

func TestProtoDependencyImportFirstOccurrence(t *testing.T) {
	document := protoImportsFixture(64)
	result, err := (ProtoAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 128))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dependencies) != 64 {
		t.Fatalf("got %d dependencies, want 64 unique imports", len(result.Dependencies))
	}
	for i, dep := range result.Dependencies {
		if dep.Kind != StructuralDependencyImport || dep.Value != fmt.Sprintf("shared/%04d.proto", i) || dep.Evidence != SymbolEvidenceStructural {
			t.Fatalf("dependency %d not in first-seen source order: %+v", i, dep)
		}
	}
	if result.Dependencies[0].Range.Start.Line != 3 {
		t.Errorf("duplicate dependency replaced the original line: %+v", result.Dependencies[0])
	}
}

// A broad allocation guard prevents regression to rebuilding a dedup map for
// every one of the 128 imports; it intentionally tolerates scanner changes.
func TestProtoBulkImportAllocationBound(t *testing.T) {
	document := protoImportsFixture(128)
	options := testAnalyzeOptions(false, 8)
	allocations := testing.AllocsPerRun(5, func() {
		result, err := (ProtoAnalyzer{}).Analyze(context.Background(), document, options)
		if err != nil || len(result.Dependencies) != 128 {
			panic("unexpected Proto dependency collection result")
		}
	})
	if allocations > 500 {
		t.Fatalf("Proto 128-import analysis allocated %.0f objects; want <= 500", allocations)
	}
}

func TestStructuralDependencyAccumulatorPreservesMergeIdentity(t *testing.T) {
	input := []StructuralDependency{
		{Kind: StructuralDependencyImport, Value: "a", Alias: "original"},
		{Kind: StructuralDependencyImport, Value: "a", Alias: "later"},
		{Kind: StructuralDependencyInclude, Value: "a"},
		{Kind: StructuralDependencyKind("kind\x00qual"), Value: "target", Alias: "legacy-first"},
		{Kind: StructuralDependencyKind("kind"), Value: "qual\x00target", Alias: "legacy-collision"},
		{Kind: StructuralDependencyImport, Value: "tail"},
	}
	var acc structuralDependencyAccumulator
	for _, dep := range input {
		acc.appendUnique(dep)
	}
	expected := appendUniqueDependencies(nil, input)
	if !reflect.DeepEqual(acc.items, expected) || len(acc.items) != 4 {
		t.Fatalf("incremental dependencies %v do not match one-shot merge %v", acc.items, expected)
	}
}

func TestStructuralDependencyAccumulatorRejectsInvalidSourceRanges(t *testing.T) {
	doc := testSourceDocument("fixture.proto", "import \"valid.proto\";")
	var acc structuralDependencyAccumulator
	acc.add(doc, StructuralDependencyImport, " valid.proto ", 8, 19)
	acc.add(doc, StructuralDependencyImport, "valid.proto", -1, 2)
	acc.add(doc, StructuralDependencyImport, "valid.proto", 0, len(doc.Text)+1)
	acc.add(doc, StructuralDependencyImport, "  ", 0, 1)
	if len(acc.items) != 1 || acc.items[0].Value != "valid.proto" {
		t.Fatalf("unexpected malformed-range handling: %+v", acc.items)
	}
}

func BenchmarkProtoDependencyImports(b *testing.B) {
	for _, count := range []int{32, 128, 512} {
		b.Run(fmt.Sprintf("imports_%d", count), func(b *testing.B) {
			document := protoImportsFixture(count)
			options := testAnalyzeOptions(false, 8)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				result, err := (ProtoAnalyzer{}).Analyze(context.Background(), document, options)
				if err != nil {
					b.Fatal(err)
				}
				if len(result.Dependencies) != count {
					b.Fatalf("imports: got %d want %d", len(result.Dependencies), count)
				}
			}
		})
	}
}
