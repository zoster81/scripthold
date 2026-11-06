package sourceintelligence

import (
	"context"
	"testing"
)

// Query parameters and JSP fragments inside a quoted attribute value are not
// independent HTML id attributes. Keep the real host element ID navigable.
func TestMarkupIDAttributesExcludeURLParameters(t *testing.T) {
	for _, tc := range []struct {
		name     string
		analyzer SourceAnalyzer
		source   string
	}{
		{
			name:     "html-query",
			analyzer: HTMLAnalyzer{},
			source: "<a href=\"/edit?id=user42&mode=edit\">Edit</a>\n" +
				"<main id=\"real-id\"></main>",
		},
		{
			name:     "jsp-nested-tag",
			analyzer: JSPAnalyzer{},
			source: "<a href=\"edit?id=<c:out value='${user.id}' />\">Edit</a>\n" +
				"<main id=\"real-id\"></main>",
		},
		{
			name:     "jsp-embedded-scriptlet",
			analyzer: JSPAnalyzer{},
			source: "<a href=\"/booking?action=cancel&id=<%= b.getId() %>\" class=\"btn\">Cancel</a>\n" +
				"<main id=\"real-id\"></main>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), testSourceDocument("fixture.jsp", tc.source), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, sym := range result.Analysis.Symbols {
				if sym.Kind != SymbolKindEntity {
					continue
				}
				if sym.Name != "real-id" {
					t.Fatalf("URL argument or nested markup falsely indexed as HTML ID: %+v", sym)
				}
				found = true
			}
			if !found {
				t.Fatalf("real host id lost: %+v", result.Analysis.Symbols)
			}
		})
	}
}
