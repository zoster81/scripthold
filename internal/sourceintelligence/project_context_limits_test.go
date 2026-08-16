package sourceintelligence

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase14ProjectContextExplicitTargetsAreDeterministicAndAtomicUnderItemLimit(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase14", "targets")
	alphaPath := filepath.Join(root, "Alpha.java")
	betaPath := filepath.Join(root, "Beta.java")
	alpha := phase12Facts(t, JavaAnalyzer{}, alphaPath, "package demo; public class Alpha { public void one() {} }\n")
	beta := phase12Facts(t, JavaAnalyzer{}, betaPath, "package demo; public class Beta { public void two() {} }\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{beta, alpha}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	alphaSelector := phase13SymbolSelector(alpha, phase12SymbolID(t, alpha, "demo.Alpha.one"))
	betaSelector := phase13SymbolSelector(beta, phase12SymbolID(t, beta, "demo.Beta.two"))
	options := ProjectContextOptions{BudgetBytes: 4096, MaxItems: 8, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly}

	left, err := model.PlanContext(context.Background(), []ProjectSelector{alphaSelector, betaSelector}, options)
	if err != nil {
		t.Fatal(err)
	}
	right, err := model.PlanContext(context.Background(), []ProjectSelector{betaSelector, alphaSelector}, options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("target order changed plan:\nleft=%+v\nright=%+v", left, right)
	}

	limited := options
	limited.MaxItems = 1
	if _, err := model.PlanContext(context.Background(), []ProjectSelector{alphaSelector, betaSelector}, limited); operation.KindOf(err) != operation.KindLimit {
		t.Fatalf("explicit target item limit error = %v kind=%v", err, operation.KindOf(err))
	}
}

func TestR27Phase14ProjectContextDepthTruncationRequiresUnseenTarget(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase14", "depth")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	cPath := filepath.Join(root, "c.ts")
	a := phase12Facts(t, TypeScriptAnalyzer{}, aPath, "import { B } from \"./b\";\nexport class A {}\n")
	b := phase12Facts(t, TypeScriptAnalyzer{}, bPath, "import { C } from \"./c\";\nexport class B {}\n")
	c := phase12Facts(t, TypeScriptAnalyzer{}, cPath, "export class C {}\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{a, b, c}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := model.PlanContext(context.Background(), []ProjectSelector{phase13PathSelector(a)}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 16, MaxDepth: 1, BodyPolicy: ProjectContextSignaturesOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Truncated || phase14HasContextCandidate(plan.Candidates, cPath, ContextDeeperRelation, 7) {
		t.Fatalf("depth-limited plan = %+v", plan)
	}

	cycleA := phase12Facts(t, TypeScriptAnalyzer{}, aPath, "import { B } from \"./b\";\nexport class A {}\n")
	cycleB := phase12Facts(t, TypeScriptAnalyzer{}, bPath, "import { A } from \"./a\";\nexport class B {}\n")
	cycleModel, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{cycleB, cycleA}, phase12ResolverLimits())
	if err != nil {
		t.Fatal(err)
	}
	cyclePlan, err := cycleModel.PlanContext(context.Background(), []ProjectSelector{phase13PathSelector(cycleA)}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 16, MaxDepth: 1, BodyPolicy: ProjectContextSignaturesOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cyclePlan.Truncated {
		t.Fatalf("visited-only cycle incorrectly marked truncated: %+v", cyclePlan)
	}
}
