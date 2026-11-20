package handler

import (
	"os"
	"testing"
)

// Original indented Sass and adjacent consumers are read only. Mixins are
// navigation facts, not proof that Jekyll, Compass or Sass compilation succeeds.
func TestSourceSymbolsOriginalSassAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Sass corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "sass-compass", language: "sass",
			search: "round-corners", primary: "compass-style.org/content/stylesheets/core/_base-classes.sass",
			negativeFile:  "compass-style.org/content/stylesheets/core/_clearing-classes.sass",
			negativeQuery: "round-corners",
			paths: []string{
				"compass-style.org/content/stylesheets/core/_base-classes.sass",
				"compass-style.org/content/stylesheets/core/_clearing-classes.sass",
				"core/templates/ellipsis/ellipsis.sass",
			},
			oracles: []extraAuditEvidence{
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "$default-rounded-corner: 4"},
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "=round-corners($num: $default-rounded-corner)"},
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "=round-bottom-corners($num: $default-rounded-corner)"},
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "  @extend .clear-bottom-right-corner"},
				{"compass-style.org/content/stylesheets/core/_clearing-classes.sass", ".clear-bottom-right-corner"},
				{"compass-style.org/content/stylesheets/partials/_code.scss", "h3 { @include round-corners;"},
				{"compass-style.org/content/stylesheets/partials/_code.scss", "  @include round-bottom-corners"},
				{"core/templates/ellipsis/ellipsis.sass", "$use-mozilla-ellipsis-binding: true"},
			},
			targets: []extraAuditSymbol{
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "default-rounded-corner", 1},
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", ".round-corners-4", 12},
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "round-corners", 18},
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "round-bottom-corners", 25},
				{"compass-style.org/content/stylesheets/core/_base-classes.sass", "square-bottom-right-corner", 68},
				{"compass-style.org/content/stylesheets/core/_clearing-classes.sass", ".clear-bottom-right-corner", 7},
				{"core/templates/ellipsis/ellipsis.sass", "use-mozilla-ellipsis-binding", 3},
			},
		},
		{
			project: "sass-dotfiles-dotfiles.github.com", language: "sass",
			search: ".type-csharp .highlight", primary: "_sass/_pygments.sass",
			negativeFile: "_sass/_minimal.sass", negativeQuery: ".type-csharp .highlight",
			paths: []string{
				"stylesheets/styles.sass",
				"_sass/_minimal.sass",
				"_sass/_pygments.sass",
			},
			oracles: []extraAuditEvidence{
				{"stylesheets/styles.sass", "@import minimal"},
				{"stylesheets/styles.sass", "@import pygments"},
				{"_layouts/default.html", "<link href='/stylesheets/styles.css' rel='stylesheet'>"},
				{"_sass/_minimal.sass", "body\r\n  padding: 50px"},
				{"_sass/_minimal.sass", "header\r\n  width: 270px"},
				{"_sass/_minimal.sass", "code\r\n  font-family: Monaco"},
				{"_sass/_pygments.sass", ".highlight\r\n  background: #ffffff"},
				{"_sass/_pygments.sass", ".type-csharp .highlight"},
				{"_sass/_pygments.sass", "  .k, .kt\r\n    color: #0000FF"},
			},
			targets: []extraAuditSymbol{
				{"_sass/_minimal.sass", "body", 1},
				{"_sass/_minimal.sass", ".wrapper", 46},
				{"_sass/_minimal.sass", "code", 56},
				{"_sass/_minimal.sass", "header", 90},
				{"_sass/_pygments.sass", ".highlight", 1},
				{"_sass/_pygments.sass", ".type-csharp .highlight", 164},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
