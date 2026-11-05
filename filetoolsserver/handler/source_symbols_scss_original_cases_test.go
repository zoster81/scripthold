package handler

// Maintenance questions selected from independently read SCSS originals.
func originalSCSSCases() []originalSCSSCase {
	return []originalSCSSCase{
		{
			project: "scss-ianlunn-hover", query: "mediumDuration", unrelated: "scss/_hacks.scss",
			files: []string{"scss/_options.scss", "scss/_mixins.scss", "scss/effects/2d-transitions/_grow.scss", "scss/hover.scss"},
			source: []originalSCSSText{
				{"scss/_options.scss", "$mediumDuration: .3s !default;"},
				{"scss/_options.scss", "$fastDuration: .1s !default;"},
				{"scss/_options.scss", "$nameSpace: 'hvr' !default;"},
				{"scss/_mixins.scss", "@mixin prefixed($property, $value)"},
				{"scss/effects/2d-transitions/_grow.scss", "@mixin grow {"},
				{"scss/effects/2d-transitions/_grow.scss", "@include prefixed(transition-duration, $mediumDuration);"},
				{"scss/hover.scss", "@import \"options\";"},
				{"scss/hover.scss", "@import \"effects/2d-transitions/grow\";"},
				{"scss/hover.scss", ".#{$nameSpace}-grow {"},
			},
			targets: []originalSCSSTarget{
				{"scss/_options.scss", "mediumDuration", 8},
				{"scss/_options.scss", "fastDuration", 7},
				{"scss/_options.scss", "nameSpace", 4},
				{"scss/effects/2d-transitions/_grow.scss", "&:hover", 7},
			},
		},
		{
			project: "scss-nostalgic-css-nes.css", query: "primary-colors", unrelated: "scss/components/_index.scss",
			files: []string{"scss/base/variables.scss", "scss/base/_index.scss", "scss/elements/buttons.scss", "scss/nes-core.scss"},
			source: []originalSCSSText{
				{"scss/base/variables.scss", "$primary-colors: ("},
				{"scss/base/variables.scss", "normal: #209cee,"},
				{"scss/base/variables.scss", "hover: #108de0,"},
				{"scss/base/variables.scss", "$default-colors: ("},
				{"scss/base/variables.scss", "$border-size: 4px !default;"},
				{"scss/base/_index.scss", "@import \"variables.scss\";"},
				{"scss/nes-core.scss", "@import \"base/_index.scss\";"},
				{"scss/nes-core.scss", "@import \"elements/_index.scss\";"},
				{"scss/elements/buttons.scss", ".nes-btn {"},
				{"scss/elements/buttons.scss", "map-get($primary-colors, \"normal\")"},
				{"scss/elements/buttons.scss", "&.is-disabled,"},
				{"scss/elements/buttons.scss", "@each $type in $types {"},
				{"scss/components/_index.scss", "@charset \"utf-8\";"},
			},
			targets: []originalSCSSTarget{
				{"scss/base/variables.scss", "primary-colors", 22},
				{"scss/base/variables.scss", "default-colors", 13},
				{"scss/base/variables.scss", "border-size", 11},
				{"scss/elements/buttons.scss", ".nes-btn", 35},
				{"scss/elements/buttons.scss", "&.is-disabled", 58},
			},
		},
	}
}
