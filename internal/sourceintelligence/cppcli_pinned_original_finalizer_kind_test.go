package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCPPCLIPinnedWholeOriginalFinalizerIsNotConstructor(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned C++/CLI originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "cppcli-actor-f8c14d9f.h"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 2895 {
		t.Fatalf("upstream Actor.h changed: %d", len(b))
	}
	for _, tc := range []struct{ name, source string }{
		{"git-lf", string(b)}, {"worktree-crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("Actor.h", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 38 {
				t.Fatalf("whole original 38 supported declarations including managed block properties, got %d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
			}
			var finalized, constructed, destructed int
			for _, s := range result.Analysis.Symbols {
				if s.NameRange.Start.Line == 37 && s.Name == "Actor" {
					finalized++
					if s.Kind != SymbolKindMethod || s.NativeKind != "finalizer-declaration" ||
						s.ParentQualifiedName != "PhysX.Actor" || s.DeclarationRange.Start.Line != 37 {
						t.Errorf("original !Actor() is a finalizer, not constructor: %+v", s)
					}
				}
				if s.NameRange.Start.Line == 33 && s.Name == "Actor" {
					constructed++
					if s.Kind != SymbolKindConstructor {
						t.Errorf("authored constructor changed: %+v", s)
					}
				}
				if s.NameRange.Start.Line == 35 && s.Name == "~Actor" {
					destructed++
					if s.Kind != SymbolKindDestructor {
						t.Errorf("authored destructor changed: %+v", s)
					}
				}
			}
			if finalized != 1 || constructed != 1 || destructed != 1 {
				t.Errorf("expected one each of native constructor, destructor and finalizer; got %d/%d/%d", constructed, destructed, finalized)
			}
		})
	}
}

func TestCPPCLIPinnedIndependentWholeMetaControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPPCLI_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned C++/CLI originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "cppcli-meta-5f5b92b8.h"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 443 {
		t.Fatalf("upstream Meta.h changed: %d", len(b))
	}
	result, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("Meta.h", string(b)), testAnalyzeOptions(true, 48))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || len(result.Analysis.Symbols) != 6 {
		t.Fatalf("independent original six declarations: count=%d complete=%v", len(result.Analysis.Symbols), result.Analysis.CoverageComplete)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range []string{"libyaraNET", "libyaraNET.Meta", "libyaraNET.Meta.Identifier", "libyaraNET.Meta.Value"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("independent authored name missing: %s", name)
		}
	}
}
