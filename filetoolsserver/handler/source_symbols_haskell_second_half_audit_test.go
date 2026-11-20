package handler

import (
	"os"
	"testing"
)

// Provider 59: two new Git-original Haskell projects, Aeson and
// optparse-applicative, distinct from the first-stage Hadolint/ShellCheck.
// Only bounded source-navigation evidence is checked; original code is never
// built or executed.
func TestSourceSymbolsHaskellSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Haskell project corpus not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "haskell59-aeson-audit-selected", language: "haskell",
			search: "lookup", primary: "src/Data/Aeson/KeyMap.hs",
			negativeFile: "src/Data/Aeson/Key.hs", negativeQuery: "lookup",
			paths: []string{
				"src/Data/Aeson/KeyMap.hs",
				"src/Data/Aeson/Key.hs",
				"src/Data/Aeson/TH.hs",
				"tests/PropertyKeys.hs",
			},
			oracles: []extraAuditEvidence{
				{"src/Data/Aeson/KeyMap.hs", "module Data.Aeson.KeyMap ("},
				{"src/Data/Aeson/KeyMap.hs", "newtype KeyMap v = KeyMap { unKeyMap :: Map Key v }"},
				{"src/Data/Aeson/KeyMap.hs", "lookup :: Key -> KeyMap v -> Maybe v"},
				{"src/Data/Aeson/KeyMap.hs", "lookup t tm = M.lookup t (unKeyMap tm)"},
				{"src/Data/Aeson/KeyMap.hs", "lookup t tm = H.lookup t (unKeyMap tm)"},
				{"src/Data/Aeson/Key.hs", "newtype Key = Key { unKey :: Text }"},
				{"src/Data/Aeson/Key.hs", "fromText :: Text -> Key"},
				{"src/Data/Aeson/Key.hs", "toText :: Key -> Text"},
				{"src/Data/Aeson/TH.hs", "lookupFieldOmit :: Maybe a -> (Value -> Parser a) -> String -> String -> Object -> Key -> Parser a"},
				{"src/Data/Aeson/TH.hs", "case KM.lookup key obj of"},
				{"src/Data/Aeson/TH.hs", "lookupFieldNoOmit :: (Value -> Parser a) -> String -> String -> Object -> Key -> Parser a"},
				{"tests/PropertyKeys.hs", "keysTests :: TestTree"},
			},
			targets: []extraAuditSymbol{
				{"src/Data/Aeson/KeyMap.hs", "KeyMap", 142},
				{"src/Data/Aeson/KeyMap.hs", "lookup", 176},
				{"src/Data/Aeson/Key.hs", "Key", 43},
				{"src/Data/Aeson/Key.hs", "fromText", 52},
				{"src/Data/Aeson/TH.hs", "lookupFieldOmit", 1121},
				{"src/Data/Aeson/TH.hs", "lookupFieldNoOmit", 1130},
				{"tests/PropertyKeys.hs", "keysTests", 26},
			},
		},
		{
			project: "haskell59-optparse-audit-selected", language: "haskell",
			search:        "editDistance",
			primary:       "src/Options/Applicative/Help/Levenshtein.hs",
			negativeFile:  "src/Options/Applicative/Common.hs",
			negativeQuery: "editDistance",
			paths: []string{
				"src/Options/Applicative/Help/Levenshtein.hs",
				"src/Options/Applicative/Extra.hs",
				"src/Options/Applicative/Common.hs",
				"src/Options/Applicative/Help.hs",
			},
			oracles: []extraAuditEvidence{
				{"src/Options/Applicative/Help/Levenshtein.hs", "module Options.Applicative.Help.Levenshtein ("},
				{"src/Options/Applicative/Help/Levenshtein.hs", "editDistance :: Eq a => [a] -> [a] -> Int"},
				{"src/Options/Applicative/Help/Levenshtein.hs", "editDistance a b ="},
				{"src/Options/Applicative/Extra.hs", "isClose a   = editDistance a arg < 3"},
				{"src/Options/Applicative/Extra.hs", "parserFailure :: ParserPrefs -> ParserInfo a"},
				{"src/Options/Applicative/Common.hs", "optMatches :: MonadP m => Bool -> OptReader a -> OptWord -> Maybe (StateT Args m a)"},
				{"src/Options/Applicative/Help.hs", "import Options.Applicative.Help.Levenshtein"},
				{"tests/test.hs", "prop_edit_distance_gezero a b = editDistance a b >= 0"},
				{"tests/test.hs", "editDistance (as ++ [a,b] ++ bs) (as ++ [b,a] ++ bs) === 1"},
			},
			targets: []extraAuditSymbol{
				{"src/Options/Applicative/Help/Levenshtein.hs", "editDistance", 22},
				{"src/Options/Applicative/Extra.hs", "parserFailure", 172},
				{"src/Options/Applicative/Common.hs", "optMatches", 86},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
