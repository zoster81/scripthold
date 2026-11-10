package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Multi-package scripts must not attribute declarations from the second package
// or later anonymous batches to the first package in the file.
func TestPLSQLPackageOwnerFollowsOriginalScriptScopes(t *testing.T) {
	source := `CREATE OR REPLACE PACKAGE Alpha AS
  PROCEDURE Start;
END Alpha;
/
CREATE OR REPLACE PACKAGE BODY Alpha AS
  PROCEDURE Start IS BEGIN NULL; END Start;
END Alpha;
/
CREATE OR REPLACE PACKAGE Beta AS
  PROCEDURE Run;
END Beta;
/
CREATE OR REPLACE PACKAGE BODY Beta AS
  PROCEDURE Run IS BEGIN NULL; END Run;
END Beta;
/
DECLARE
  PROCEDURE Helper IS BEGIN NULL; END Helper;
BEGIN
  NULL;
END;
/`
	result, err := (PLSQLAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("supported source unexpectedly partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{"Alpha", "Alpha.Start", "Beta", "Beta.Run", "Helper"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("expected declaration %q absent: %+v", want, result.Analysis.Symbols)
		}
	}
	for _, falseOwner := range []string{"Alpha.Run", "Alpha.Helper", "Beta.Helper"} {
		if _, ok := byName[falseOwner]; ok {
			t.Errorf("fabricated scope: %q", falseOwner)
		}
	}
	alpha, beta, helper := 0, 0, 0
	for _, symbol := range result.Analysis.Symbols {
		switch symbol.QualifiedName {
		case "Alpha":
			alpha++
		case "Beta":
			beta++
		case "Helper":
			helper++
			if symbol.ParentQualifiedName != "" {
				t.Errorf("anonymous script declaration has package parent: %+v", symbol)
			}
		}
	}
	if alpha != 1 || beta != 1 || helper != 1 {
		t.Fatalf("package duplicates or lost script local: Alpha=%d Beta=%d Helper=%d", alpha, beta, helper)
	}
}

func TestPLSQLScopeIgnoresSlashesInQuotedTextAndComments(t *testing.T) {
	source := `CREATE OR REPLACE PACKAGE First_Pkg AS
  note varchar2(10) := '
/
';
  /*
/
  */
  PROCEDURE Last_Method;
END First_Pkg;
/
CREATE OR REPLACE PACKAGE Second_Pkg AS
  PROCEDURE Next_Method;
END Second_Pkg;
/
`
	source = strings.ReplaceAll(source, "\n", "\r\n")
	result, err := (PLSQLAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 64))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{"First_Pkg.Last_Method", "Second_Pkg.Next_Method"} {
		if _, found := byName[want]; !found {
			t.Errorf("CRLF or opaque-slash source lost %q: %+v", want, result.Analysis.Symbols)
		}
	}
	if _, fabricated := byName["First_Pkg.Next_Method"]; fabricated {
		t.Fatalf("second package method assigned to first package: %+v", result.Analysis.Symbols)
	}
}
