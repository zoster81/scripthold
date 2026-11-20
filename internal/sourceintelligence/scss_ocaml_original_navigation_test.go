package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The original OCaml manual combines nested rules, repeated selector names,
// multiline groups, and media-scoped selectors without defining Sass variables.
func TestSCSSOCamlOriginalManualNavigation(t *testing.T) {
	check := func(t *testing.T, source string, wants map[int]string) {
		t.Helper()
		got, err := (SCSSAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("manual.scss", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		if got.Analysis.Truncated || !got.Analysis.CoverageComplete {
			t.Fatalf("supported SCSS source marked partial: %+v", got.Analysis)
		}
		seen := make(map[int]bool, len(wants))
		for _, sym := range got.Analysis.Symbols {
			if sym.Kind != SymbolKindSelector || strings.Contains(sym.Name, "@include") ||
				strings.Contains(sym.Name, "$font-") || strings.HasPrefix(sym.Name, "content:") {
				t.Fatalf("non-selector source text indexed as a declaration: %+v", sym)
			}
			for line, name := range wants {
				if sym.Name == name && sym.NameRange.Start.Line == line {
					seen[line] = true
				}
			}
		}
		if len(seen) != len(wants) {
			t.Fatalf("original SCSS symbols missing: seen=%v wants=%v all=%v",
				seen, wants, sortedSymbolQualifiedNames(got.Analysis.Symbols))
		}
	}
	fixture := ".content {\n  #part-title {\n    span { color: red; }\n  }\n}\n.a,\n.b { color: blue; }\n@media only screen {\n  nav.toc { color: green; }\n}\n"
	wants := map[int]string{1: ".content", 2: "#part-title", 3: "span", 6: ".a", 7: ".b", 9: "nav.toc"}
	for _, tc := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(tc.name, func(t *testing.T) {
			check(t, strings.ReplaceAll(fixture, "\n", tc.eol), wants)
		})
	}
	if root := os.Getenv("SCRIPTHOLD_REAL_SCSS_OCAML_ROOT"); root != "" {
		t.Run("original-ocaml-manual", func(t *testing.T) {
			path := filepath.Join(root, "manual.scss")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("bounded original SCSS file unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(data)
			if strings.Count(source, "/* CSS file for the Ocaml manual */") != 1 ||
				strings.Count(source, "div.caml-example.toplevel code.caml-input::before,") != 1 ||
				strings.Count(source, "@media only screen and (max-width:95ex){") != 1 {
				t.Fatal("original OCaml source anchors changed")
			}
			check(t, source, map[int]string{
				13: ".content", 15: "#part-title", 20: "span",
				60: "ul.toc ul.toc ul.toc", 221: "nav.toc", 223: "&.brand",
				297: "div.caml-example.toplevel code.caml-input::before",
				298: "div.caml-example.toplevel div.caml-input::before",
				358: ".caml-input", 359: "span.ocamlkeyword",
				386: ".content #part-menu", 391: "nav.toc", 394: "header",
			})
		})
	}
}
