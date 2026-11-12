package handler

import (
	"os"
	"testing"
)

// Provider 73 uses two independent original Jinja origins: pallets/flask
// and apache/superset. Selected files were fully inspected and verified
// against immutable upstream Git blobs before development source navigation.
func TestSourceSymbolsJinjaSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Jinja project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "jinja73-flask-audit-selected", language: "jinja",
			search:        "content",
			primary:       "examples/tutorial/flaskr/templates/auth/login.html",
			negativeFile:  "examples/tutorial/flaskr/templates/blog/index.html",
			negativeQuery: "password",
			paths: []string{
				"examples/tutorial/flaskr/templates/base.html",
				"examples/tutorial/flaskr/templates/auth/login.html",
				"examples/tutorial/flaskr/templates/blog/index.html",
			},
			oracles: []extraAuditEvidence{
				{"examples/tutorial/flaskr/templates/base.html", "{% block title %}{% endblock %}"},
				{"examples/tutorial/flaskr/templates/base.html", "{% block content %}{% endblock %}"},
				{"examples/tutorial/flaskr/templates/base.html", "get_flashed_messages()"},
				{"examples/tutorial/flaskr/templates/auth/login.html", "{% extends 'base.html' %}"},
				{"examples/tutorial/flaskr/templates/auth/login.html", "{% block title %}Log In{% endblock %}"},
				{"examples/tutorial/flaskr/templates/auth/login.html", "<input type=\"password\" name=\"password\""},
				{"examples/tutorial/flaskr/templates/blog/index.html", "{% for post in posts %}"},
				{"examples/tutorial/flaskr/templates/blog/index.html", "url_for('blog.update', id=post['id'])"},
			},
			targets: []extraAuditSymbol{
				{"examples/tutorial/flaskr/templates/base.html", "title", 2},
				{"examples/tutorial/flaskr/templates/base.html", "header", 18},
				{"examples/tutorial/flaskr/templates/base.html", "content", 23},
				{"examples/tutorial/flaskr/templates/auth/login.html", "header", 3},
				{"examples/tutorial/flaskr/templates/auth/login.html", "title", 4},
				{"examples/tutorial/flaskr/templates/auth/login.html", "content", 7},
				{"examples/tutorial/flaskr/templates/blog/index.html", "header", 3},
				{"examples/tutorial/flaskr/templates/blog/index.html", "content", 10},
			},
		},
		{
			project: "jinja73-superset-audit-selected", language: "jinja",
			search:        "js_bundle",
			primary:       "superset/templates/superset/partials/asset_bundle.html",
			negativeFile:  "superset/templates/superset/macros.html",
			negativeQuery: "js_bundle",
			paths: []string{
				"superset/templates/superset/macros.html",
				"superset/templates/superset/partials/asset_bundle.html",
				"superset/templates/superset/spa.html",
			},
			oracles: []extraAuditEvidence{
				{"superset/templates/superset/macros.html", "{% macro get_nonce() -%}"},
				{"superset/templates/superset/macros.html", "{%- if csp_nonce is defined -%}"},
				{"superset/templates/superset/partials/asset_bundle.html", "{% import \"superset/macros.html\" as macros %}"},
				{"superset/templates/superset/partials/asset_bundle.html", "{% macro js_bundle(assets_prefix, filename) %}"},
				{"superset/templates/superset/partials/asset_bundle.html", "async nonce=\"{{ macros.get_nonce() }}\""},
				{"superset/templates/superset/partials/asset_bundle.html", "{% macro css_bundle(assets_prefix, filename) %}"},
				{"superset/templates/superset/spa.html", "import css_bundle, js_bundle with context"},
				{"superset/templates/superset/spa.html", "{% block head_meta %}"},
				{"superset/templates/superset/spa.html", "{% block head_css %}"},
				{"superset/templates/superset/spa.html", "{% block tail_js %}"},
				{"superset/templates/superset/spa.html", "language_pack_src"},
			},
			targets: []extraAuditSymbol{
				{"superset/templates/superset/macros.html", "get_nonce", 19},
				{"superset/templates/superset/partials/asset_bundle.html", "js_bundle", 21},
				{"superset/templates/superset/partials/asset_bundle.html", "css_bundle", 31},
				{"superset/templates/superset/spa.html", "title", 24},
				{"superset/templates/superset/spa.html", "head_meta", 33},
				{"superset/templates/superset/spa.html", "head_css", 91},
				{"superset/templates/superset/spa.html", "head_js", 117},
				{"superset/templates/superset/spa.html", "body", 142},
				{"superset/templates/superset/spa.html", "tail_js", 167},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
