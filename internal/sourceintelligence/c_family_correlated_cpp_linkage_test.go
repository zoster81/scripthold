package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Separate #if defined(__cplusplus) guards often open and close the same
// linkage scope, across intervening includes. The structural variants must
// select matching branches without fabricating nested or global symbols.
func TestCFamilyCorrelatedCPPLinkageGuardAcrossInclude(t *testing.T) {
	source := "#if defined(__cplusplus)\nextern \"C\"\n{\n#endif\nint before(void);\n#include \"stable.h\"\n#if defined(__cplusplus)\n}\n#endif\nint after(void);\n"
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				got, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.value)), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if !got.Analysis.CoverageComplete || got.Analysis.Truncated {
					t.Fatalf("balanced correlated __cplusplus guard is not incomplete: %+v", got.Analysis.Diagnostics)
				}
				names := symbolsByQualifiedName(got.Analysis.Symbols)
				if len(got.Analysis.Symbols) != 2 {
					t.Fatalf("expected exactly two real functions, got %+v", got.Analysis.Symbols)
				}
				for _, name := range []string{"before", "after"} {
					s, ok := names[name]
					if !ok || s.Kind != SymbolKindFunction || s.ParentQualifiedName != "" {
						t.Errorf("lost or falsely nested global declaration %q: %+v found=%v", name, s, ok)
					}
				}
			})
		}
	}
}
