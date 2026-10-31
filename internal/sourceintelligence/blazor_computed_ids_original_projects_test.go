package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestBlazorQualifiedOriginalComputedIDs(t *testing.T) {
	cases := []struct {
		name, env, forbidden string
	}{
		{"DevToysButton", "SCRIPTHOLD_REAL_BLAZOR_DEVTOYS_BUTTON", "@Id"},
		{"RadzenAIChat", "SCRIPTHOLD_REAL_BLAZOR_RADZEN_AICHAT", "@GetId()"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("set qualified original Blazor source path")
			}
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatal("original Blazor fixture must be regular and at most 1 MiB")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			doc := sourceDocumentForScanner(string(data))
			doc.Path = path
			result, err := (BlazorAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("original Blazor analysis incomplete: %+v", result.Analysis)
			}
			for _, sym := range result.Analysis.Symbols {
				if sym.Kind == SymbolKindEntity && sym.Name == tc.forbidden {
					t.Errorf("computed runtime id became static entity: %+v", sym)
				}
			}
		})
	}
}

func TestBlazorComputedIDsAndLiteralIDsWithLineEndings(t *testing.T) {
	const source = "<main id=\"fixed\"></main>\n<button id=@Id></button>\n<div id=\"@GetId()\"></div>\n<aside id=\"prefix-@Id\"></aside>\n"
	for _, tc := range []struct{ name, text string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := (BlazorAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("incomplete: %+v", result.Analysis)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			if got, ok := byName["fixed"]; !ok || got.Kind != SymbolKindEntity || got.NameRange.Start.Line != 1 {
				t.Errorf("missing real static ID: present=%v symbol=%+v", ok, got)
			}
			for _, name := range []string{"@Id", "@GetId()", "prefix-@Id"} {
				if got, ok := byName[name]; ok {
					t.Errorf("computed id %q falsely static: %+v", name, got)
				}
			}
		})
	}
}
