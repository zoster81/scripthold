package sourceintelligence

import (
	"context"
	"path/filepath"
	"testing"
)

func TestProjectContextDeeperRelationsAreAlwaysSignatures(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase14", "representation")
	aPath := filepath.Join(root, "a.ts")
	bPath := filepath.Join(root, "b.ts")
	cPath := filepath.Join(root, "c.ts")
	a := projectResolverFacts(t, TypeScriptAnalyzer{}, aPath, "import { B } from \"./b\";\nexport class A { value() { return 1; } }\n")
	b := projectResolverFacts(t, TypeScriptAnalyzer{}, bPath, "import { C } from \"./c\";\nexport class B { value() { return 2; } }\n")
	c := projectResolverFacts(t, TypeScriptAnalyzer{}, cPath, "export class C { value() { return 3; } }\n")
	model, err := BuildProjectModel(context.Background(), registry, []ProjectFileFacts{a, b, c}, projectResolverLimitsForTest())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := model.PlanContext(context.Background(), []ProjectSelector{queryPathSelector(a)}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 16, MaxDepth: 2, BodyPolicy: ProjectContextPrefer,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range plan.Candidates {
		if candidate.Entity.Path == cPath && candidate.Reason == ContextDeeperRelation {
			if candidate.Representation != ContextSignature {
				t.Fatalf("deeper candidate retained %q under prefer: %+v", candidate.Representation, candidate)
			}
			return
		}
	}
	t.Fatalf("deeper C candidate missing: %+v", plan)
}
