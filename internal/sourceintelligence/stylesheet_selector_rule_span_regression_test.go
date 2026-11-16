package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
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

// Multiline SCSS selector lists may contain comments between authored members.
// Navigation must retain the real selector names and one shared rule body.
func TestSCSSCommentInterleavedSelectorGroupsFromOriginalDart(t *testing.T) {
	check := func(t *testing.T, source string, firstLine, firstEnd, secondLine, secondEnd int) {
		t.Helper()
		result, err := (SCSSAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("style.scss", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		wants := map[string]struct{ nameLine, start, end int }{
			"& .na": {firstLine, firstLine, firstEnd},
			"& .nb": {firstLine + 1, firstLine, firstEnd},
			"& .nt": {firstEnd - 2, firstLine, firstEnd},
			"& .vc": {secondLine, secondLine, secondEnd},
			"& .vg": {secondLine + 1, secondLine, secondEnd},
		}
		seen := make(map[string]bool, len(wants))
		for _, symbol := range result.Analysis.Symbols {
			if want, ok := wants[symbol.Name]; ok && symbol.Kind == SymbolKindSelector {
				seen[symbol.Name] = true
				if symbol.NameRange.Start.Line != want.nameLine ||
					symbol.DeclarationRange.Start.Line != want.start ||
					symbol.DeclarationRange.End.Line != want.end {
					t.Errorf("SCSS original comment interleaved selector range %q: %+v", symbol.Name, symbol)
				}
			}
			if symbol.Name == "& .n" || symbol.Name == "& .err" ||
				strings.Contains(symbol.Name, "Name.Attribute") ||
				strings.Contains(symbol.Name, "Name.Builtin") ||
				strings.Contains(symbol.Name, "Name.Tag") {
				t.Fatalf("comment-only text became SCSS symbol: %+v", symbol)
			}
		}
		if len(seen) != len(wants) {
			t.Fatalf("SCSS authored group names missing: seen=%v symbols=%v",
				seen, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	const fixture = ".highlight {\n  pre {\n    /*& .n,*/ /* Name */\n    & .na, /* Name.Attribute */\n    & .nb, /* Name.Builtin */\n    & .nt /* Name.Tag */ {\n      color: red;\n    }\n    & .vc,\n    & .vg { color: blue; }\n  }\n}\n"
	for _, eol := range []struct{ name, value string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(eol.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(fixture, "\n", eol.value), 4, 8, 9, 10)
		})
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_SCSS_DART_ROOT"); root != "" {
		t.Run("original-dart-style", func(t *testing.T) {
			path := filepath.Join(root, "style.scss")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded SCSS original unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, "& .na, /* Name.Attribute */") != 1 ||
				strings.Count(source, "& .nt /* Name.Tag */ {") != 1 ||
				strings.Count(source, "& .vc,\n    & .vg,") != 1 {
				t.Fatal("original Dart SCSS source oracle changed")
			}
			check(t, source, 255, 269, 271, 276)
		})
	}
}

// Interpolation braces in SCSS property values are not CSS rule delimiters.
// The original stylesheet has two interpolations separated by a slash.
func TestSCSSValueInterpolationDoesNotInventSelectorsOriginalDart(t *testing.T) {
	check := func(t *testing.T, source string, codeLine int) {
		t.Helper()
		result, err := (SCSSAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("interpolated.scss", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		foundCode := false
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Kind != SymbolKindSelector {
				continue
			}
			if symbol.Name == "/#" || symbol.Name == "#" {
				t.Fatalf("SCSS interpolation invented a selector: %+v", symbol)
			}
			if symbol.Name == "code" && symbol.NameRange.Start.Line == codeLine {
				if symbol.DeclarationRange.Start.Line != codeLine ||
					symbol.DeclarationRange.End.Line != codeLine+3 {
					t.Errorf("interpolated SCSS source rule has incorrect bounds: %+v", symbol)
				}
				foundCode = true
			}
		}
		if !foundCode {
			t.Fatalf("SCSS authored code selector absent: %v", sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	const fixture = "code {\n  font: normal #{$code-font-size}/#{$code-line-height} $monospace;\n  color: red;\n}\n.next { color: blue; }\n"
	for _, tc := range []struct{ name, newline string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(fixture, "\n", tc.newline), 1)
		})
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_SCSS_DART_ROOT"); root != "" {
		t.Run("original-dart-scss-interpolated-font", func(t *testing.T) {
			path := filepath.Join(root, "style.scss")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded SCSS original unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, "font: normal #{$code-font-size}/#{$code-line-height} $monospace;") != 1 ||
				strings.Count(source, "font: normal #{$code-font-size * .9}/#{$code-line-height * .9} $monospace !important;") != 1 {
				t.Fatal("original Dart SCSS interpolation source oracle changed")
			}
			check(t, source, 195)
		})
	}
}

// Malformed interpolations must not expose fake CSS rule boundaries or
// claim complete source coverage; neighboring real selectors stay navigable.
func TestSCSSMalformedInterpolationPreservesNeighboringRules(t *testing.T) {
	const fixture = ".outer {\n  width: #{$width;\n  height: 2px;\n}\n.next { color: red; }\n"
	for _, eol := range []struct{ name, newline string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(eol.name, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "\n", eol.newline)
			result, err := (SCSSAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("broken-interpolation.scss", source), testAnalyzeOptions(true, 48))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete {
				t.Fatalf("unterminated interpolation marked complete: %+v", result.Analysis)
			}
			seen := map[string]bool{}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Kind != SymbolKindSelector {
					continue
				}
				switch symbol.Name {
				case ".outer", ".next":
					seen[symbol.Name] = true
				default:
					t.Errorf("SCSS interpolation created unexpected selector: %+v", symbol)
				}
			}
			if !seen[".outer"] || !seen[".next"] {
				t.Fatalf("valid surrounding rules missing: seen=%v", seen)
			}
		})
	}
}

// String and comment contents must be masked before interpolation scanning.
func TestSCSSInterpolationMaskKeepsQuotedAndCommentBracesOpaque(t *testing.T) {
	const fixture = ".real {\n  content: \"#{quoted}\";\n  /* #{comment} { } */\n  width: #{$size + 1}px;\n}\n.next { color: red; }\n"
	for _, eol := range []struct{ name, newline string }{
		{"LF", "\n"}, {"CRLF", "\r\n"},
	} {
		t.Run(eol.name, func(t *testing.T) {
			result, err := (SCSSAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("quoted-interpolation.scss", strings.ReplaceAll(fixture, "\n", eol.newline)),
				testAnalyzeOptions(true, 48))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete {
				t.Fatalf("balanced SCSS quoted/comment interpolation became partial: %+v", result.Analysis)
			}
			seen := map[string]bool{}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Kind == SymbolKindSelector {
					switch symbol.Name {
					case ".real", ".next":
						seen[symbol.Name] = true
					default:
						t.Errorf("quoted/comment interpolation generated false selector: %+v", symbol)
					}
				}
			}
			if !seen[".real"] || !seen[".next"] {
				t.Fatalf("real selector lost with interpolation masking: %+v", result.Analysis.Symbols)
			}
		})
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
