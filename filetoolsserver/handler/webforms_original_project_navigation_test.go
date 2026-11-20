package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Original DevExpress and Telerik WebForms pages supply independent editing
// questions. The original applications and their code-behind are never run.
func TestWebFormsOriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, target, excerpt, related, codeBehind, oracle, unrelated string
		line, relatedLine                                                  int
	}{
		{
			name:   "DevExpress dashboard control and its BeforeRender hook",
			env:    "SCRIPTHOLD_REAL_WEBFORMS_DEVEXPRESS",
			target: "ASPxDashboard1", excerpt: `ID="ASPxDashboard1"`, line: 16,
			related: "selectBox", relatedLine: 12,
			codeBehind: "Default.aspx.cs", oracle: `ClientSideEvents-BeforeRender="onBeforeRender"`,
			unrelated: "RadEditor2",
		},
		{
			name:   "Telerik editor skin and chart configuration",
			env:    "SCRIPTHOLD_REAL_WEBFORMS_TELERIK",
			target: "RadEditor2", excerpt: `ID="RadEditor2"`, line: 56,
			related: "RadHtmlChart1", relatedLine: 35,
			codeBehind: "Default.aspx.cs", oracle: `Skin="Glow"`,
			unrelated: "ASPxDashboard1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_WEBFORMS_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original WebForms sources not configured")
			}
			if root == "" || path == "" {
				t.Fatal("partially configured original WebForms source fixture")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("original source unavailable: %v", err)
			}
			if !strings.Contains(string(raw), tc.excerpt) || !strings.Contains(string(raw), tc.oracle) ||
				!strings.Contains(string(raw), tc.codeBehind) {
				t.Fatal("original WebForms source did not match direct source oracle")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "aspnet-webforms", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("WebForms source digest incomplete: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "aspnet-webforms",
				MaxFiles: 1, MaxSymbols: 64, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated {
				t.Fatalf("WebForms source outline unavailable: err=%v outline=%+v", err, outline)
			}
			targetFound, relatedFound := false, false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line {
					targetFound = true
				}
				if sym.Name == tc.related && sym.NameRange.Start.Line == tc.relatedLine {
					relatedFound = true
				}
			}
			if !targetFound || !relatedFound {
				t.Fatalf("missing original task target or related control: %+v", outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "aspnet-webforms",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || len(found.Files) != 1 {
				t.Fatalf("WebForms exact find failed: err=%v found=%+v", err, found)
			}
			symbolID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line {
					symbolID = sym.ID
				}
			}
			file := found.Files[0]
			if symbolID == "" || file.Encoding == "" ||
				file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("WebForms source selector missing: %+v", found)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "aspnet-webforms",
				Encoding: file.Encoding, SymbolID: symbolID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("WebForms fingerprint-verified source excerpt wrong: err=%v shown=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "aspnet-webforms", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || search.Search == nil || !search.Coverage.CoverageComplete ||
				search.Coverage.ResultsTruncated {
				t.Fatalf("WebForms structural search unavailable: err=%v result=%+v", err, search)
			}
			linked := false
			for _, hit := range search.Search.Matches {
				if hit.Name == tc.target && hit.SymbolID == symbolID &&
					hit.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("WebForms structural search omitted exact source selector")
			}
			_, relations, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "relations", Relation: "dependencies", Paths: []string{path},
				Language: "aspnet-webforms",
				Subject:  &SourceSelectorInput{Kind: "path", Path: path, SourceFingerprint: file.SourceFingerprint},
				MaxFiles: 1, MaxResults: 32, MaxNodes: 32, MaxEdges: 64, MaxDepth: 2,
			})
			if err != nil || relations.Relations == nil || !relations.Coverage.CoverageComplete {
				t.Fatalf("WebForms code-behind source relationship missing: err=%v result=%+v", err, relations)
			}
			codeBehindFound := false
			for _, edge := range relations.Relations.Relations {
				if edge.Target.QualifiedName == tc.codeBehind &&
					edge.Source.SourceFingerprint == file.SourceFingerprint {
					codeBehindFound = true
				}
			}
			if !codeBehindFound {
				t.Fatalf("WebForms code-behind dependency missing: %+v", relations.Relations)
			}
			_, unrelated, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "aspnet-webforms",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || !unrelated.CoverageComplete || len(unrelated.Symbols) != 0 {
				t.Fatalf("fabricated unrelated WebForms control: err=%v result=%+v", err, unrelated.Symbols)
			}
		})
	}
}
