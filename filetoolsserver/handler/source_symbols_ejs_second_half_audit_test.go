package handler

import (
	"os"
	"testing"
)

// Provider 75 qualifies two new original EJS project origins, not the
// previously used Todo and ecommerce cases. Originals were fully inspected,
// independently grep-checked and verified against immutable upstream Git blobs.
func TestSourceSymbolsEJSSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original EJS project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "ejs75-express-audit-selected", language: "ejs",
			search:        "user",
			primary:       "examples/route-separation/views/users/edit.ejs",
			negativeFile:  "examples/route-separation/views/posts/index.ejs",
			negativeQuery: "user",
			paths: []string{
				"examples/route-separation/views/users/edit.ejs",
				"examples/route-separation/views/users/view.ejs",
				"examples/route-separation/views/users/index.ejs",
				"examples/route-separation/views/posts/index.ejs",
			},
			oracles: []extraAuditEvidence{
				{"examples/route-separation/views/users/edit.ejs", "<div id=\"user\">"},
				{"examples/route-separation/views/users/edit.ejs", "<form action=\"?_method=put\" method=\"post\">"},
				{"examples/route-separation/views/users/edit.ejs", "name=\"user[name]\""},
				{"examples/route-separation/views/users/edit.ejs", "name=\"user[email]\""},
				{"examples/route-separation/views/users/view.ejs", "<p>Email: <%= user.email %></p>"},
				{"examples/route-separation/views/users/index.ejs", "<div id=\"users\">"},
				{"examples/route-separation/views/users/index.ejs", "/user/<%= index %>/edit"},
				{"examples/route-separation/views/posts/index.ejs", "<dl id=\"posts\">"},
			},
			targets: []extraAuditSymbol{
				{"examples/route-separation/views/users/edit.ejs", "user", 5},
				{"examples/route-separation/views/users/view.ejs", "user", 5},
				{"examples/route-separation/views/users/index.ejs", "users", 5},
				{"examples/route-separation/views/posts/index.ejs", "posts", 5},
			},
		},
		{
			project: "ejs75-express-blog-audit-selected", language: "ejs",
			search:        "bs-example-navbar-collapse-1",
			primary:       "views/partials/nav.ejs",
			negativeFile:  "views/pages/signup.ejs",
			negativeQuery: "bs-example-navbar-collapse-1",
			paths: []string{
				"views/pages/signup.ejs",
				"views/pages/login.ejs",
				"views/partials/header.ejs",
				"views/partials/nav.ejs",
			},
			oracles: []extraAuditEvidence{
				{"views/pages/signup.ejs", "<% include ../partials/nav %>"},
				{"views/pages/signup.ejs", "<form action=\"/signup\" method=\"post\">"},
				{"views/pages/signup.ejs", "name=\"email\""},
				{"views/pages/signup.ejs", "name=\"password\""},
				{"views/pages/login.ejs", "<form action=\"/login\" method=\"post\">"},
				{"views/partials/header.ejs", "<body id=\"page-top\" class=\"index\">"},
				{"views/partials/nav.ejs", "data-target=\"#bs-example-navbar-collapse-1\""},
				{"views/partials/nav.ejs", "id=\"bs-example-navbar-collapse-1\""},
				{"views/partials/nav.ejs", "<% if (token) { %>"},
			},
			targets: []extraAuditSymbol{
				{"views/partials/header.ejs", "page-top", 34},
				{"views/partials/nav.ejs", "bs-example-navbar-collapse-1", 18},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
