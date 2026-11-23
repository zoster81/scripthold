package handler

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

// Original Blade projects must return the same source entities with and without
// an exact kind filter, including their public symbol identities and ordering.
func TestSourceQueryBladeIndependentOriginalKindFilteredParity(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("independent original Blade projects not configured")
	}
	for _, tc := range []struct {
		name, project, query, primary string
		line                          int
		files                         []string
	}{
		{
			name: "Akaunting dashboard", project: "blade74-akaunting-audit-selected",
			query:   "show-more-actions-new-dashboard",
			primary: "resources/views/common/dashboards/show.blade.php", line: 120,
			files: []string{
				"resources/views/common/dashboards/index.blade.php",
				"resources/views/common/dashboards/show.blade.php",
				"resources/views/common/items/index.blade.php",
			},
		},
		{
			name: "Breeze login form", project: "blade74-breeze-audit-selected",
			query:   "email",
			primary: "stubs/default/resources/views/auth/login.blade.php", line: 11,
			files: []string{
				"stubs/default/resources/views/auth/login.blade.php",
				"stubs/default/resources/views/auth/register.blade.php",
				"stubs/default/resources/views/auth/forgot-password.blade.php",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				requireAdditionalOriginalFile(t, path)
				paths = append(paths, path)
			}
			h := NewHandler([]string{root})
			input := SourceQueryInput{
				Operation: "search", Paths: paths, Language: "blade", Encoding: "utf-8",
				Query: tc.query, Match: "exact", Mode: "structural",
				MaxFiles: len(paths), MaxResults: 64,
			}
			query := func(in SourceQueryInput) SourceQueryOutput {
				t.Helper()
				failed, result, err := h.SourceQuery(context.Background(), nil, in)
				if err != nil || failed != nil || result.Search == nil ||
					!result.Coverage.CoverageComplete || result.Coverage.ResultsTruncated {
					t.Fatalf("original Blade source search incomplete: error=%v failed=%+v result=%+v", err, failed, result)
				}
				return result
			}
			all := query(input)
			input.Kinds = []string{"entity"}
			filtered := query(input)
			if all.Index.Fingerprint != filtered.Index.Fingerprint {
				t.Fatalf("original Blade source generation changed: all=%+v filtered=%+v", all.Index, filtered.Index)
			}
			want := make([]SourceSearchMatch, 0, len(all.Search.Matches))
			foundPrimary := false
			for _, match := range all.Search.Matches {
				if match.Kind != sourceintelligence.SymbolKindEntity {
					continue
				}
				want = append(want, match)
				if match.Name == tc.query && filepath.Clean(match.Path) == filepath.Join(root, filepath.FromSlash(tc.primary)) &&
					match.Range.Start.Line == tc.line && match.SymbolID != "" {
					foundPrimary = true
				}
			}
			if !foundPrimary || !reflect.DeepEqual(filtered.Search.Matches, want) {
				t.Fatalf("original Blade kind filter lost/changed source facts: foundPrimary=%t filtered=%+v want=%+v",
					foundPrimary, filtered.Search.Matches, want)
			}
		})
	}
}

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
