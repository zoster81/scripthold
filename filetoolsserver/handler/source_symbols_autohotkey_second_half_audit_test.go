package handler

import (
	"os"
	"testing"
)

// Provider 80 uses two independent original AutoHotkey v2 projects, not the
// earlier Window Spy and Popup Calendar projects. Selected files were read
// completely and their Git blobs checked against immutable upstream revisions.
func TestSourceSymbolsAutoHotkeySecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original AutoHotkey snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "ahk80-thqby-audit-selected", language: "autohotkey",
			search: "parse", primary: "JSON.ahk",
			negativeFile: "Base64.ahk", negativeQuery: "parse",
			paths: []string{"JSON.ahk", "Base64.ahk"},
			oracles: []extraAuditEvidence{
				{"JSON.ahk", "class JSON {"},
				{"JSON.ahk", "static parse(text, keepbooltype := false, as_map := true) {"},
				{"JSON.ahk", "static stringify(obj, expandlevel := unset, space :="},
				{"JSON.ahk", "UC(S, e := 1) {"},
				{"Base64.ahk", "Class Base64 {"},
				{"Base64.ahk", "static Encode(Buf, Codec :="},
				{"Base64.ahk", "static Decode(VarIn, Codec :="},
			},
			targets: []extraAuditSymbol{
				{"JSON.ahk", "JSON", 9},
				{"JSON.ahk", "parse", 18},
				{"JSON.ahk", "UC", 85},
				{"JSON.ahk", "stringify", 104},
				{"JSON.ahk", "CO", 107},
				{"JSON.ahk", "ES", 132},
				{"JSON.ahk", "CL", 156},
				{"Base64.ahk", "Base64", 1},
				{"Base64.ahk", "Encode", 9},
				{"Base64.ahk", "Decode", 26},
			},
		},
		{
			project: "ahk80-evilc-audit-selected", language: "autohotkey",
			search: "CheckboxChanged", primary: "AHK v2/Monitor.ahk",
			negativeFile: "AHK v2/Lib/AutoHotInterception.ahk", negativeQuery: "CheckboxChanged",
			paths: []string{"AHK v2/Monitor.ahk", "AHK v2/Lib/AutoHotInterception.ahk"},
			oracles: []extraAuditEvidence{
				{"AHK v2/Monitor.ahk", "#include Lib\\AutoHotInterception.ahk"},
				{"AHK v2/Monitor.ahk", "CheckboxChanged(id, ctrl, info){"},
				{"AHK v2/Monitor.ahk", "AHI.SubscribeKeyboard(id, false, KeyboardEvent.Bind(id))"},
				{"AHK v2/Monitor.ahk", "MouseAxisEvent(id, info, x, y){"},
				{"AHK v2/Lib/AutoHotInterception.ahk", "class AutoHotInterception {"},
				{"AHK v2/Lib/AutoHotInterception.ahk", "SubscribeKeyboard(id, block, callback, concurrent := false) {"},
				{"AHK v2/Lib/AutoHotInterception.ahk", "SubscribeMouseMoveRelative(id, block, callback, concurrent := false) {"},
				{"AHK v2/Lib/AutoHotInterception.ahk", "CreateContextManager(id) {"},
			},
			targets: []extraAuditSymbol{
				{"AHK v2/Monitor.ahk", "CheckboxChanged", 155},
				{"AHK v2/Monitor.ahk", "FilterMove", 176},
				{"AHK v2/Monitor.ahk", "KeyboardEvent", 201},
				{"AHK v2/Monitor.ahk", "MouseAxisEvent", 217},
				{"AHK v2/Lib/AutoHotInterception.ahk", "AutoHotInterception", 3},
				{"AHK v2/Lib/AutoHotInterception.ahk", "SubscribeKeyboard", 177},
				{"AHK v2/Lib/AutoHotInterception.ahk", "SubscribeMouseMoveRelative", 209},
				{"AHK v2/Lib/AutoHotInterception.ahk", "CreateContextManager", 227},
				{"AHK v2/Lib/AutoHotInterception.ahk", "ContextManager", 247},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
