//go:build !race

package sourceintelligence

import (
	"context"
	"encoding/hex"
	"path/filepath"
	"testing"
)

// Keep the allocation budget outside race builds because race instrumentation changes allocation behavior.
func TestBuildProjectModelAvoidsRepeatedPathCanonicalization(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	facts := make([]ProjectFileFacts, 0, 65)
	facts = append(facts, projectResolverFacts(t, JavaAnalyzer{}, "project/allocation/Base.java", "package demo; public class Base {}\n"))
	for index := 0; index < 64; index++ {
		suffix := hex.EncodeToString([]byte{byte(index)})
		path := filepath.Join("project", "allocation", "Child"+suffix+".java")
		text := "package demo; public class Child" + suffix + " extends Base {}\n"
		facts = append(facts, projectResolverFacts(t, JavaAnalyzer{}, path, text))
	}
	limits := projectResolverLimitsForTest()
	limits.MaxFiles = len(facts)

	var observed int
	allocations := testing.AllocsPerRun(5, func() {
		model, buildErr := BuildProjectModel(context.Background(), registry, facts, limits)
		if buildErr != nil {
			panic(buildErr)
		}
		observed += len(model.References())
	})
	if observed == 0 {
		t.Fatal("allocation guard produced no project references")
	}
	if allocations > 3900 {
		t.Fatalf("BuildProjectModel allocations = %.0f, want <= 3900", allocations)
	}
}
