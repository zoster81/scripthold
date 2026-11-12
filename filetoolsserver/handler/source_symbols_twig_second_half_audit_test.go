package handler

import (
	"os"
	"testing"
)

// Provider 72: phpMyAdmin and Sylius are independent original Twig origins,
// not the earlier Symfony profiler, Craft CMS, or storefront audits.
// All selected sources were read in full and verified against pinned Git blobs.
func TestSourceSymbolsTwigSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Twig projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "twig72-phpmyadmin-audit-selected", language: "twig",
			search: "content", primary: "resources/templates/setup/home/index.twig",
			negativeFile: "resources/templates/setup/base.twig", negativeQuery: "title",
			paths: []string{
				"resources/templates/setup/base.twig",
				"resources/templates/setup/home/index.twig",
				"resources/templates/setup/form/index.twig",
			},
			oracles: []extraAuditEvidence{
				{"resources/templates/setup/base.twig", "{% block content %}{% endblock %}"},
				{"resources/templates/setup/base.twig", "for page in pages"},
				{"resources/templates/setup/home/index.twig", "{% extends 'setup/base.twig' %}"},
				{"resources/templates/setup/home/index.twig", "{% block content %}"},
				{"resources/templates/setup/home/index.twig", "server_count > 0"},
				{"resources/templates/setup/home/index.twig", "name=\"submit_download\""},
				{"resources/templates/setup/form/index.twig", "{{ page|raw }}"},
			},
			targets: []extraAuditSymbol{
				{"resources/templates/setup/base.twig", "content", 45},
				{"resources/templates/setup/home/index.twig", "content", 2},
				{"resources/templates/setup/form/index.twig", "content", 2},
			},
		},
		{
			project: "twig72-sylius-audit-selected", language: "twig",
			search: "form_row", primary: "src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig",
			negativeFile:  "src/Sylius/Bundle/ShopBundle/templates/cart/index.html.twig",
			negativeQuery: "form_row",
			paths: []string{
				"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig",
				"src/Sylius/Bundle/ShopBundle/templates/cart/index.html.twig",
				"src/Sylius/Bundle/AdminBundle/templates/order/show.html.twig",
			},
			oracles: []extraAuditEvidence{
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "{% block form_row -%}"},
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "{%- block form_errors -%}"},
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "{% block _sylius_shop_product_review_rating_widget %}"},
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "if 'csrf_token' in child.vars.block_prefixes"},
				{"src/Sylius/Bundle/ShopBundle/templates/cart/index.html.twig", "{% hook 'index' with"},
				{"src/Sylius/Bundle/AdminBundle/templates/order/show.html.twig", "{% block body %}"},
				{"src/Sylius/Bundle/AdminBundle/templates/order/show.html.twig", "{% hook 'show#stylesheets' with"},
			},
			targets: []extraAuditSymbol{
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "form_row", 3},
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "form_errors", 19},
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "checkbox_row", 41},
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "radio_row", 58},
				{"src/Sylius/Bundle/ShopBundle/templates/form/theme.html.twig", "form_end", 77},
				{"src/Sylius/Bundle/ShopBundle/templates/cart/index.html.twig", "title", 5},
				{"src/Sylius/Bundle/ShopBundle/templates/cart/index.html.twig", "content", 7},
				{"src/Sylius/Bundle/AdminBundle/templates/order/show.html.twig", "body", 13},
				{"src/Sylius/Bundle/AdminBundle/templates/order/show.html.twig", "stylesheets", 17},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
