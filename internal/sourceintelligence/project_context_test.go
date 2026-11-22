package sourceintelligence

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

// TestProjectContextCyclicParentsTerminate guards a malformed analyzer result:
// project-model construction accepts parent IDs but planning must not follow
// repeated parent IDs indefinitely.
func TestProjectContextCyclicParentsTerminate(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		pair bool
	}{
		{name: "self-parent"},
		{name: "two-symbol-cycle", pair: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			facts := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join("project", "context", "cycle.ts"), "export class First {}\nexport class Second {}\n")
			firstID := projectSymbolID(t, facts, "First")
			secondID := projectSymbolID(t, facts, "Second")
			for i := range facts.Analysis.Analysis.Symbols {
				switch facts.Analysis.Analysis.Symbols[i].ID {
				case firstID:
					if tc.pair {
						facts.Analysis.Analysis.Symbols[i].ParentID = secondID
					} else {
						facts.Analysis.Analysis.Symbols[i].ParentID = firstID
					}
				case secondID:
					if tc.pair {
						facts.Analysis.Analysis.Symbols[i].ParentID = firstID
					}
				}
			}
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, projectResolverLimitsForTest())
			if err != nil {
				t.Fatal(err)
			}
			plan, err := model.PlanContext(context.Background(), []ProjectSelector{querySymbolSelector(facts, firstID)}, ProjectContextOptions{
				BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly,
			})
			if err != nil {
				t.Fatal(err)
			}
			if !containsContextSymbol(plan.Candidates, firstID) {
				t.Fatalf("explicit selector disappeared from cyclic-parent plan: %+v", plan.Candidates)
			}
		})
	}
}

func TestProjectContextSymbolTargetExcludesUnrelatedDependentDeclarations(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "context", "reverse")
	basePath := filepath.Join(root, "base.ts")
	clientPath := filepath.Join(root, "client.ts")
	base := projectResolverFacts(t, TypeScriptAnalyzer{}, basePath, "export class Base {}\n")
	client := projectResolverFacts(t, TypeScriptAnalyzer{}, clientPath, "import { Base } from \"./base\";\nexport class Derived extends Base {}\nexport class Unrelated {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{client, base}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	baseID := projectSymbolID(t, base, "Base")
	derivedID := projectSymbolID(t, client, "Derived")
	unrelatedID := projectSymbolID(t, client, "Unrelated")
	options := ProjectContextOptions{BudgetBytes: 8192, MaxItems: 20, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
	symbolPlan, err := model.PlanContext(context.Background(), []ProjectSelector{querySymbolSelector(base, baseID)}, options)
	if err != nil {
		t.Fatal(err)
	}
	if !containsContextSymbol(symbolPlan.Candidates, derivedID) {
		t.Fatalf("symbol context omitted proven subtype: %+v", symbolPlan.Candidates)
	}
	if containsContextSymbol(symbolPlan.Candidates, unrelatedID) {
		t.Fatalf("symbol context included unrelated dependent declaration: %+v", symbolPlan.Candidates)
	}
	baseRef := projectReferenceByTarget(t, model.References(), clientPath, "Base")
	positionPlan, err := model.PlanContext(context.Background(), []ProjectSelector{{
		Kind: ProjectSelectorPosition, Path: clientPath, Position: &baseRef.Range.Start, SourceFingerprint: client.SourceFingerprint,
	}}, options)
	if err != nil {
		t.Fatal(err)
	}
	if !containsContextSymbol(positionPlan.Candidates, baseID) || containsContextSymbol(positionPlan.Candidates, unrelatedID) {
		t.Fatalf("position context should target the resolved symbol, not unrelated dependents: %+v", positionPlan.Candidates)
	}
	pathPlan, err := model.PlanContext(context.Background(), []ProjectSelector{queryPathSelector(base)}, options)
	if err != nil {
		t.Fatal(err)
	}
	if !containsContextSymbol(pathPlan.Candidates, unrelatedID) {
		t.Fatalf("explicit file context lost broad dependent declarations: %+v", pathPlan.Candidates)
	}
	mixedPlan, err := model.PlanContext(context.Background(), []ProjectSelector{querySymbolSelector(base, baseID), queryPathSelector(base)}, options)
	if err != nil {
		t.Fatal(err)
	}
	if !containsContextSymbol(mixedPlan.Candidates, unrelatedID) {
		t.Fatalf("mixed context lost explicit path scope: %+v", mixedPlan.Candidates)
	}
}

func TestProjectContextOutgoingIndexPreservesLegacyPlansAcrossFiles(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name         string
		analyzer     SourceAnalyzer
		extension    string
		isTypeScript bool
	}{
		{"java", JavaAnalyzer{}, "java", false},
		{"typescript", TypeScriptAnalyzer{}, "ts", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join("project", "outgoing", tc.name)
			baseText := "package demo; public class Base {}\n"
			if tc.isTypeScript {
				baseText = "export class Base {}\n"
			}
			base := projectResolverFacts(t, tc.analyzer, filepath.Join(root, "Base."+tc.extension), baseText)
			facts := []ProjectFileFacts{base}
			for index := 0; index < 70; index++ {
				source := fmt.Sprintf("package demo; public class Child%03d extends Base {}\n", index)
				if tc.isTypeScript {
					source = fmt.Sprintf("import { Base } from './Base';\nexport class Child%03d extends Base {}\n", index)
				}
				fact := projectResolverFacts(t, tc.analyzer, filepath.Join(root, fmt.Sprintf("Child%03d.%s", index, tc.extension)), source)
				if index == 1 || index == 2 {
					for pos := range fact.Analysis.Analysis.Symbols {
						if fact.Analysis.Analysis.Symbols[pos].Kind == SymbolKindClass {
							fact.Analysis.Analysis.Symbols[pos].ID = "cross-file-duplicate-id"
						}
					}
				}
				facts = append(facts, fact)
			}
			model, err := BuildProjectModel(context.Background(), registry, facts, projectResolverLimitsForTest())
			if err != nil {
				t.Fatal(err)
			}
			if len(model.references) < 70 || len(model.referencesBySource) < 70 {
				t.Fatalf("expected type references indexed by source: refs=%d paths=%d", len(model.references), len(model.referencesBySource))
			}
			// Symbol and path selectors do not require the source-position index.
			// Disabling it on a shallow model copy restores the original global
			// outgoing scan without changing the underlying immutable facts.
			linear := *model
			linear.referencesBySource = nil
			name := func(value string) string {
				if tc.isTypeScript {
					return value
				}
				return "demo." + value
			}
			symbol := func(fact ProjectFileFacts, qualified string) ProjectSelector {
				return ProjectSelector{Kind: ProjectSelectorSymbol, Path: fact.Path, SourceFingerprint: fact.SourceFingerprint, SymbolID: projectSymbolID(t, fact, name(qualified))}
			}
			baseSelector := symbol(base, "Base")
			lateSelector := symbol(facts[len(facts)-1], "Child069")
			duplicateA := ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts[2].Path, SourceFingerprint: facts[2].SourceFingerprint, SymbolID: "cross-file-duplicate-id"}
			duplicateB := ProjectSelector{Kind: ProjectSelectorSymbol, Path: facts[3].Path, SourceFingerprint: facts[3].SourceFingerprint, SymbolID: "cross-file-duplicate-id"}
			pathSelector := ProjectSelector{Kind: ProjectSelectorPath, Path: facts[len(facts)-1].Path, SourceFingerprint: facts[len(facts)-1].SourceFingerprint}
			cases := [][]ProjectSelector{
				{baseSelector}, {lateSelector}, {duplicateA}, {duplicateB},
				{pathSelector}, {lateSelector, duplicateA, duplicateB},
			}
			for _, policy := range []ProjectContextBodyPolicy{ProjectContextPrefer, ProjectContextSignaturesOnly} {
				options := ProjectContextOptions{BudgetBytes: 1 << 20, MaxItems: 200, MaxDepth: 2, BodyPolicy: policy}
				for index, targets := range cases {
					got, gotErr := model.PlanContext(context.Background(), targets, options)
					want, wantErr := linear.PlanContext(context.Background(), targets, options)
					if operation.KindOf(gotErr) != operation.KindOf(wantErr) || !reflect.DeepEqual(got, want) {
						t.Fatalf("%s policy=%s case=%d indexedError=%v legacyError=%v indexed=%+v legacy=%+v", tc.name, policy, index, gotErr, wantErr, got, want)
					}
				}
			}
		})
	}
}

