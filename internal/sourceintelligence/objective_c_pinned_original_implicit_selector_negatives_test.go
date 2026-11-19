package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original upstream Ctags Objective-C source has three untyped selector
// argument names that must never become invented method declarations.
func TestObjectiveCPinnedOriginalImplicitSelectorNegatives(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OBJECTIVEC_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned Objective-C original is not configured")
	}
	path := filepath.Join(root, "ctags-objc-signature.m")
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 957 {
		t.Fatalf("unexpected original source metadata: %v %v", info, err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (ObjectiveCAnalyzer{}).Analyze(context.Background(),
		testSourceDocument(path, string(contents)), testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 13 {
		t.Fatalf("expected 13 supported typed declarations, got %d coverage=%v: %+v",
			len(result.Analysis.Symbols), result.Analysis.CoverageComplete, result.Analysis.Symbols)
	}
	for _, s := range result.Analysis.Symbols {
		if strings.Contains(s.Name, "idarg") {
			t.Errorf("argument invented as method: %+v", s)
		}
	}
}

func TestObjectiveCImplicitSelectorArgumentIsNotMethod(t *testing.T) {
	source := "@interface X\n- doNothing: (id) argument;\n- (void)realMethod: (id) argument;\n@end\n"
	result, err := (ObjectiveCAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("selector.m", source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Analysis.Symbols) != 2 ||
		result.Analysis.Symbols[0].Name != "X" ||
		result.Analysis.Symbols[1].Name != "realMethod:" {
		t.Fatalf("untyped argument must not become a method: %+v", result.Analysis.Symbols)
	}
}
