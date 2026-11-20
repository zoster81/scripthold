package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestFortranTypePrefixedFunctionNavigation(t *testing.T) {
	source := `module fpm_release
  type :: version_t
  end type version_t
  type(version_t) :: not_a_function
  type(version_t), pointer :: another_variable
contains
  type(version_t) function fpm_version()
    type(version_t) :: value
    fpm_version = value
  end function fpm_version
end module fpm_release
`
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "lf", text: source},
		{name: "crlf", text: strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (FortranAnalyzer{}).Analyze(context.Background(), testSourceDocument("fpm_release.F90", tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			got, ok := byName["fpm_release.fpm_version"]
			if !ok || got.NameRange.Start.Line != 7 || got.Kind != SymbolKindFunction {
				t.Fatalf("typed function fpm_version not found at line 7: %+v exists=%v symbols=%v", got, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
			for _, name := range []string{"fpm_release.not_a_function", "fpm_release.another_variable", "fpm_release.fpm_version.value"} {
				if _, exists := byName[name]; exists {
					t.Fatalf("variable %s misclassified as function", name)
				}
			}
		})
	}
}
