package handler

// Original-maintenance tasks were established from source and tests before structural lookup.
func originalHTMLCases() []originalHTMLCase {
	return []originalHTMLCase{
		{
			project: "html-novnc-novnc", query: "noVNC_setting_view_only", unrelated: "vnc_lite.html",
			files: []string{"vnc.html", "vnc_lite.html", "tests/vnc_playback.html"},
			source: []originalHTMLText{
				{"vnc.html", "id=\"noVNC_setting_view_only\""},
				{"vnc.html", "View only"},
				{"vnc.html", "id=\"noVNC_keyboard_button\""},
				{"vnc.html", "id=\"noVNC_connect_button\""},
				{"vnc.html", "id=\"noVNC_setting_resize\""},
				{"app/ui.js", "UI.initSetting('view_only', false);"},
				{"app/ui.js", "UI.addSettingChangeHandler('view_only', UI.updateViewOnly);"},
				{"app/ui.js", "UI.rfb.viewOnly = UI.getSetting('view_only');"},
				{"app/ui.js", "document.getElementById('noVNC_keyboard_button')"},
				{"tests/test.rfb.js", "should not send the keys if we are set as view_only"},
				{"tests/test.rfb.js", "client._viewOnly = true;"},
				{"vnc_lite.html", "sendCtrlAltDelButton"},
			},
			targets: []originalHTMLTarget{
				{"vnc.html", "noVNC_setting_view_only", 215},
				{"vnc.html", "noVNC_keyboard_button", 131},
				{"vnc.html", "noVNC_connect_button", 358},
				{"vnc.html", "noVNC_setting_resize", 230},
			},
		},
		{
			project: "html-jgraph-drawio-tools", query: "createBtn", unrelated: "tools/generate.html",
			files: []string{"index.html", "tools/csv.html", "tools/generate.html", "tools/base64.html"},
			source: []originalHTMLText{
				{"index.html", "href=\"./tools/csv.html\""},
				{"index.html", "CSV Import"},
				{"tools/csv.html", "id=\"url\""},
				{"tools/csv.html", "id=\"title\""},
				{"tools/csv.html", "id=\"desc\""},
				{"tools/csv.html", "id=\"createBtn\""},
				{"tools/csv.html", "id=\"resultLink\""},
				{"tools/csv.html", "id=\"copyBtn\""},
				{"tools/csv.html", "pako.deflateRaw(encodeURIComponent(data))"},
				{"tools/csv.html", "var url = urlInput.value.trim();"},
				{"tools/csv.html", "format: 'csv'"},
				{"tools/csv.html", "DT.copyToClipboard(generatedUrl)"},
				{"tools/csv.html", "../utils/drawio-tools.js"},
				{"utils/drawio-tools.js", "copyToClipboard"},
				{"tools/generate.html", "id=\"engine\""},
			},
			targets: []originalHTMLTarget{
				{"tools/csv.html", "url", 55},
				{"tools/csv.html", "title", 63},
				{"tools/csv.html", "desc", 68},
				{"tools/csv.html", "createBtn", 171},
				{"tools/csv.html", "resultLink", 177},
				{"tools/csv.html", "copyBtn", 173},
			},
		},
	}
}