func containsContextSymbol(values []ProjectContextCandidate, symbolID string) bool {
	for _, value := range values {
		if value.Entity.SymbolID == symbolID {
			return true
		}
	}
	return false
}

func TestProjectContextPrioritiesAndBudgetDegradation(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase14", "java")
	basePath := filepath.Join(root, "Base.java")
	servicePath := filepath.Join(root, "Service.java")
	base := projectResolverFacts(t, JavaAnalyzer{}, basePath, "package demo; public class Base { public void base() {} }\n")
	service := projectResolverFacts(t, JavaAnalyzer{}, servicePath, "package demo; public class Service extends Base { public int run(int value) { return value + 1; } }\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{service, base}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	runID := projectSymbolID(t, service, "demo.Service.run")
	serviceID := projectSymbolID(t, service, "demo.Service")
	selector := querySymbolSelector(service, runID)

	plan, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextPrefer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) < 2 {
		t.Fatalf("context plan = %+v", plan)
	}
	if got := plan.Candidates[0]; got.Entity.SymbolID != runID || got.Reason != ContextTarget || got.Priority != 1 || got.Representation != ContextBody {
		t.Fatalf("target candidate = %+v", got)
	}
	if got := plan.Candidates[1]; got.Entity.SymbolID != serviceID || got.Reason != ContextEnclosing || got.Priority != 2 {
		t.Fatalf("enclosing candidate = %+v", got)
	}

	run := contextSymbolByID(t, service, runID)
	_, _, signature, _ := run.SourceOffsets()
	if signature == nil {
		t.Fatal("run signature offsets are missing")
	}
	signatureBytes := signature.End - signature.Start
	degraded, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, ProjectContextOptions{
		BudgetBytes: signatureBytes, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextPrefer,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(degraded.Candidates) != 1 || degraded.Candidates[0].Representation != ContextSignature || degraded.UsedBytes != signatureBytes || !degraded.Truncated {
		t.Fatalf("degraded context = %+v", degraded)
	}
	if _, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, ProjectContextOptions{
		BudgetBytes: signatureBytes - 1, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextPrefer,
	}); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("undersized target budget error = %v kind=%v", err, operation.KindOf(err))
	}

	signaturesOnly, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 1, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(signaturesOnly.Candidates) != 1 || signaturesOnly.Candidates[0].Representation != ContextSignature || !signaturesOnly.Truncated {
		t.Fatalf("signatures-only context = %+v", signaturesOnly)
	}
}

