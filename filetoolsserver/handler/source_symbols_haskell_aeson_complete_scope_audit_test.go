package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Original Aeson source was completely read before the independent oracle.
// The structural inventory includes module headers, named top-level type
// signatures, type declarations and direct class/instance body declarations
// in both CPP branches. Instance members are currently indexed with module
// ownership (no semantic typeclass association). Record fields, local let/where
// bindings, constructor patterns and symbolic operators remain unsupported.
// Haskell CPP branch activity is not evaluated by source_symbols.
func TestHaskellAesonSelectedOriginalCompleteDeclarationInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original Aeson source corpus not configured")
	}
	project := filepath.Join(root, "haskell59-aeson-audit-selected")
	type decl struct {
		file                       string
		line                       int
		name, kind, native, parent string
	}
	sourceFirstOracle := []decl{
		{"src/Data/Aeson/KeyMap.hs", 14, "Data.Aeson.KeyMap", "module", "module", ""},
		{"src/Data/Aeson/KeyMap.hs", 142, "KeyMap", "type", "newtype", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 147, "empty", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 151, "null", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 155, "size", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 159, "singleton", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 163, "member", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 167, "delete", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 171, "alterF", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 176, "lookup", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 182, "insert", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 188, "insertWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 192, "map", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 198, "mapWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 201, "foldMapWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 204, "foldr", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 207, "foldr'", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 210, "foldl", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 213, "foldl'", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 219, "foldrWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 224, "traverse", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 229, "traverseWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 235, "fromListWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 245, "fromList", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 251, "toList", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 257, "elems", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 262, "toAscList", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 267, "difference", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 272, "union", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 276, "unionWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 280, "unionWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 284, "intersection", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 288, "intersectionWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 292, "intersectionWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 296, "keys", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 300, "toHashMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 304, "fromHashMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 308, "toMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 312, "fromMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 315, "coercionToHashMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 319, "coercionToMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 324, "mapKeyVal", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 330, "filter", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 334, "filterWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 338, "mapMaybe", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 342, "mapMaybeWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 356, "KeyMap", "type", "newtype", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 360, "empty", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 364, "null", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 368, "size", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 372, "singleton", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 376, "member", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 380, "delete", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 384, "alterF", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 389, "lookup", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 395, "insert", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 401, "insertWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 405, "map", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 411, "mapWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 414, "foldMapWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 417, "foldr", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 420, "foldr'", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 423, "foldl", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 426, "foldl'", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 432, "foldrWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 437, "traverse", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 442, "traverseWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 448, "fromListWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 454, "fromList", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 460, "toList", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 466, "elems", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 471, "toAscList", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 476, "difference", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 481, "union", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 485, "unionWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 489, "unionWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 493, "intersection", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 497, "intersectionWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 501, "intersectionWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 505, "keys", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 509, "toHashMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 513, "fromHashMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 517, "toMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 521, "fromMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 524, "coercionToHashMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 528, "coercionToMap", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 533, "mapKeyVal", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 539, "filter", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 543, "filterWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 547, "mapMaybe", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 551, "mapMaybeWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 571, "alignWith", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 575, "alignWithKey", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 579, "toHashMapText", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 583, "fromHashMapText", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 589, "toMapText", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 595, "fromMapText", "function", "type-signature", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/Key.hs", 14, "Data.Aeson.Key", "module", "module", ""},
		{"src/Data/Aeson/Key.hs", 43, "Key", "type", "newtype", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 46, "fromString", "function", "type-signature", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 49, "toString", "function", "type-signature", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 52, "fromText", "function", "type-signature", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 55, "toText", "function", "type-signature", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 59, "coercionToText", "function", "type-signature", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 64, "toShortText", "function", "type-signature", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 68, "fromShortText", "function", "type-signature", "Data.Aeson.Key"},
		{"src/Data/Aeson/TH.hs", 82, "Data.Aeson.TH", "module", "module", ""},
		{"src/Data/Aeson/TH.hs", 155, "deriveJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 168, "deriveJSON1", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 181, "deriveJSON2", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 204, "deriveToJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 214, "deriveToJSON1", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 224, "deriveToJSON2", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 232, "deriveToJSONCommon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 245, "mkToJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 253, "mkLiftToJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 261, "mkLiftToJSON2", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 266, "mkToJSONCommon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 274, "mkToEncoding", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 282, "mkLiftToEncoding", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 290, "mkLiftToEncoding2", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 295, "mkToEncodingCommon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 301, "LetInsert", "type", "type", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 306, "consToValue", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 345, "conStr", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 350, "conTxt", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 354, "conString", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 358, "isNullary", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 364, "opaqueSumToValue", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 373, "recordSumToValue", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 424, "argsToValue", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 509, "array", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 531, "objectE", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 537, "mconcatE", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 542, "fromPairsE", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 549, "pairE", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 571, "deriveFromJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 581, "deriveFromJSON1", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 591, "deriveFromJSON2", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 599, "deriveFromJSONCommon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 611, "mkParseJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 619, "mkLiftParseJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 627, "mkLiftParseJSON2", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 632, "mkParseJSONCommon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 641, "consFromJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 876, "parseNullaryMatches", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 897, "parseUnaryMatches", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 909, "parseRecord", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 959, "nonEmpty", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 963, "getValField", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 973, "matchCases", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 978, "parseArgs", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1058, "parseProduct", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1101, "matchFailed", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1111, "parseTypeMismatch", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1121, "lookupFieldOmit", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1130, "lookupFieldNoOmit", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1136, "unknownFieldFail", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1141, "noArrayFail", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1144, "noObjectFail", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1147, "firstElemNoStringFail", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1150, "wrongPairCountFail", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1155, "noStringFail", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1158, "noMatchFail", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1162, "not2ElemArray", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1165, "conNotFoundFail2ElemArray", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1170, "conNotFoundFailObjectSingleField", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1175, "conNotFoundFailTaggedObject", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1180, "parseTypeMismatch'", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1190, "deriveJSONBoth", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1204, "deriveJSONClass", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1238, "mkFunCommon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1262, "FunArg", "type", "data", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1264, "dispatchFunByType", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1312, "dispatchToJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1315, "dispatchOmitField", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1318, "dispatchParseJSON", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1321, "dispatchOmittedField", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1329, "buildTypeInstance", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1455, "deriveConstraint", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1554, "checkExistentialContext", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1600, "TyVarMap", "type", "type", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1603, "hasKindStar", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1609, "isStarOrVar", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1615, "newNameList", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1621, "hasKindVarChain", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1629, "tyKind", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1635, "varTToNameMaybe", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1642, "varTToName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1645, "flatten3", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1648, "triple", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1652, "applyTyCon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1656, "isTyVar", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1671, "isInTypeFamilyApp", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1694, "unSigT", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1701, "allDistinct", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1711, "mentionsName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1722, "predMentionsName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1736, "unapplyTy", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1756, "uncurryTy", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1767, "uncurryKind", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1770, "createKindChain", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1778, "conNameExp", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1786, "fieldLabel", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1792, "valueConName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1800, "applyCon", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1809, "canEtaReduce", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1823, "applySubstitutionKind", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1826, "substNameWithKind", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1829, "substNamesWithKindStar", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1838, "derivingKindError", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1858, "etaReductionError", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1865, "datatypeContextError", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1876, "outOfPlaceTyVarError", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1892, "existentialContextError", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1904, "Arity", "type", "data", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1908, "Direction", "type", "data", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1911, "JSONFun", "type", "data", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1914, "ToJSONFun", "type", "data", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1916, "targetToJSONFun", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1921, "JSONClass", "type", "data", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1925, "toJSONClass", "variable", "value-binding", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1926, "toJSON1Class", "variable", "value-binding", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1927, "toJSON2Class", "variable", "value-binding", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1928, "fromJSONClass", "variable", "value-binding", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1929, "fromJSON1Class", "variable", "value-binding", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1930, "fromJSON2Class", "variable", "value-binding", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1932, "jsonClassName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1940, "jsonFunOmitName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1951, "jsonFunValName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1962, "jsonFunListName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1973, "jsonFunValOrListName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1979, "arityInt", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1982, "allowExQuant", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1991, "StarKindStatus", "type", "data", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 1997, "canRealizeKindStar", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 2005, "starKindStatusToName", "function", "type-signature", "Data.Aeson.TH"},
		{"src/Data/Aeson/TH.hs", 2011, "catKindVarNames", "function", "type-signature", "Data.Aeson.TH"},
		{"tests/PropertyKeys.hs", 4, "PropertyKeys", "module", "module", ""},
		{"tests/PropertyKeys.hs", 26, "keysTests", "function", "type-signature", "PropertyKeys"},
	}
	// Source-read independent direct instance members (not derived from tool output).
	sourceFirstOracle = append(sourceFirstOracle, []decl{
		{"src/Data/Aeson/Key.hs", 76, "readPrec", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 79, "showsPrec", "function", "function-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 82, "fromString", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 89, "rnf", "function", "function-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 95, "mempty", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 96, "mappend", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 100, "lift", "function", "function-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 102, "lift", "function", "function-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 106, "liftTyped", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 108, "liftTyped", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 113, "arbitrary", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 114, "shrink", "function", "function-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 118, "coarbitrary", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/Key.hs", 122, "function", "variable", "value-binding", "Data.Aeson.Key"},
		{"src/Data/Aeson/KeyMap.hs", 605, "readPrec", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 610, "readListPrec", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 613, "showsPrec", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 617, "foldMap", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 619, "foldr", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 620, "foldr'", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 621, "foldl", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 622, "foldl'", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 623, "null", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 624, "length", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 627, "traverse", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 633, "mempty", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 634, "mappend", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 638, "Item", "type", "type", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 639, "fromList", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 640, "toList", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 647, "lift", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 650, "liftTyped", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 652, "liftTyped", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 661, "hashWithSalt", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 665, "hashWithSalt", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 673, "rnf", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 680, "imap", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 683, "ifoldr", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 686, "itraverse", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 696, "unzip", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 699, "zipWith", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 702, "izipWith", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 705, "alignWith", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 708, "ialignWith", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 711, "nil", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 718, "filter", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 719, "mapMaybe", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 724, "ifilter", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 725, "imapMaybe", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 735, "liftArbitrary", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 736, "liftShrink", "function", "function-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 740, "arbitrary", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 741, "shrink", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 745, "coarbitrary", "variable", "value-binding", "Data.Aeson.KeyMap"},
		{"src/Data/Aeson/KeyMap.hs", 749, "function", "variable", "value-binding", "Data.Aeson.KeyMap"},
	}...)
	files := []string{"src/Data/Aeson/KeyMap.hs", "src/Data/Aeson/Key.hs", "src/Data/Aeson/TH.hs", "tests/PropertyKeys.hs"}
	originals := map[string][]string{}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		path := filepath.Join(project, filepath.FromSlash(f))
		requireAdditionalOriginalFile(t, path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > 1<<20 {
			t.Fatalf("oversized source %s", f)
		}
		originals[f] = strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
		paths = append(paths, path)
	}
	expected := map[string]decl{}
	for _, d := range sourceFirstOracle {
		lines, found := originals[d.file]
		if !found || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], d.name) {
			t.Fatalf("invalid original declaration oracle %s:%d %s", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, exists := expected[key]; exists {
			t.Fatalf("duplicate oracle location %s", key)
		}
		expected[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "haskell", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: len(files), MaxSymbols: 512,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(files) {
		t.Fatalf("incomplete Haskell Aeson outline err=%v failed=%+v count=%d truncated=%v complete=%v files=%d", err, failed, len(out.Symbols), out.Truncated, out.CoverageComplete, out.FilesParsed)
	}
	actual := map[string]bool{}
	for _, symbol := range out.Symbols {
		relative, err := filepath.Rel(project, symbol.Path)
		if err != nil {
			t.Errorf("unexpected path %s", symbol.Path)
			continue
		}
		key := filepath.ToSlash(relative) + ":" + strconv.Itoa(symbol.NameRange.Start.Line)
		if actual[key] {
			t.Errorf("duplicate indexed location %s", key)
		}
		actual[key] = true
		d, ok := expected[key]
		if !ok {
			t.Errorf("false top-level Haskell declaration %s kind=%s name=%s parent=%s", key, symbol.Kind, symbol.Name, symbol.ParentQualifiedName)
			continue
		}
		if string(symbol.Kind) != d.kind || symbol.Name != d.name || symbol.NativeKind != d.native || symbol.ParentQualifiedName != d.parent {
			t.Errorf("Haskell symbol incorrect at %s: got %s/%s native=%s owner=%s; want %+v", key, symbol.Kind, symbol.Name, symbol.NativeKind, symbol.ParentQualifiedName, d)
		}
	}
	missing := 0
	for key, d := range expected {
		if !actual[key] {
			missing++
			if missing <= 15 {
				t.Errorf("missing selected original Haskell declaration %s %s", key, d.name)
			}
		}
	}
	if missing > 15 {
		t.Errorf("and %d more missing original declaration locations", missing-15)
	}
	if len(out.Symbols) != len(expected) {
		t.Errorf("full source-first supported declaration inventory %d vs handler %d", len(expected), len(out.Symbols))
	}
	t.Logf("Aeson fully read originals=%d; independent direct named declarations=%d; matched=%d; missing=%d; no CPP runtime claim", len(files), len(expected), len(actual), missing)
}
