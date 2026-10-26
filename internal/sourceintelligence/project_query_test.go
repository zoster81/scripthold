package sourceintelligence

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestProjectQueryDependenciesDependentsTraceImpactAndCycles(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "graph")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	cPath := filepath.Join(root, "c.ts")
	a := projectResolverFacts(t, TypeScriptAnalyzer{}, aPath, "import { B } from \"./b\";\nexport class A extends B {}\n")
	b := projectResolverFacts(t, TypeScriptAnalyzer{}, bPath, "import { C } from \"./c\";\nexport class B extends C {}\n")
	c := projectResolverFacts(t, TypeScriptAnalyzer{}, cPath, "import { A } from \"./a\";\nexport class C {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{c, a, b}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	limits := queryLimits()

	dependencies, err := model.QueryRelations(context.Background(), RelationDependencies, queryPathSelector(a), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Records) != 1 || dependencies.Records[0].Source.Path != aPath || dependencies.Records[0].Target.Path != bPath || dependencies.Records[0].Evidence != SymbolEvidenceProjectResolved || dependencies.Records[0].Resolution != ResolutionResolved {
		t.Fatalf("dependencies = %+v", dependencies)
	}

	dependents, err := model.QueryRelations(context.Background(), RelationDependents, queryPathSelector(b), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependents.Records) != 1 || dependents.Records[0].Source.Path != aPath || dependents.Records[0].Target.Path != bPath {
		t.Fatalf("dependents = %+v", dependents)
	}

	trace, err := model.QueryRelations(context.Background(), RelationTrace, queryPathSelector(a), queryPathSelector(c), nil, limits)
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

	impact, err := model.QueryRelations(context.Background(), RelationImpact, queryPathSelector(c), ProjectSelector{}, nil, limits)
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

func TestProjectQueryReferencesDefinitionsInheritanceAndImplementations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "types")
	contractPath := filepath.Join(root, "Contract.java")
	basePath := filepath.Join(root, "Base.java")
	implPath := filepath.Join(root, "Impl.java")
	contract := projectResolverFacts(t, JavaAnalyzer{}, contractPath, "package demo; public interface Contract {}\n")
	base := projectResolverFacts(t, JavaAnalyzer{}, basePath, "package demo; public class Base {}\n")
	impl := projectResolverFacts(t, JavaAnalyzer{}, implPath, "package demo; public class Impl extends Base implements Contract {}\n")
	contractID := projectSymbolID(t, contract, "demo.Contract")
	baseID := projectSymbolID(t, base, "demo.Base")
	implID := projectSymbolID(t, impl, "demo.Impl")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{impl, base, contract}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	limits := queryLimits()

	inheritance, err := model.QueryRelations(context.Background(), RelationInheritance, querySymbolSelector(impl, implID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(inheritance.Records) != 1 || inheritance.Records[0].Source.SymbolID != implID || inheritance.Records[0].Target.SymbolID != baseID || inheritance.Records[0].Evidence != SymbolEvidenceProjectResolved {
		t.Fatalf("inheritance = %+v", inheritance)
	}

	implementations, err := model.QueryRelations(context.Background(), RelationImplementations, querySymbolSelector(contract, contractID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(implementations.Records) != 1 || implementations.Records[0].Source.SymbolID != implID || implementations.Records[0].Target.SymbolID != contractID {
		t.Fatalf("implementations = %+v", implementations)
	}

	references, err := model.QueryRelations(context.Background(), RelationReferences, querySymbolSelector(base, baseID), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if len(references.Records) != 1 || references.Records[0].Source.SymbolID != implID || references.Records[0].Target.SymbolID != baseID {
		t.Fatalf("references = %+v", references)
	}

	baseReference := projectReferenceByTarget(t, model.References(), implPath, "Base")
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

func TestStructuralSearchUsesNormalizedSymbolsAndRelations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "search")
	base := projectResolverFacts(t, RustAnalyzer{}, filepath.Join(root, "store.rs"), "pub trait Store<T> { fn get(&self) -> T; }\n")
	itemPath := filepath.Join(root, "item.rs")
	item := projectResolverFacts(t, RustAnalyzer{}, itemPath, "use crate::store::Store;\npub struct Item<T> { value: T }\nimpl<T> Store<T> for Item<T> { fn get(&self) -> T { todo!() } }\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{item, base}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}

	symbols, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Sto", Match: ProjectSearchPrefix, MaxResults: 16})
	if err != nil {
		t.Fatal(err)
	}
	foundSymbol := false
	for _, match := range symbols.Matches {
		if match.SymbolID == "" {
			if match.Name != "" || match.Kind != "" || match.QualifiedName != "" {
				t.Fatalf("non-symbol search match claims symbol identity: %+v", match)
			}
			continue
		}
		if match.Name == "Store" && match.Kind == SymbolKindTrait && match.QualifiedName != "" {
			foundSymbol = true
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
			if match.Name != "" || match.Kind != "" || match.QualifiedName != "" {
				t.Fatalf("resolved relation search claims declaration identity: %+v", match)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("relation search = %+v", relations)
	}
}

func TestProjectSearchTopKMatchesGlobalSortWithBoundedStorage(t *testing.T) {
	matches := make([]ProjectSearchMatch, 0, 4_096)
	for i := 0; i < 4_096; i++ {
		index := (i * 7919) % 4_096
		matches = append(matches, ProjectSearchMatch{
			Path: fmt.Sprintf("source-%03d.ts", index%113),
			Range: Range{
				Start: Position{Line: index/113 + 1, Column: 2},
				End:   Position{Line: index/113 + 1, Column: 6},
			},
			SymbolID: fmt.Sprintf("s-%04d", index),
			Evidence: SymbolEvidenceStructural,
		})
	}
	for _, maximum := range []int{1, 2, 7, 64, 4_096, 4_097} {
		t.Run(fmt.Sprint(maximum), func(t *testing.T) {
			collector := newProjectSearchTopK(maximum)
			for _, match := range matches {
				collector.append(match)
				if len(collector.matches) > maximum {
					t.Fatalf("collector retained %d matches with limit %d", len(collector.matches), maximum)
				}
			}
			got := collector.result()
			want := finalizeProjectSearch(append([]ProjectSearchMatch(nil), matches...), maximum)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("top-K diverged from global ordering at limit %d: got=%+v want=%+v", maximum, got, want)
			}
		})
	}
}

func TestProjectSearchContinuesWithoutSkippingEqualKeys(t *testing.T) {
	items := []ProjectSearchMatch{
		{Path: "z.ts", Name: "Z"},
		{Path: "a.ts", Name: "A"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "M"},
		{Path: "m.ts", Name: "Z"},
		{Path: "m.ts", Name: "A"},
		{Path: "zz.ts", Name: "Last"},
	}
	want := finalizeProjectSearch(append([]ProjectSearchMatch(nil), items...), len(items))
	for _, pageSize := range []int{1, 2, 3, 5} {
		var got []ProjectSearchMatch
		var after *ProjectSearchMatch
		afterOccurrences := 0
		for page := 0; page < 16; page++ {
			collector := newProjectSearchTopK(pageSize)
			collector.after = after
			collector.afterOccurrences = afterOccurrences
			for _, match := range items {
				collector.append(match)
				if len(collector.matches) > pageSize {
					t.Fatalf("page %d retained too many results", page)
				}
			}
			result := collector.result()
			got = append(got, result.Matches...)
			if !result.Truncated {
				break
			}
			if len(result.Matches) == 0 || result.LastOccurrences <= 0 {
				t.Fatalf("cursor did not advance: %+v", result)
			}
			last := result.Matches[len(result.Matches)-1]
			after = &last
			afterOccurrences = result.LastOccurrences
		}
		if !reflect.DeepEqual(got, want.Matches) {
			t.Fatalf("page size %d changed global order: got=%+v want=%+v", pageSize, got, want.Matches)
		}
	}
}

func TestProjectStructuralSearchRejectsInvalidContinuation(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	facts := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join("project", "pages", "only.ts"), "export class Item {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	missing := ProjectSearchMatch{Path: "missing.ts", Name: "Item"}
	for _, testCase := range []struct {
		name  string
		after *ProjectSearchMatch
		count int
		want  operation.Kind
	}{
		{name: "count without anchor", count: 1, want: operation.KindInvalidInput},
		{name: "anchor without count", after: &missing, want: operation.KindInvalidInput},
		{name: "missing anchor", after: &missing, count: 1, want: operation.KindConflict},
		{name: "impossible repeated match", after: &ProjectSearchMatch{Path: facts.Path, Name: "Item"}, count: 99, want: operation.KindConflict},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{
				Query: "Item", Match: ProjectSearchExact, MaxResults: 2,
				After: testCase.after, AfterOccurrences: testCase.count,
			})
			if operation.KindOf(err) != testCase.want {
				t.Fatalf("continuation error=%v kind=%v want=%v", err, operation.KindOf(err), testCase.want)
			}
		})
	}
}

func TestProjectStructuralSearchPageMatchesUnpagedResults(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "query", "pages")
	one := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "one.ts"), "export class Base {}\nexport class Other extends Base {}\n")
	two := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "two.ts"), "import { Base } from \"./one\";\nexport class Child extends Base {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{two, one}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	want, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{Query: "Base", Match: ProjectSearchExact, MaxResults: 32})
	if err != nil || len(want.Matches) < 3 {
		t.Fatalf("unpaged search: %+v err=%v", want, err)
	}
	var combined []ProjectSearchMatch
	var after *ProjectSearchMatch
	afterOccurrences := 0
	for page := 0; page < 20; page++ {
		result, err := model.StructuralSearch(context.Background(), ProjectSearchOptions{
			Query: "Base", Match: ProjectSearchExact, MaxResults: 2,
			After: after, AfterOccurrences: afterOccurrences,
		})
		if err != nil {
			t.Fatal(err)
		}
		combined = append(combined, result.Matches...)
		if !result.Truncated {
			break
		}
		if len(result.Matches) == 0 || result.LastOccurrences <= 0 {
			t.Fatalf("page did not advance: %+v", result)
		}
		last := result.Matches[len(result.Matches)-1]
		after = &last
		afterOccurrences = result.LastOccurrences
	}
	if !reflect.DeepEqual(combined, want.Matches) {
		t.Fatalf("paged search diverges: got=%+v want=%+v", combined, want.Matches)
	}
}