func TestProjectContextPositionTargetsDefinitionAndDependencyDepth(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase14", "graph")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	cPath := filepath.Join(root, "c.ts")
	a := projectResolverFacts(t, TypeScriptAnalyzer{}, aPath, "import { B } from \"./b\";\nexport class A extends B {}\n")
	b := projectResolverFacts(t, TypeScriptAnalyzer{}, bPath, "import { C } from \"./c\";\nexport class B extends C {}\n")
	c := projectResolverFacts(t, TypeScriptAnalyzer{}, cPath, "export class C {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{c, a, b}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	baseRef := projectReferenceByTarget(t, model.References(), aPath, "B")
	bID := projectSymbolID(t, b, "B")
	positionPlan, err := model.PlanContext(context.Background(), []ProjectSelector{{
		Kind: ProjectSelectorPosition, Path: aPath, Position: &baseRef.Range.Start, SourceFingerprint: a.SourceFingerprint,
	}}, ProjectContextOptions{BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly})
	if err != nil {
		t.Fatal(err)
	}
	if len(positionPlan.Candidates) == 0 || positionPlan.Candidates[0].Entity.SymbolID != bID || positionPlan.Candidates[0].Reason != ContextTarget {
		t.Fatalf("position context = %+v", positionPlan)
	}

	pathPlan, err := model.PlanContext(context.Background(), []ProjectSelector{queryPathSelector(a)}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasContextCandidate(pathPlan.Candidates, aPath, ContextTarget, 1) ||
		!hasContextCandidate(pathPlan.Candidates, bPath, ContextDirectDependency, 3) ||
		!hasContextCandidate(pathPlan.Candidates, cPath, ContextDeeperRelation, 7) {
		t.Fatalf("dependency-depth context = %+v", pathPlan)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := model.PlanContext(cancelled, []ProjectSelector{queryPathSelector(a)}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly,
	}); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("context cancellation error = %v kind=%v", err, operation.KindOf(err))
	}
}

func contextSymbolByID(t *testing.T, facts ProjectFileFacts, id string) NormalizedSymbol {
	t.Helper()
	for _, symbol := range facts.Analysis.Analysis.Symbols {
		if symbol.ID == id {
			return symbol
		}
	}
	t.Fatalf("symbol %s not found in %s", id, facts.Path)
	return NormalizedSymbol{}
}

func hasContextCandidate(values []ProjectContextCandidate, path string, reason ContextReason, priority int) bool {
	for _, value := range values {
		if value.Entity.Path == path && value.Reason == reason && value.Priority == priority {
			return true
		}
	}
	return false
}

// Repeated imports of one file must keep the first dependency's evidence
// and deterministic root order, without duplicating context candidates.
func TestProjectContextDirectDependenciesRepeatedTargetsPreserveFirstEvidence(t *testing.T) {
	const sourcePath, targetPath = "project/client.ts", "project/shared.ts"
	model := &ProjectModel{
		symbolsByFile: map[string][]projectSymbolRecord{
			projectPathKey(targetPath): {{
				pathKey: projectPathKey(targetPath),
				symbol: NormalizedSymbol{
					ID: "class-id", Kind: SymbolKindClass,
					declarationOffsets: OffsetRange{Start: 0, End: 10},
				},
				entity: RelationEntity{Path: targetPath, SymbolID: "class-id"},
			}},
		},
		dependenciesBySource: map[string][]ProjectDependency{
			projectPathKey(sourcePath): {
				{Targets: []RelationEntity{{Path: targetPath}, {Path: targetPath}}, Evidence: SymbolEvidenceStructural, Resolution: ResolutionResolved},
				{Targets: []RelationEntity{{Path: targetPath}}, Evidence: SymbolEvidenceScopeResolved, Resolution: ResolutionAmbiguous},
			},
		},
	}
	planner := projectContextPlanner{
		model:   model,
		options: ProjectContextOptions{BudgetBytes: 1024, MaxItems: 4, BodyPolicy: ProjectContextSignaturesOnly},
		seen:    make(map[projectContextKey]struct{}),
	}
	seeds := []projectContextSeed{{record: projectSymbolRecord{pathKey: projectPathKey(sourcePath)}}}
	if err := planner.addDirectDependencies(context.Background(), seeds); err != nil {
		t.Fatal(err)
	}
	if len(planner.plan.Candidates) != 1 {
		t.Fatalf("duplicate dependency targets yielded %d candidates, want 1", len(planner.plan.Candidates))
	}
	candidate := planner.plan.Candidates[0]
	if candidate.Entity.Path != targetPath || candidate.Entity.SymbolID != "class-id" ||
		candidate.Reason != ContextDirectDependency ||
		candidate.Evidence != SymbolEvidenceStructural || candidate.Resolution != ResolutionResolved {
		t.Fatalf("first dependency evidence or root ordering changed: %+v", candidate)
	}
}

// A planner may borrow already-sorted, immutable model records when every
// symbol is a root. Nested symbols still require a separately filtered slice.
func TestProjectContextPlannerBorrowsFlatImmutableRootSymbols(t *testing.T) {
	flat := make([]projectSymbolRecord, 2, 4)
	flat[0] = projectSymbolRecord{symbol: NormalizedSymbol{ID: "a", Kind: SymbolKindClass}}
	flat[1] = projectSymbolRecord{symbol: NormalizedSymbol{ID: "b", Kind: SymbolKindClass}}
	nested := []projectSymbolRecord{
		{symbol: NormalizedSymbol{ID: "root", Kind: SymbolKindModule}},
		{symbol: NormalizedSymbol{ID: "child", ParentID: "root", Kind: SymbolKindClass}},
	}
	model := &ProjectModel{symbolsByFile: map[string][]projectSymbolRecord{
		"flat": flat, "nested": nested,
	}}
	planner := projectContextPlanner{model: model}
	borrowed := planner.contextRootSymbols("flat")
	if len(borrowed) != len(flat) || &borrowed[0] != &flat[0] {
		t.Fatal("flat root records were copied instead of borrowing the immutable model slice")
	}
	if cap(borrowed) != len(borrowed) {
		t.Fatalf("borrowed root capacity = %d, want %d to prevent append into model storage", cap(borrowed), len(borrowed))
	}
	_ = append(borrowed, projectSymbolRecord{symbol: NormalizedSymbol{ID: "extra"}})
	if flat[:cap(flat)][len(flat)].symbol.ID != "" {
		t.Fatal("appending to borrowed roots modified the model backing storage")
	}
	if repeated := planner.contextRootSymbols("flat"); &repeated[0] != &flat[0] {
		t.Fatal("cached flat roots did not preserve the model-owned backing array")
	}
	filtered := planner.contextRootSymbols("nested")
	if len(filtered) != 2 || &filtered[0] == &nested[0] {
		t.Fatal("nested roots unexpectedly borrowed unfiltered model storage")
	}
}

// Sorted, parentless model records are immutable after construction. A path
// context may read them without an additional map or mutable copy.
func TestProjectContextModelBorrowsSortedFlatRoots(t *testing.T) {
	records := make([]projectSymbolRecord, 3, 6)
	for index, name := range []string{"A", "B", "C"} {
		records[index] = projectSymbolRecord{
			symbol: NormalizedSymbol{ID: name, Name: name, Kind: SymbolKindClass},
			entity: RelationEntity{Path: "flat.ts", QualifiedName: name, SymbolID: name},
		}
	}
	model := &ProjectModel{symbolsByFile: map[string][]projectSymbolRecord{"flat": records}}
	roots := model.contextRootSymbols("flat")
	if len(roots) != len(records) || &roots[0] != &records[0] {
		t.Fatal("sorted flat roots copied instead of borrowing immutable model records")
	}
	if cap(roots) != len(roots) {
		t.Fatalf("borrowed roots capacity=%d, want %d", cap(roots), len(roots))
	}
	_ = append(roots, projectSymbolRecord{symbol: NormalizedSymbol{ID: "external"}})
	if records[:cap(records)][len(records)].symbol.ID != "" {
		t.Fatal("append to borrowed roots modified the model storage")
	}

	// Synthetic unsorted records must retain the historical sorted result.
	unsorted := []projectSymbolRecord{records[1], records[0], records[2]}
	model.symbolsByFile["flat"] = unsorted
	got := model.contextRootSymbols("flat")
	if &got[0] == &unsorted[0] || got[0].symbol.ID != "A" || got[1].symbol.ID != "B" {
		t.Fatalf("unsorted roots did not retain sorted, independent output: %+v", got)
	}

	// Equal canonical keys may contain distinct declaration details. Keep
	// the original copied/sorted path for ties rather than changing tie order.
	tied := []projectSymbolRecord{records[0], records[0]}
	tied[1].symbol.Name = "OtherDeclaration"
	model.symbolsByFile["flat"] = tied
	got = model.contextRootSymbols("flat")
	if len(got) != len(tied) || &got[0] == &tied[0] {
		t.Fatal("equal sort keys must retain the independent fallback")
	}
	if got[0].symbol.Name != tied[0].symbol.Name || got[1].symbol.Name != tied[1].symbol.Name {
		t.Fatalf("tied declaration order changed: %+v", got)
	}
}

// A reference-free plan must not allocate a per-seed relation graph merely
// to determine that there are no outgoing or reverse type relations.
func TestProjectContextNoReferencesAvoidsGraphScratch(t *testing.T) {
	seeds := make([]projectContextSeed, 128)
	for index := range seeds {
		id := fmt.Sprintf("sym-%03d", index)
		seeds[index] = projectContextSeed{record: projectSymbolRecord{
			symbol:  NormalizedSymbol{ID: id},
			entity:  RelationEntity{Path: "flat.ts", SymbolID: id},
			pathKey: "flat",
		}}
	}
	planner := projectContextPlanner{model: &ProjectModel{}}
	ctx := context.Background()
	allocations := testing.AllocsPerRun(30, func() {
		if err := planner.addReverseAndTypeRelations(ctx, seeds, nil); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("reference-free reverse planning used %.0f unnecessary heap allocations", allocations)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := planner.addReverseAndTypeRelations(cancelled, seeds, nil); operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("cancelled empty-reference plan returned %v", err)
	}
}

// Reverse import dependencies remain visible in path context even without
// any structural reference edge in the project model.
func TestProjectContextNoReferencesRetainsReverseDependents(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "context", "no-references")
	base := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "base.ts"), "export class Base {}\n")
	client := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "client.ts"),
		"import { Base } from './base';\nexport class Client {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{base, client}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	if len(model.references) != 0 {
		t.Fatalf("fixture must lack structural reference edges; got %d", len(model.references))
	}
	if len(model.dependentsByTarget[projectPathKey(base.Path)]) == 0 {
		t.Fatal("fixture has no reverse dependency")
	}
	plan, err := model.PlanContext(context.Background(), []ProjectSelector{queryPathSelector(base)},
		ProjectContextOptions{BudgetBytes: 8192, MaxItems: 16, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly})
	if err != nil {
		t.Fatal(err)
	}
	if !containsContextSymbol(plan.Candidates, projectSymbolID(t, client, "Client")) {
		t.Fatalf("reference-free reverse dependency lost client root: %+v", plan.Candidates)
	}
}

