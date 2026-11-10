package handler

import (
	"os"
	"testing"
)

// Provider 52: two further independent Git-original Go projects, distinct
// from the initial Cobra and Chi source-navigation qualifications.
// Only source facts and local development MCP handlers are inspected.
func TestSourceSymbolsGoSecondHalfIndependentAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Go projects not configured")
	}
	for _, scenario := range []extraAuditScenario{
		{
			project: "go-gorilla-mux", language: "go",
			search: "Methods", primary: "route.go",
			negativeFile: "middleware.go", negativeQuery: "Methods",
			paths: []string{"route.go", "mux.go", "mux_test.go", "middleware.go"},
			oracles: []extraAuditEvidence{
				{"route.go", "func (r *Route) Methods(methods ...string) *Route {"},
				{"route.go", "methods[k] = strings.ToUpper(v)"},
				{"route.go", "return r.addMatcher(methodMatcher(methods))"},
				{"mux.go", "func (r *Router) Methods(methods ...string) *Route {"},
				{"mux.go", "return r.NewRoute().Methods(methods...)"},
				{"mux_test.go", "func TestMethods(t *testing.T) {"},
				{"mux_test.go", "Methods route, bad method"},
				{"middleware.go", "func getAllMethodsForRoute(r *Router, req *http.Request) ([]string, error) {"},
			},
			targets: []extraAuditSymbol{
				{"route.go", "Methods", 398},
				{"mux.go", "Methods", 360},
				{"mux_test.go", "TestMethods", 714},
				{"middleware.go", "getAllMethodsForRoute", 74},
			},
		},
		{
			project: "go-fsnotify", language: "go",
			search: "AddWith", primary: "fsnotify.go",
			negativeFile: "fsnotify_test.go", negativeQuery: "AddWith",
			paths: []string{"fsnotify.go", "backend_inotify.go", "backend_windows.go", "fsnotify_test.go"},
			oracles: []extraAuditEvidence{
				{"fsnotify.go", "func (w *Watcher) AddWith(path string, opts ...addOpt) error { return w.b.AddWith(path, opts...) }"},
				{"fsnotify.go", "func (w *Watcher) Remove(path string) error { return w.b.Remove(path) }"},
				{"backend_inotify.go", "func (w *inotify) AddWith(path string, opts ...addOpt) error {"},
				{"backend_inotify.go", "if !w.xSupports(with.op) {"},
				{"backend_windows.go", "func (w *readDirChangesW) AddWith(name string, opts ...addOpt) error {"},
				{"fsnotify_test.go", "func TestAdd(t *testing.T) {"},
				{"fsnotify_test.go", "func TestRemove(t *testing.T) {"},
			},
			targets: []extraAuditSymbol{
				{"fsnotify.go", "AddWith", 295},
				{"backend_inotify.go", "AddWith", 185},
				{"backend_windows.go", "AddWith", 110},
				{"fsnotify.go", "Remove", 305},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, scenario)
	}
}
