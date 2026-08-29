package sourceintelligence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestLegacyFormatsPreserveFixedAndFreeStructure(t *testing.T) {
	t.Run("fortran-fixed-continuation-offsets", func(t *testing.T) {
		text := "C     SUBROUTINE Fake()\r\n      MODULE LEGACY\r\n      USE ISO_C_BINDING\r\n      SUBROUTINE\r\n     & RUN(X)\r\n      END SUBROUTINE RUN\r\n      END MODULE LEGACY\r\n"
		document := scientificLegacyFunctionalTestDocument("legacy.f", text)
		result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(true, 64))
		if err != nil {
			t.Fatal(err)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, want := range []string{"LEGACY", "LEGACY.RUN"} {
			if _, ok := byName[want]; !ok {
				t.Fatalf("missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
		if _, leaked := byName["Fake"]; leaked {
			t.Fatalf("fixed-form comment leaked Fake: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
		run := byName["LEGACY.RUN"]
		_, nameRange, _, _ := run.SourceOffsets()
		if got := document.Text[nameRange.Start:nameRange.End]; got != "RUN" {
			t.Fatalf("continued Fortran name range points to %q, want RUN: %+v", got, nameRange)
		}
		if got := dependencyValues(result.Dependencies); !sameStringSet(got, []string{"ISO_C_BINDING"}) {
			t.Fatalf("fixed Fortran dependencies=%v", got)
		}
	})

	t.Run("fortran-type-spec-is-not-type-definition", func(t *testing.T) {
		text := "module demo\n  type(worker) :: item\ncontains\n  subroutine run()\n  end subroutine run\nend module demo\n"
		result, err := (FortranAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.f90", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if _, leaked := symbolsByQualifiedName(result.Analysis.Symbols)["demo.item"]; leaked {
			t.Fatalf("Fortran type-spec variable was overclaimed as a type: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	})

	t.Run("cobol-fixed-comment-and-free-format", func(t *testing.T) {
		fixed := "      * PROGRAM-ID. FAKE.\r\n       IDENTIFICATION DIVISION.\r\n       PROGRAM-ID. REAL.\r\n       PROCEDURE DIVISION.\r\n       MAIN SECTION.\r\n           COPY COMMON.\r\n"
		fixedResult, err := (COBOLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("fixed.cob", fixed), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		fixedNames := symbolsByQualifiedName(fixedResult.Analysis.Symbols)
		if _, ok := fixedNames["REAL"]; !ok {
			t.Fatalf("fixed COBOL missing REAL: %v", sortedSymbolQualifiedNames(fixedResult.Analysis.Symbols))
		}
		if _, leaked := fixedNames["FAKE"]; leaked {
			t.Fatalf("fixed COBOL indicator comment leaked FAKE")
		}

		free := "IDENTIFICATION DIVISION.\nPROGRAM-ID. FREEDEMO.\nPROCEDURE DIVISION.\n*> COPY FAKE.\nMAIN SECTION.\nCOPY COMMON.\n"
		freeResult, err := (COBOLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("free.cbl", free), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := symbolsByQualifiedName(freeResult.Analysis.Symbols)["FREEDEMO"]; !ok {
			t.Fatalf("free-form COBOL missing program: %v", sortedSymbolQualifiedNames(freeResult.Analysis.Symbols))
		}
		if got := dependencyValues(freeResult.Dependencies); !sameStringSet(got, []string{"COMMON"}) {
			t.Fatalf("free COBOL dependencies=%v", got)
		}
	})
}

func TestCOBOLFreeFormDetectionHandlesRealWorldSignals(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "statement-before-division",
			text: "move \"/\" to routing-pattern.\nIDENTIFICATION DIVISION.\nPROGRAM-ID. ROUTES.\n",
			want: "ROUTES",
		},
		{
			name: "compiler-directive-first",
			text: ">>IF GCVERSION >= 32\n>>END-IF\nIDENTIFICATION DIVISION.\nPROGRAM-ID. DECODE.\n",
			want: "DECODE",
		},
		{
			name: "literal-crosses-fixed-margin",
			text: "       IDENTIFICATION DIVISION.\n       PROGRAM-ID. WEBGAME.\n       PROCEDURE DIVISION.\n           CALL \"setElementProperty\" USING \".loading-message\" \"innerHTML\" \"Press Any Key To Start\".\n",
			want: "WEBGAME",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (COBOLAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument(tc.name+".cob", tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("valid free-form COBOL reported partial: %+v", result.Analysis.Diagnostics)
			}
			if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)[tc.want]; !ok {
				t.Fatalf("missing COBOL program %s: %v", tc.want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestRealSourceFortranBareEndClosesTrackedProgramUnits(t *testing.T) {
	text := "module demo\n" +
		"contains\n" +
		"  subroutine first()\n" +
		"  end\n" +
		"  integer function second()\n" +
		"    second = 1\n" +
		"  end function second\n" +
		"end\n"
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.f90", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Fortran bare end reported partial: %+v", result.Analysis.Diagnostics)
	}
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"demo", "demo.first", "demo.second"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Fortran bare end missing %s: %v", want, names)
		}
	}
}

func TestRealSourceFortranSelectTypeGuardsDoNotOpenDerivedTypeScopes(t *testing.T) {
	text := "program main\n" +
		"  class(*), allocatable :: value\n" +
		"  select type (value)\n" +
		"  type is (integer)\n" +
		"    print *, value\n" +
		"  class default\n" +
		"    print *, 0\n" +
		"  end select\n" +
		"contains\n" +
		"  subroutine after()\n" +
		"  end subroutine after\n" +
		"end program main\n"
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("main.f90", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Fortran select type guard lowered coverage: %+v", result.Analysis.Diagnostics)
	}
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"main", "main.after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Fortran select type guard missing %s: %v", want, names)
		}
	}
	if containsSortedString(names, "main.integer") {
		t.Fatalf("Fortran type guard leaked derived type: %v", names)
	}
}

func TestDynamicAndFunctionalBoundariesStayConservative(t *testing.T) {
	t.Run("matlab-transpose-and-control-scopes", func(t *testing.T) {
		text := "classdef Worker\n  methods\n    function out = first(obj, data)\n      value = data';\n      if value\n        out = value;\n      end\n    end\n    function out = second(obj, x)\n      out = x;\n    end\n  end\nend\n"
		result, err := (MATLABAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("Worker.m", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete {
			t.Fatalf("valid MATLAB transpose/control source lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, want := range []string{"Worker", "Worker.first", "Worker.second"} {
			if _, ok := byName[want]; !ok {
				t.Fatalf("MATLAB scope missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	})

	t.Run("haskell-qualified-import-and-equation", func(t *testing.T) {
		text := "module Demo where\nimport qualified Data.Map as M\ndata Worker = Worker Int\nrun (Worker x) = x\nanswer = 42\npromoted = 'Worker\n"
		result, err := (HaskellAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("Demo.hs", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete {
			t.Fatalf("valid Haskell promoted constructor lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		if got := dependencyValues(result.Dependencies); !sameStringSet(got, []string{"Data.Map"}) {
			t.Fatalf("Haskell dependencies=%v", got)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if symbol, ok := byName["Demo.run"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Haskell equation function=%+v exists=%v", symbol, ok)
		}
		if symbol, ok := byName["Demo.answer"]; !ok || symbol.Kind != SymbolKindVariable {
			t.Fatalf("Haskell value binding=%+v exists=%v", symbol, ok)
		}
	})

	t.Run("ocaml-value-versus-function", func(t *testing.T) {
		text := "module Demo = struct\n  let answer = 42\n  let run x = x\nend\n"
		result, err := (OCamlAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.ml", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if symbol, ok := byName["Demo.answer"]; !ok || symbol.Kind != SymbolKindVariable {
			t.Fatalf("OCaml value binding=%+v exists=%v", symbol, ok)
		}
		if symbol, ok := byName["Demo.run"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("OCaml function binding=%+v exists=%v", symbol, ok)
		}
	})

	t.Run("ocaml-character-literals-preserve-type-variables", func(t *testing.T) {
		text := "let classify c =\n" +
			"  if c = '0' || c = '9' || c = ' ' || c = '~' || c = '\"' || c = '\\\\' || c = '\\'' then c else c\n" +
			"let id (x : 'a) = x\n" +
			"let after = 1\n"
		result, err := (OCamlAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("chars.ml", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid OCaml character literals lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		for _, name := range []string{"classify", "id", "after"} {
			if _, ok := byName[name]; !ok {
				t.Fatalf("OCaml declaration %q missing after character literals/type variable: %v", name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		}
	})

	t.Run("julia-compact-function", func(t *testing.T) {
		text := "module Demo\ncompact(x) = x\nadjoint = matrix'\nmacro tagged(ex)\n  ex\nend\nend\n"
		result, err := (JuliaAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.jl", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete {
			t.Fatalf("valid Julia adjoint source lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		byName := symbolsByQualifiedName(result.Analysis.Symbols)
		if symbol, ok := byName["Demo.compact"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Julia compact function=%+v exists=%v", symbol, ok)
		}
		if symbol, ok := byName["Demo.tagged"]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("Julia macro declaration=%+v exists=%v", symbol, ok)
		}
	})
}

func TestMATLABLikeInlineControlAndImplicitFunctionEnd(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "function out = run(x)\n" +
				"  if isempty(x), out = 0; return; end\n" +
				"  out = x;\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("run.m", text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s inline control/implicit function end reported partial: %+v", tc.name, result.Analysis.Diagnostics)
			}
			if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["run"]; !ok || symbol.Kind != SymbolKindFunction {
				t.Fatalf("%s implicit-end function=%+v exists=%v symbols=%v", tc.name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestMATLABLikeOneLineFunctionsCloseTheirOwnScope(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "classdef Worker\n" +
				"  methods\n" +
				"    function out = first(obj); out = obj; end\n" +
				"    function out = second(obj); if obj, out = obj; else, out = []; end; end\n" +
				"  end\n" +
				"end\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("Worker.m", text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s one-line functions reported partial: %+v", tc.name, result.Analysis.Diagnostics)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []string{"Worker", "Worker.first", "Worker.second"} {
				if _, ok := byName[want]; !ok {
					t.Fatalf("%s one-line function scope missing %s: %v", tc.name, want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if _, leaked := byName["Worker.first.second"]; leaked {
				t.Fatalf("%s one-line function left a phantom nested scope: %v", tc.name, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestMATLABLikeBlockCommentDelimitersRequireDedicatedLines(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			documented := "function out = documented(x)\n" +
				"%{previous} is ordinary documentation text\n" +
				"  value = '[)';\n" +
				"  out = x;\n" +
				"end\n"
			documentedResult, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("documented.m", documented), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !documentedResult.Analysis.CoverageComplete || documentedResult.Analysis.Truncated {
				t.Fatalf("valid %s documentation comment reported partial: %+v", tc.name, documentedResult.Analysis.Diagnostics)
			}
			if !containsSortedString(sortedSymbolQualifiedNames(documentedResult.Analysis.Symbols), "documented") {
				t.Fatalf("%s declaration missing after documentation comment: %v", tc.name, sortedSymbolQualifiedNames(documentedResult.Analysis.Symbols))
			}

			block := "function out = blocked(x)\n" +
				"  %{\n" +
				"  function fake()\n" +
				"  %}\n" +
				"  out = x;\n" +
				"end\n"
			blockResult, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("blocked.m", block), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !blockResult.Analysis.CoverageComplete || blockResult.Analysis.Truncated {
				t.Fatalf("valid %s dedicated-line block comment reported partial: %+v", tc.name, blockResult.Analysis.Diagnostics)
			}
			names := sortedSymbolQualifiedNames(blockResult.Analysis.Symbols)
			if !containsSortedString(names, "blocked") {
				t.Fatalf("%s declaration missing around block comment: %v", tc.name, names)
			}
			if containsSortedString(names, "blocked.fake") || containsSortedString(names, "fake") {
				t.Fatalf("%s block-comment contents leaked declarations: %v", tc.name, names)
			}

			broken := "function out = broken(x)\n" +
				"  %{\n" +
				"  out = x;\n" +
				"end\n"
			partial, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("broken.m", broken), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if partial.Analysis.CoverageComplete || !hasAnalysisDiagnostic(partial.Analysis.Diagnostics, tc.name+"-unterminated-comment") {
				t.Fatalf("unterminated %s dedicated-line block comment was accepted: %+v", tc.name, partial.Analysis)
			}
		})
	}
}

func TestMATLABLikeImplicitSubfunctionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "function out = first(x)\n" +
				"  if x, out = x; end\n" +
				"function out = second(x)\n" +
				"  out = x + 1;\n" +
				"function out = third(x)\n" +
				"  out = x + 2;\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("subfunctions.m", text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s implicit subfunction boundaries reported partial: %+v", tc.name, result.Analysis.Diagnostics)
			}
			for _, want := range []string{"first", "second", "third"} {
				if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), want) {
					t.Fatalf("%s implicit subfunction %q missing: %v", tc.name, want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
		})
	}
}

func TestMATLABLikeCaseInlineControlPreservesImplicitSubfunctionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "function out = first(x)\n" +
				"  switch x\n" +
				"    case 1, if x > 0\n" +
				"      out = x;\n" +
				"    end\n" +
				"    case 2, if x > 1\n" +
				"      out = x + 1;\n" +
				"    end\n" +
				"  end\n" +
				"function out = second(x)\n" +
				"  out = x + 1;\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("case-inline-control.m", text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s case-inline control reported partial: %+v", tc.name, result.Analysis.Diagnostics)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			for _, want := range []string{"first", "second"} {
				if !containsSortedString(names, want) {
					t.Fatalf("%s implicit subfunction %q missing after case-inline control: %v", tc.name, want, names)
				}
			}
			if containsSortedString(names, "first.second") {
				t.Fatalf("%s case-inline control left a phantom function scope: %v", tc.name, names)
			}
		})
	}
}

func TestMATLABLikeLogicalLineContinuationPreservesNestedControlScopes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "function out = first(x)\n" +
				"  if x, ...\n" +
				"    if x > 1\n" +
				"      out = x;\n" +
				"    end\n" +
				"  end\n" +
				"function out = second(x)\n" +
				"  out = x + 1;\n" +
				"function out = third(x)\n" +
				"  out = x + 2;\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("continued-nested-control.m", text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s continued nested control reported partial: %+v", tc.name, result.Analysis.Diagnostics)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			for _, want := range []string{"first", "second", "third"} {
				if !containsSortedString(names, want) {
					t.Fatalf("%s implicit subfunction %q missing after continued nested control: %v", tc.name, want, names)
				}
			}
			if containsSortedString(names, "first.second") || containsSortedString(names, "second.third") {
				t.Fatalf("%s continued nested control left a phantom function scope: %v", tc.name, names)
			}
		})
	}
}

func TestMATLABLikeMethodsAndPropertiesAssignmentsDoNotOpenScopes(t *testing.T) {
	assignments := []struct {
		name       string
		assignment string
	}{
		{name: "methods", assignment: "  methods = {'WLS', 'OLS'};\n"},
		{name: "properties-indexed", assignment: "  properties(1) = x;\n"},
	}
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		for _, assignment := range assignments {
			t.Run(tc.name+"/"+assignment.name, func(t *testing.T) {
				text := "function out = run(x)\n" +
					assignment.assignment +
					"  if x\n" +
					"    out = x;\n" +
					"  else\n" +
					"    out = 0;\n" +
					"  end\n"
				result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("keyword-assignment.m", text), testAnalyzeOptions(false, 64))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
					t.Fatalf("valid %s %s assignment reported partial: %+v", tc.name, assignment.name, result.Analysis.Diagnostics)
				}
				if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "run") {
					t.Fatalf("%s %s assignment lost function symbol: %v", tc.name, assignment.name, names)
				}
			})
		}
	}
}

func TestMATLABLikeAttributedMethodsAndPropertiesRemainBlockHeaders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "classdef Worker\n" +
				"  properties (SetAccess = private)\n" +
				"    value\n" +
				"  end\n" +
				"  methods (Static = true)\n" +
				"    function out = run(x)\n" +
				"      out = x;\n" +
				"    end\n" +
				"  end\n" +
				"end\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("Worker.m", text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s attributed class blocks reported partial: %+v", tc.name, result.Analysis.Diagnostics)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			if symbol, ok := byName["Worker.run"]; !ok || symbol.Kind != SymbolKindMethod {
				t.Fatalf("%s attributed methods block lost method hierarchy: %+v exists=%v symbols=%v", tc.name, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
			}
		})
	}
}

func TestMATLABExplicitNestedFunctionRemainsStructured(t *testing.T) {
	text := "function out = outer(x)\n" +
		"  function y = inner(z)\n" +
		"    y = z;\n" +
		"  end\n" +
		"  out = inner(x);\n" +
		"end\n"
	result, err := (MATLABAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("nested.m", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid MATLAB nested function reported partial: %+v", result.Analysis.Diagnostics)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	if symbol, ok := byName["outer"]; !ok || symbol.Kind != SymbolKindFunction {
		t.Fatalf("MATLAB outer function=%+v exists=%v symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if symbol, ok := byName["outer.inner"]; !ok || symbol.Kind != SymbolKindMethod {
		t.Fatalf("MATLAB nested function=%+v exists=%v symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestMATLABBranchLineTerminatorClosesScope(t *testing.T) {
	text := "function out = first(x)\n" +
		"  if x\n" +
		"    out = 1;\n" +
		"  else, out = 0; end\n" +
		"end\n" +
		"function out = second(x)\n" +
		"  out = x;\n" +
		"end\n"
	result, err := (MATLABAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("branches.m", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid MATLAB branch-line terminator reported partial: %+v", result.Analysis.Diagnostics)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, want := range []string{"first", "second"} {
		if symbol, ok := byName[want]; !ok || symbol.Kind != SymbolKindFunction {
			t.Fatalf("MATLAB branch-line terminator missing top-level %s: %+v exists=%v symbols=%v", want, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	if _, leaked := byName["first.second"]; leaked {
		t.Fatalf("MATLAB branch-line terminator nested second function: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestMATLABBranchInlineScopeDoesNotCloseOuterScope(t *testing.T) {
	text := "function out = run(x)\n" +
		"  switch x\n" +
		"    case 1, if x, out = 1; end\n" +
		"    otherwise, out = 0;\n" +
		"  end\n" +
		"end\n"
	result, err := (MATLABAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("branch-inline.m", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid MATLAB branch-inline scope reported partial: %+v", result.Analysis.Diagnostics)
	}
	if symbol, ok := symbolsByQualifiedName(result.Analysis.Symbols)["run"]; !ok || symbol.Kind != SymbolKindFunction {
		t.Fatalf("MATLAB branch-inline function=%+v exists=%v symbols=%v", symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestMATLABArgumentsBlockClosesBeforeFunction(t *testing.T) {
	text := "function out = run(x)\n" +
		"  arguments\n" +
		"    x (1,1) double = 1\n" +
		"  end\n" +
		"  for i = 1:2, out = x + i; end\n" +
		"end\n"
	result, err := (MATLABAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("run.m", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid MATLAB arguments block reported partial: %+v", result.Analysis.Diagnostics)
	}
}

func TestMATLABLikeUnmatchedEndRemainsIncomplete(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{name: "matlab", analyzer: MATLABAnalyzer{}},
		{name: "octave", analyzer: OctaveAnalyzer{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "function run()\nend\nend\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("unmatched.m", text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("unmatched %s end reported complete: %+v", tc.name, result.Analysis)
			}
		})
	}
}

func TestMATLABMixedFunctionEndStyleRemainsIncomplete(t *testing.T) {
	for _, text := range []string{
		"function first()\nend\nfunction second()\n",
		"function first()\nfunction second()\nend\n",
	} {
		result, err := (MATLABAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("mixed.m", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
			t.Fatalf("mixed explicit/implicit MATLAB function endings reported complete: %+v", result.Analysis)
		}
	}
}

func TestMATLABLikeCharacterVectorsStayOpaqueWithoutHidingTranspose(t *testing.T) {
	for _, tc := range []struct {
		name     string
		path     string
		analyzer SourceAnalyzer
		end      string
	}{
		{name: "matlab", path: "demo.m", analyzer: MATLABAnalyzer{}, end: "end"},
		{name: "octave", path: "demo.m", analyzer: OctaveAnalyzer{}, end: "endfunction"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "function out = first(data)\n" +
				"  pattern = '[)';\n" +
				"  message = 'value(]';\n" +
				"  out = data';\n" +
				tc.end + "\n" +
				"function out = after(value)\n" +
				"  out = value;\n" +
				tc.end + "\n"
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument(tc.path, text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("valid %s character vectors/transpose reported partial: %+v", tc.name, result.Analysis.Diagnostics)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			for _, want := range []string{"first", "after"} {
				if !containsSortedString(names, want) {
					t.Fatalf("%s declaration %q missing after character vectors: %v", tc.name, want, names)
				}
			}

			brokenText := "function broken()\n  value = 'unterminated\n" + tc.end + "\n"
			broken, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument(tc.path, brokenText), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if broken.Analysis.CoverageComplete {
				t.Fatalf("unterminated %s character vector was accepted: %+v", tc.name, broken.Analysis)
			}
		})
	}
}

func TestRBacktickIdentifiersKeepDelimiterNamesOpaque(t *testing.T) {
	text := "extract <- function(items) {\n" +
		"  lapply(items, `[[`, \"parsed\")\n" +
		"}\n" +
		"after <- function() 1\n"
	result, err := (RAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.R", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid R backtick identifier reported partial: %+v", result.Analysis.Diagnostics)
	}
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"extract", "after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("R declaration %q missing after backtick identifier: %v", want, names)
		}
	}

	broken, err := (RAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("broken.R", "value <- `unterminated\n"), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if broken.Analysis.CoverageComplete {
		t.Fatalf("unterminated R backtick identifier was accepted: %+v", broken.Analysis)
	}
}

func TestRMultilineStringsRemainOpaque(t *testing.T) {
	text := "before <- function() {\n" +
		"  value <- 'first line\n" +
		"second line'\n" +
		"  other <- \"third line\n" +
		"fourth line\"\n" +
		"}\n" +
		"after <- function() 1\n"
	result, err := (RAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("multiline.R", text), testAnalyzeOptions(false, 64))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid R multiline strings reported partial: %+v", result.Analysis.Diagnostics)
	}
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"before", "after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("R declaration %q missing around multiline strings: %v", want, names)
		}
	}
}

func TestLispReaderFormsDoNotLeakDeclarations(t *testing.T) {
	t.Run("common-lisp-reader-character-delimiters", func(t *testing.T) {
		text := `(defpackage :demo)
(in-package :demo)
(defun Real () (list #\) #\} #\Space))
(defun After () nil)
`
		result, err := (CommonLispAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("reader.lisp", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Common Lisp reader characters reported partial: %+v", result.Analysis.Diagnostics)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, want := range []string{"demo.Real", "demo.After"} {
			if !containsSortedString(names, want) {
				t.Fatalf("Common Lisp declaration %q missing after reader character: %v", want, names)
			}
		}
	})

	t.Run("common-lisp-block-comment-and-quote", func(t *testing.T) {
		text := "#| (defun BlockFake () nil) |#\n'(defun QuotedFake () nil)\n(defpackage :demo)\n(in-package :demo)\n(defun Real () nil)\n"
		result, err := (CommonLispAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.lisp", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, fake := range []string{"BlockFake", "QuotedFake", "demo.BlockFake", "demo.QuotedFake"} {
			if containsSortedString(names, fake) {
				t.Fatalf("Common Lisp reader/comment leaked %s: %v", fake, names)
			}
		}
		if !containsSortedString(names, "demo.Real") {
			t.Fatalf("Common Lisp missing real declaration: %v", names)
		}
	})

	t.Run("clojure-discard-and-quote", func(t *testing.T) {
		text := "(ns demo.core)\n#_(defn DiscardedFake [] nil)\n'(defn QuotedFake [] nil)\n(defn real [] nil)\n"
		result, err := (ClojureAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.clj", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, fake := range []string{"demo.core.DiscardedFake", "demo.core.QuotedFake"} {
			if containsSortedString(names, fake) {
				t.Fatalf("Clojure reader form leaked %s: %v", fake, names)
			}
		}
		if !containsSortedString(names, "demo.core.real") {
			t.Fatalf("Clojure missing real declaration: %v", names)
		}
	})
}

func TestUnclosedStructuralScopesLowerCoverage(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		path     string
		text     string
	}{
		{"fortran-module", FortranAnalyzer{}, "demo.f90", "module Demo\ncontains\nsubroutine run()\nend subroutine run\n"},
		{"ada-package", AdaAnalyzer{}, "demo.ads", "package Demo is\n  procedure Run;\n"},
		{"matlab-class", MATLABAnalyzer{}, "Worker.m", "classdef Worker\n  methods\n    function run(obj)\n    end\n"},
		{"octave-if", OctaveAnalyzer{}, "demo.m", "function run()\n  if true\n    x = 1;\n"},
		{"julia-module", JuliaAnalyzer{}, "demo.jl", "module Demo\nfunction run()\nend\n"},
		{"ocaml-module", OCamlAnalyzer{}, "demo.ml", "module Demo = struct\n  let run x = x\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument(tc.path, tc.text), testAnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("unclosed %s scope reported complete: %+v", tc.name, result.Analysis)
			}
		})
	}
}
func TestScientificLegacyFunctionalCancellationAndSymbolLimits(t *testing.T) {
	for _, analyzer := range scientificLegacyFunctionalAnalyzers() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := analyzer.Analyze(ctx, scientificLegacyFunctionalTestDocument("cancel.fixture", "module Demo\n"), testAnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}

	for _, tc := range []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"fortran", FortranAnalyzer{}, generatedFortran(1200)},
		{"cobol", COBOLAnalyzer{}, generatedCOBOL(1200)},
		{"ada", AdaAnalyzer{}, generatedAda(1200)},
		{"matlab", MATLABAnalyzer{}, generatedMATLAB(1200)},
		{"octave", OctaveAnalyzer{}, generatedOctave(1200)},
		{"julia", JuliaAnalyzer{}, generatedJulia(1200)},
		{"r", RAnalyzer{}, generatedR(1200)},
		{"haskell", HaskellAnalyzer{}, generatedHaskell(1200)},
		{"ocaml", OCamlAnalyzer{}, generatedOCaml(1200)},
		{"common-lisp", CommonLispAnalyzer{}, generatedCommonLisp(1200)},
		{"clojure", ClojureAnalyzer{}, generatedClojure(1200)},
		{"emacs-lisp", EmacsLispAnalyzer{}, generatedEmacsLisp(1200)},
	} {
		t.Run("limit-"+tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), scientificLegacyFunctionalTestDocument("limit.fixture", tc.text), testAnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result symbols=%d truncated=%v complete=%v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete)
			}
		})
	}
}

func scientificLegacyFunctionalTestDocument(path, text string) *SourceDocument {
	return &SourceDocument{Path: path, Text: text, Encoding: "utf-8", lineStarts: buildLineStarts(text)}
}

func scientificLegacyFunctionalAnalyzers() []SourceAnalyzer {
	return []SourceAnalyzer{FortranAnalyzer{}, COBOLAnalyzer{}, AdaAnalyzer{}, MATLABAnalyzer{}, OctaveAnalyzer{}, JuliaAnalyzer{}, RAnalyzer{}, HaskellAnalyzer{}, OCamlAnalyzer{}, CommonLispAnalyzer{}, ClojureAnalyzer{}, EmacsLispAnalyzer{}}
}

func generatedFortran(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "subroutine f%04d()\nend subroutine f%04d\n", i, i)
	}
	return b.String()
}

func generatedCOBOL(count int) string {
	var b strings.Builder
	fmt.Fprintln(&b, "       IDENTIFICATION DIVISION.")
	fmt.Fprintln(&b, "       PROGRAM-ID. DEMO.")
	fmt.Fprintln(&b, "       PROCEDURE DIVISION.")
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "       S%04d SECTION.\n", i)
	}
	return b.String()
}

func generatedAda(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "procedure F%04d;\n", i)
	}
	return b.String()
}

func generatedMATLAB(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d()\nend\n", i)
	}
	return b.String()
}

func generatedOctave(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d()\nendfunction\n", i)
	}
	return b.String()
}

func generatedJulia(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d()\nend\n", i)
	}
	return b.String()
}

func generatedR(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "f%04d <- function() { 1 }\n", i)
	}
	return b.String()
}

func generatedHaskell(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "f%04d :: Int -> Int\n", i)
	}
	return b.String()
}

func generatedOCaml(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "let f%04d x = x\n", i)
	}
	return b.String()
}

func generatedCommonLisp(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "(defun f%04d () nil)\n", i)
	}
	return b.String()
}

func generatedClojure(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "(defn f%04d [] nil)\n", i)
	}
	return b.String()
}

func generatedEmacsLisp(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "(defun f%04d () nil)\n", i)
	}
	return b.String()
}