// Appending one tiny file after a large file must not reserve another
// large record buffer solely to preserve amortized growth.
func TestProjectContextAppendPathSeedsBoundsSkewedGrowth(t *testing.T) {
	const largeCount = 2048
	model := &ProjectModel{
		files: map[string]projectFileRecord{
			"large.ts": {facts: ProjectFileFacts{Path: "large.ts", SourceFingerprint: "fingerprint"}},
			"tiny.ts":  {facts: ProjectFileFacts{Path: "tiny.ts", SourceFingerprint: "fingerprint"}},
		},
		symbolsByFile: make(map[string][]projectSymbolRecord),
	}
	records := make([]projectSymbolRecord, largeCount)
	for i := range records {
		id := fmt.Sprintf("sym-%04d", i)
		records[i] = projectSymbolRecord{
			pathKey: "large.ts",
			symbol:  NormalizedSymbol{ID: id},
			entity:  RelationEntity{Path: "large.ts", QualifiedName: id, SymbolID: id},
		}
	}
	model.symbolsByFile["large.ts"] = records
	model.symbolsByFile["tiny.ts"] = []projectSymbolRecord{{
		pathKey: "tiny.ts",
		symbol:  NormalizedSymbol{ID: "tiny"},
		entity:  RelationEntity{Path: "tiny.ts", QualifiedName: "tiny", SymbolID: "tiny"},
	}}
	large := ProjectSelector{Kind: ProjectSelectorPath, Path: "large.ts", SourceFingerprint: "fingerprint"}
	tiny := ProjectSelector{Kind: ProjectSelectorPath, Path: "tiny.ts", SourceFingerprint: "fingerprint"}
	seeds, err := model.appendContextSeedsForSelector(nil, large)
	if err != nil || len(seeds) != largeCount {
		t.Fatalf("large seeds: %d err=%v", len(seeds), err)
	}
	seeds, err = model.appendContextSeedsForSelector(seeds, tiny)
	if err != nil || len(seeds) != largeCount+1 || seeds[largeCount].record.symbol.ID != "tiny" {
		t.Fatalf("skewed seeds: %d err=%v", len(seeds), err)
	}
	if cap(seeds) > len(seeds)+4 {
		t.Fatalf("skewed request reserved an unnecessary large buffer: len=%d cap=%d", len(seeds), cap(seeds))
	}
}

