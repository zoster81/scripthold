package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The original sources are opt-in external fixtures, never copied into the repository.
// An explicitly configured but incomplete corpus is a failing test setup.
func TestPLSQLOriginalQualifiedPackageHeadersAndOwners(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PLSQL_QUALIFIED_ROOT")
	if root == "" {
		t.Skip("set SCRIPTHOLD_REAL_PLSQL_QUALIFIED_ROOT to the verified original-source corpus")
	}
	type wantSymbol struct {
		line   int
		name   string
		native string
	}
	tests := []struct {
		file        string
		packageName string
		packageLine int
		routines    []wantSymbol
	}{
		{
			file: "ut_expectation_processor.pks", packageName: "ut_expectation_processor", packageLine: 1,
			routines: []wantSymbol{
				{23, "nulls_are_equal", "function"},
				{25, "nulls_are_equal", "procedure"},
				{27, "get_status", "function"},
				{29, "clear_expectations", "procedure"},
				{31, "get_all_expectations", "function"},
				{33, "get_failed_expectations", "function"},
				{35, "add_expectation_result", "procedure"},
				{37, "report_failure", "procedure"},
				{39, "set_xml_nls_params", "procedure"},
				{41, "reset_nls_params", "procedure"},
				{49, "who_called_expectation", "function"},
				{51, "add_warning", "procedure"},
				{53, "add_depreciation_warning", "procedure"},
				{55, "get_warnings", "function"},
				{57, "invalidation_exception_found", "function"},
				{58, "set_invalidation_exception", "procedure"},
				{59, "reset_invalidation_exception", "procedure"},
			},
		},
		{
			file: "xdbConfiguration.sql", packageName: "COE_CONFIGURATION", packageLine: 56,
			routines: []wantSymbol{
				{59, "setHTTPport", "procedure"},
				{60, "setFTPport", "procedure"},
				{61, "getDatabaseSummary", "function"},
				{62, "folderDatabaseSummary", "procedure"},
				{72, "getDatabaseSummary", "function"},
				{139, "folderDatabaseSummary", "procedure"},
				{162, "setXDBport", "procedure"},
				{177, "setHTTPport", "procedure"},
				{183, "setFTPport", "procedure"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			bytes, err := os.ReadFile(filepath.Join(root, tc.file))
			if err != nil {
				t.Fatalf("configured original corpus is missing or unreadable: %v", err)
			}
			doc := sourceDocumentForScanner(string(bytes))
			doc.Path = tc.file
			result, err := (PLSQLAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 100))
			if err != nil {
				t.Fatal(err)
			}
			symbols := result.Analysis.Symbols
			packages := 0
			for _, sym := range symbols {
				if sym.Kind == SymbolKindPackage && sym.Name == tc.packageName {
					packages++
					if sym.NameRange.Start.Line != tc.packageLine {
						t.Errorf("package %s should start at source spec line %d, got %d", tc.packageName, tc.packageLine, sym.NameRange.Start.Line)
					}
				}
			}
			if packages != 1 {
				t.Errorf("expected one canonical package %s, found %d", tc.packageName, packages)
			}
			for _, want := range tc.routines {
				found := false
				for _, sym := range symbols {
					if sym.Name != want.name || sym.NameRange.Start.Line != want.line || sym.NativeKind != want.native {
						continue
					}
					found = true
					if sym.ParentQualifiedName != tc.packageName || sym.QualifiedName != tc.packageName+"."+want.name || sym.ParentID == "" {
						t.Errorf("routine %s line %d has wrong package owner: qualified=%q parent=%q id=%q",
							want.name, want.line, sym.QualifiedName, sym.ParentQualifiedName, sym.ParentID)
					}
					break
				}
				if !found {
					t.Errorf("missing authored %s %s at line %d", want.native, want.name, want.line)
				}
			}
			if len(symbols) != len(tc.routines)+1 {
				t.Errorf("unexpected structural symbols: expected %d, got %d", len(tc.routines)+1, len(symbols))
			}
		})
	}
}

func TestPLSQLQualifiedPackageHeaderBoundaries(t *testing.T) {
	source := `Rem CREATE OR REPLACE PACKAGE RemFake AS
-- CREATE OR REPLACE PACKAGE CommentFake AUTHID DEFINER AS
/* CREATE OR REPLACE NONEDITIONABLE PACKAGE BlockFake AUTHID CURRENT_USER AS */
SELECT 'CREATE OR REPLACE PACKAGE StringFake AS' FROM dual;
CREATE OR REPLACE NONEDITIONABLE PACKAGE First AUTHID CURRENT_USER AS
  PROCEDURE Do_First;
  PROCEDURE Overload(x integer);
  PROCEDURE Overload(x varchar2);
END First;
/
CREATE OR REPLACE EDITIONABLE PACKAGE Second
AUTHID DEFINER
IS
  FUNCTION Do_Second RETURN NUMBER;
END Second;
/
CREATE OR REPLACE PACKAGE BODY First AS
  PROCEDURE Do_First IS BEGIN NULL; END;
END First;
/
CREATE OR REPLACE PACKAGE Invalid AUTHID FORGED AS
  PROCEDURE Not_Owned;
END Invalid;
/
`
	for _, tc := range []struct {
		name string
		text string
	}{
		{name: "lf", text: source},
		{name: "crlf", text: strings.ReplaceAll(source, "\n", "\r\n")},
		{name: "utf8-bom", text: "\uFEFF" + source},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (PLSQLAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range []string{"First", "Second", "First.Do_First", "First.Overload", "Second.Do_Second"} {
				if _, ok := byName[name]; !ok {
					t.Errorf("valid authored declaration absent: %q", name)
				}
			}
			for _, name := range []string{"RemFake", "CommentFake", "BlockFake", "StringFake", "Invalid", "Invalid.Not_Owned"} {
				if _, ok := byName[name]; ok {
					t.Errorf("invalid or opaque package invented: %q", name)
				}
			}
			overloads, firstPackages := 0, 0
			for _, sym := range result.Analysis.Symbols {
				if sym.QualifiedName == "First.Overload" {
					overloads++
				}
				if sym.QualifiedName == "First" {
					firstPackages++
				}
				if sym.Name == "Not_Owned" && sym.ParentQualifiedName != "" {
					t.Errorf("unsupported authid attached a false owner: %+v", sym)
				}
			}
			if overloads != 2 || firstPackages != 1 {
				t.Errorf("lost overloads or merged package unexpectedly: overloads=%d firstPackages=%d", overloads, firstPackages)
			}
		})
	}
}
