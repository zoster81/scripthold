package sourceintelligence

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
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