// A multi-file request should grow its temporary seed buffer amortized,
// without changing the order, identity, or source-fingerprint checks.
func TestProjectContextAppendPathSeedsAmortizesGrowth(t *testing.T) {
	const fileCount, perFile = 4, 4
	model := &ProjectModel{
		files:         make(map[string]projectFileRecord, fileCount),
		symbolsByFile: make(map[string][]projectSymbolRecord, fileCount),
	}
	selectors := make([]ProjectSelector, 0, fileCount)
	for fileIndex := 0; fileIndex < fileCount; fileIndex++ {
		path := fmt.Sprintf("src/file-%d.ts", fileIndex)
		key := projectPathKey(path)
		model.files[key] = projectFileRecord{facts: ProjectFileFacts{Path: path, SourceFingerprint: "fingerprint"}}
		records := make([]projectSymbolRecord, perFile)
		for symbolIndex := range records {
			id := fmt.Sprintf("s%02d", symbolIndex)
			records[symbolIndex] = projectSymbolRecord{
				pathKey: key, symbol: NormalizedSymbol{ID: id, Name: id},
				entity: RelationEntity{Path: path, QualifiedName: id, SymbolID: id},
			}
		}
		model.symbolsByFile[key] = records
		selectors = append(selectors, ProjectSelector{Kind: ProjectSelectorPath, Path: path, SourceFingerprint: "fingerprint"})
	}
	var seeds []projectContextSeed
	for index, selector := range selectors {
		var err error
		seeds, err = model.appendContextSeedsForSelector(seeds, selector)
		if err != nil {
			t.Fatal(err)
		}
		if len(seeds) != (index+1)*perFile {
			t.Fatalf("after %d paths: %d seeds", index+1, len(seeds))
		}
		for i, seed := range seeds {
			if seed.record.entity.Path != selectors[i/perFile].Path ||
				seed.record.symbol.ID != fmt.Sprintf("s%02d", i%perFile) {
				t.Fatalf("out-of-order seed %d: %+v", i, seed.record)
			}
		}
		if cap(seeds) > 2*len(seeds) {
			t.Fatalf("seed buffer over-allocated: len=%d cap=%d", len(seeds), cap(seeds))
		}
		if index == 2 && cap(seeds) < 4*perFile {
			t.Fatalf("seed buffer did not amortize growth: len=%d cap=%d", len(seeds), cap(seeds))
		}
	}
}

// Mixed selectors append to the request-owned buffer without replacing prior
// seeds or weakening path fingerprint validation.
func TestProjectContextAppendSelectorSeedsReusesRequestBuffer(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	facts := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join("project", "context", "append-seeds.ts"),
		"export class Alpha {}\nexport class Beta {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	selector := queryPathSelector(facts)
	original, err := model.contextSeedsForSelector(selector)
	if err != nil || len(original) < 2 {
		t.Fatalf("path seed fixture invalid: %d seeds, error=%v", len(original), err)
	}
	buffer := make([]projectContextSeed, 1, len(original)+2)
	buffer[0].record.symbol.ID = "preserved"
	appended, err := model.appendContextSeedsForSelector(buffer, selector)
	if err != nil || len(appended) != len(original)+1 {
		t.Fatalf("appended path seeds changed: count=%d err=%v", len(appended), err)
	}
	if &appended[0] != &buffer[0] || appended[0].record.symbol.ID != "preserved" {
		t.Fatal("path append did not retain request-owned seed buffer and prefix")
	}
	if !reflect.DeepEqual(appended[1:], original) {
		t.Fatal("path append altered selector seed order or values")
	}
	stale := selector
	stale.SourceFingerprint = selector.SourceFingerprint + "-stale"
	if _, err := model.appendContextSeedsForSelector(appended, stale); operation.KindOf(err) != operation.KindConflict {
		t.Fatalf("stale path fingerprint must fail, got %v", err)
	}
	if len(appended) != len(original)+1 {
		t.Fatal("failed selector changed caller-owned seed count")
	}
}

// Already offset-ordered flat records should use the existing model slice
// directly; reverse-name source order must retain the indexed sort fallback.
func TestProjectContextFlatPathMonotonicOrderAvoidsIndexScratch(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, reverseNames := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse-names-%t", reverseNames), func(t *testing.T) {
			var source strings.Builder
			for index := 0; index < 128; index++ {
				name := index
				if reverseNames {
					name = 127 - index
				}
				fmt.Fprintf(&source, "export class Sort%03d {}\n", name)
			}
			facts := projectResolverFacts(t, TypeScriptAnalyzer{},
				filepath.Join("project", "context", "sorted-direct.ts"), source.String())
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, projectResolverLimitsForTest())
			if err != nil {
				t.Fatal(err)
			}
			selector := queryPathSelector(facts)
			baseline := *model
			baseline.referencesByTargetIndices = map[string][]int{"unmatched": nil}
			for _, limit := range []int{8, 16} {
				for _, budget := range []int{64, 4096, 1 << 20} {
					options := ProjectContextOptions{BudgetBytes: budget, MaxItems: limit, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
					want, wantErr := baseline.PlanContext(context.Background(), []ProjectSelector{selector}, options)
					got, gotErr := model.PlanContext(context.Background(), []ProjectSelector{selector}, options)
					if operation.KindOf(wantErr) != operation.KindOf(gotErr) || !reflect.DeepEqual(got, want) {
						t.Fatalf("reverse=%t limit=%d budget=%d error=%v wantError=%v got=%+v want=%+v",
							reverseNames, limit, budget, gotErr, wantErr, got, want)
					}
				}
			}
			if !reverseNames {
				options := ProjectContextOptions{BudgetBytes: 1 << 20, MaxItems: 16, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
				allocations := testing.AllocsPerRun(30, func() {
					if _, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, options); err != nil {
						panic(err)
					}
				})
				if allocations > 25 {
					t.Fatalf("monotonic flat path allocated %.0f times; retained unnecessary sort scratch", allocations)
				}
				// Equal offsets and IDs are comparison ties. Retain the
				// original indexed sort rather than assuming a stable order.
				pathKey := projectPathKey(facts.Path)
				tied := *model
				records := append([]projectSymbolRecord(nil), model.symbolsByFile[pathKey]...)
				records[1].symbol.declarationOffsets.Start = records[0].symbol.declarationOffsets.Start
				records[1].symbol.ID = records[0].symbol.ID
				records[1].entity.SymbolID = records[0].entity.SymbolID
				tied.symbolsByFile = map[string][]projectSymbolRecord{pathKey: records}
				tiedBaseline := tied
				tiedBaseline.referencesByTargetIndices = map[string][]int{"unmatched": nil}
				want, wantErr := tiedBaseline.PlanContext(context.Background(), []ProjectSelector{selector}, options)
				got, gotErr := tied.PlanContext(context.Background(), []ProjectSelector{selector}, options)
				if operation.KindOf(wantErr) != operation.KindOf(gotErr) || !reflect.DeepEqual(got, want) {
					t.Fatalf("equal comparator keys changed plan: got=%+v want=%+v err=%v baselineErr=%v", got, want, gotErr, wantErr)
				}
				tiedAllocs := testing.AllocsPerRun(30, func() {
					if _, err := tied.PlanContext(context.Background(), []ProjectSelector{selector}, options); err != nil {
						panic(err)
					}
				})
				if tiedAllocs < 27 {
					t.Fatalf("equal comparator keys bypassed original sort: %.0f allocations", tiedAllocs)
				}
			}
		})
	}
}

