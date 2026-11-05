package handler

// Two accepted independent Android XML projects, source-read before structural checks.
func originalXMLCases() []originalXMLCase {
	return []originalXMLCase{
		{
			project: "xml-ganadist-cairoclock", query: "hello_world",
			unrelated: "mobile/src/main/res/menu/blank.xml",
			files: []string{
				"mobile/src/main/res/values/strings.xml",
				"mobile/src/main/res/layout/activity_blank.xml",
				"mobile/src/main/AndroidManifest.xml",
				"mobile/src/main/res/menu/blank.xml",
			},
			source: []originalXMLText{
				{"mobile/src/main/res/values/strings.xml", "<string name=\"app_name\">CairoClock</string>"},
				{"mobile/src/main/res/values/strings.xml", "<string name=\"hello_world\">Hello world!</string>"},
				{"mobile/src/main/res/values/strings.xml", "<string name=\"action_settings\">Settings</string>"},
				{"mobile/src/main/res/layout/activity_blank.xml", "android:text=\"@string/hello_world\""},
				{"mobile/src/main/AndroidManifest.xml", "android:label=\"@string/app_name\""},
				{"mobile/src/main/res/menu/blank.xml", "@string/action_settings"},
			},
			targets: []originalXMLTarget{
				{"mobile/src/main/res/values/strings.xml", "app_name", 4},
				{"mobile/src/main/res/values/strings.xml", "hello_world", 6},
				{"mobile/src/main/res/values/strings.xml", "action_settings", 7},
			},
		},
		{
			project: "xml-pramodmg-localization", query: "lasttext",
			unrelated: "app/src/main/res/layout/toolbar.xml",
			files: []string{
				"app/src/main/res/values/strings.xml",
				"app/src/main/res/values-hi/strings.xml",
				"app/src/main/res/values-ta/strings.xml",
				"app/src/main/res/layout/activity_main.xml",
				"app/src/main/AndroidManifest.xml",
			},
			source: []originalXMLText{
				{"app/src/main/res/values/strings.xml", "<string name=\"lasttext\"> Welcome Buddy </string>"},
				{"app/src/main/res/values-hi/strings.xml", "<string name=\"lasttext\"> आपका स्वागत है दोस्त </string>"},
				{"app/src/main/res/values-ta/strings.xml", "<string name=\"lasttext\"> வரவேற்பு நண்பா </string>"},
				{"app/src/main/res/layout/activity_main.xml", "android:text=\"@string/lasttext\""},
				{"app/src/main/res/layout/activity_main.xml", "android:id=\"@+id/textView2\""},
				{"app/src/main/res/values/strings.xml", "<string name=\"toolbar_heading\"> Select Language </string>"},
				{"app/src/main/res/values-ta/strings.xml", "<string name=\"but2\">Change Languaguage</string>"},
			},
			targets: []originalXMLTarget{
				{"app/src/main/res/values/strings.xml", "lasttext", 11},
				{"app/src/main/res/values-hi/strings.xml", "lasttext", 12},
				{"app/src/main/res/values-ta/strings.xml", "lasttext", 12},
				{"app/src/main/res/values/strings.xml", "toolbar_heading", 14},
			},
		},
	}
}
