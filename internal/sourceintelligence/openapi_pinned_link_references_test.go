package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Source-first authored inventory frozen from the complete pinned official
// OpenAPI link-object example, before querying the development analyzer.
// Link Object operationId values are references, never operations.
func TestOpenAPIPinnedWholeLinkObjectReferencesAreNotOperations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OPENAPI_LINK_EXAMPLE_ROOT")
	if root == "" {
		t.Skip("pinned official OpenAPI original not configured")
	}
	path := filepath.Join(root, "openapi-links-original.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 5280 {
		t.Fatalf("original source bytes changed: %d", len(data))
	}
	want := map[string]struct {
		kind, native string
		line         int
	}{
		"getUserByName":               {"operation", "operation", 8},
		"getRepositoriesByOwner":      {"operation", "operation", 27},
		"getRepository":               {"operation", "operation", 48},
		"getPullRequestsByRepository": {"operation", "operation", 72},
		"getPullRequestsById":         {"operation", "operation", 103},
		"mergePullRequest":            {"operation", "operation", 132},
		"user":                        {"type", "schema", 179},
		"repository":                  {"type", "schema", 186},
		"pullrequest":                 {"type", "schema", 193},
	}
	for _, tc := range []struct{ name, src string }{
		{"LF", string(data)}, {"CRLF", strings.ReplaceAll(string(data), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (OpenAPIAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.src), testAnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete ||
				len(result.Analysis.Symbols) != len(want) {
				t.Errorf("whole official original has %d returned structural symbols, want %d (six HTTP operations plus three schemas)", len(result.Analysis.Symbols), len(want))
			}
			seen := map[string]bool{}
			for _, s := range result.Analysis.Symbols {
				w, ok := want[s.Name]
				if !ok || seen[s.Name] || string(s.Kind) != w.kind || s.NativeKind != w.native ||
					s.NameRange.Start.Line != w.line || s.DeclarationRange.Start.Line != w.line ||
					s.ParentID != "" || s.ParentQualifiedName != "" {
					t.Errorf("invented operation/link reference, wrong owner or wrong source: %+v expected=%+v", s, w)
				}
				seen[s.Name] = true
			}
			for name := range want {
				if !seen[name] {
					t.Errorf("missing supported authored source declaration %q", name)
				}
			}
		})
	}
}

func TestOpenAPILinkObjectReferencesAndMethodFallbackControls(t *testing.T) {
	const src = "openapi: 3.0.0\npaths:\n  /pets:\n    get:\n      operationId: getPets\n" +
		"      responses:\n        '200':\n          links:\n            another:\n              operationId: getOther\n" +
		"    post:\n      responses:\n        '201':\n          description: created\n" +
		"components:\n  links:\n    Another:\n      operationId: getPets\n" +
		"  schemas:\n    Pet:\n      type: object\n      properties:\n        sub:\n          operationId: fakeOperation\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", src}, {"CRLF", strings.ReplaceAll(src, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (OpenAPIAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, want := range []struct {
				name, native string
				line         int
			}{
				{"getPets", "operation", 5}, {"POST /pets", "http-method", 11}, {"Pet", "schema", 20},
			} {
				s, ok := byName[want.name]
				if !ok || s.NativeKind != want.native || s.NameRange.Start.Line != want.line {
					t.Errorf("actual operation/schema lost: %+v expected %+v", s, want)
				}
			}
			if len(result.Analysis.Symbols) == 3 {
				if s, ok := byName["getPets"]; !ok || s.NameRange.Start.Line != 5 {
					t.Errorf("getPets must originate at the path method, not a component link: %+v", s)
				}
			}
			for _, forbidden := range []string{"getOther", "fakeOperation"} {
				if _, ok := byName[forbidden]; ok {
					t.Errorf("invented nested/link reference %q", forbidden)
				}
			}
			if len(result.Analysis.Symbols) != 3 || result.Analysis.Truncated {
				t.Fatalf("invalid supported operations and reference controls: %+v", result.Analysis)
			}
		})
	}
}
