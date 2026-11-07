package handler

import (
	"os"
	"testing"
)

// Source-first original SCSS maintenance scenarios. Imports, theme conditions,
// compilation, Jekyll interpolation and browser behavior remain unexecuted.
func TestSourceSymbolsOriginalSCSSAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original SCSS corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "scss-academicpages-academicpages.github.io", language: "scss",
			search: "primary-color", primary: "_sass/theme/_default_light.scss",
			negativeFile: "_sass/_themes.scss", negativeQuery: "primary-color",
			paths: []string{
				"_sass/theme/_default_light.scss",
				"_sass/theme/_default_dark.scss",
				"_sass/layout/_navigation.scss",
				"_sass/layout/_masthead.scss",
				"_sass/_themes.scss",
			},
			oracles: []extraAuditEvidence{
				{"assets/css/main.scss", "\"themes\""},
				{"assets/css/main.scss", "theme/{{ site.site_theme"},
				{"assets/css/main.scss", "\"layout/masthead\""},
				{"assets/css/main.scss", "\"layout/navigation\""},
				{"_sass/theme/_default_light.scss", "$primary-color"},
				{"_sass/theme/_default_light.scss", "--global-masthead-link-color-hover  : mix(#000, #{$gray}, 25%);"},
				{"_sass/theme/_default_dark.scss", "$primary-color"},
				{"_sass/theme/_default_dark.scss", "html[data-theme=\"dark\"]"},
				{"_sass/layout/_navigation.scss", "color: var(--global-masthead-link-color-hover);"},
				{"_sass/layout/_masthead.scss", "background: var(--global-bg-color);"},
				{"_sass/_themes.scss", "$sans-serif"},
			},
			targets: []extraAuditSymbol{
				{"_sass/theme/_default_light.scss", "primary-color", 5},
				{"_sass/theme/_default_light.scss", "gray", 6},
				{"_sass/theme/_default_light.scss", ":root", 30},
				{"_sass/theme/_default_dark.scss", "primary-color", 6},
				{"_sass/theme/_default_dark.scss", "gray", 7},
				{"_sass/_themes.scss", "sans-serif", 18},
			},
		},
		{
			project: "scss-cobiwave-simplefolio", language: "scss",
			search: "secondary-color", primary: "src/sass/abstracts/_variables.scss",
			negativeFile: "src/sass/layout/_footer.scss", negativeQuery: "secondary-color",
			paths: []string{
				"src/sass/abstracts/_variables.scss",
				"src/sass/components/_buttons.scss",
				"src/sass/sections/_hero.scss",
				"src/styles.scss",
				"src/sass/layout/_footer.scss",
			},
			oracles: []extraAuditEvidence{
				{"src/styles.scss", "@import \"./sass/abstracts/variables\";"},
				{"src/styles.scss", "@import \"./sass/components/buttons\";"},
				{"src/styles.scss", "@import \"./sass/sections/hero\";"},
				{"src/sass/abstracts/_variables.scss", "$primary-color: #02aab0;"},
				{"src/sass/abstracts/_variables.scss", "$secondary-color: #00cdac;"},
				{"src/sass/components/_buttons.scss", ".cta-btn--hero {"},
				{"src/sass/components/_buttons.scss", "$secondary-color 100%"},
				{"src/sass/sections/_hero.scss", ".hero-cta {"},
				{"src/sass/layout/_footer.scss", ".footer {"},
				{"src/index.html", "class=\"cta-btn cta-btn--hero\""},
				{"src/data/scrollRevealConfig.js", "element: \".hero-cta\""},
			},
			targets: []extraAuditSymbol{
				{"src/sass/abstracts/_variables.scss", "primary-color", 2},
				{"src/sass/abstracts/_variables.scss", "secondary-color", 3},
				{"src/sass/components/_buttons.scss", ".cta-btn", 2},
				{"src/sass/components/_buttons.scss", ".cta-btn--hero", 25},
				{"src/sass/sections/_hero.scss", "#hero", 1},
				{"src/sass/layout/_footer.scss", ".footer", 1},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
