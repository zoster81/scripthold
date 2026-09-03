package sourceintelligence

import (
	"reflect"
	"testing"
)

func TestAppendUniqueDependenciesPreservesOrderAndFirstOccurrence(t *testing.T) {
	base := []StructuralDependency{
		{Kind: StructuralDependencyImport, Value: "shared", Alias: "base-import", Evidence: SymbolEvidenceStructural},
		{Kind: StructuralDependencyInclude, Value: "shared", Alias: "base-include", Evidence: SymbolEvidenceStructural},
		{Kind: StructuralDependencyImport, Value: "duplicate", Alias: "first", Evidence: SymbolEvidenceStructural},
	}
	extra := []StructuralDependency{
		{Kind: StructuralDependencyImport, Value: "duplicate", Alias: "second", Evidence: SymbolEvidenceProjectResolved},
		{Kind: StructuralDependencyReference, Value: "shared", Alias: "reference", Evidence: SymbolEvidenceStructural},
		{Kind: StructuralDependencyImport, Value: "tail", Alias: "tail", Evidence: SymbolEvidenceStructural},
	}
	want := []StructuralDependency{base[0], base[1], base[2], extra[1], extra[2]}

	if got := appendUniqueDependencies(base, extra); !reflect.DeepEqual(got, want) {
		t.Fatalf("merged dependencies = %+v, want %+v", got, want)
	}
}

func TestAppendUniqueDependenciesPreservesEmbeddedNULCollision(t *testing.T) {
	first := StructuralDependency{
		Kind:     StructuralDependencyKind("import\x00qualified"),
		Value:    "target",
		Alias:    "first",
		Evidence: SymbolEvidenceStructural,
	}
	colliding := StructuralDependency{
		Kind:     StructuralDependencyImport,
		Value:    "qualified\x00target",
		Alias:    "second",
		Evidence: SymbolEvidenceProjectResolved,
	}

	got := appendUniqueDependencies([]StructuralDependency{first}, []StructuralDependency{colliding})
	if len(got) != 1 || !reflect.DeepEqual(got[0], first) {
		t.Fatalf("embedded-NUL compatibility = %+v, want first dependency only", got)
	}
}

func TestAppendUniqueDependenciesAllocationBudget(t *testing.T) {
	const dependencyCount = 512
	base := make([]StructuralDependency, 0, dependencyCount)
	extra := make([]StructuralDependency, 0, dependencyCount)
	for index := 0; index < dependencyCount; index++ {
		base = append(base, StructuralDependency{
			Kind:     StructuralDependencyImport,
			Value:    dependencyMergeTestValue(index),
			Evidence: SymbolEvidenceStructural,
		})
		extra = append(extra, StructuralDependency{
			Kind:     StructuralDependencyImport,
			Value:    dependencyMergeTestValue(index + dependencyCount/2),
			Evidence: SymbolEvidenceStructural,
		})
	}
	const expected = dependencyCount + dependencyCount/2

	allocations := testing.AllocsPerRun(20, func() {
		merged := appendUniqueDependencies(base, extra)
		if len(merged) != expected {
			panic("unexpected dependency merge count")
		}
	})
	if allocations > 16 {
		t.Fatalf("appendUniqueDependencies allocations = %.0f, want <= 16", allocations)
	}
}

func dependencyMergeTestValue(index int) string {
	const digits = "0123456789"
	var suffix [4]byte
	for position := len(suffix) - 1; position >= 0; position-- {
		suffix[position] = digits[index%10]
		index /= 10
	}
	return "module/base/" + string(suffix[:])
}
