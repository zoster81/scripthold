package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Opt-in fixtures are byte-identical with independent original Twig projects.
// Related layouts/partials and distractors are checked in their tracked sources;
// tests exercise local handlers only, never run third-party applications.
func TestTwigOriginalTemplateTaskNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, extends, mainExcerpt, laterExcerpt, notDeclared string
		startLine, lastLine                                                int
	}{
		{
			name:   "Symfony profiler AJAX toolbar",
			env:    "SCRIPTHOLD_REAL_TWIG_PROFILER",
			target: "toolbar", extends: "@WebProfiler/Profiler/layout.html.twig",
			mainExcerpt:  "sf-toolbar-ajax-request-counter",
			laterExcerpt: "include('@WebProfiler/Profiler/toolbar_item.html.twig'",
			notDeclared:  "layout",
			startLine:    3, lastLine: 35,
		},
		{
			name:   "Craft CMS 400 error message",
			env:    "SCRIPTHOLD_REAL_TWIG_CRAFTCMS",
			target: "message", extends: "_layouts/message",
			mainExcerpt:  "The request could not be understood by the server",
			laterExcerpt: "{% endblock %}",
			notDeclared:  "title",
			startLine:    4, lastLine: 7,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path, root := os.Getenv(tc.env), os.Getenv("SCRIPTHOLD_REAL_TWIG_ROOT")
			if path == "" || root == "" {
				t.Skip("original Twig source fixture not configured")
			}
			ctx := context.Background()
			h := NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "twig", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("incomplete template digest: error=%v digest=%+v", err, digest)
			}
			linked := false
			for _, dep := range digest.Digests[0].Dependencies {
				if dep.Value == tc.extends {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("missing template extends link %q: %+v", tc.extends, digest.Digests[0].Dependencies)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "twig",
				MaxFiles: 1, MaxSymbols: 40, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated ||
				outline.FilesParsed != 1 {
				t.Fatalf("incomplete template outline: error=%v result=%+v", err, outline)
			}
			present := false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.startLine {
					present = true
				}
			}
			if !present {
				t.Fatalf("outline omitted relevant template block %s", tc.target)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "twig", Query: tc.target,
				Match: "exact", MaxFiles: 1, MaxSymbols: 20, FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated ||
				found.FilesParsed != 1 || len(found.Files) != 1 {
				t.Fatalf("incomplete template find: error=%v result=%+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && string(sym.Kind) == "section" &&
					sym.NameRange.Start.Line == tc.startLine &&
					sym.DeclarationRange.End.Line == tc.lastLine &&
					sym.BodyRange != nil {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("missing full source scope %s: %+v", tc.target, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatal("missing valid source fingerprint or encoding")
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "twig", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.mainExcerpt) ||
				!strings.Contains(shown.Show.Text, tc.laterExcerpt) {
				t.Fatalf("fingerprint-bound source excerpt truncated: error=%v show=%+v", err, shown.Show)
			}
			_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "twig", Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 20,
			})
			if err != nil || searched.Search == nil || !searched.Coverage.CoverageComplete ||
				searched.Coverage.ResultsTruncated {
				t.Fatalf("incomplete template structural search: error=%v result=%+v", err, searched)
			}
			matched := false
			for _, m := range searched.Search.Matches {
				if m.Name == tc.target && m.SymbolID == id &&
					m.SourceFingerprint == file.SourceFingerprint {
					matched = true
				}
			}
			if !matched {
				t.Fatalf("structural search omitted %s", tc.target)
			}
			_, absent, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "twig", Query: tc.notDeclared,
				Match: "exact", MaxFiles: 1, MaxSymbols: 20,
			})
			if err != nil || !absent.CoverageComplete || len(absent.Symbols) != 0 {
				t.Fatalf("unrelated or local template value promoted to declaration %s: %+v", tc.notDeclared, absent)
			}
		})
	}
}
