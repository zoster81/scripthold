package handler

import (
	"os"
	"testing"
)

// Original Haskell projects are inspected as read-only source evidence.
// Neither the Haskell programs nor their test suites are executed.
func TestSourceSymbolsOriginalHaskellInitialQualification(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Haskell corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "haskell-hadolint-hadolint", language: "haskell",
			search: "insertFromAlias", primary: "src/Hadolint/Rule/DL3006.hs",
			negativeFile: "src/Hadolint/Rule/DL3007.hs", negativeQuery: "insertFromAlias",
			paths: []string{
				"src/Hadolint/Rule/DL3006.hs",
				"src/Hadolint/Rule/DL3007.hs",
				"src/Hadolint/Process.hs",
				"test/Hadolint/Rule/DL3006Spec.hs",
			},
			oracles: []extraAuditEvidence{
				{"src/Hadolint/Rule/DL3006.hs", "module Hadolint.Rule.DL3006 (rule) where"},
				{"src/Hadolint/Rule/DL3006.hs", "rule = customRule check (emptyState Set.empty)"},
				{"src/Hadolint/Rule/DL3006.hs", "code = \"DL3006\""},
				{"src/Hadolint/Rule/DL3006.hs", "BaseImage {digest = Just _} -> newState"},
				{"src/Hadolint/Rule/DL3006.hs", "BaseImage {image = (Image _ \"scratch\")} -> newState"},
				{"src/Hadolint/Rule/DL3006.hs", "if Text.isPrefixOf \"$\" i || Set.member i (state st)"},
				{"src/Hadolint/Rule/DL3006.hs", "insertFromAlias :: BaseImage -> Set.Set Text -> Set.Set Text"},
				{"src/Hadolint/Rule/DL3006.hs", "insertFromAlias BaseImage {alias = Just a} st = st |> Set.insert (unImageAlias a)"},
				{"src/Hadolint/Rule/DL3007.hs", "module Hadolint.Rule.DL3007 (rule) where"},
				{"src/Hadolint/Rule/DL3007.hs", "code = \"DL3007\""},
				{"src/Hadolint/Rule/DL3007.hs", "check (From BaseImage {tag = Just t}) = t /= \"latest\""},
				{"src/Hadolint/Process.hs", "    <> Hadolint.Rule.DL3006.rule"},
				{"src/Hadolint/Process.hs", "    <> Hadolint.Rule.DL3007.rule"},
				{"test/Hadolint/Rule/DL3006Spec.hs", "ruleCatches \"DL3006\" \"FROM debian\""},
				{"test/Hadolint/Rule/DL3006Spec.hs", "it \"local aliases are OK to be untagged\""},
				{"test/Hadolint/Rule/DL3006Spec.hs", "ruleCatchesNot \"DL3006\" \"FROM scratch\""},
				{"test/Hadolint/Rule/DL3006Spec.hs", "onBuildRuleCatches \"DL3006\" $ Text.unlines dockerFile"},
			},
			targets: []extraAuditSymbol{
				{"src/Hadolint/Rule/DL3006.hs", "Hadolint.Rule.DL3006", 1},
				{"src/Hadolint/Rule/DL3006.hs", "rule", 9},
				{"src/Hadolint/Rule/DL3006.hs", "insertFromAlias", 32},
				{"src/Hadolint/Rule/DL3007.hs", "rule", 6},
				{"src/Hadolint/Process.hs", "failures", 137},
				{"test/Hadolint/Rule/DL3006Spec.hs", "spec", 9},
			},
		},
		{
			project: "haskell-koalaman-shellcheck", language: "haskell",
			search: "checkUnquotedExpansions", primary: "src/ShellCheck/Analytics.hs",
			negativeFile: "src/ShellCheck/Formatter/TTY.hs", negativeQuery: "checkUnquotedExpansions",
			paths: []string{
				"src/ShellCheck/Analytics.hs",
				"src/ShellCheck/Analyzer.hs",
				"src/ShellCheck/Formatter/TTY.hs",
			},
			oracles: []extraAuditEvidence{
				{"src/ShellCheck/Analytics.hs", "    ,checkUnquotedExpansions"},
				{"src/ShellCheck/Analytics.hs", "prop_checkUnquotedExpansions1 = verify checkUnquotedExpansions \"rm $(ls)\""},
				{"src/ShellCheck/Analytics.hs", "prop_checkUnquotedExpansions4 = verifyNot checkUnquotedExpansions \"[[ $(foo) == cow ]]\""},
				{"src/ShellCheck/Analytics.hs", "prop_checkUnquotedExpansions11 = verifyNot checkUnquotedExpansions \"ps -p $(pgrep foo)\""},
				{"src/ShellCheck/Analytics.hs", "checkUnquotedExpansions params ="},
				{"src/ShellCheck/Analytics.hs", "unless (null contents || shouldBeSplit t || isQuoteFree (shellType params) tree t || usedAsCommandName tree t) $"},
				{"src/ShellCheck/Analytics.hs", "warn (getId t) 2046 \"Quote this to prevent word splitting.\""},
				{"src/ShellCheck/Analytics.hs", "getCommandNameFromExpansion t `elem` [Just \"seq\", Just \"pgrep\"]"},
				{"src/ShellCheck/Analytics.hs", "checkRedirectToSame params s@(T_Pipeline _ _ list) ="},
				{"src/ShellCheck/Analyzer.hs", "import ShellCheck.Analytics"},
				{"src/ShellCheck/Analyzer.hs", "analyzeScript :: AnalysisSpec -> AnalysisResult"},
				{"src/ShellCheck/Analyzer.hs", "ShellCheck.Analytics.checker spec"},
				{"src/ShellCheck/Analyzer.hs", "ShellCheck.Checks.Commands.checker spec"},
				{"src/ShellCheck/Formatter/TTY.hs", "module ShellCheck.Formatter.TTY (format) where"},
				{"src/ShellCheck/Formatter/TTY.hs", "format :: FormatterOptions -> IO Formatter"},
			},
			targets: []extraAuditSymbol{
				{"src/ShellCheck/Analytics.hs", "prop_checkUnquotedExpansions1", 795},
				{"src/ShellCheck/Analytics.hs", "checkUnquotedExpansions", 808},
				{"src/ShellCheck/Analytics.hs", "checkRedirectToSame", 834},
				{"src/ShellCheck/Analyzer.hs", "analyzeScript", 34},
				{"src/ShellCheck/Analyzer.hs", "checkers", 43},
				{"src/ShellCheck/Formatter/TTY.hs", "format", 45},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
