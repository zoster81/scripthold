package handler

// Independently source-confirmed CSS maintenance questions and unrelated alternatives.
func originalCSSCases() []originalCSSCase {
	return []originalCSSCase{
		{
			project: "css-animate-css-animate.css", query: ".animated.slower",
			unrelated: "source/fading_entrances/fadeIn.css",
			files:     []string{"source/_vars.css", "source/_base.css", "source/animate.css", "source/fading_entrances/fadeIn.css"},
			source: []originalCSSText{
				{"source/_vars.css", "--animate-duration: 1s;"},
				{"source/_vars.css", "--animate-delay: 1s;"},
				{"source/_base.css", "animation-duration: var(--animate-duration);"},
				{"source/_base.css", ".animated.slower {"},
				{"source/_base.css", "calc(var(--animate-duration) * 3)"},
				{"source/_base.css", "(prefers-reduced-motion: reduce)"},
				{"source/_base.css", "animation-duration: 1ms !important;"},
				{"source/animate.css", "@import '_vars.css';"},
				{"source/animate.css", "@import '_base.css';"},
				{"source/animate.css", "@import 'fading_entrances/fadeIn.css';"},
				{"source/fading_entrances/fadeIn.css", "fadeIn"},
			},
			targets: []originalCSSTarget{
				{"source/_vars.css", ":root", 1},
				{"source/_base.css", ".animated", 1},
				{"source/_base.css", ".animated.slower", 54},
				{"source/_base.css", ".animated.faster", 42},
			},
		},
		{
			project: "css-necolas-normalize.css", query: "abbr[title]", unrelated: "test.html",
			files: []string{"normalize.css"},
			source: []originalCSSText{
				{"normalize.css", "html {"},
				{"normalize.css", "line-height: 1.15;"},
				{"normalize.css", "body {"},
				{"normalize.css", "margin: 0;"},
				{"normalize.css", "h1 {"},
				{"normalize.css", "abbr[title] {"},
				{"normalize.css", "text-decoration: underline dotted;"},
				{"test.html", "href=\"normalize.css\""},
				{"test.html", "should have a line height of 1.15"},
				{"test.html", "should have no margin (opinionated)"},
				{"test.html", "<h2 class=\"Test-describe\"><code>html</code></h2>"},
			},
			targets: []originalCSSTarget{
				{"normalize.css", "html", 11},
				{"normalize.css", "body", 23},
				{"normalize.css", "h1", 40},
				{"normalize.css", "abbr[title]", 85},
			},
		},
	}
}
