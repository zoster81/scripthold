package sourceintelligence

import (
	"context"
	"testing"
)

// JSON escaped keys have the same logical names as their decoded object keys.
// A fabricated raw \u sequence cannot be a safe edit target.
func TestJSONEscapedKeysDecodeNamesAndRejectMalformedEscapes(t *testing.T) {
	cases := []struct {
		name, source string
		want         []string
		forbidden    []string
		complete     bool
	}{
		{
			name:      "unicode and surrogate pair",
			source:    `{"service":{"na\u006De":"api","\uD83D\uDE00":true}}`,
			want:      []string{"service", "service.name", "service.😀"},
			forbidden: []string{"service.nau006De", "service.uD83DuDE00"},
			complete:  true,
		},
		{
			name:      "invalid escape is not a declaration",
			source:    `{"service":{"bad\q":1}}`,
			want:      []string{"service"},
			forbidden: []string{"service.badq"},
			complete:  false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (JSONAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.source), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete != tc.complete {
				t.Errorf("JSON coverage=%v want=%v diagnostics=%+v", result.Analysis.CoverageComplete, tc.complete, result.Analysis.Diagnostics)
			}
			got := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, name := range tc.want {
				if sym, ok := got[name]; !ok || sym.ID == "" {
					t.Errorf("expected exact JSON key name %q absent or unsafe: %+v", name, sym)
				}
			}
			for _, name := range tc.forbidden {
				if sym, ok := got[name]; ok {
					t.Errorf("invented JSON key %q: %+v", name, sym)
				}
			}
		})
	}
}
