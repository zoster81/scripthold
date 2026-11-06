package handler

import (
	"os"
	"testing"
)

// These original JavaScript maintenance tasks and source oracles were
// established by bounded direct reads and grep before source_* lookups.
func TestSourceSymbolsOriginalJavaScriptAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original JavaScript corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "javascript-aksonov-react-native-router-flux", language: "javascript",
			search: "createReducer", primary: "src/Reducer.js",
			negativeFile: "src/NavBar.js", negativeQuery: "createReducer",
			paths: []string{
				"src/Reducer.js", "src/State.js", "src/Store.js",
				"src/ActionConst.js", "__tests__/scenes.test.js",
			},
			oracles: []extraAuditEvidence{
				{"src/Reducer.js", "export default function createReducer(store) {"},
				{"src/Reducer.js", "if (type === ActionConst.POP_TO) {"},
				{"src/Reducer.js", "nextScene = getActiveState(newState).routeName;"},
				{"src/Reducer.js", "return nextScene === routeName ? newState : state;"},
				{"src/Reducer.js", "if (type === ActionConst.REPLACE) {"},
				{"src/Reducer.js", "const res = popPrevious(newState, routeName);"},
				{"src/State.js", "export function getActiveState(param, parent) {"},
				{"src/State.js", "export function popPrevious(state, routeName) {"},
				{"src/ActionConst.js", "export const POP_TO = 'REACT_NATIVE_ROUTER_FLUX_POP_TO';"},
				{"src/Store.js", "import createReducer from './Reducer';"},
				{"src/Store.js", "[ActionConst.POP_TO]: 'popTo',"},
				{"src/Store.js", "const reducer = createReducer(this);"},
				{"src/Store.js", "this.dispatch({ type: ActionConst.POP_TO, routeName, params });"},
				{"__tests__/scenes.test.js", "// navigationStore.popTo('NOT_A_REAL_SCENE');"},
				{"__tests__/scenes.test.js", "// TODO: fix react-native-gesture-handler error"},
			},
			targets: []extraAuditSymbol{
				{"src/Reducer.js", "createReducer", 6},
				{"src/State.js", "getActiveState", 38},
				{"src/State.js", "popPrevious", 75},
				{"src/Store.js", "popTo", 939},
				{"src/ActionConst.js", "POP_TO", 7},
			},
		},
		{
			project: "javascript-final-form-react-final-form", language: "javascript",
			search: "AutoSave", primary: "examples/auto-save-with-debounce/AutoSave.js",
			negativeFile:  "examples/auto-save-selective-debounce/AutoSave.js",
			negativeQuery: "componentWillReceiveProps",
			paths: []string{
				"examples/auto-save-with-debounce/AutoSave.js",
				"examples/auto-save-with-debounce/index.js",
				"examples/auto-save-selective-debounce/AutoSave.js",
			},
			oracles: []extraAuditEvidence{
				{"examples/auto-save-with-debounce/AutoSave.js", "class AutoSave extends React.Component {"},
				{"examples/auto-save-with-debounce/AutoSave.js", "componentWillReceiveProps(nextProps) {"},
				{"examples/auto-save-with-debounce/AutoSave.js", "clearTimeout(this.timeout);"},
				{"examples/auto-save-with-debounce/AutoSave.js", "this.timeout = setTimeout(this.save, this.props.debounce);"},
				{"examples/auto-save-with-debounce/AutoSave.js", "save = async () => {"},
				{"examples/auto-save-with-debounce/AutoSave.js", "const difference = diff(this.state.values, values);"},
				{"examples/auto-save-with-debounce/AutoSave.js", "this.promise = save(difference);"},
				{"examples/auto-save-with-debounce/AutoSave.js", "<FormSpy {...props} subscription={{ values: true }} component={AutoSave} />"},
				{"examples/auto-save-with-debounce/index.js", "import AutoSave from \"./AutoSave\";"},
				{"examples/auto-save-with-debounce/index.js", "const save = async (values) => {"},
				{"examples/auto-save-with-debounce/index.js", "<AutoSave debounce={1000} save={save} />"},
				{"examples/auto-save-selective-debounce/AutoSave.js", "class AutoSave extends React.Component {"},
				{"examples/auto-save-selective-debounce/AutoSave.js", "componentDidUpdate() {"},
				{"examples/auto-save-selective-debounce/AutoSave.js", "splitValues = (values) => {"},
				{"examples/auto-save-selective-debounce/AutoSave.js", "debounced.includes(key)"},
			},
			targets: []extraAuditSymbol{
				{"examples/auto-save-with-debounce/AutoSave.js", "AutoSave", 5},
				{"examples/auto-save-with-debounce/AutoSave.js", "componentWillReceiveProps", 11},
				{"examples/auto-save-with-debounce/AutoSave.js", "save", 18},
				{"examples/auto-save-with-debounce/index.js", "save", 9},
				{"examples/auto-save-selective-debounce/AutoSave.js", "AutoSave", 8},
				{"examples/auto-save-selective-debounce/AutoSave.js", "splitValues", 39},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
