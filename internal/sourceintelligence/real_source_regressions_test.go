package sourceintelligence

import (
	"context"
	"testing"
)

func requireRealSourceComplete(t *testing.T, analyzer SourceAnalyzer, text string) AnalyzerResult {
	t.Helper()
	result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("%s valid source reported partial: %+v", analyzer.Language(), result.Analysis)
	}
	return result
}

func TestRealSourceElixirAnonymousFnAndMultilineStrings(t *testing.T) {
	text := "defmodule M do\n" +
		"  @doc \"line one\nline two\"\n" +
		"  Enum.reduce([1], 0, fn x, acc ->\n" +
		"    x + acc\n" +
		"  end)\n" +
		"  def map(xs) do\n" +
		"    Enum.map(xs, fn x -> x end)\n" +
		"  end\n" +
		"end\n" +
		"defprotocol P do\n  def run(x)\nend\n" +
		"defimpl P, for: Atom do\nend\n"
	requireRealSourceComplete(t, ElixirAnalyzer{}, text)
}

func TestRealSourceElixirSigilsDoNotLeakDelimiters(t *testing.T) {
	text := "~w(hello #{ [\"has\", []] } world)s\n" +
		"~s{Escapes \\{ and \\}, with {balancing} # text }\n" +
		"~R'this + i\\s \"a\" regex'\n" +
		"defmodule M do\nend\n"
	requireRealSourceComplete(t, ElixirAnalyzer{}, text)
}

func TestRealSourceElixirNestedStringInterpolationStaysOpaque(t *testing.T) {
	text := `defmodule Demo do
  def headers(email, api_token) do
    [
      {"Accept", "application/json"},
      {"Authorization", "Basic #{Base.encode64("#{email}:#{api_token}")}"}
    ]
  end
end
`
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "Demo.headers") {
		t.Fatalf("nested Elixir interpolation lost function: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceElixirSigilDelimiterInsideInterpolationStaysOpaque(t *testing.T) {
	text := `defmodule Demo do
  def route(opts) do
    ~s|live "/item/:#{opts[:primary_key] || :id}"|
  end
end
`
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "Demo.route") {
		t.Fatalf("interpolated Elixir sigil lost function: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceElixirCharacterLiteralsDoNotLeakDelimiters(t *testing.T) {
	text := `defmodule Demo do
  def split(<<"\"", rest::binary>>, prev) when prev in [?., ?(, ?,] do
    {rest, ?)}
  end
end
`
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "Demo.split") {
		t.Fatalf("Elixir character literals lost function: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceElixirStructuralTokensHandleMultilineDoAndKeywordAtoms(t *testing.T) {
	text := `defmodule Demo do
  def run(value) do
    with x <- value,
         y <- (cond do
           true -> x
         end) do
      y
    end
  end

  def keywords do
    [
      :fn,
      :case,
      :with
    ]
  end
end
`
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo.run", "Demo.keywords"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Elixir structural token regression missing %s: %v", want, names)
		}
	}
}

func TestRealSourceElixirStructuralFnAllowsSpacedColonNeighbors(t *testing.T) {
	text := `defmodule Demo do
  def callbacks do
    %{fun: fn value -> value end}
  end

  def validate(changeset) do
    validate_change(changeset, :field, fn :field, value ->
      value
    end)
  end

  def keywords do
    [:fn, fn: :value]
  end
end
`
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.callbacks", "Demo.validate", "Demo.keywords"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Elixir colon adjacency regression missing %s: %v", want, names)
		}
	}
}

func TestRealSourceElixirMultilineKeywordBodyClearsPendingDeclaration(t *testing.T) {
	text := `defmodule Demo do
  def direct(
        value
      ),
      do: value

  def guarded(
        value
      )
      when is_integer(value),
      do: value
end
`
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.direct", "Demo.guarded"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Elixir multiline keyword body missing %s: %v", want, names)
		}
	}
}

func TestRealSourceElixirGuardContinuationRetainsStructuralScopes(t *testing.T) {
	text := "defmodule Demo do\n" +
		"  def eval(source, opts)\n" +
		"      when is_binary(source) and is_list(opts) do\n" +
		"    source\n" +
		"  end\n" +
		"  def next(), do: :ok\n" +
		"end\n"
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.eval", "Demo.next"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Elixir guard continuation missing %s: %v", want, names)
		}
	}
}

func TestRealSourceElixirPipedCaseRetainsStructuralScopes(t *testing.T) {
	text := "defmodule Demo do\n" +
		"  def render(value) do\n" +
		"    value\n" +
		"    |> case do\n" +
		"      nil -> :none\n" +
		"      _ -> :some\n" +
		"    end\n" +
		"  end\n" +
		"  def next(), do: :ok\n" +
		"end\n"
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.render", "Demo.next"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Elixir piped case missing %s: %v", want, names)
		}
	}
}

func TestRealSourceElixirCustomMacroDoBlocksRetainStructuralScopes(t *testing.T) {
	text := "defmodule Demo do\n" +
		"  typedstruct enforce: true do\n" +
		"    field(:id, integer())\n" +
		"  end\n" +
		"  deffilter Filter, id: integer() do\n" +
		"    _ -> true\n" +
		"  end\n" +
		"  def run(value) do\n" +
		"    marker = :do\n" +
		"    if value, do: marker\n" +
		"  end\n" +
		"end\n"
	result := requireRealSourceComplete(t, ElixirAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.run"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Elixir custom macro block missing %s: %v", want, names)
		}
	}
}

func TestRealSourceFortranModuleSubprogramsRetainStructuralScopes(t *testing.T) {
	text := "submodule(parent_mod) child_mod\n" +
		"contains\n" +
		"  module subroutine run()\n" +
		"  end subroutine run\n" +
		"  module function value() result(x)\n" +
		"    integer :: x\n" +
		"    x = 1\n" +
		"  end function value\n" +
		"end submodule child_mod\n"
	result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"child_mod", "child_mod.run", "child_mod.value"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Fortran module subprogram missing %s: %v", want, names)
		}
	}
}

func TestRealSourceFortranCompactEndKeywordsCloseStructuralScopes(t *testing.T) {
	text := "module demo\n" +
		"  type item\n" +
		"    integer :: field\n" +
		"  endtype item\n" +
		"contains\n" +
		"  subroutine run()\n" +
		"  endsubroutine run\n" +
		"  function value() result(x)\n" +
		"    integer :: x\n" +
		"    x = 1\n" +
		"  endfunction value\n" +
		"endmodule demo\n"
	result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"demo", "demo.item", "demo.run", "demo.value"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Fortran compact end keyword missing %s: %v", want, names)
		}
	}
}

