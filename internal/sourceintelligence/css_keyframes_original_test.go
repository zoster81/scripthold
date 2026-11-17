package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type cssKeyframesWant struct {
	name string
	line int
}

func TestCSSKeyframesOriginalSourceNavigation(t *testing.T) {
	const portable = "@keyframes pulse {\n  from { opacity: 0; }\n  50%,\n  75% { opacity: .8; }\n  to { opacity: 1; }\n}\n.pulse { animation-name: pulse; }\n"
	for _, tc := range []struct{ name, source string }{
		{"LF", portable},
		{"CRLF", strings.ReplaceAll(portable, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			checkCSSOriginalKeyframes(t, tc.source, []cssKeyframesWant{{"pulse", 1}}, []string{".pulse"})
		})
	}
	t.Run("prefixed-and-unprefixed-keyframes", func(t *testing.T) {
		const source = "@-webkit-keyframes fade { from { opacity: 0; } }\n" +
			"@keyframes fade { to { opacity: 1; } }\n" +
			".after { animation-name: fade; }\n"
		result, err := (CSSAnalyzer{}).Analyze(context.Background(),
			testSourceDocument("animation.css", source), testAnalyzeOptions(true, 512))
		if err != nil {
			t.Fatal(err)
		}
		lines := make(map[int]bool)
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Kind == SymbolKindSection && symbol.NativeKind == "keyframes" && symbol.Name == "fade" {
				lines[symbol.NameRange.Start.Line] = true
			}
			if symbol.Kind == SymbolKindSelector && (symbol.Name == "from" || symbol.Name == "to") {
				t.Errorf("vendor animation stop falsely promoted to selector: %+v", symbol)
			}
		}
		if !lines[1] || !lines[2] {
			t.Errorf("prefixed/unprefixed authored animations missing: %v", lines)
		}
	})
	t.Run("unsupported-headers-and-bodies-fail-closed", func(t *testing.T) {
		for _, body := range []string{
			"@keyframes 3invalid { from { opacity: 0; } }\n",
			"@keyframes broken { to { opacity: 1; }\n",
			"@keyframes huge {" + strings.Repeat(" ", 66000) + " to { opacity: 1; } }\n",
		} {
			source := "/* @keyframes fake { 20% { x: y; } } */\n" +
				".before { content: \"@keyframes hidden { from { x: y; } }\"; }\n" +
				body
			result, err := (CSSAnalyzer{}).Analyze(context.Background(),
				testSourceDocument("animation.css", source), testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			seenWarning := false
			for _, diagnostic := range result.Analysis.Diagnostics {
				if diagnostic.Code == "css-unsupported-keyframes" && diagnostic.Range != nil {
					seenWarning = true
				}
			}
			for _, symbol := range result.Analysis.Symbols {
				if symbol.Kind == SymbolKindSection && symbol.NativeKind == "keyframes" {
					t.Errorf("unsupported/fake CSS keyframes unexpectedly indexed: %+v", symbol)
				}
				if symbol.Kind == SymbolKindSelector &&
					(symbol.Name == "from" || symbol.Name == "to" || symbol.Name == "20%") {
					t.Errorf("unsupported/fake stop became selector: %+v", symbol)
				}
			}
			if !seenWarning || result.Analysis.CoverageComplete {
				t.Error("unsupported keyframe header/body lacks ranged incomplete-coverage diagnostic")
			}
		}
	})
	for _, tc := range []struct {
		name, env, marker string
		animations        []cssKeyframesWant
		selectors         []string
	}{
		{"animate-css", "SCRIPTHOLD_REAL_CSS_KEYFRAMES_ANIMATE", "@keyframes hinge {",
			[]cssKeyframesWant{{"hinge", 1}}, []string{".hinge"}},
		{"hover-css", "SCRIPTHOLD_REAL_CSS_KEYFRAMES_HOVER", "@keyframes circle {",
			[]cssKeyframesWant{{"circle", 478}, {"initials", 491}}, []string{".ild-ident"}},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent complete CSS original source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 100 || info.Size() > 15000 {
				t.Fatalf("original CSS source missing/nonregular/oversized: %v %v", info, err)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(content), tc.marker) {
				t.Fatal("pinned original CSS marker drift")
			}
			checkCSSOriginalKeyframes(t, string(content), tc.animations, tc.selectors)
		})
	}
}

func checkCSSOriginalKeyframes(t *testing.T, source string, animations []cssKeyframesWant, selectors []string) {
	t.Helper()
	result, err := (CSSAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("animation.css", source), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(source, "\n")
	seen := map[string]bool{}
	for _, symbol := range result.Analysis.Symbols {
		if symbol.Kind == SymbolKindSelector {
			switch symbol.Name {
			case "from", "to", "0%", "20%", "40%", "50%", "60%", "75%", "80%", "100%", "20%, 60%", "40%, 80%":
				t.Errorf("CSS keyframe stop falsely indexed as ordinary selector: %+v", symbol)
			}
		}
		if symbol.Kind == SymbolKindSection && symbol.NativeKind == "keyframes" {
			for _, w := range animations {
				if symbol.Name != w.name || symbol.NameRange.Start.Line != w.line {
					continue
				}
				column := strings.Index(lines[w.line-1], w.name) + 1
				if column < 1 || symbol.NameRange.Start.Column != column ||
					symbol.DeclarationRange.Start.Line != w.line ||
					symbol.DeclarationRange.End.Line <= w.line {
					t.Errorf("authored CSS animation %q wrong source range: %+v", w.name, symbol)
				}
				seen[w.name] = true
			}
		}
	}
	for _, w := range animations {
		if !seen[w.name] {
			t.Errorf("authored CSS animation %q line %d not navigable", w.name, w.line)
		}
	}
	for _, want := range selectors {
		found := false
		for _, s := range result.Analysis.Symbols {
			if s.Kind == SymbolKindSelector && s.Name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("neighboring original CSS selector %q lost", want)
		}
	}
}
