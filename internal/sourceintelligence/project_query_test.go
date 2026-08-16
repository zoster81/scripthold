package sourceintelligence

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase13ProjectQueryDependenciesDependentsTraceImpactAndCycles(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "graph")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	cPath := filepath.Join(root, "c.ts")
	a := phase12Facts(t, TypeScriptAnalyzer{}, aPath, "import { B } from \"./b\";\nexport class A extends B {}\n")
	b := phase12Facts(t, TypeScriptAnalyzer{}, bPath, "import { C } from \"./c\";\nexport class B extends C {}\n")
	c := phase12Facts(t, TypeScriptAnalyzer{}, cPath, "import { A } from \"./a\";\nexport class C {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{c, a, b}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	limits := phase13QueryLimits()

	dependencies, err := model.QueryRelations(context.Background(), RelationDependencies, phase13PathSelector(a), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Records) != 1 || dependencies.Records[0].Source.Path != aPath || dependencies.Records[0].Target.Path != bPath || dependencies.Records[0].Evidence != SymbolEvidenceProjectResolved || dependencies.Records[0].Resolution != ResolutionResolved {
		t.Fatalf("dependencies = %+v", dependencies)
	}

	dependents, err := model.QueryRelations(context.Background(), RelationDependents, phase13PathSelector(b), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependents.Records) != 1 || dependents.Records[0].Source.Path != aPath || dependents.Records[0].Target.Path != bPath {
		t.Fatalf("dependents = %+v", dependents)
	}

	trace, err := model.QueryRelations(context.Background(), RelationTrace, phase13PathSelector(a), phase13PathSelector(c), nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Records) != 2 || trace.Records[0].Source.Path != aPath || trace.Records[0].Target.Path != bPath || trace.Records[1].Source.Path != bPath || trace.Records[1].Target.Path != cPath {
		t.Fatalf("trace = %+v", trace)
	}
	for _, record := range trace.Records {
		if record.Kind != RelationTrace {
			t.Fatalf("trace kind = %q", record.Kind)
		}
	}

	impact, err := model.QueryRelations(context.Background(), RelationImpact, phase13PathSelector(c), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(impact.Records) != 2 || impact.Records[0].Source.Path != bPath || impact.Records[0].Target.Path != cPath || impact.Records[1].Source.Path != aPath || impact.Records[1].Target.Path != bPath {
		t.Fatalf("impact = %+v", impact)
	}

	cycles, err := model.QueryRelations(context.Background(), RelationCycles, ProjectSelector{}, ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(cycles.Records) != 3 {
		t.Fatalf("cycles = %+v", cycles)
	}
	for _, record := range cycles.Records {
		if record.Kind != RelationCycles || record.Resolution != ResolutionResolved {
			t.Fatalf("cycle record = %+v", record)
		}
	}
}

func TestR27Phase13ProjectQueryReferencesDefinitionsInheritanceAndImplementations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "types")
	contractPath := filepath.Join(root, "Contract.java")
	basePath := filepath.Join(root, "Base.java")
	implPath := filepath.Join(root, "Impl.java")
	contract := phase12Facts(t, JavaAnalyzer{}, contractPath, "package demo; public interface Contract {}\n")
	base := phase12Facts(t, JavaAnalyzer{}, basePath, "package demo; public class Base {}\n")
	impl := phase12Facts(t, JavaAnalyzer{}, implPath, "package demo; public class Impl extends Base implements Contract {}\n")
	contractID := phase12SymbolID(t, contract, "demo.Contract")
	baseID := phase12SymbolID(t, base, "demo.Base")
	implID := phase12SymbolID(t, impl, "demo.Impl")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{impl, base, contract}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	limits := phase13QueryLimits()

	inheritance, err := model.QueryRelations(context.Background(), RelationInheritance, phase13SymbolSelector(impl, implID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(inheritance.Records) != 1 || inheritance.Records[0].Source.SymbolID != implID || inheritance.Records[0].Target.SymbolID != baseID || inheritance.Records[0].Evidence != SymbolEvidenceProjectResolved {
		t.Fatalf("inheritance = %+v", inheritance)
	}

	implementations, err := model.QueryRelations(context.Background(), RelationImplementations, phase13SymbolSelector(contract, contractID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(implementations.Records) != 1 || implementations.Records[0].Source.SymbolID != implID || implementations.Records[0].Target.SymbolID != contractID {
		t.Fatalf("implementations = %+v", implementations)
	}

	references, err := model.QueryRelations(context.Background(), RelationReferences, phase13SymbolSelector(base, baseID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(references.Records) != 1 || references.Records[0].Source.SymbolID != implID || references.Records[0].Target.SymbolID != baseID {
		t.Fatalf("references = %+v", references)
	}

	baseReference := phase12ReferenceByTarget(t, model.References(), implPath, "Base")
	definitions, err := model.QueryRelations(context.Background(), RelationDefinitions, ProjectSelector{
		Kind: ProjectSelectorPosition, Path: implPath, Position: &baseReference.Range.Start, SourceFingerprint: impl.SourceFingerprint,
	}, ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions.Records) != 1 || definitions.Records[0].Target.SymbolID != baseID || definitions.Records[0].Resolution != ResolutionResolved {
		t.Fatalf("definitions = %+v", definitions)
	}
}

func TestR27Phase13StructuralSearchUsesNormalizedSymbolsAndRelations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "search")
	base := phase12Facts(t, RustAnalyzer{}, filepath.Join(root, "store.rs"), "pub trait Store<T> { fn get(&self) -> T; }\n")
	itemPath := filepath.Join(root, "item.rs")
	item := phase12Facts(t, RustAnalyzer{}, itemPath, "use crate::store::Store;\npub struct Item<T> { value: T }\nimpl<T> Store<T> for Item<T> { fn get(&self) -> T { todo!() } }\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{item, base}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}

	symbols, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Sto", Match: ProjectSearchPrefix, MaxResults: 16})
	if err != nil {
		t.Fatal(err)
	}
	foundSymbol := false
	for _, match := range symbols.Matches {
		if match.SymbolID != "" {
			foundSymbol = true
			break
		}
	}
	if !foundSymbol {
		t.Fatalf("symbol search = %+v", symbols)
	}

	relations, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Store<T>", Match: ProjectSearchExact, Evidence: []SymbolEvidence{SymbolEvidenceProjectResolved}, MaxResults: 16})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, match := range relations.Matches {
		if match.Path == itemPath && match.SymbolID == "" && match.Evidence == SymbolEvidenceProjectResolved {
			found = true
		}
	}
	if !found {
		t.Fatalf("relation search = %+v", relations)
	}
}

func TestR27Phase13StructuralSearchTruncatesAfterGlobalOrdering(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "search-order")
	earlyPath := filepath.Join(root, "a.ts")
	latePath := filepath.Join(root, "z.ts")
	latestPath := filepath.Join(root, "zz.ts")
	early := phase12Facts(t, TypeScriptAnalyzer{}, earlyPath, "export class Child extends Base {}\n")
	late := phase12Facts(t, TypeScriptAnalyzer{}, latePath, "export class Base {}\n")
	latest := phase12Facts(t, TypeScriptAnalyzer{}, latestPath, "export class Base {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{latest, late, early}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}

	result, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Base", Match: ProjectSearchExact, MaxResults: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Matches) != 1 || result.Matches[0].Path != earlyPath || result.Matches[0].SymbolID != "" {
		t.Fatalf("globally truncated search = %+v", result)
	}
}

func TestR27Phase13ProjectQueryDeterminismLimitsCancellationAndUnsupported(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "hardening")
	first := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join(root, "a.ts"), "import { B } from \"./b\";\nexport class A extends B {}\n")
	second := phase12Facts(t, TypeScriptAnalyzer{}, filepath.Join(root, "b.ts"), "export class B {}\n")
	left, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{first, second}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{second, first}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	limits := phase13QueryLimits()
	leftResult, err := left.QueryRelations(context.Background(), RelationDependencies, phase13PathSelector(first), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	rightResult, err := right.QueryRelations(context.Background(), RelationDependencies, phase13PathSelector(first), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(leftResult, rightResult) {
		t.Fatalf("input order changed query output:\nleft=%+v\nright=%+v", leftResult, rightResult)
	}

	stale := phase13PathSelector(first)
	stale.SourceFingerprint = phase13ZeroDigest()
	if _, err := left.QueryRelations(context.Background(), RelationDependencies, stale, ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("stale selector error = %v kind=%v", err, operation.KindOf(err))
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := left.QueryRelations(cancelled, RelationDependencies, phase13PathSelector(first), ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancel error = %v kind=%v", err, operation.KindOf(err))
	}

	tooSmall := limits
	tooSmall.MaxNodes = 1
	if _, err := left.QueryRelations(context.Background(), RelationTrace, phase13PathSelector(first), phase13PathSelector(second), nil, tooSmall); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("node limit error = %v kind=%v", err, operation.KindOf(err))
	}

	if _, err := left.QueryRelations(context.Background(), RelationCallers, phase13SymbolSelector(second, phase12SymbolID(t, second, "B")), ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindUnsupported {
		t.Fatalf("callers error = %v kind=%v", err, operation.KindOf(err))
	}
}

func phase13PathSelector(facts ProjectFileFacts) ProjectSelector {
	return ProjectSelector{Kind: ProjectSelectorPath, Path: facts.Path, SourceFingerprint: facts.SourceFingerprint}
}

func phase13SymbolSelector(facts ProjectFileFacts, symbolID string) ProjectSelector {
	return ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts.Path, SymbolID: symbolID, SourceFingerprint: facts.SourceFingerprint}
}

func phase13ZeroDigest() string {
	return "0000000000000000000000000000000000000000000000000000000000000000"
}

func phase13QueryLimits() ProjectQueryLimits {
	return ProjectQueryLimits{MaxResults: 256, MaxNodes: 256, MaxEdges: 1024, MaxDepth: 16}
}
