package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Double-quoted Nix values can span physical lines. Their text must not
// become sibling let bindings or terminate the enclosing let block.
func TestNixDoubleQuotedMultilineStringCannotInventBindings(t *testing.T) {
	check := func(t *testing.T, source string, forbidden []string) []string {
		t.Helper()
		result, err := (NixAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("transposition.nix", source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, name := range forbidden {
			if _, ok := symbolsByQualifiedName(result.Analysis.Symbols)[name]; ok {
				t.Errorf("double-quoted Nix text invented declaration %q: %v", name, names)
			}
		}
		return names
	}
	const simple = "let\n" +
		"  # an unmatched \" quote in a comment must not hide code\n" +
		"  message = \"first\n" +
		"  phantom = 42;\n" +
		"in prematurely\n" +
		"  escaped \\\" tail\";\n" +
		"  actual = 7;\n" +
		"in actual\n"
	for _, ending := range []struct{ name, sep string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(ending.name, func(t *testing.T) {
			names := check(t, strings.ReplaceAll(simple, "\n", ending.sep), []string{"phantom"})
			if !slices.Contains(names, "message") || !slices.Contains(names, "actual") {
				t.Fatalf("Nix sibling let bindings were lost to quoted text: %v", names)
			}
		})
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_NIX_FLAKE_PARTS_ROOT"); root != "" {
		t.Run("source-derived-original-flake-parts", func(t *testing.T) {
			path := filepath.Join(root, "transposition.nix")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Nix source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if !strings.Contains(source, "  transpositionModule = {") ||
				!strings.Contains(source, "  perInputAttributeError = {") ||
				!strings.Contains(source, "\nlet\n") {
				t.Fatal("original flake-parts let-binding oracle changed")
			}
			baseline := check(t, source, nil)
			if !slices.Contains(baseline, "transpositionModule") ||
				!slices.Contains(baseline, "perInputAttributeError") {
				t.Fatalf("original Nix let binding baseline not navigable: %v", baseline)
			}
			derived := strings.Replace(source, "\nlet\n",
				"\nlet\n  message = \"first\n  phantom = 42;\nin prematurely\n  tail\";\n", 1)
			names := check(t, derived, []string{"phantom"})
			if !slices.Contains(names, "transpositionModule") ||
				!slices.Contains(names, "perInputAttributeError") ||
				!slices.Contains(names, "message") {
				t.Fatalf("valid original let bindings lost: baseline=%v derived=%v", baseline, names)
			}
		})
	}
}

// A standalone string is a valid Nix expression but not an indexed let
// binding. Masking its contents must not claim the whole file is covered.
func TestNixStandaloneDoubleQuotedRootRemainsIncomplete(t *testing.T) {
	for _, tc := range []struct{ name, source string }{
		{"single-line", "\"hello\"\n"},
		{"multi-line", "\"first\n  phantom = 1;\n  tail\"\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (NixAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("root.nix", tc.source), testAnalyzeOptions(true, 16))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated ||
				len(result.Analysis.Symbols) != 0 {
				t.Fatalf("unsupported Nix root string misclassified: %+v", result.Analysis)
			}
		})
	}
}

// A Nix block comment is not a let scope. Source-like text inside one must
// never create edit targets or terminate the real enclosing let expression.
func TestNixBlockCommentCannotInventBindings(t *testing.T) {
	check := func(t *testing.T, source string) []string {
		t.Helper()
		masked := maskNixIndentedStrings(source)
		if len(masked) != len(source) || strings.Count(masked, "\n") != strings.Count(source, "\n") ||
			strings.Contains(masked, "phantom =") {
			t.Fatalf("Nix block-comment masking changed source offsets or leaked comment text: %q", masked)
		}
		result, err := (NixAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("perSystem.nix", source), testAnalyzeOptions(true, 256))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		if slices.Contains(names, "phantom") {
			t.Errorf("block comment invented a Nix binding: %v", names)
		}
		return names
	}
	const fixture = "let\n" +
		"  before = \"/* quoted content */\";\n" +
		"  /*\n" +
		"  phantom = 123;\n" +
		"in prematurely\n" +
		"  \"a misleading double quote\"\n" +
		"  */\n" +
		"  after = 42;\n" +
		"in after\n"
	for _, ending := range []struct{ name, sep string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(ending.name, func(t *testing.T) {
			names := check(t, strings.ReplaceAll(fixture, "\n", ending.sep))
			if !slices.Equal(names, []string{"after", "before"}) {
				t.Fatalf("real Nix let bindings were lost or fabricated: %v", names)
			}
		})
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_NIX_FLAKE_PARTS_ROOT"); root != "" {
		t.Run("source-derived-original-perSystem", func(t *testing.T) {
			path := filepath.Join(root, "perSystem.nix")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Nix source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			const anchor = "    doesn't extend to arbitrary `system`s.\n"
			if strings.Count(source, anchor) != 1 ||
				!strings.Contains(source, "  otherMemoizedSystems = flake-parts-lib.memoizeStr config.perSystem;") {
				t.Fatal("original flake-parts block-comment oracle changed")
			}
			baseline := check(t, source)
			for _, name := range []string{"rootConfig", "throwAliasError'", "throwAliasError", "otherMemoizedSystems"} {
				if !slices.Contains(baseline, name) {
					t.Fatalf("original Nix let binding %q missing: %v", name, baseline)
				}
			}
			derived := strings.Replace(source, anchor, anchor+
				"  phantom = 123;\nin prematurely\n", 1)
			names := check(t, derived)
			if !slices.Equal(names, baseline) {
				t.Fatalf("adding comments changed valid bindings: baseline=%v derived=%v", baseline, names)
			}
		})
	}
}

// A missing outer `in` must not be reported as a completely indexed Nix
// expression, even when its direct let bindings are still navigable.
func TestNixOuterLetMissingInReportsIncompleteCoverage(t *testing.T) {
	test := func(t *testing.T, source string, wantComplete bool) {
		t.Helper()
		result, err := (NixAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("home-manager.nix", source), testAnalyzeOptions(true, 40))
		if err != nil {
			t.Fatal(err)
		}
		if result.Analysis.Truncated || result.Analysis.CoverageComplete != wantComplete {
			t.Fatalf("outer let coverage mismatch: complete=%t analysis=%+v", wantComplete, result.Analysis)
		}
		if !slices.Contains(sortedSymbolQualifiedNames(result.Analysis.Symbols), "cfg") {
			t.Fatalf("authored Nix binding lost: %+v", result.Analysis.Symbols)
		}
		if !wantComplete {
			found := false
			for _, diagnostic := range result.Analysis.Diagnostics {
				if diagnostic.Code == "nix-unclosed-let" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing outer let diagnostic: %+v", result.Analysis.Diagnostics)
			}
		}
	}
	const valid = "let\n  cfg = config.programs.home-manager;\nin cfg\n"
	const malformed = "let\n  cfg = config.programs.home-manager;\n"
	for _, tc := range []struct {
		name, source string
		complete     bool
	}{
		{"LF-valid", valid, true},
		{"LF-missing-in", malformed, false},
		{"CRLF-valid", strings.ReplaceAll(valid, "\n", "\r\n"), true},
		{"CRLF-missing-in", strings.ReplaceAll(malformed, "\n", "\r\n"), false},
	} {
		t.Run(tc.name, func(t *testing.T) { test(t, tc.source, tc.complete) })
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_NIX_HOME_MANAGER_ROOT"); root != "" {
		t.Run("source-derived-home-manager", func(t *testing.T) {
			path := filepath.Join(root, "home-manager.nix")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original Nix source unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			const authored = "let\n  cfg = config.programs.home-manager;\nin\n"
			if strings.Count(source, authored) != 1 {
				t.Fatal("original Home Manager let oracle changed")
			}
			standalone := strings.Replace(authored, "\nin\n", "\nin cfg\n", 1)
			test(t, standalone, true)
			test(t, strings.Replace(standalone, "\nin cfg\n", "", 1), false)
			// Keep the original wrapped document as a distinct partial-coverage
			// case; its outer lambda is outside this bounded let-only index.
			result, err := (NixAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("home-manager.nix", source), testAnalyzeOptions(true, 40))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete {
				t.Fatal("original lambda-wrapped source unexpectedly complete")
			}
		})
	}
}

// Existing direct Nix let bindings remain navigable, but unsupported inherit
// clauses must not silently count toward complete source coverage.
func TestNixOuterLetInheritCannotClaimCompleteCoverage(t *testing.T) {
	const original = "let\n" +
		"  inherit (lib)\n" +
		"    filterAttrs\n" +
		"    mapAttrs\n" +
		"    mkOption\n" +
		"    types\n" +
		"    ;\n" +
		"  inherit (lib.strings)\n" +
		"    escapeNixIdentifier\n" +
		"    ;\n" +
		"  selected = filterAttrs;\n" +
		"in selected\n"
	cases := []struct{ name, source string }{
		{"LF", original}, {"CRLF", strings.ReplaceAll(original, "\n", "\r\n")},
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_NIX_FLAKE_PARTS_ROOT"); root != "" {
		path := filepath.Join(root, "transposition.nix")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			t.Fatalf("bounded original flake-parts source unavailable: %v %v", info, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		inheritFragment := strings.TrimSuffix(strings.TrimPrefix(original, "let\n"), "  selected = filterAttrs;\nin selected\n")
		if strings.Count(string(data), inheritFragment) != 1 {
			t.Fatal("original flake-parts inherit declarations changed")
		}
		cases = append(cases, struct{ name, source string }{"source-derived-original", "let\n" + inheritFragment + "  selected = filterAttrs;\nin selected\n"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (NixAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("transposition.nix", tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("unindexed let inherit falsely reported complete: %+v", result.Analysis)
			}
			if !slices.Equal(sortedSymbolQualifiedNames(result.Analysis.Symbols), []string{"selected"}) {
				t.Fatalf("Nix inherit fabricated names or direct binding lost: %+v", result.Analysis.Symbols)
			}
			found := false
			for _, diagnostic := range result.Analysis.Diagnostics {
				if diagnostic.Code == "nix-unmodeled-inherit" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing Nix inherit limitation diagnostic: %+v", result.Analysis.Diagnostics)
			}
		})
	}
}

func TestNixInheritLexicalBoundaries(t *testing.T) {
	cases := []struct {
		name, source string
		partial      bool
	}{
		{
			name:    "nested-attribute-inherit",
			source:  "let\n  selected = {\n    inherit (lib) filterAttrs;\n  };\nin selected\n",
			partial: true,
		},
		{
			name:   "quoted-and-commented-inherit",
			source: "let\n  # inherit (lib) fake;\n  selected = \"inherit (lib) phantom\";\n  /* inherit (lib) decoy; */\nin selected\n",
		},
		{
			name:   "different-identifier",
			source: "let\n  inheritance = 42;\n  selected = inheritance;\nin selected\n",
		},
	}
	for _, tc := range cases {
		for _, ending := range []struct{ name, sep string }{
			{"LF", "\n"}, {"CRLF", "\r\n"},
		} {
			t.Run(tc.name+"-"+ending.name, func(t *testing.T) {
				result, err := (NixAnalyzer{}).Analyze(context.Background(),
					testSourceDocument("lexical.nix", strings.ReplaceAll(tc.source, "\n", ending.sep)),
					testAnalyzeOptions(true, 32))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || result.Analysis.CoverageComplete == tc.partial {
					t.Fatalf("Nix inherit lexical boundary coverage mismatch: %+v", result.Analysis)
				}
				names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
				if !slices.Contains(names, "selected") ||
					slices.Contains(names, "fake") || slices.Contains(names, "phantom") ||
					slices.Contains(names, "decoy") || slices.Contains(names, "filterAttrs") {
					t.Fatalf("Nix inherit invented declarations or lost selected: %v", names)
				}
			})
		}
	}
}

func TestNixTopLevelAttributeSetReportsIncompleteCoverage(t *testing.T) {
	source := "# A flake-like root document\n{\n  description = \"demo\";\n  outputs = self: { package = \"example\"; };\n}\n"
	result, err := (NixAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 32))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("unmodeled root attribute set incorrectly declared complete: %+v", result.Analysis)
	}
	found := false
	for _, diagnostic := range result.Analysis.Diagnostics {
		if diagnostic.Code == "nix-unmodeled-root-structure" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing root Nix structure limitation: %+v", result.Analysis.Diagnostics)
	}
	if len(result.Analysis.Symbols) != 0 {
		t.Fatalf("unparsed root attributes invented declarations: %+v", result.Analysis.Symbols)
	}
}

// Only bindings of the outer let are safe top-level edit targets. Nested
// attrsets and function-local let expressions are not the same declarations.
func TestNixOuterLetDoesNotInventNestedBindings(t *testing.T) {
	const source = `let
  outer = {
    name = "inside";
    nested = {
      option = true;
    };
  };
  builder = value:
    let
      local = 1;
    in local;
  after = 42;
in
{
  result = outer;
}
`
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (NixAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 40))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]int{"outer": 2, "builder": 8, "after": 12}
			for _, symbol := range result.Analysis.Symbols {
				line, ok := want[symbol.Name]
				if !ok {
					t.Errorf("nested Nix binding misreported at top level: %+v", symbol)
				} else if symbol.NameRange.Start.Line != line {
					t.Errorf("top-level Nix binding misplaced: %+v", symbol)
				} else {
					delete(want, symbol.Name)
				}
			}
			if len(want) != 0 {
				t.Errorf("real top-level let bindings missing: %+v", want)
			}
			if result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Errorf("Nix result attrset must still report partial coverage: %+v", result.Analysis)
			}
		})
	}
}

