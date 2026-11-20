package handler

import (
	"os"
	"testing"
)

// Provider 54: original CodeIgniter and WordPress source, independently
// selected from their public Git histories and kept byte-for-byte intact
// in isolated, optional corpus fixtures. Neither project is executed.
// The first qualification used Guzzle and PHPMailer instead.
func TestSourceSymbolsPHPSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original PHP projects not configured")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "php54-codeigniter-audit-selected", language: "php",
			search: "index", primary: "app/Controllers/Home.php",
			negativeFile: "app/Config/Filters.php", negativeQuery: "index",
			paths: []string{
				"app/Config/Routes.php",
				"app/Config/Routing.php",
				"app/Config/Filters.php",
				"app/Controllers/BaseController.php",
				"app/Controllers/Home.php",
			},
			oracles: []extraAuditEvidence{
				{"app/Config/Routes.php", "$routes->get('/', 'Home::index');"},
				{"app/Config/Routing.php", "class Routing extends BaseRouting"},
				{"app/Config/Filters.php", "class Filters extends BaseFilters"},
				{"app/Controllers/Home.php", "class Home extends BaseController"},
				{"app/Controllers/Home.php", "public function index(): string"},
				{"app/Controllers/BaseController.php", "abstract class BaseController extends Controller"},
				{"app/Controllers/BaseController.php", "public function initController(RequestInterface $request, ResponseInterface $response, LoggerInterface $logger)"},
			},
			targets: []extraAuditSymbol{
				{"app/Controllers/Home.php", "Home", 5},
				{"app/Controllers/Home.php", "index", 7},
				{"app/Config/Routing.php", "Routing", 19},
				{"app/Config/Filters.php", "Filters", 16},
				{"app/Controllers/BaseController.php", "BaseController", 21},
				{"app/Controllers/BaseController.php", "initController", 33},
			},
		},
		{
			project: "php54-wordpress-audit-selected", language: "php",
			search: "add_filter", primary: "src/wp-includes/plugin.php",
			negativeFile: "src/wp-includes/class-wp-error.php", negativeQuery: "add_filter",
			paths: []string{
				"src/wp-includes/plugin.php",
				"src/wp-includes/class-wp-hook.php",
				"src/wp-includes/class-wp-error.php",
			},
			oracles: []extraAuditEvidence{
				{"src/wp-includes/plugin.php", "function add_filter( $hook_name, $callback, $priority = 10, $accepted_args = 1 )"},
				{"src/wp-includes/plugin.php", "function apply_filters( $hook_name, $value, ...$args )"},
				{"src/wp-includes/class-wp-hook.php", "final class WP_Hook implements Iterator, ArrayAccess"},
				{"src/wp-includes/class-wp-hook.php", "public function add_filter( $hook_name, $callback, $priority, $accepted_args )"},
				{"src/wp-includes/class-wp-hook.php", "public function remove_filter( $hook_name, $callback, $priority )"},
				{"src/wp-includes/class-wp-hook.php", "public function apply_filters( $value, $args )"},
				{"src/wp-includes/class-wp-error.php", "class WP_Error"},
			},
			targets: []extraAuditSymbol{
				{"src/wp-includes/plugin.php", "add_filter", 122},
				{"src/wp-includes/plugin.php", "apply_filters", 174},
				{"src/wp-includes/class-wp-hook.php", "add_filter", 91},
				{"src/wp-includes/class-wp-hook.php", "remove_filter", 205},
				{"src/wp-includes/class-wp-hook.php", "apply_filters", 328},
				{"src/wp-includes/class-wp-error.php", "get_error_code", 91},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
}
