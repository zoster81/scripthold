package sourceintelligence

import (
	"context"
	"testing"
)

func TestPerlLexicallyScopedPackageRestoresOwnerOutsideBlock(t *testing.T) {
	source := "package MIME::Lite::HTML;\n" +
		"sub before { return 1; }\n" +
		"{\n" +
		"    package RequestAgent;\n" +
		"    sub new { return 'brace } in quoted text'; }\n" +
		"    sub get_basic_credentials { return 1; }\n" +
		"}\n" +
		"sub new { return 2; }\n" +
		"sub after { return 3; }\n" +
		"package Later;\n" +
		"sub final { return 4; }\n"
	result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source), testAnalyzeOptions(true, 80))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, qualified := range []string{
		"MIME::Lite::HTML.before",
		"RequestAgent.new",
		"RequestAgent.get_basic_credentials",
		"MIME::Lite::HTML.new",
		"MIME::Lite::HTML.after",
		"Later.final",
	} {
		if sym, ok := byName[qualified]; !ok || sym.ID == "" {
			t.Errorf("missing Perl subroutine declaration %q: %+v", qualified, sym)
		}
	}
	for _, forbidden := range []string{"RequestAgent.after", "RequestAgent.final", "Later.after"} {
		if sym, ok := byName[forbidden]; ok {
			t.Errorf("Perl lexical package leaked across closing brace as %q: %+v", forbidden, sym)
		}
	}
}
