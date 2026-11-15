package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// A selector edit requires the whole authored rule, including its declarations.
// Every member of a comma-separated group must reveal the same complete rule.
func TestStylesheetSelectorRuleSpansAndGroupOwnership(t *testing.T) {
	const fixture = ".rc-footer, .rc-footer-light {\n" +
		"  color: rgba(255, 255, 255, 0.4);\n" +
		"  background-color: #000;\n" +
		"}\n" +
		".rc-footer a {\n" +
		"  color: #40a9ff;\n" +
		"}\n" +
		"@media only screen and (max-width: 767.99px) {\n" +
		"  .rc-footer {\n" +
		"    text-align: center;\n" +
		"  }\n" +
		"}\n"
	for _, variant := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"css", CSSAnalyzer{}},
		{"scss", SCSSAnalyzer{}},
		{"less", LessAnalyzer{}},
	} {
		for _, eol := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(variant.name+"/"+eol.name, func(t *testing.T) {
				result, err := variant.analyzer.Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(fixture, "\n", eol.newline)),
					testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatal(err)
				}
				if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
					t.Fatalf("supported stylesheet source not completely scanned: %+v", result.Analysis)
				}
				seen := map[string]bool{}
				for _, symbol := range result.Analysis.Symbols {
					if symbol.Kind != SymbolKindSelector {
						continue
					}
					switch {
					case symbol.Name == ".rc-footer" && symbol.NameRange.Start.Line == 1,
						symbol.Name == ".rc-footer-light" && symbol.NameRange.Start.Line == 1:
						seen[symbol.Name] = true
						if symbol.DeclarationRange.Start.Line != 1 ||
							symbol.DeclarationRange.Start.Column != 1 ||
							symbol.DeclarationRange.End.Line != 4 ||
							!strings.Contains(symbol.Signature, ".rc-footer, .rc-footer-light {") ||
							strings.Contains(symbol.Signature, "background-color: #000;") ||
							strings.Contains(symbol.Signature, ".rc-footer a") {
							t.Errorf("group member must expose one whole authored rule: %+v", symbol)
						}
					case symbol.Name == ".rc-footer a" && symbol.NameRange.Start.Line == 5:
						seen[symbol.Name] = true
						if symbol.DeclarationRange.End.Line != 7 ||
							!strings.Contains(symbol.Signature, ".rc-footer a") ||
							strings.Contains(symbol.Signature, "color: #40a9ff;") ||
							strings.Contains(symbol.Signature, "@media") {
							t.Errorf("sibling selector excerpt crossed bounds: %+v", symbol)
						}
					case symbol.Name == ".rc-footer" && symbol.NameRange.Start.Line == 9:
						seen["nested"] = true
						if symbol.DeclarationRange.End.Line != 11 ||
							!strings.Contains(symbol.Signature, ".rc-footer") ||
							strings.Contains(symbol.Signature, "text-align: center;") ||
							strings.Contains(symbol.Signature, "background-color: #000;") {
							t.Errorf("nested media selector excerpt is wrong: %+v", symbol)
						}
					}
				}
				for _, name := range []string{".rc-footer", ".rc-footer-light", ".rc-footer a", "nested"} {
					if !seen[name] {
						t.Errorf("expected %q missing among %+v", name, result.Analysis.Symbols)
					}
				}
			})
		}
	}
}

func TestStylesheetRuleBoundariesIgnoreQuotedAndCommentBraces(t *testing.T) {
	const source = ".icon {\n" +
		"  content: \"literal } brace\";\n" +
		"  /* a misleading { block } */\n" +
		"  color: red;\n" +
		"}\n" +
		".next { color: blue; }\n"
	for _, variant := range []struct {
		name     string
		analyzer SourceAnalyzer
	}{
		{"css", CSSAnalyzer{}}, {"scss", SCSSAnalyzer{}}, {"less", LessAnalyzer{}},
	} {
		for _, eol := range []struct{ name, newline string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(variant.name+"/"+eol.name, func(t *testing.T) {
				result, err := variant.analyzer.Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(source, "\n", eol.newline)),
					testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
					t.Fatalf("valid stylesheet must be complete: %+v", result.Analysis)
				}
				seen := map[string]bool{}
				for _, sym := range result.Analysis.Symbols {
					if sym.Name == ".icon" || sym.Name == ".next" {
						seen[sym.Name] = true
						if sym.Name == ".icon" && (sym.DeclarationRange.End.Line != 5 ||
							strings.Contains(sym.Signature, "color: red;") ||
							strings.Contains(sym.Signature, ".next")) {
							t.Errorf("quoted/comment braces truncated or exceeded rule: %+v", sym)
						}
						if sym.Name == ".next" && (sym.DeclarationRange.Start.Line != 6 ||
							!strings.Contains(sym.Signature, ".next") ||
							strings.Contains(sym.Signature, "color: blue;")) {
							t.Errorf("neighbor selector omitted or wrong: %+v", sym)
						}
					}
				}
				if len(seen) != 2 {
					t.Errorf("missing original neighboring stylesheet selectors: %+v", seen)
				}
				broken, err := variant.analyzer.Analyze(context.Background(),
					sourceDocumentForScanner(strings.ReplaceAll(".unclosed { color: red;\n", "\n", eol.newline)),
					testAnalyzeOptions(true, 16))
				if err != nil {
					t.Fatal(err)
				}
				if broken.Analysis.CoverageComplete || broken.Analysis.Truncated {
					t.Fatalf("unclosed rule must be marked incomplete: %+v", broken.Analysis)
				}
			})
		}
	}
}