// Nix indentation is only formatting: an inner attribute can be written
// at the same column as the surrounding let bindings.
func TestNixNestedAttributeAtSameIndentIsNotTopLevel(t *testing.T) {
	const source = "let\n" +
		"  container = {\n" +
		"  decoy = true;\n" +
		"  };\n" +
		"  selected = 42;\n" +
		"in selected\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (NixAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 20))
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, sym := range result.Analysis.Symbols {
				if sym.Name == "decoy" {
					t.Fatalf("nested Nix key falsely classified by indentation: %+v", sym)
				}
				found[sym.Name] = true
			}
			if !found["container"] || !found["selected"] {
				t.Fatalf("top-level Nix bindings missing: %+v", result.Analysis.Symbols)
			}
		})
	}
}

func TestNixPlainLetExpressionRemainsComplete(t *testing.T) {
	source := "let\n  answer = 42;\nin answer\n"
	result, err := (NixAnalyzer{}).Analyze(context.Background(),
		sourceDocumentForScanner(source), testAnalyzeOptions(true, 16))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Analysis.CoverageComplete || result.Analysis.Truncated ||
		len(result.Analysis.Diagnostics) != 0 {
		t.Fatalf("simple indexed let expression misclassified: %+v", result.Analysis)
	}
	names := symbolsByQualifiedName(result.Analysis.Symbols)
	if _, ok := names["answer"]; !ok || len(names) != 1 {
		t.Fatalf("plain Nix binding unexpectedly absent: %+v", result.Analysis.Symbols)
	}
}

