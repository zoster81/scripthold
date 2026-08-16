package sourceintelligence

import (
	"context"
	"path/filepath"
	"testing"
)

func TestR27Phase14ProjectContextConflictingResolutionStatesBecomeAmbiguous(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join("project", "phase14", "resolution")
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
	bKey := projectPathKey(bPath)
	if len(model.dependenciesBySource[bKey]) != 1 {
		t.Fatalf("B dependencies = %+v", model.dependenciesBySource[bKey])
	}
	resolved := model.dependenciesBySource[bKey][0]
	ambiguous := resolved
	ambiguous.Evidence = SymbolEvidenceStructural
	ambiguous.Resolution = ResolutionAmbiguous
	model.dependenciesBySource[bKey] = append(model.dependenciesBySource[bKey], ambiguous)

	plan, err := model.PlanContext(context.Background(), []ProjectSelector{phase13PathSelector(a)}, ProjectContextOptions{
		BudgetBytes: 4096, MaxItems: 16, MaxDepth: 2, BodyPolicy: ProjectContextSignaturesOnly,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range plan.Candidates {
		if candidate.Entity.Path == cPath && candidate.Reason == ContextDeeperRelation {
			if candidate.Evidence != SymbolEvidenceStructural || candidate.Resolution != ResolutionAmbiguous {
				t.Fatalf("merged deeper candidate = %+v", candidate)
			}
			return
		}
	}
	t.Fatalf("merged deeper C candidate missing: %+v", plan)
}
