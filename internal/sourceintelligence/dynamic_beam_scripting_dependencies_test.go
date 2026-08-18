package sourceintelligence

import (
	"context"
	"testing"
)

func TestStaticDependencyFormsDoNotRequireQuotes(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
		want     []string
	}{
		{"posix-dot-source", ShellAnalyzer{}, ". ./lib/common.sh\nbuild() { :; }\n", []string{"./lib/common.sh"}},
		{"bash-source", BashAnalyzer{}, "source ./lib/common.bash\nbuild() { :; }\n", []string{"./lib/common.bash"}},
		{"tcl-source", TclAnalyzer{}, "source lib/common.tcl\nproc run {} {}\n", []string{"lib/common.tcl"}},
		{"gleam-import-alias", GleamAnalyzer{}, "import gleam/base as base\npub fn run() { base.identity(1) }\n", []string{"gleam/base"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if got := dependencyValues(result.Dependencies); !sameStringSet(got, tc.want) {
				t.Fatalf("dependencies=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestDynamicDependencyTargetsStayConservative(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"posix-expansion", ShellAnalyzer{}, ". $LIB_DIR/common.sh\nrun() { :; }\n"},
		{"bash-command-substitution", BashAnalyzer{}, "source $(resolve_lib)\nrun() { :; }\n"},
		{"tcl-substitution", TclAnalyzer{}, "source $libPath\nproc run {} {}\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Dependencies) != 0 {
				t.Fatalf("dynamic dependency was overclaimed: %+v", result.Dependencies)
			}
		})
	}
}
