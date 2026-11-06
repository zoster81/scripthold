package handler

import (
	"os"
	"testing"
)

// Scenarios come from independent source reads on genuinely Flow-annotated
// implementations, not TypeScript projects or declaration-only Flow stubs.
func TestSourceSymbolsOriginalFlowAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Flow corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "flow-facebook-draft-js", language: "flow",
			search:        "getDefaultKeyBinding",
			primary:       "src/component/utils/getDefaultKeyBinding.js",
			negativeFile:  "src/component/utils/KeyBindingUtil.js",
			negativeQuery: "getDefaultKeyBinding",
			paths: []string{
				"src/component/utils/getDefaultKeyBinding.js",
				"src/component/utils/KeyBindingUtil.js",
				"src/component/base/DraftEditor.react.js",
				"src/component/handlers/edit/editOnKeyDown.js",
				"src/model/constants/DraftEditorCommand.js",
			},
			oracles: []extraAuditEvidence{
				{"src/component/utils/getDefaultKeyBinding.js", "@flow strict"},
				{"src/component/utils/getDefaultKeyBinding.js", "function shouldRemoveWord(e: SyntheticKeyboardEvent<>): boolean {"},
				{"src/component/utils/getDefaultKeyBinding.js", "function getZCommand(e: SyntheticKeyboardEvent<>): ?DraftEditorCommand {"},
				{"src/component/utils/getDefaultKeyBinding.js", "return e.shiftKey ? 'redo' : 'undo';"},
				{"src/component/utils/getDefaultKeyBinding.js", "function getDefaultKeyBinding("},
				{"src/component/utils/getDefaultKeyBinding.js", "return getZCommand(e) || null;"},
				{"src/component/utils/getDefaultKeyBinding.js", "return getBackspaceCommand(e);"},
				{"src/component/utils/KeyBindingUtil.js", "isCtrlKeyCommand(e: SyntheticKeyboardEvent<>): boolean {"},
				{"src/component/utils/KeyBindingUtil.js", "hasCommandModifier(e: SyntheticKeyboardEvent<>): boolean {"},
				{"src/component/base/DraftEditor.react.js", "keyBindingFn: getDefaultKeyBinding,"},
				{"src/component/handlers/edit/editOnKeyDown.js", "function editOnKeyDown("},
				{"src/component/handlers/edit/editOnKeyDown.js", "const command = editor.props.keyBindingFn(e);"},
				{"src/component/handlers/edit/editOnKeyDown.js", "const newState = onKeyCommand(command, editorState, e);"},
				{"src/model/constants/DraftEditorCommand.js", "export type DraftEditorCommand ="},
				{"src/model/constants/DraftEditorCommand.js", "| 'backspace-to-start-of-line'"},
			},
			targets: []extraAuditSymbol{
				{"src/component/utils/getDefaultKeyBinding.js", "shouldRemoveWord", 29},
				{"src/component/utils/getDefaultKeyBinding.js", "getZCommand", 36},
				{"src/component/utils/getDefaultKeyBinding.js", "getDefaultKeyBinding", 61},
				{"src/component/handlers/edit/editOnKeyDown.js", "onKeyCommand", 42},
				{"src/component/handlers/edit/editOnKeyDown.js", "editOnKeyDown", 86},
				{"src/model/constants/DraftEditorCommand.js", "DraftEditorCommand", 19},
			},
		},
		{
			project: "flow-jsflow-facebook-react-native", language: "flow",
			search:        "PixelRatio",
			primary:       "packages/react-native/Libraries/Utilities/PixelRatio.js",
			negativeFile:  "packages/react-native/Libraries/Utilities/Platform.android.js",
			negativeQuery: "PixelRatio",
			paths: []string{
				"packages/react-native/Libraries/Utilities/PixelRatio.js",
				"packages/react-native/Libraries/Utilities/Dimensions.js",
				"packages/react-native/Libraries/Utilities/__tests__/PixelRatio-itest.js",
				"packages/react-native/Libraries/Utilities/Platform.android.js",
			},
			oracles: []extraAuditEvidence{
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "@flow strict-local"},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "const Dimensions = require('./Dimensions').default;"},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "class PixelRatio {"},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "static get(): number {"},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "return Dimensions.get('window').scale;"},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "return Dimensions.get('window').fontScale || PixelRatio.get();"},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "return Math.round(layoutSize * PixelRatio.get());"},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "return Math.round(layoutSize * ratio) / ratio;"},
				{"packages/react-native/Libraries/Utilities/Dimensions.js", "class Dimensions {"},
				{"packages/react-native/Libraries/Utilities/Dimensions.js", "static set(dims: Readonly<DimensionsPayload>): void {"},
				{"packages/react-native/Libraries/Utilities/Dimensions.js", "scale: windowPhysicalPixels.scale,"},
				{"packages/react-native/Libraries/Utilities/__tests__/PixelRatio-itest.js", "Dimensions.set({"},
				{"packages/react-native/Libraries/Utilities/__tests__/PixelRatio-itest.js", "expect(PixelRatio.getFontScale()).toEqual(2);"},
				{"packages/react-native/Libraries/Utilities/__tests__/PixelRatio-itest.js", "expect(PixelRatio.getPixelSizeForLayoutSize(400)).toEqual(800);"},
				{"packages/react-native/Libraries/Utilities/__tests__/PixelRatio-itest.js", "expect(PixelRatio.roundToNearestPixel(8.4)).toEqual(8.5);"},
				{"packages/react-native/Libraries/Utilities/Platform.android.js", "OS: 'android',"},
				{"packages/react-native/Libraries/Utilities/Platform.android.js", "select: <T>(spec: PlatformSelectSpec<T>): T =>"},
			},
			targets: []extraAuditSymbol{
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "PixelRatio", 62},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "getFontScale", 102},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "getPixelSizeForLayoutSize", 111},
				{"packages/react-native/Libraries/Utilities/PixelRatio.js", "roundToNearestPixel", 121},
				{"packages/react-native/Libraries/Utilities/Dimensions.js", "Dimensions", 39},
				{"packages/react-native/Libraries/Utilities/Dimensions.js", "set", 68},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
