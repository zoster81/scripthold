package sourceintelligence

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

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
