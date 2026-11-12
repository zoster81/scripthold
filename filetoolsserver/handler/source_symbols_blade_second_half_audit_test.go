package handler

import (
	"os"
	"testing"
)

// Provider 74 uses two further independent original Blade project origins:
// Akaunting and Laravel Breeze. The selected templates were independently
// read in full and checked against pinned upstream Git blobs before source_*.
func TestSourceSymbolsBladeSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("additional original Blade project snapshots not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "blade74-akaunting-audit-selected", language: "blade",
			search:        "show-more-actions-new-dashboard",
			primary:       "resources/views/common/dashboards/show.blade.php",
			negativeFile:  "resources/views/common/items/index.blade.php",
			negativeQuery: "show-more-actions-new-dashboard",
			paths: []string{
				"resources/views/common/dashboards/index.blade.php",
				"resources/views/common/dashboards/show.blade.php",
				"resources/views/common/items/index.blade.php",
			},
			oracles: []extraAuditEvidence{
				{"resources/views/common/dashboards/index.blade.php", "id=\"index-more-actions-new-dasboard\""},
				{"resources/views/common/dashboards/index.blade.php", "route('dashboards.create')"},
				{"resources/views/common/dashboards/show.blade.php", "@section('dashboard_action')"},
				{"resources/views/common/dashboards/show.blade.php", "id=\"show-more-actions-dashboard\""},
				{"resources/views/common/dashboards/show.blade.php", "id=\"dashboard-slider\""},
				{"resources/views/common/dashboards/show.blade.php", "id=\"show-more-actions-add-widget\""},
				{"resources/views/common/dashboards/show.blade.php", "id=\"show-more-actions-new-dashboard\""},
				{"resources/views/common/items/index.blade.php", "id=\"index-more-actions-new-item\""},
				{"resources/views/common/items/index.blade.php", "id=\"index-more-actions-import-item\""},
				{"resources/views/common/items/index.blade.php", "id=\"index-more-actions-export-item\""},
			},
			targets: []extraAuditSymbol{
				{"resources/views/common/dashboards/index.blade.php", "index-more-actions-new-dasboard", 6},
				{"resources/views/common/dashboards/show.blade.php", "show-more-actions-dashboard", 45},
				{"resources/views/common/dashboards/show.blade.php", "dashboard-left", 80},
				{"resources/views/common/dashboards/show.blade.php", "dashboard-slider", 82},
				{"resources/views/common/dashboards/show.blade.php", "show-more-actions-add-widget", 109},
				{"resources/views/common/dashboards/show.blade.php", "show-more-actions-new-dashboard", 120},
				{"resources/views/common/items/index.blade.php", "index-more-actions-new-item", 12},
				{"resources/views/common/items/index.blade.php", "index-more-actions-import-item", 25},
			},
		},
		{
			project: "blade74-breeze-audit-selected", language: "blade",
			search:        "email",
			primary:       "stubs/default/resources/views/auth/login.blade.php",
			negativeFile:  "stubs/default/resources/views/auth/forgot-password.blade.php",
			negativeQuery: "password_confirmation",
			paths: []string{
				"stubs/default/resources/views/auth/login.blade.php",
				"stubs/default/resources/views/auth/register.blade.php",
				"stubs/default/resources/views/auth/forgot-password.blade.php",
			},
			oracles: []extraAuditEvidence{
				{"stubs/default/resources/views/auth/login.blade.php", "action=\"{{ route('login') }}\""},
				{"stubs/default/resources/views/auth/login.blade.php", "id=\"email\""},
				{"stubs/default/resources/views/auth/login.blade.php", "id=\"password\""},
				{"stubs/default/resources/views/auth/login.blade.php", "id=\"remember_me\""},
				{"stubs/default/resources/views/auth/login.blade.php", "Route::has('password.request')"},
				{"stubs/default/resources/views/auth/register.blade.php", "action=\"{{ route('register') }}\""},
				{"stubs/default/resources/views/auth/register.blade.php", "id=\"name\""},
				{"stubs/default/resources/views/auth/register.blade.php", "id=\"password_confirmation\""},
				{"stubs/default/resources/views/auth/forgot-password.blade.php", "action=\"{{ route('password.email') }}\""},
				{"stubs/default/resources/views/auth/forgot-password.blade.php", "id=\"email\""},
			},
			targets: []extraAuditSymbol{
				{"stubs/default/resources/views/auth/login.blade.php", "email", 11},
				{"stubs/default/resources/views/auth/login.blade.php", "password", 19},
				{"stubs/default/resources/views/auth/login.blade.php", "remember_me", 30},
				{"stubs/default/resources/views/auth/register.blade.php", "name", 8},
				{"stubs/default/resources/views/auth/register.blade.php", "email", 15},
				{"stubs/default/resources/views/auth/register.blade.php", "password_confirmation", 35},
				{"stubs/default/resources/views/auth/forgot-password.blade.php", "email", 15},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
