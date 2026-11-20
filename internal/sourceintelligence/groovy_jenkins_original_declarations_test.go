package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The two complete upstream originals are optional, immutable external fixtures.
// A configured missing or unreadable corpus is an error rather than a silent skip.
func TestGroovyOriginalJenkinsAuthoredDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_GROOVY_JENKINS_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_GROOVY_JENKINS_ROOT to the complete original Jenkinsfile corpus")
	}
	for _, tc := range []struct {
		file string
		want []struct {
			name   string
			kind   SymbolKind
			line   int
			column int
		}
	}{
		{file: "packaging-Jenkinsfile", want: []struct {
			name   string
			kind   SymbolKind
			line   int
			column int
		}{
			{"jobProperties", SymbolKindVariable, 1, 5},
			{"nodeWithTimeout", SymbolKindFunction, 73, 6},
		}},
		{file: "jenkins-design-Jenkinsfile"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			source, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil {
				t.Fatalf("original source unreadable: %v", err)
			}
			doc := sourceDocumentForScanner(string(source))
			doc.Path = tc.file
			out, err := (GroovyAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !out.Analysis.CoverageComplete || out.Analysis.Truncated {
				t.Errorf("complete original reported incomplete/truncated: %+v", out.Analysis)
			}
			for _, want := range tc.want {
				found := false
				for _, got := range out.Analysis.Symbols {
					if got.Name != want.name || got.NameRange.Start.Line != want.line ||
						got.NameRange.Start.Column != want.column || got.Kind != want.kind {
						continue
					}
					found = true
					if got.ParentID != "" || got.ParentQualifiedName != "" ||
						got.QualifiedName != want.name {
						t.Errorf("wrong owner for original %s: %+v", want.name, got)
					}
					break
				}
				if !found {
					t.Errorf("authored declaration %s at %d:%d (%s) missing; actual=%v",
						want.name, want.line, want.column, want.kind,
						sortedSymbolQualifiedNames(out.Analysis.Symbols))
				}
			}
			if len(out.Analysis.Symbols) != len(tc.want) {
				for _, sym := range out.Analysis.Symbols {
					matched := false
					for _, want := range tc.want {
						if sym.Name == want.name && sym.NameRange.Start.Line == want.line &&
							sym.NameRange.Start.Column == want.column && sym.Kind == want.kind {
							matched = true
						}
					}
					if !matched {
						t.Errorf("original DSL call or other nondeclaration invented as %s %s at %d:%d",
							sym.Kind, sym.Name, sym.NameRange.Start.Line, sym.NameRange.Start.Column)
					}
				}
				t.Errorf("declared symbol count=%d want=%d", len(out.Analysis.Symbols), len(tc.want))
			}
		})
	}
}
