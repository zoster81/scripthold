package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

func TestTOMLMultilineStringDeclarationRange(t *testing.T) {
	const source = `[package]
description = """
A project description
[fake.section]
ghost = 123
still in a description
"""
name = "real"
extended-description = '''Hello
[another.fake]
missing = 1
last line'''
version = "1"
`
	for _, tc := range []struct{ name, text string }{
		{"LF", source},
		{"CRLF", strings.ReplaceAll(source, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (TOMLAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(res.Analysis.Symbols)
			for _, want := range []struct {
				key        string
				start, end int
			}{
				{"package.description", 2, 7},
				{"package.extended-description", 9, 12},
			} {
				got, ok := byName[want.key]
				if !ok || got.Kind != SymbolKindKey || got.DeclarationRange.Start.Line != want.start ||
					got.DeclarationRange.End.Line != want.end {
					t.Errorf("incomplete multiline TOML key %q: %+v", want.key, got)
				}
			}
			for _, name := range []string{"fake.section", "fake.section.ghost", "another.fake", "package.ghost", "package.missing"} {
				if got, ok := byName[name]; ok {
					t.Errorf("multiline string invented TOML declaration %q: %+v", name, got)
				}
			}
			if s, ok := byName["package.name"]; !ok || s.Kind != SymbolKindKey {
				t.Errorf("neighboring TOML key absent: %+v", s)
			}
			if s, ok := byName["package.version"]; !ok || s.Kind != SymbolKindKey {
				t.Errorf("subsequent TOML key absent: %+v", s)
			}
			if !res.Analysis.CoverageComplete || res.Analysis.Truncated {
				t.Errorf("valid multiline strings marked incomplete: %+v", res.Analysis)
			}
		})
	}
}