func TestRealSourceFortranTypeNamedVariableAssignmentDoesNotOpenDerivedType(t *testing.T) {
	text := "module demo\n" +
		"contains\n" +
		"  subroutine create(type_tab)\n" +
		"    integer :: ii, type\n" +
		"    type = type_tab(ii)\n" +
		"  end subroutine create\n" +
		"end module demo\n"
	result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	if !containsSortedString(names, "demo.create") {
		t.Fatalf("Fortran type-named variable source missing subroutine: %v", names)
	}
	if containsSortedString(names, "demo.create.ii") {
		t.Fatalf("Fortran type-named variable assignment leaked a derived type: %v", names)
	}
}

func TestRealSourceFortranMemberNamedFunctionDoesNotOpenProcedure(t *testing.T) {
	text := "module demo\n" +
		"contains\n" +
		"  subroutine run()\n" +
		"    call consume(state%function, full=.true.)\n" +
		"  end subroutine run\n" +
		"end module demo\n"
	result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	if !containsSortedString(names, "demo.run") {
		t.Fatalf("Fortran member-named function source missing subroutine: %v", names)
	}
	if containsSortedString(names, "demo.run.full") {
		t.Fatalf("Fortran member named function leaked a procedure: %v", names)
	}
}

func TestRealSourceFortranFixedFormCPPDirectivesStayOpaque(t *testing.T) {
	text := "#define IMPLICIT_STATEMENT IMPLICIT INTEGER(4) (I-N), REAL(4) (A-H, O-Z)\n" +
		"#define IFAC_TYPE REAL(4)\n" +
		"      PROGRAM demo\n" +
		"      IMPLICIT_STATEMENT\n" +
		"      END\n"
	document := scientificLegacyFunctionalTestDocument("demo.f", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("fixed-form CPP directives lowered coverage: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo") {
		t.Fatalf("fixed-form CPP source missing program: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranFyppCallContinuationStaysOpaque(t *testing.T) {
	text := "module demo\n" +
		"  #:call hash_map(prefix='demo', &\n" +
		"     key_type='INTEGER, DIMENSION(2)', &\n" +
		"     value_type='INTEGER', &\n" +
		"     value_default_init=' = 0')\n" +
		"  #:endcall hash_map\n" +
		"contains\n" +
		"  subroutine run()\n" +
		"  end subroutine run\n" +
		"end module demo\n"
	document := scientificLegacyFunctionalTestDocument("demo.F", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("FYPP call continuation lowered coverage: %+v", result.Analysis)
	}
	for _, want := range []string{"demo", "demo.run"} {
		if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), want) {
			t.Fatalf("FYPP source missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestRealSourceFortranInlineFyppMarkersPreserveContinuation(t *testing.T) {
	text := "module demo\n" +
		"contains\n" +
		"  subroutine run()\n" +
		"    value = (#{if first}#one#{endif}# &\n" +
		"             #{if second}# + two &#{endif}#\n" +
		"             #{if third}# + three#{endif}#)\n" +
		"  end subroutine run\n" +
		"end module demo\n"
	document := scientificLegacyFunctionalTestDocument("demo.F", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("inline FYPP markers interrupted continuation: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo.run") {
		t.Fatalf("inline FYPP source missing run: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranFixedVendorDirectiveContinuationStaysOpaque(t *testing.T) {
	text := "      PROGRAM demo\n" +
		"!dir$ omp offload target(mic:0)\n" +
		"& in (buffer:length(n), align(512))\n" +
		"      END\n"
	document := scientificLegacyFunctionalTestDocument("demo.f", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("fixed-form vendor directive continuation lowered coverage: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo") {
		t.Fatalf("fixed-form vendor directive source missing program: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranFixedFormBangInLabelColumnsIsComment(t *testing.T) {
	text := "      PROGRAM demo\n" +
		"    !        CALL fake(\n" +
		"    ! 1           value)\n" +
		"      END\n"
	document := scientificLegacyFunctionalTestDocument("demo.f", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("fixed-form bang comment in label columns lowered coverage: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo") {
		t.Fatalf("fixed-form label-column bang comment lost program: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranFixedFormCommentOnlyCodeLineDoesNotBreakContinuation(t *testing.T) {
	text := "      PROGRAM demo\n" +
		"      CALL worker(\n" +
		"            ! explanation between statement lines\n" +
		"     1      first, second)\n" +
		"      END\n"
	document := scientificLegacyFunctionalTestDocument("demo.f", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("fixed-form comment-only code line broke continuation: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo") {
		t.Fatalf("fixed-form comment-only line lost program: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranFixedFormInlineCommentKeepsContinuation(t *testing.T) {
	text := "      PROGRAM demo\n" +
		"      IF (I .EQ. 1  ! inline explanation\n" +
		"     .    .AND. J .EQ. 2) THEN\n" +
		"      ENDIF\n" +
		"      END\n"
	document := scientificLegacyFunctionalTestDocument("demo.f", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("fixed-form inline comment consumed continuation: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo") {
		t.Fatalf("fixed-form inline-comment source missing program: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranFixedFormHollerithPayloadStaysOpaque(t *testing.T) {
	text := "      PROGRAM demo\n" +
		" 1000 FORMAT(\n" +
		"     & 5X,8HABC'D)E?,I3/)\n" +
		"      END\n"
	document := scientificLegacyFunctionalTestDocument("demo.f", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("fixed-form Hollerith payload leaked lexical delimiters: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo") {
		t.Fatalf("fixed-form Hollerith source missing program: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranUppercaseFCanContainFreeFormSource(t *testing.T) {
	text := "module demo\n" +
		"  use dependency, only: first, &\n" +
		"                        second\n" +
		"contains\n" +
		"  subroutine run()\n" +
		"  end subroutine run\n" +
		"end module demo\n"
	document := scientificLegacyFunctionalTestDocument("demo.F", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid free-form .F source reported partial: %+v", result.Analysis)
	}
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"demo", "demo.run"} {
		if !containsSortedString(names, want) {
			t.Fatalf("free-form .F source missing %s: %v", want, names)
		}
	}
}

func TestRealSourceFortranFixedFormExtendedColumnsAndTabs(t *testing.T) {
	text := "      PROGRAM demo\n" +
		"      CALL worker(\n" +
		"     1                     IFLAG  ,NEL    ,PMIN   ,OFF    ,EINT   ,MU  ,MU2,\n" +
		"     2                     ESPE   ,DVOL   ,DF     ,VNEW   ,PSH    ,\n" +
		"     3                     PNEW   ,DPDM   ,DPDE   ,VAREOS ,NVAREOS,MAT_PARAM%EOS)\n" +
		"      DO j=1,1\n" +
		"        DO i=1,1\n" +
		"\t   Y(i+j) = 0.0D0\n" +
		"\tEND DO\n" +
		"\tY(j+j) = 1.0D0\n" +
		"      END DO\n" +
		"      END\n"
	document := scientificLegacyFunctionalTestDocument("demo.f", text)
	result, err := (FortranAnalyzer{}).Analyze(context.Background(), document, testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("valid extended/tab fixed-form source reported partial: %+v", result.Analysis)
	}
	if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), "demo") {
		t.Fatalf("fixed-form program symbol missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
}

func TestRealSourceFortranCPPDirectiveStringFragmentsStayOpaque(t *testing.T) {
	text := "module release\n" +
		"#ifdef __GFORTRAN__\n" +
		"#  define STRINGIFY_START(X) \"&\n" +
		"#  define STRINGIFY_END(X) &X\"\n" +
		"#else\n" +
		"#  define STRINGIFY_START(X) &\n" +
		"#  define STRINGIFY_END(X) X\n" +
		"#endif\n" +
		"contains\n" +
		"  function version() result(value)\n" +
		"    integer :: value\n" +
		"    value = 1\n" +
		"  end function version\n" +
		"end module release\n"
	result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"release", "release.version"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Fortran CPP directive regression missing %s: %v", want, names)
		}
	}
}

func TestRealSourceFortranCPPContinuedMacroStaysOpaque(t *testing.T) {
	text := "module demo\n" +
		"#define VERSION_CHECK(MAJOR, MINOR) ((MAJOR) && \\\n" +
		"  ((MAJOR) * 100 + (MINOR)))\n" +
		"contains\n" +
		"  subroutine run()\n" +
		"  end subroutine run\n" +
		"end module demo\n"
	result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
	for _, want := range []string{"demo", "demo.run"} {
		if !containsSortedString(sortedSymbolQualifiedNames(result.Analysis.Symbols), want) {
			t.Fatalf("Fortran continued CPP macro missing %s: %v", want, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
}

func TestRealSourceFortranConditionalBranchesRemainMutuallyExclusive(t *testing.T) {
	text := "module demo\n" +
		"#ifdef EXTENDED_TYPE\n" +
		"  type, extends(base) :: item\n" +
		"#else\n" +
		"  type :: item\n" +
		"#endif\n" +
		"  end type item\n" +
		"contains\n" +
		"  subroutine run()\n" +
		"#ifdef LEGACY_CALL\n" +
		"    call worker(buffer, &\n" +
		"#else\n" +
		"    call worker(inplace, &\n" +
		"#endif\n" +
		"      value)\n" +
		"  end subroutine run\n" +
		"end module demo\n"
	result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"demo", "demo.item", "demo.run"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Fortran conditional structural union missing %s: %v", want, names)
		}
	}
}

func TestConditionalVariantSelectionPacksIndependentBranches(t *testing.T) {
	groups := make([]conditionalGroup, 40)
	for index := range groups {
		groups[index] = conditionalGroup{
			parentGroup:  -1,
			parentBranch: -1,
			branches:     []conditionalBranch{{}, {}},
		}
	}
	selections, ok := conditionalSelections(groups)
	if !ok {
		t.Fatal("independent conditional branches exceeded the bounded variant budget")
	}
	if len(selections) != 2 {
		t.Fatalf("independent conditional variants=%d want 2", len(selections))
	}
	for groupID := range groups {
		seen := [2]bool{}
		for _, selection := range selections {
			if selection[groupID] >= 0 && selection[groupID] < len(seen) {
				seen[selection[groupID]] = true
			}
		}
		if !seen[0] || !seen[1] {
			t.Fatalf("conditional group %d branch coverage=%v", groupID, seen)
		}
	}
}

func TestRealSourceHaskellCharacterLiteralsDoNotOpenStrings(t *testing.T) {
	text := "module Demo where\n" +
		"quote :: Char -> Bool\n" +
		"quote '\"' = True\n" +
		"quote _ = False\n" +
		"slash :: Char\n" +
		"slash = '\\\\'\n" +
		"prime' = 1\n"
	result := requireRealSourceComplete(t, HaskellAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.quote", "Demo.slash", "Demo.prime'"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Haskell character literal regression missing %s: %v", want, names)
		}
	}
}

func TestRealSourceHaskellQuasiQuotesStayOpaque(t *testing.T) {
	text := "module Demo where\n" +
		"template = [trimming|\n" +
		"module Fake where\n" +
		"data Hidden = Hidden\n" +
		"fake :: Int -> Int\n" +
		"fake x = x\n" +
		"|]\n" +
		"after :: Int -> Int\n" +
		"after x = x\n"
	result := requireRealSourceComplete(t, HaskellAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	if !containsSortedString(names, "Demo.after") {
		t.Fatalf("Haskell declaration after quasiquote missing: %v", names)
	}
	for _, forbidden := range []string{"Fake", "Fake.Hidden", "Fake.fake", "Demo.Hidden", "Demo.fake"} {
		if containsSortedString(names, forbidden) {
			t.Fatalf("Haskell quasiquote leaked %s: %v", forbidden, names)
		}
	}
}

func TestRealSourceJuliaCharacterLiteralsDoNotLeakDelimiters(t *testing.T) {
	text := "module Demo\n" +
		"function quote(quotemark::Char = '\"')\n" +
		"  chars = ['\\\\', '{', '}']\n" +
		"  quotemark in chars\n" +
		"end\n" +
		"adjoint = matrix'\n" +
		"after(x) = x\n" +
		"end\n"
	result := requireRealSourceComplete(t, JuliaAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.quote", "Demo.after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Julia character literal regression missing %s: %v", want, names)
		}
	}
}

func TestRealSourceJuliaInlineEndClosesStructuralScopes(t *testing.T) {
	text := "module Demo\n" +
		"macro tagged(ex) ex end\n" +
		"function return_type end\n" +
		"function after(x)\n" +
		"  if x > 0\n" +
		"    x\n" +
		"  end\n" +
		"end\n" +
		"end\n"
	result := requireRealSourceComplete(t, JuliaAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.tagged", "Demo.return_type", "Demo.after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Julia inline end missing %s: %v", want, names)
		}
	}
}

func TestRealSourceJuliaOrdinaryMultilineStringsRemainOpaque(t *testing.T) {
	text := "module Demo\n" +
		"function work()\n" +
		"  error(\"line one\n" +
		"function Fake() end\n" +
		"line three\")\n" +
		"end\n" +
		"function after() end\n" +
		"end\n"
	result := requireRealSourceComplete(t, JuliaAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.work", "Demo.after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Julia multiline string missing %s: %v", want, names)
		}
	}
	if containsSortedString(names, "Demo.work.Fake") || containsSortedString(names, "Fake") {
		t.Fatalf("Julia multiline string leaked Fake: %v", names)
	}
}

func TestRealSourceJuliaDelimiterContinuedFunctionSignature(t *testing.T) {
	text := "module Demo\n" +
		"function work(\n" +
		"    value,\n" +
		"    other\n" +
		") end\n" +
		"function after() end\n" +
		"end\n"
	result := requireRealSourceComplete(t, JuliaAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Demo", "Demo.work", "Demo.after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Julia delimiter-continued signature missing %s: %v", want, names)
		}
	}
}

func TestRealSourceFortranContinuedCharacterLiteralRequiresLeadingMarker(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		text := "module demo\n" +
			"contains\n" +
			"  subroutine work()\n" +
			"    character(len=*), parameter :: message = 'alpha &\n" +
			"      &function fake() beta'\n" +
			"  end subroutine work\n" +
			"  subroutine after()\n" +
			"  end\n" +
			"end module demo\n"
		result := requireRealSourceComplete(t, FortranAnalyzer{}, text)
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, want := range []string{"demo", "demo.work", "demo.after"} {
			if !containsSortedString(names, want) {
				t.Fatalf("Fortran continued literal missing %s: %v", want, names)
			}
		}
		if containsSortedString(names, "demo.work.fake") || containsSortedString(names, "demo.fake") {
			t.Fatalf("Fortran continued literal leaked fake function: %v", names)
		}
	})

	t.Run("gnu-missing-leading-continuation-marker", func(t *testing.T) {
		text := "module demo\n" +
			"  character(len=*), parameter :: message = 'alpha &\n" +
			"  beta'\n" +
			"end module demo\n"
		requireRealSourceComplete(t, FortranAnalyzer{}, text)
	})

	t.Run("missing-trailing-continuation-marker", func(t *testing.T) {
		text := "module demo\n" +
			"  character(len=*), parameter :: message = 'alpha\n" +
			"  beta'\n" +
			"end module demo\n"
		result, err := (FortranAnalyzer{}).Analyze(context.Background(), scientificLegacyFunctionalTestDocument("demo.f90", text), testAnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete || !hasAnalysisDiagnostic(result.Analysis.Diagnostics, "fortran-unterminated-string") {
			t.Fatalf("invalid Fortran unterminated literal was accepted: %+v", result.Analysis)
		}
	})
}

func TestRealSourceRubyAssignedCaseBlock(t *testing.T) {
	text := "module M\n" +
		"  def run(value)\n" +
		"    result = case value\n" +
		"    when 1 then :one\n" +
		"    else :other\n" +
		"    end\n" +
		"    result\n" +
		"  end\n" +
		"end\n"
	requireRealSourceComplete(t, RubyAnalyzer{}, text)
}

func TestRealSourceRubyAssignedBeginBlock(t *testing.T) {
	text := "module M\n" +
		"  def run\n" +
		"    @value ||= begin\n" +
		"      1\n" +
		"    rescue\n" +
		"      2\n" +
		"    ensure\n" +
		"      cleanup\n" +
		"    end\n" +
		"  end\n" +
		"end\n"
	requireRealSourceComplete(t, RubyAnalyzer{}, text)
}

func TestRealSourceRubyPrivateClassMethodDef(t *testing.T) {
	text := "class M\n" +
		"  private_class_method def self.run(value)\n" +
		"    value\n" +
		"  end\n" +
		"end\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "M.run") {
		t.Fatalf("Ruby private class method missing: %v", names)
	}
}

func TestRealSourceRubyDoBlockParameters(t *testing.T) {
	text := "module M\n" +
		"  [1].each do |value|\n" +
		"    value\n" +
		"  end\n" +
		"  def run\n" +
		"    1\n" +
		"  end\n" +
		"end\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "M.run") {
		t.Fatalf("Ruby method missing after do block: %v", names)
	}
}

func TestRealSourceRubyInterpolatedPercentLiteralPreservesDelimiterState(t *testing.T) {
	text := "class FailureApp\r\n" +
		"  def http_auth\r\n" +
		"    self.headers[\"WWW-Authenticate\"] = %(Basic realm=#{Devise.http_authentication_realm.inspect}) if http_auth_header?\r\n" +
		"  end\r\n" +
		"  def after\r\n" +
		"    1\r\n" +
		"  end\r\n" +
		"end\r\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"FailureApp.after", "FailureApp.http_auth"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Ruby declaration %q missing after interpolated percent literal: %v", want, names)
		}
	}
}

func TestRealSourceRubyPercentLiteralFamiliesStayOpaque(t *testing.T) {
	text := "class Literals\n" +
		"  def values\n" +
		"    a = %Q{value #{source.call} # still literal}\n" +
		"    b = %q[nested [value] # still literal]\n" +
		"    c = %w<one two # literal>\n" +
		"    d = %r|value#fragment|\n" +
		"  end\n" +
		"  def after\n" +
		"  end\n" +
		"end\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "Literals.after") {
		t.Fatalf("Ruby method missing after percent-literal families: %v", names)
	}
}

func TestRubyPercentLiteralProjectionDoesNotMaskModulo(t *testing.T) {
	text := "value = 7%(2)\n"
	document := sourceDocumentForScanner(text)
	scan, err := ScanSource(context.Background(), document, RubyScannerProfile(), ScannerLimits{MaxTokens: 64, MaxTokenBytes: 1024, MaxNesting: 64})
	if err != nil {
		t.Fatal(err)
	}
	if masked, changed := maskRubyPercentLiterals(text, scan.Tokens, 64); changed || masked != text {
		t.Fatalf("Ruby modulo expression was projected as a percent literal: changed=%v masked=%q", changed, masked)
	}
}

func TestRealSourceRubySingletonClassReceiverWithSameLineEnd(t *testing.T) {
	text := "module Devise\n" +
		"  module Models\n" +
		"    def self.config(mod)\n" +
		"      class << mod; attr_accessor :available_configs; end\n" +
		"      mod.available_configs = []\n" +
		"    end\n" +
		"    def self.after\n" +
		"    end\n" +
		"  end\n" +
		"end\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	if !containsSortedString(names, "Devise.Models.config") || !containsSortedString(names, "Devise.Models.after") {
		t.Fatalf("Ruby singleton-class receiver changed surrounding ownership: %v", names)
	}
	if containsSortedString(names, "Devise.Models.config.mod") || containsSortedString(names, "Devise.Models.mod") {
		t.Fatalf("Ruby singleton-class receiver was promoted to a class symbol: %v", names)
	}
}

func TestRealSourceRubyERBTemplateSyntaxStaysOpaque(t *testing.T) {
	text := "class DeviseCreate<%= table_name.camelize %> < ActiveRecord::Migration<%= migration_version %>\r\n" +
		"  def change\r\n" +
		"    create_table :<%= table_name %> do |t|\r\n" +
		"<%= migration_data -%>\r\n" +
		"<% attributes.each do |attribute| -%>\r\n" +
		"      t.<%= attribute.type %> :<%= attribute.name %>\r\n" +
		"<% end -%>\r\n" +
		"    end\r\n" +
		"  end\r\n" +
		"end\r\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	if !containsSortedString(names, "DeviseCreate") || !containsSortedString(names, "DeviseCreate.change") {
		t.Fatalf("Ruby declarations surrounding ERB template syntax are missing: %v", names)
	}
	for _, forbidden := range []string{"table_name", "migration_data", "attribute", "attributes"} {
		if containsSortedString(names, forbidden) {
			t.Fatalf("Ruby ERB expression leaked symbol %q: %v", forbidden, names)
		}
	}
}

func TestRubyERBProjectionDoesNotMaskOpaqueRubyString(t *testing.T) {
	text := "value = \"<% not template %>\"\ndef after\nend\n"
	document := sourceDocumentForScanner(text)
	scan, err := ScanSource(context.Background(), document, RubyScannerProfile(), ScannerLimits{MaxTokens: 64, MaxTokenBytes: 1024, MaxNesting: 64})
	if err != nil {
		t.Fatal(err)
	}
	if masked, changed := maskRubyERBTemplateSyntax(text, scan.Tokens); changed || masked != text {
		t.Fatalf("Ruby string content was projected as ERB syntax: changed=%v masked=%q", changed, masked)
	}
}

func TestRealSourceRubyInterpolatedSlashRegexpStaysOpaque(t *testing.T) {
	text := "module OrmHelpers\r\n" +
		"  def migration_exists?(table_name)\r\n" +
		"    Dir.glob(\"migrations/[0-9]*_*.rb\").grep(/\\d+_add_devise_to_#{table_name}.rb$/).first\r\n" +
		"  end\r\n" +
		"  def after\r\n" +
		"  end\r\n" +
		"end\r\n"
	result := requireRealSourceComplete(t, RubyAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"OrmHelpers", "OrmHelpers.migration_exists?", "OrmHelpers.after"} {
		if !containsSortedString(names, want) {
			t.Fatalf("Ruby declaration missing after interpolated slash regexp: want=%q symbols=%v", want, names)
		}
	}
}

func TestRubySlashRegexpProjectionDoesNotMaskDivision(t *testing.T) {
	text := "value = total / count\ndef after\nend\n"
	document := sourceDocumentForScanner(text)
	scan, err := ScanSource(context.Background(), document, RubyScannerProfile(), ScannerLimits{MaxTokens: 64, MaxTokenBytes: 1024, MaxNesting: 64})
	if err != nil {
		t.Fatal(err)
	}
	if masked, changed := maskRubySlashRegexLiterals(text, scan.Tokens, 64); changed || masked != text {
		t.Fatalf("Ruby division was projected as a regexp: changed=%v masked=%q", changed, masked)
	}
}

func TestRealSourceKotlinBodylessClassAtEOF(t *testing.T) {
	text := "package addressbook\nclass Country(val name: String)"
	result := requireRealSourceComplete(t, KotlinAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "addressbook.Country") {
		t.Fatalf("Kotlin EOF class missing: %v", names)
	}
}

func TestRealSourceAutoHotkeySemicolonInsideCommandArgument(t *testing.T) {
	text := "FileSelectFile, file,,, Images (*.gif; *.jpg; *.png)\n" +
		"helper()\n{\n  return\n}\n"
	requireRealSourceComplete(t, AutoHotkeyAnalyzer{}, text)
}

func TestRealSourceAutoHotkeyOpaqueStringAndContinuationForms(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{
			name: "legacy-command-apostrophe",
			text: "MsgBox, Don't hide this\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-command-expanded-variable-apostrophes",
			text: "MsgBox, 64, %AppName%, %t_Lang161%`n`n'%SchedDate%'\n" +
				"helper() { return }\n",
		},
		{
			name: "single-quoted-string",
			text: "value := 'quoted \" text { [ ( still data'\n" +
				"helper() { return }\n",
		},
		{
			name: "single-quoted-continuation-string",
			text: "get_identify_regex() => '\n" +
				"(\n" +
				"opaque regex data { [ ( \"\n" +
				")'\n" +
				"helper() { return }\n",
		},
		{
			name: "double-quoted-continuation-string",
			text: "mcode := \"\n" +
				"(LTrim Join\n" +
				"ABC{[()]}DEF\n" +
				")\"\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-continuation-section",
			text: "Script =\n" +
				"(\n" +
				"#define PmcName \"Pulover's Macro Creator\"\n" +
				"opaque text { [ (\n" +
				")\n" +
				"helper() { return }\n",
		},
		{
			name: "parenthesized-expression-is-not-continuation-section",
			text: "outer() {\n" +
				"  Call(\n" +
				"    (flag ? 1 : 2)\n" +
				"  )\n" +
				"}\n" +
				"helper() { return }\n",
		},
		{
			name: "block-comment-parenthesis-is-not-continuation-section",
			text: "class UIA {\n" +
				"  /*\n" +
				"  (comment text)\n" +
				"  */\n" +
				"  method() {\n" +
				"    Call(\n" +
				"      1\n" +
				"    )\n" +
				"  }\n" +
				"}\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-assignment-regex-payload",
			text: "pattern=iS)^\\t*global Value:=\"(.+?)\"\n" +
				"helper() { return }\n",
		},
		{
			name: "multiline-expression-string-continuation",
			text: "SetOnlyList := \"GoHome,GoBack,GoForward\n" +
				"    ,Navigate,Focus,Click\"\n" +
				"helper() { return }\n",
		},
		{
			name: "apostrophe-hotkey-label",
			text: "'::\n" +
				"<!'::\n" +
				"helper() { return }\n",
		},
		{
			name: "single-quoted-colon-string-remains-a-string",
			text: "value := '::'\n" +
				"helper() { return }\n",
		},
		{
			name: "modulo-before-single-quoted-string-remains-a-string",
			text: "value := 5 % 'opaque { [ ('\n" +
				"helper() { return }\n",
		},
		{
			name: "v2-equality-expression-remains-structural",
			text: "outer() {\n" +
				"  x = Call(\n" +
				"    1\n" +
				"  )\n" +
				"}\n" +
				"helper() { return }\n",
		}, {
			name: "chained-single-quoted-continuations",
			text: "assert_match pattern, '\n" +
				"(\n" +
				"first\n" +
				")', ['\n" +
				"(\n" +
				"second\n" +
				")']\n" +
				"helper() { return }\n",
		},
		{
			name: "quoted-continuation-closing-line-content",
			text: "value := '(?>\n" +
				"(Join|\n" +
				"body\n" +
				"))'\n" +
				"helper() { return }\n",
		},
		{
			name: "spaced-legacy-raw-assignment",
			text: "pattern = iS)\"name\": \".*\"\n" +
				"other = \"name\": \".*\"\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-command-backtick-escaped-quotes",
			text: "MsgBox, 35, title, text`\"%CurrentFileName%`\"\n" +
				"helper() { return }\n",
		},
		{
			name: "quoted-literal-backtick",
			text: "value := \"``\"\n" +
				"helper() { return }\n",
		},
		{
			name: "square-bracket-hotkey-labels",
			text: "[::sendinput, x\n" +
				"+[::sendinput, y\n" +
				"helper() { return }\n",
		},
		{
			name: "v2-array-remains-structural",
			text: "value := [1, 2]\n" +
				"helper() { return }\n",
		},
		{
			name: "legacy-command-raw-brace-transition",
			text: "StringReplace, sKey, tKey, +, Shift Down}{\n" +
				"StringReplace, sKey, sKey, !, Alt Down}{\n" +
				"helper() { return }\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := requireRealSourceComplete(t, AutoHotkeyAnalyzer{}, tc.text)
			if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "helper") {
				t.Fatalf("AutoHotkey helper declaration missing after opaque data: %v", names)
			}
		})
	}
}

func TestRealSourceLispCharacterLiteralsAndMultilineStrings(t *testing.T) {
	t.Run("clojure", func(t *testing.T) {
		text := "(def semi (int \\;))\n(def doc \"line one\nline two\")\n"
		requireRealSourceComplete(t, ClojureAnalyzer{}, text)
	})
	t.Run("emacs-lisp", func(t *testing.T) {
		text := "(defconst quote-char ?\\\")\n(defconst brace-char ?\\{)\n(defun real () \"line one\nline two\")\n"
		requireRealSourceComplete(t, EmacsLispAnalyzer{}, text)
	})
}

func TestRealSourceScalaSymbolLiteral(t *testing.T) {
	text := "object M {\n  val symbol = 'symbol\n  val char = 'a'\n}\n"
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaTripleStringClosingQuoteRun(t *testing.T) {
	text := "object M {\n  val value = \"\"\"one \", two \"\", three \"\"\"\"\"\"\n}\n"
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceDQStringFamilies(t *testing.T) {
	text := "void real() {\n" +
		"  auto a = q\"{text { nested } text}\";\n" +
		"  auto b = q\"[text [ nested ] text]\";\n" +
		"  auto c = q\"/text \\\" quoted /\";\n" +
		"  auto d = q\"TOKEN\ntext { with braces }\nTOKEN\";\n" +
		"  auto e = q{ token { nested } string };\n" +
		"}\n"
	requireRealSourceComplete(t, DAnalyzer{}, text)
}

func TestRealSourceNimDocBlockComment(t *testing.T) {
	text := "proc real =\n" +
		"  ##[\n" +
		"  docs [with] delimiters\n" +
		"  ]##\n" +
		"  discard\n"
	requireRealSourceComplete(t, NimAnalyzer{}, text)
}

func TestRealSourceNimNumericSuffixAndQuotedIdentifier(t *testing.T) {
	text := "template real =\n" +
		"  let a = 0xdeadBEEF'wrap\n" +
		"  let b = -123'32\n" +
		"proc `'quoted`(a: string): int = discard\n"
	result := requireRealSourceComplete(t, NimAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "real") {
		t.Fatalf("Nim declaration missing after numeric suffixes: %v", names)
	}
}

func TestRealSourceNimStrategicIndentationAllowsContinuationLevels(t *testing.T) {
	text := "proc classify(value: int) =\n" +
		"  case value\n" +
		"  of 1, 2,\n" +
		"       3:\n" +
		"    discard\n" +
		"  else:\n" +
		"    discard\n" +
		"  if value > 0 and\n" +
		"      value < 10:\n" +
		"    discard\n" +
		"proc after() = discard\n"
	result := requireRealSourceComplete(t, NimAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "classify") || !containsSortedString(names, "after") {
		t.Fatalf("Nim declarations missing after continuation indentation: %v", names)
	}
}

func TestRealSourceTwigCompositeSymbolsStayOrdered(t *testing.T) {
	text := "{% macro input(name) %}<input name=\"{{ name }}\">{% endmacro %}\n<div id=\"later\"></div>\n"
	result := requireRealSourceComplete(t, TwigAnalyzer{}, text)
	for index := 1; index < len(result.Analysis.Symbols); index++ {
		left := result.Analysis.Symbols[index-1]
		right := result.Analysis.Symbols[index]
		if right.declarationOffsets.Start < left.declarationOffsets.Start || right.declarationOffsets.Start == left.declarationOffsets.Start && right.declarationOffsets.End < left.declarationOffsets.End {
			t.Fatalf("Twig symbols out of source order at %d: left=%+v right=%+v", index, left, right)
		}
	}
}

func TestRealSourcePowerShellNestedHereStringsInsideInterpolation(t *testing.T) {
	text := "function Real {\n" +
		"@\"\n" +
		"outer\n" +
		"$(\n" +
		"@\"\ninner $value\n\"@\n" +
		"@'\nliteral\n'@\n" +
		")\n" +
		"outer tail\n" +
		"\"@\n" +
		"}\n"
	result := requireRealSourceComplete(t, PowerShellAnalyzer{}, text)
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "Real") {
		t.Fatalf("PowerShell function missing after nested here-string: %v", names)
	}
}

func TestRealSourceAstroSelfClosingScriptDoesNotCaptureLaterScript(t *testing.T) {
	text := `---
const id = "analytics";
---
{id && <script is:inline src={` + "`" + `https://example.test/tag.js?id=${id}` + "`" + `} />}
<script is:inline>
window.analytics = { enabled: true };
</script>
`
	requireRealSourceComplete(t, AstroAnalyzer{}, text)
}

func TestRealSourceASPNetWebFormsServerCommentIsOpaque(t *testing.T) {
	text := `<%@ Page Language="C#" %>
<%-- Styling the "Copy: Button Box --%>
<script runat="server">
public void Real() { }
</script>
`
	requireRealSourceComplete(t, ASPNetWebFormsAnalyzer{}, text)
}

func TestRealSourceBlazorRenderFragmentMarkupIsOpaqueToCSharp(t *testing.T) {
	text := `@code {
    private string Prefix { get; set; } = "demo";
    private void SetIcon()
    {
        RenderFragment item = @<span class="@($"{Prefix}-item")">value</span>;
        @* punctuation in Razor comment :) *@
    }
}
`
	requireRealSourceComplete(t, BlazorAnalyzer{}, text)
}

func TestRealSourceBlazorRenderFragmentBlocksAreOpaqueToCSharp(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{
			name: "explicit-ul-fragment",
			text: `@code {
    private void Show()
    {
        Service.Add(
            @<ul>
                <li>Here's a <strong>bold item</strong></li>
            </ul>
        );
    }
}
`,
		},
		{
			name: "text-fragment",
			text: `@code {
    RenderFragment<string> Build = version =>
        @<text>
            <Widget Anchor="@($"get-started/{version}")">Install</Widget>
            <ul>
                <li>Via Visual Studio's package manager.</li>
                <li>By editing your application's project file.</li>
            </ul>
        </text>;
}
`,
		},
		{
			name: "line-leading-markup-lambda-body",
			text: `@code {
    RenderFragment<bool> Build = value => builder =>
    {
        <Grid Rows="@(new List<int>
            {
                1,
                2
            })">
            <Cell Text="@value" />
        </Grid>
    };
}
`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			requireRealSourceComplete(t, BlazorAnalyzer{}, tc.text)
		})
	}
}

func TestRealSourceBlazorMarkupMaskingRemainsFailClosed(t *testing.T) {
	t.Run("csharp-generic-and-comparison", func(t *testing.T) {
		text := `@code {
    private List<int> Items { get; } = new();
    private bool Less(int left, int right) => left < right;
}
`
		result := requireRealSourceComplete(t, BlazorAnalyzer{}, text)
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, want := range []string{"Items", "Less"} {
			if !containsSortedString(names, want) {
				t.Fatalf("Blazor C# declaration %q missing after markup masking: %v", want, names)
			}
		}
	})
	t.Run("unterminated-markup-remains-partial", func(t *testing.T) {
		text := `@code {
    private void Build()
    {
        @<div>
            broken
    }
}
`
		result, err := (BlazorAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.CoverageComplete {
			t.Fatalf("unterminated Blazor markup unexpectedly reported complete: %+v", result.Analysis)
		}
	})
}

func TestRealSourceDOrdinaryStringMaySpanPhysicalLines(t *testing.T) {
	text := `void real()
{
    auto files = "
        one.d two.d
        three.d
    ";
}
`
	requireRealSourceComplete(t, DAnalyzer{}, text)
}

func TestRealSourceDartRawStringsDoNotUseBackslashEscapes(t *testing.T) {
	text := `void real(String value) {
  final escaped = value.replaceAll(r'\', r'\\').replaceAll("'", r"\'");
}
`
	requireRealSourceComplete(t, DartAnalyzer{}, text)
}

func TestRealSourceDAnonymousUnionInsideStruct(t *testing.T) {
	text := `struct Request
{
    int prefix;
    union
    {
        int command;
        ubyte[5] header;
    }
    int suffix;
}
`
	result := requireRealSourceComplete(t, DAnalyzer{}, text)
	names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
	for _, want := range []string{"Request", "Request.command", "Request.header", "Request.prefix", "Request.suffix"} {
		if !containsSortedString(names, want) {
			t.Fatalf("D anonymous-union declaration %q missing: %v", want, names)
		}
	}
}

func TestRealSourceEJSSplitControlFlowSharesOneJavaScriptProjection(t *testing.T) {
	text := `<main>
<% if (items.length) { %>
  <span><%= items[0] %></span>
<% } else { %>
  <span>empty</span>
<% } %>
</main>
`
	requireRealSourceComplete(t, EJSAnalyzer{}, text)
}

func TestRealSourceScalaTypedConstructorColonIsNotBodyColon(t *testing.T) {
	text := `package demo
case class Record(name: String, count: Int) extends Product
final class Failure(cause: Throwable) extends Exception(cause)
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaUnterminatedColonBodyRemainsPartial(t *testing.T) {
	text := `class Broken:
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete {
		t.Fatalf("unterminated Scala colon body unexpectedly reported complete: %+v", result.Analysis)
	}
}

func TestRealSourcePOSIXShellCasePatternsAreNotDelimiterClosers(t *testing.T) {
	text := `pick() {
    case "$1" in
        alpha | beta) printf '%s\\n' ok ;;
        *) printf '%s\\n' fallback ;;
    esac
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellParenthesizedCasePattern(t *testing.T) {
	text := `pick() {
    case "$1" in
        ( alpha | beta ) printf '%s\\n' ok ;;
        ( * ) printf '%s\\n' fallback ;;
    esac
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourceScalaInterpolatedExpressionMaySpanLines(t *testing.T) {
	text := `object Demo {
  def value(names: List[String]): String =
    s"names = [${names.mkString("\\\"", "\\\", \\\"", "\\\"")}]"
  def other(flag: Boolean): String =
    s"/${if (flag) { "yes" }
    else { "no" }}/done"
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaNestedColonBodyUsesPhysicalIndentation(t *testing.T) {
	text := `object Outer {
  object Inner:
    private var value: Int = 0
    def get: Int = value
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaInitializerBracesAreNotDeclarationBodies(t *testing.T) {
	text := `object Demo {
  val mapped = List(1, 2).map { value => value + 1 }
  def plural(values: List[Int]) = s"${values.size}${if (values.size == 1) "" else "s"}"
  def block(value: Int) = {
    value + 1
  }
}
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Scala initializer source reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	mapped, mappedOK := byName["Demo.mapped"]
	plural, pluralOK := byName["Demo.plural"]
	block, blockOK := byName["Demo.block"]
	if !mappedOK || !pluralOK || !blockOK {
		t.Fatalf("Scala declarations missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if mapped.BodyRange != nil {
		t.Fatalf("val initializer lambda became declaration body: %+v", mapped.BodyRange)
	}
	if plural.BodyRange != nil {
		t.Fatalf("interpolation expression became method body: %+v", plural.BodyRange)
	}
	if block.BodyRange == nil {
		t.Fatalf("real braced method body was not recognized: %+v", block)
	}
}

func TestRealSourceScalaOrdinaryStringsEndingLikeInterpolatorPrefixesStayOpaque(t *testing.T) {
	text := `object Json {
  def append(c: Char, sb: StringBuilder): Unit = c match {
    case '\f' => sb.append("\\f")
    case '\n' => sb.append("\\n")
    case _ => sb.append("application/vnd.github.v3.raw")
  }
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaCustomInterpolatorExpressionMaySpanLines(t *testing.T) {
	text := `object Demo {
  def value(flag: Boolean): String =
    em"prefix ${if (flag) { "yes" }
    else { "" }} suffix"
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaDefInitializerLambdaIsNotMethodBody(t *testing.T) {
	text := `object Demo {
  def lookup: Int = List(1).collectFirst { case value => value }.get
  def block: Int = {
    1
  }
}
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Scala def initializer source reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	lookup, lookupOK := byName["Demo.lookup"]
	block, blockOK := byName["Demo.block"]
	if !lookupOK || !blockOK {
		t.Fatalf("Scala declarations missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if lookup.BodyRange != nil {
		t.Fatalf("def initializer lambda became method body: %+v", lookup.BodyRange)
	}
	if block.BodyRange == nil {
		t.Fatalf("direct braced method body was not recognized: %+v", block)
	}
}
func TestRealSourceScalaTripleQuotesInsideLineCommentsStayOpaque(t *testing.T) {
	text := `class InitializeListener {
//  private val system = ConfigFactory.parseString("""
//    |akka {
//    |  daemonic = on
//    |}
//  """.stripMargin)

  override def contextInitialized(): Unit = {
    ()
  }
}
`
	requireRealSourceComplete(t, ScalaAnalyzer{}, text)
}

func TestRealSourceScalaSameLineDeclarationAfterBracedDef(t *testing.T) {
	text := `object Sym {
  def nextSymId = { value += 1; value }; private var value = 0
}
`
	result, err := (ScalaAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(true, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid Scala same-line declaration source reported partial: %+v", result.Analysis)
	}
	byName := symbolsByQualifiedName(result.Analysis.Symbols)
	nextSymID, ok := byName["Sym.nextSymId"]
	if !ok {
		t.Fatalf("Scala method missing: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
	}
	if nextSymID.BodyRange == nil || nextSymID.SignatureRange == nil {
		t.Fatalf("Scala method ranges missing: %+v", nextSymID)
	}
	if nextSymID.SignatureRange.End.Line > nextSymID.BodyRange.Start.Line ||
		(nextSymID.SignatureRange.End.Line == nextSymID.BodyRange.Start.Line && nextSymID.SignatureRange.End.Column > nextSymID.BodyRange.Start.Column) {
		t.Fatalf("Scala signature extends into/past body: signature=%+v body=%+v", nextSymID.SignatureRange, nextSymID.BodyRange)
	}
}

func TestRealSourceGraphQLHashInsideDirectiveStringIsNotAComment(t *testing.T) {
	text := `type View implements Node {
  fields(orderBy: FieldOrder = {field: POSITION, direction: ASC}): FieldConnection
    @deprecated(reason: "View#fields remains available")
}
`
	requireRealSourceComplete(t, GraphQLAnalyzer{}, text)
}

func TestRealSourceGraphQLDescriptionQuotedFragmentDoesNotPoisonDirectiveString(t *testing.T) {
	text := `"""
See "[guide](docs#anchor)" for details.
"""
type View {
  value: String @deprecated(reason: "View#value remains available")
}
`
	result, err := (GraphQLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), testAnalyzeOptions(false, 256))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete {
		t.Fatalf("valid GraphQL description/directive source reported partial: %+v", result.Analysis)
	}
	if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "View") {
		t.Fatalf("GraphQL description/directive masking hid following type: %v", names)
	}
}

func TestRealSourcePOSIXShellArithmeticShiftsAreNotHeredocs(t *testing.T) {
	text := `hash_value() {
    value=$(( (1 << 16) + ($1 >> 2) ))
    printf '%s\\n' "$value"
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellBracketCommandIsNotStructuralDelimiter(t *testing.T) {
	text := `install_test() {
    if ! extern -pv [ >/dev/null && testcmd=$(extern -pv test); then
        ln -s $testcmd $HOME/bin/[
        if $HOME/bin/[ 1 -eq 1 ] 2>/dev/null; then
            PATH=$PATH command rm $HOME/bin/[
        fi
    fi
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellNegatedCaseCommandIsRecognized(t *testing.T) {
	text := `probe() {
    { ! : || ! case x in x) ;; esac; } && exit 1
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}

func TestRealSourcePOSIXShellNestedCaseMayStartCaseArmBody(t *testing.T) {
	text := `check_range() {
    case $2 in
        @*) case ${2#@} in (*[!0-9-]*) return 1; esac ;;
        *) case $2 in (*[!0-9]*) return 1; esac ;;
    esac
}
`
	requireRealSourceComplete(t, ShellAnalyzer{}, text)
}
func TestRealSourceTOMLArraysOfTablesAndMultilineArrays(t *testing.T) {
	text := `[tool.demo]
commands = [
  ["mypy"],
  ["pyright", "--verifytypes", "demo"],
]

[[tool.demo.overrides]]
module = "first"

[[tool.demo.overrides]]
module = "second"
`
	requireRealSourceComplete(t, TOMLAnalyzer{}, text)
}