// A capacity hint must not change resolved context, repeated-import
// deduplication, or the byte budget's ability to reject candidates.
func TestProjectContextDependencyCapacityHintPreservesPlans(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var sharedSource, clientSource strings.Builder
	for index := 0; index < 96; index++ {
		fmt.Fprintf(&sharedSource, "export class Item%d {}\n", index)
	}
	for index := 0; index < 48; index++ {
		fmt.Fprintf(&clientSource, "import { Item%d } from \"./shared\";\n", index)
	}
	clientSource.WriteString("export class Client {}\n")
	root := filepath.Join("project", "context", "capacity")
	shared := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "shared.ts"), sharedSource.String())
	client := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join(root, "client.ts"), clientSource.String())
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{shared, client}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	selector := queryPathSelector(client)
	options := ProjectContextOptions{BudgetBytes: 1 << 20, MaxItems: 127, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}
	withoutHint, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, options)
	if err != nil {
		t.Fatal(err)
	}
	options.MaxItems = 128
	withHint, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(withHint.Candidates) < 90 || !reflect.DeepEqual(withoutHint, withHint) {
		t.Fatalf("capacity hint changed broad dependency plan: without=%d with=%d", len(withoutHint.Candidates), len(withHint.Candidates))
	}
	options.BudgetBytes = 64
	tight, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, options)
	if err != nil {
		t.Fatal(err)
	}
	if tight.UsedBytes > options.BudgetBytes || !tight.Truncated || len(tight.Candidates) >= len(withHint.Candidates) {
		t.Fatalf("byte-limited plan lost budget/truncation: used=%d candidates=%d truncated=%t", tight.UsedBytes, len(tight.Candidates), tight.Truncated)
	}
}

// Flat-path roots may stop only after a nonrequired target has proved
// the item limit is exceeded. Byte pressure alone must not end scanning.
func TestProjectContextFlatTargetsComplete(t *testing.T) {
	for _, tc := range []struct {
		name      string
		items     int
		limit     int
		truncated bool
		want      bool
	}{
		{name: "at-item-limit-exact", items: 8, limit: 8},
		{name: "at-item-limit-overflow", items: 8, limit: 8, truncated: true, want: true},
		{name: "byte-budget-only", items: 5, limit: 8, truncated: true},
		{name: "empty-plan", items: 0, limit: 8},
		{name: "underfilled-truncation", items: 7, limit: 8, truncated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			planner := projectContextPlanner{
				options: ProjectContextOptions{MaxItems: tc.limit},
				plan: ProjectContextPlan{
					Candidates: make([]ProjectContextCandidate, tc.items),
					Truncated:  tc.truncated,
				},
			}
			if got := planner.flatTargetsComplete(); got != tc.want {
				t.Fatalf("stop=%t want=%t items=%d limit=%d truncated=%t",
					got, tc.want, tc.items, tc.limit, tc.truncated)
			}
		})
	}
}

