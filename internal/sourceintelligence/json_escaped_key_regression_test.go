package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A JSONC editor file is not necessarily strict JSON. A trailing comma,
// invalid literal, or content after the root must never claim complete coverage,
// even when the lexical navigator can still show some key locations.
func TestJSONMalformedPackageManifestNeverClaimsCompleteCoverage(t *testing.T) {
	cases := []struct {
		name, source string
		complete     bool
	}{
		{"valid Vue-style export", "{\n  \"name\": \"vue\",\n  \"exports\": {\"default\": \"./index.mjs\"}\n}", true},
		{"trailing conditional export comma", "{\n  \"name\": \"vue\",\n  \"exports\": {\"default\": \"./index.mjs\",}\n}", false},
		{"invalid Svelte-style boolean", "{\n  \"name\": \"svelte\",\n  \"private\": yes\n}", false},
		{"content after package root", "{\n  \"name\": \"svelte\"\n} // editor-only note", false},
	}
	for _, tc := range cases {
		for _, ending := range []struct{ name, text string }{
			{"LF", tc.source}, {"CRLF", strings.ReplaceAll(tc.source, "\n", "\r\n")},
		} {
			t.Run(tc.name+"/"+ending.name, func(t *testing.T) {
				result, err := (JSONAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(ending.text), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || result.Analysis.CoverageComplete != tc.complete {
					t.Fatalf("strict JSON coverage invalid for %s: %+v", tc.name, result.Analysis)
				}
				if !tc.complete {
					found := false
					for _, diagnostic := range result.Analysis.Diagnostics {
						if diagnostic.Code == "json-invalid-document" {
							found = true
						}
					}
					if !found {
						t.Fatalf("JSON-invalid source lacks explicit diagnostic: %+v", result.Analysis.Diagnostics)
					}
				}
				seenName := false
				for _, symbol := range result.Analysis.Symbols {
					if symbol.Name == "name" && symbol.ID != "" {
						seenName = true
					}
					if symbol.Name == "editor-only" || symbol.Name == "note" {
						t.Fatalf("invalid text elevated into a JSON key: %+v", symbol)
					}
				}
				if !seenName {
					t.Fatalf("independently present key missing from best-effort JSON navigation: %+v", result.Analysis.Symbols)
				}
			})
		}
	}
}

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