func TestStructuralSearchTruncatesAfterGlobalOrdering(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "search-order")
	earlyPath := filepath.Join(root, "a.ts")
	latePath := filepath.Join(root, "z.ts")
	latestPath := filepath.Join(root, "zz.ts")
	early := projectResolverFacts(t, TypeScriptAnalyzer{}, earlyPath, "export class Child extends Base {}\n")
	late := projectResolverFacts(t, TypeScriptAnalyzer{}, latePath, "export class Base {}\n")
	latest := projectResolverFacts(t, TypeScriptAnalyzer{}, latestPath, "export class Base {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{latest, late, early}, projectResolverLimitsForTest())
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

func TestProjectQueryDeterminismLimitsCancellationAndUnsupported(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase13", "hardening")
	first := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "a.ts"), "import { B } from \"./b\";\nexport class A extends B {}\n")
	second := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "b.ts"), "export class B {}\n")
	left, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{first, second}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{second, first}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	limits := queryLimits()
	leftResult, err := left.QueryRelations(context.Background(), RelationDependencies, queryPathSelector(first), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	rightResult, err := right.QueryRelations(context.Background(), RelationDependencies, queryPathSelector(first), ProjectSelector{}, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(leftResult, rightResult) {
		t.Fatalf("input order changed query output:\nleft=%+v\nright=%+v", leftResult, rightResult)
	}

	stale := queryPathSelector(first)
	stale.SourceFingerprint = queryZeroDigest()
	if _, err := left.QueryRelations(context.Background(), RelationDependencies, stale, ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("stale selector error = %v kind=%v", err, operation.KindOf(err))
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := left.QueryRelations(cancelled, RelationDependencies, queryPathSelector(first), ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancel error = %v kind=%v", err, operation.KindOf(err))
	}

	tooSmall := limits
	tooSmall.MaxNodes = 1
	if _, err := left.QueryRelations(context.Background(), RelationTrace, queryPathSelector(first), queryPathSelector(second), nil, tooSmall); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("node limit error = %v kind=%v", err, operation.KindOf(err))
	}

	if _, err := left.QueryRelations(context.Background(), RelationKind("callers"), querySymbolSelector(second, projectSymbolID(t, second, "B")), ProjectSelector{}, nil, limits); operation.KindOf(err) != operation.KindInvalidInput {
		t.Fatalf("callers error = %v kind=%v", err, operation.KindOf(err))
	}
}

func queryPathSelector(facts ProjectFileFacts) ProjectSelector {
	return ProjectSelector{Kind: ProjectSelectorPath, Path: facts.Path, SourceFingerprint: facts.SourceFingerprint}
}

func querySymbolSelector(facts ProjectFileFacts, symbolID string) ProjectSelector {
	return ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts.Path, SymbolID: symbolID, SourceFingerprint: facts.SourceFingerprint}
}

func queryZeroDigest() string {
	return "0000000000000000000000000000000000000000000000000000000000000000"
}

func queryLimits() ProjectQueryLimits {
	return ProjectQueryLimits{MaxResults: 256, MaxNodes: 256, MaxEdges: 1024, MaxDepth: 16}
}