// The compact flat-file planner must agree byte-for-byte with the general
// planner, including item/byte limits, ordering and truncation.
func TestProjectContextFlatPathCompactPlanMatchesGeneral(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	for index := 0; index < 128; index++ {
		fmt.Fprintf(&source, "export class Flat%04d {}\n", index)
	}
	facts := projectResolverFacts(t, TypeScriptAnalyzer{}, filepath.Join("project", "context", "compact-flat.ts"), source.String())
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	selector := queryPathSelector(facts)
	// The otherwise unused target index selects the existing general route
	// without changing any actual reference or dependency result.
	baseline := *model
	baseline.referencesByTargetIndices = map[string][]int{"unused-ref": nil}
	for _, maxItems := range []int{1, 8, 16} {
		for _, budget := range []int{64, 4096, 1 << 20} {
			options := ProjectContextOptions{MaxItems: maxItems, MaxDepth: 2, BudgetBytes: budget, BodyPolicy: ProjectContextSignaturesOnly}
			want, err := baseline.PlanContext(context.Background(), []ProjectSelector{selector}, options)
			if err != nil {
				t.Fatalf("baseline items=%d budget=%d: %v", maxItems, budget, err)
			}
			got, err := model.PlanContext(context.Background(), []ProjectSelector{selector}, options)
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("compact plan changed items=%d budget=%d: got=%+v want=%+v err=%v", maxItems, budget, got, want, err)
			}
			fast, eligible, err := model.planFlatPathContext(context.Background(), selector, options)
			if !eligible || err != nil || !reflect.DeepEqual(fast, want) {
				t.Fatalf("eligible compact plan mismatched items=%d budget=%d: eligible=%t err=%v", maxItems, budget, eligible, err)
			}
		}
	}
	options := ProjectContextOptions{MaxItems: 8, MaxDepth: 2, BudgetBytes: 4096, BodyPolicy: ProjectContextSignaturesOnly}
	for name, mutate := range map[string]func(*ProjectModel){
		"structural-index": func(copy *ProjectModel) {
			copy.referencesByTargetIndices = map[string][]int{"other": nil}
		},
		"outgoing-dependency": func(copy *ProjectModel) {
			copy.dependenciesBySource = map[string][]ProjectDependency{projectPathKey(facts.Path): {{}}}
		},
		"incoming-dependency": func(copy *ProjectModel) {
			copy.dependentsByTarget = map[string][]ProjectDependency{projectPathKey(facts.Path): {{}}}
		},
		"nested-record": func(copy *ProjectModel) {
			records := append([]projectSymbolRecord(nil), model.symbolsByFile[projectPathKey(facts.Path)]...)
			records[0].symbol.ParentID = "unknown-parent"
			copy.symbolsByFile = map[string][]projectSymbolRecord{projectPathKey(facts.Path): records}
		},
		"equal-sort-keys": func(copy *ProjectModel) {
			records := append([]projectSymbolRecord(nil), model.symbolsByFile[projectPathKey(facts.Path)]...)
			records[1].entity.QualifiedName = records[0].entity.QualifiedName
			records[1].entity.SymbolID = records[0].entity.SymbolID
			copy.symbolsByFile = map[string][]projectSymbolRecord{projectPathKey(facts.Path): records}
		},
	} {
		t.Run(name, func(t *testing.T) {
			copy := *model
			mutate(&copy)
			if _, eligible, err := copy.planFlatPathContext(context.Background(), selector, options); eligible || err != nil {
				t.Fatalf("unsafe fast path accepted model: eligible=%t err=%v", eligible, err)
			}
		})
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = model.PlanContext(canceled, []ProjectSelector{selector}, options)
	if operation.KindOf(err) != operation.KindCancelled {
		t.Fatalf("canceled compact plan error=%v", err)
	}
}

// A broad flat file must use the compact path without changing the exact
// general plan, including when a byte budget truncates before the item limit.
func TestProjectContextBroadFlatPathMatchesGeneral(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	var source strings.Builder
	for index := 0; index < 2048; index++ {
		fmt.Fprintf(&source, "export class Flat%05d {}\n", index)
	}
	document := sourceDocumentForScanner(source.String())
	document.Path = filepath.Join("project", "context", "large-flat.ts")
	document.SourceFingerprint = benchmarkDigest(document.Text)
	analyzed, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 8192))
	if err != nil || !analyzed.Analysis.CoverageComplete {
		t.Fatalf("analyze broad flat context: %v", err)
	}
	facts := ProjectFileFacts{Path: document.Path, Language: "typescript", SourceFingerprint: document.SourceFingerprint, Analysis: analyzed}
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, ProjectResolverLimits{
		MaxFiles: 1, MaxSymbols: 8192, MaxDependencies: 8192, MaxReferences: 8192, MaxCandidatesPerResolution: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	selector := queryPathSelector(facts)
	baseline := *model
	baseline.referencesByTargetIndices = map[string][]int{"unused-ref": nil}
	for _, maxItems := range []int{128, 256} {
		for _, budget := range []int{64, 4096, 1 << 20} {
			options := ProjectContextOptions{MaxItems: maxItems, MaxDepth: 2, BudgetBytes: budget, BodyPolicy: ProjectContextSignaturesOnly}
			want, err := baseline.PlanContext(context.Background(), []ProjectSelector{selector}, options)
			if err != nil {
				t.Fatalf("general plan items=%d budget=%d: %v", maxItems, budget, err)
			}
			got, eligible, err := model.planFlatPathContext(context.Background(), selector, options)
			if !eligible || err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("broad flat compact plan items=%d budget=%d eligible=%t err=%v candidates=%d want=%d", maxItems, budget, eligible, err, len(got.Candidates), len(want.Candidates))
			}
		}
	}
}

// Moderately wide flat files should retain exactly the general planner's
// ordering, budget and truncation behavior when the compact gate is lowered.
func TestProjectContextMidrangeFlatPathMatchesGeneral(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		count, maxItems int
	}{{512, 128}, {1024, 256}} {
		t.Run(fmt.Sprintf("symbols-%d/items-%d", tc.count, tc.maxItems), func(t *testing.T) {
			var source strings.Builder
			for index := 0; index < tc.count; index++ {
				fmt.Fprintf(&source, "export class Flat%05d {}\n", index)
			}
			document := sourceDocumentForScanner(source.String())
			document.Path = filepath.Join("project", "context", fmt.Sprintf("midrange-flat-%d.ts", tc.count))
			document.SourceFingerprint = benchmarkDigest(document.Text)
			analyzed, err := (TypeScriptAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 8192))
			if err != nil || !analyzed.Analysis.CoverageComplete {
				t.Fatalf("analyze midrange flat context: %v", err)
			}
			facts := ProjectFileFacts{Path: document.Path, Language: "typescript", SourceFingerprint: document.SourceFingerprint, Analysis: analyzed}
			model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{facts}, ProjectResolverLimits{
				MaxFiles: 1, MaxSymbols: 8192, MaxDependencies: 8192, MaxReferences: 8192, MaxCandidatesPerResolution: 64,
			})
			if err != nil {
				t.Fatal(err)
			}
			selector := queryPathSelector(facts)
			general := *model
			general.referencesByTargetIndices = map[string][]int{"unused-ref": nil}
			for _, budget := range []int{64, 4096, 1 << 20} {
				options := ProjectContextOptions{MaxItems: tc.maxItems, MaxDepth: 2, BudgetBytes: budget, BodyPolicy: ProjectContextSignaturesOnly}
				want, wantErr := general.PlanContext(context.Background(), []ProjectSelector{selector}, options)
				got, eligible, gotErr := model.planFlatPathContext(context.Background(), selector, options)
				if !eligible || operation.KindOf(gotErr) != operation.KindOf(wantErr) || !reflect.DeepEqual(got, want) {
					t.Fatalf("midrange plan items=%d budget=%d eligible=%t err=%v generalErr=%v got=%+v want=%+v", tc.maxItems, budget, eligible, gotErr, wantErr, got, want)
				}
				public, publicErr := model.PlanContext(context.Background(), []ProjectSelector{selector}, options)
				if operation.KindOf(publicErr) != operation.KindOf(wantErr) || !reflect.DeepEqual(public, want) {
					t.Fatalf("public midrange context differs from general plan items=%d budget=%d err=%v generalErr=%v", tc.maxItems, budget, publicErr, wantErr)
				}
			}
			if tc.count == 512 {
				// The conservative gate must keep smaller ratios on the general path.
				options := ProjectContextOptions{MaxItems: 256, MaxDepth: 2, BudgetBytes: 1 << 20, BodyPolicy: ProjectContextSignaturesOnly}
				if _, eligible, err := model.planFlatPathContext(context.Background(), selector, options); eligible || err != nil {
					t.Fatalf("ratio-two flat path should stay general: eligible=%t err=%v", eligible, err)
				}
			}
		})
	}
}