func TestNixIndentedStringsCannotInventLetBindings(t *testing.T) {
	source := strings.Join([]string{
		"let",
		"  explanation = ''",
		"    phantom = 1;",
		"    ''${interpolation} phantom2 = 2;",
		"    ''' phantom3 = 3;",
		"    in result",
		"  '';",
		"  # '' a commented delimiter is not an opening quote",
		"  quoted = \"double '' quoted\";",
		"  answer = 42;",
		"in answer",
		"",
	}, "\n")
	for _, tc := range []struct{ name, source string }{
		{"lf", source},
		{"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			masked := maskNixIndentedStrings(tc.source)
			if len(masked) != len(tc.source) ||
				strings.Count(masked, "\n") != strings.Count(tc.source, "\n") ||
				strings.Contains(masked, "phantom") ||
				!strings.Contains(masked, "answer = 42;") {
				t.Fatalf("Nix quoted text masking changed offsets or escaped real code: %q", masked)
			}
			result, err := (NixAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("simple let expression incorrectly partial after string masking: %+v", result.Analysis)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range []string{"explanation", "quoted", "answer"} {
				if _, ok := got[name]; !ok {
					t.Errorf("real Nix let binding %s omitted: %+v", name, got)
				}
			}
			for _, fake := range []string{"phantom", "phantom2", "phantom3", "result"} {
				if sym, ok := got[fake]; ok {
					t.Errorf("Nix string falsely indexed %s: %+v", fake, sym)
				}
			}
		})
	}
}
