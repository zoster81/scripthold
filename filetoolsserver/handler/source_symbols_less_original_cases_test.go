package handler

// Maintenance oracles were derived independently from accepted original Less projects.
func originalLessCases() []originalLessCase {
	return []originalLessCase{
		{
			project: "less-antmobile", query: "&-primary", unrelated: "src/global/global.less",
			files: []string{"src/global/theme-default.less", "src/global/global.less", "src/components/button/button.less", "src/components/button/button.patch.less"},
			source: []originalLessText{
				{"src/global/theme-default.less", "--adm-color-primary: #1677ff;"},
				{"src/global/theme-default.less", "--adm-color-success: #00b578;"},
				{"src/global/global.less", "@import './theme-default.less';"},
				{"src/components/button/button.less", "@class-prefix-button: ~'adm-button';"},
				{"src/components/button/button.less", "--color: var(--adm-color-primary);"},
				{"src/components/button/button.less", "&-primary {"},
				{"src/components/button/button.less", "&-disabled {"},
				{"src/components/button/button.patch.less", "background-color: #1677ff;"},
				{"src/components/button/button.patch.less", "border-color: #1677ff;"},
				{"src/components/button/button.tsx", "[`\u0024{classPrefix}-\u0024{props.color}\u0060]: props.color"},
				{"src/components/button/tests/button.test.tsx", "<Button color='primary'>Primary</Button>"},
			},
			targets: []originalLessTarget{
				{"src/global/theme-default.less", ":root", 1},
				{"src/components/button/button.less", "class-prefix-button", 1},
				{"src/components/button/button.less", "&-primary", 82},
				{"src/components/button/button.less", "&-disabled", 100},
				{"src/components/button/button.patch.less", ".adm-button", 3},
				{"src/components/button/button.patch.less", "&-primary", 40},
			},
		},
		{
			project: "less-talkingdata-iview-weapp", query: "primary-color", unrelated: "src/badge/index.less",
			files: []string{"src/styles/_base.less", "src/styles/_mixins.less", "src/button/index.less", "src/badge/index.less"},
			source: []originalLessText{
				{"src/styles/_base.less", "@primary-color          : #2d8cf0;"},
				{"src/styles/_base.less", "@btn-primary-bg         : @primary-color;"},
				{"src/styles/_base.less", "@btn-disable-bg         : @background-color-base;"},
				{"src/styles/_mixins.less", "@import \"./_base.less\";"},
				{"src/button/index.less", ".button-size(@padding; @font-size;) {"},
				{"src/button/index.less", ".btn-primary() {"},
				{"src/button/index.less", ".btn-color(@primary-color);"},
				{"src/button/index.less", "&-primary{"},
				{"src/button/index.less", "&-disabled{"},
				{"src/button/index.less", "@import \"../styles/_base.less\";"},
				{"src/button/index.wxml", "{{ 'i-btn-' + type }}"},
				{"src/button/index.js", "// default, primary, ghost, info, success, warning, error"},
			},
			targets: []originalLessTarget{
				{"src/styles/_base.less", "primary-color", 2},
				{"src/styles/_base.less", "btn-primary-bg", 49},
				{"src/styles/_base.less", "btn-disable-bg", 41},
				{"src/button/index.less", ".button-size(@padding; @font-size;)", 4},
				{"src/button/index.less", ".btn-primary()", 14},
				{"src/button/index.less", ".i-btn", 23},
				{"src/button/index.less", "&-primary", 67},
				{"src/button/index.less", "&-disabled", 119},
			},
		},
	}
}