// Sorting may inspect model-owned declaration offsets without allocating
// cloned signature/body ranges or mutating any retained symbol.
func TestProjectContextSeedSortAvoidsOffsetClone(t *testing.T) {
	const count = 128
	seeds := make([]projectContextSeed, count)
	for i := range seeds {
		start := count - i
		signature := OffsetRange{Start: start, End: start + 2}
		body := OffsetRange{Start: start + 2, End: start + 4}
		seeds[i] = projectContextSeed{record: projectSymbolRecord{
			pathKey: "same.ts",
			symbol: NormalizedSymbol{
				ID:                 fmt.Sprintf("id-%03d", i),
				declarationOffsets: OffsetRange{Start: start, End: start + 4},
				signatureOffsets:   &signature,
				bodyOffsets:        &body,
			},
		}}
	}
	allocs := testing.AllocsPerRun(30, func() { sortContextSeeds(seeds) })
	if allocs > 10 {
		t.Fatalf("seed sort cloned unused offset ranges: %.0f allocations", allocs)
	}
	for i := 1; i < len(seeds); i++ {
		if seeds[i-1].record.symbol.declarationOffsets.Start > seeds[i].record.symbol.declarationOffsets.Start {
			t.Fatalf("declaration order changed at %d", i)
		}
	}
	for _, seed := range seeds {
		if seed.record.symbol.signatureOffsets == nil || seed.record.symbol.bodyOffsets == nil {
			t.Fatal("sorting corrupted immutable optional offsets")
		}
	}
	// Canonical tie behavior is path, then declaration start, then symbol ID.
	ties := []projectContextSeed{
		{record: projectSymbolRecord{pathKey: "b", symbol: NormalizedSymbol{ID: "z", declarationOffsets: OffsetRange{Start: 1}}}},
		{record: projectSymbolRecord{pathKey: "a", symbol: NormalizedSymbol{ID: "z", declarationOffsets: OffsetRange{Start: 2}}}},
		{record: projectSymbolRecord{pathKey: "a", symbol: NormalizedSymbol{ID: "b", declarationOffsets: OffsetRange{Start: 1}}}},
		{record: projectSymbolRecord{pathKey: "a", symbol: NormalizedSymbol{ID: "a", declarationOffsets: OffsetRange{Start: 1}}}},
	}
	sortContextSeeds(ties)
	for i, want := range []string{"a", "b", "z", "z"} {
		if ties[i].record.symbol.ID != want {
			t.Fatalf("seed tie ordering at %d: %s instead of %s", i, ties[i].record.symbol.ID, want)
		}
	}
}

func TestProjectContextRootSymbolsPreserveParentKindsAndFallback(t *testing.T) {
	record := func(id, parent string, kind SymbolKind) projectSymbolRecord {
		return projectSymbolRecord{
			symbol: NormalizedSymbol{ID: id, ParentID: parent, Kind: kind},
			entity: RelationEntity{Path: "source.ts", SymbolID: id, QualifiedName: id},
		}
	}
	cases := []struct {
		name    string
		records []projectSymbolRecord
		wantIDs []string
	}{
		{
			name: "root container and nested members",
			records: []projectSymbolRecord{
				record("root", "", SymbolKindModule),
				record("child", "root", SymbolKindClass),
				record("nested", "child", SymbolKindMethod),
				record("top", "", SymbolKindClass),
				record("orphan", "missing", SymbolKindClass),
			},
			wantIDs: []string{"child", "root", "top"},
		},
		{
			name: "fallback when no root qualifies",
			records: []projectSymbolRecord{
				record("b", "missing", SymbolKindMethod),
				record("a", "missing", SymbolKindClass),
			},
			wantIDs: []string{"a", "b"},
		},
		{
			name: "latest parent kind replaces older container",
			records: []projectSymbolRecord{
				record("duplicate", "", SymbolKindModule),
				record("child", "duplicate", SymbolKindClass),
				record("duplicate", "", SymbolKindClass),
			},
			wantIDs: []string{"duplicate", "duplicate"},
		},
		{
			name: "latest parent kind replaces older noncontainer",
			records: []projectSymbolRecord{
				record("duplicate", "", SymbolKindClass),
				record("child", "duplicate", SymbolKindMethod),
				record("duplicate", "", SymbolKindPackage),
			},
			wantIDs: []string{"child", "duplicate", "duplicate"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &ProjectModel{symbolsByFile: map[string][]projectSymbolRecord{"file": tc.records}}
			got := model.contextRootSymbols("file")
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("root symbols=%d, want %d: %+v", len(got), len(tc.wantIDs), got)
			}
			for i, want := range tc.wantIDs {
				if got[i].symbol.ID != want {
					t.Fatalf("root symbol %d=%q, want %q", i, got[i].symbol.ID, want)
				}
			}
		})
	}
}
