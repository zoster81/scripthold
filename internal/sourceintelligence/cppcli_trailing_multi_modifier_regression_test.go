package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Real original C++/CLI projects declare managed static helpers as
// "ref class MimeTypes abstract sealed". Previously only the first trailing
// modifier was masked and the whole class was silently replaced with a
// fabricated namespace variable named "sealed".
func TestCPPCLITrailingMultipleClassModifiers(t *testing.T) {
	for _, tc := range []struct {
		name, eol, modifiers string
	}{
		{"abstract-sealed-lf", "\n", "abstract sealed"},
		{"sealed-abstract-crlf", "\r\n", "sealed abstract"},
		{"single-abstract-lf", "\n", "abstract"},
		{"single-sealed-crlf", "\r\n", "sealed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code := strings.Join([]string{
				"namespace NTagLib {",
				"public ref class MimeTypes " + tc.modifiers,
				"{",
				"public:",
				"static int Normalize(int value) { return value; }",
				"static int GetMimeTypeFromData(int value) { return value; }",
				"};",
				"public ref class Picture { public: static int Value() { return 0; } };",
				"}",
				"",
			}, tc.eol)
			out, err := (CPPCLIAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("cover.cpp", code), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]bool{
				"NTagLib":                               false,
				"NTagLib.MimeTypes":                     false,
				"NTagLib.MimeTypes.Normalize":           false,
				"NTagLib.MimeTypes.GetMimeTypeFromData": false,
				"NTagLib.Picture":                       false,
				"NTagLib.Picture.Value":                 false,
			}
			for _, symbol := range out.Analysis.Symbols {
				if symbol.Name == "sealed" || symbol.Name == "abstract" {
					t.Errorf("managed class modifier fabricated as a symbol: %+v", symbol)
				}
				if _, ok := want[symbol.QualifiedName]; ok {
					want[symbol.QualifiedName] = true
				}
			}
			for name, found := range want {
				if !found {
					t.Errorf("managed class or method omitted: %s", name)
				}
			}
			if !out.Analysis.CoverageComplete || out.Analysis.Truncated {
				t.Errorf("otherwise valid bounded C++/CLI document not fully analyzed: %+v", out.Analysis)
			}
		})
	}
}
