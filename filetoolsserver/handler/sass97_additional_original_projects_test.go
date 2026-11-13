package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type sass97OriginalTarget struct {
	name    string
	line    int
	excerpt string
}

// Each original is a separate, single-source Git repository (not a multi-file
// project). Read and independently pinned upstream originals establish the
// expected edit areas; this checks the bounded navigation, not compiled CSS.
func TestSass97AdditionalOriginalSourceNavigation(t *testing.T) {
	timeline := os.Getenv("SCRIPTHOLD_REAL_SASS97_TIMELINE_ROOT")
	wallet := os.Getenv("SCRIPTHOLD_REAL_SASS97_WALLET_ROOT")
	if timeline == "" && wallet == "" {
		t.Skip("both independent pinned original Sass repositories are unavailable")
	}
	if timeline == "" || wallet == "" {
		t.Fatal("both independent Sass original source roots must be configured together")
	}
	cases := []struct {
		name, root, negativeName string
		targets                  []sass97OriginalTarget
		evidence                 []string
	}{
		{
			name: "Timeline navigation selectors versus period and card positioning",
			root: timeline,
			targets: []sass97OriginalTarget{
				{name: "bg-reg", line: 9, excerpt: "$bg-reg: #dfe3e6"},
				{name: ".periods-container", line: 44, excerpt: "border-bottom: 5px solid $bg-reg"},
				{name: ".timeline-container", line: 124, excerpt: "border-color: $bg-dark"},
				{name: ".cards-container", line: 211, excerpt: "transition: transform .3s ease-in-out"},
			},
			evidence: []string{
				".pres-timeline",
				"  .btn-back, .btn-next",
				"  .timeline",
				"  &:before",
				"  &:after",
				"// background: url('data:image/svg+xml",
			},
			negativeName: "data:image/svg+xml",
		},
		{
			name: "Responsive wallet width and selected coin appearance",
			root: wallet,
			targets: []sass97OriginalTarget{
				{name: "break-small", line: 13, excerpt: "$break-small: 500px"},
				{name: "#app", line: 47, excerpt: "@media (max-width: $break-small)"},
				{name: "#coin-list", line: 131, excerpt: "height: 20%"},
				{name: ".selected-coin", line: 160, excerpt: "background: linear-gradient(to top left"},
				{name: "#eth-card", line: 177, excerpt: "background: radial-gradient(top"},
			},
			evidence: []string{
				"@media (max-height: 430px)",
				"  border-bottom: 100vh solid $secondary-bg",
				"  background: lighten($app-primary-bg, 5%)",
				"  background: linear-gradient(to top left",
			},
			negativeName: "background",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(tc.root, "style.sass")
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("pinned original Sass file missing or oversized: info=%v err=%v", info, err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(raw)
			lines := strings.Split(text, "\n")
			for _, expected := range tc.evidence {
				if !strings.Contains(text, expected) {
					t.Fatalf("independent original task evidence missing %q", expected)
				}
			}
			for _, target := range tc.targets {
				if target.line < 1 || target.line > len(lines) || !strings.Contains(lines[target.line-1], target.name) || !strings.Contains(text, target.excerpt) {
					t.Fatalf("original source oracle mismatch %s:%d %s", path, target.line, target.name)
				}
			}
			h := NewHandler([]string{tc.root})
			ctx := context.Background()
			_, overview, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: []string{path},
				Language: "sass", MaxFiles: 1,
			})
			if err != nil || overview.Truncated || overview.FilesParsed != 1 || len(overview.OverviewFiles) != 1 {
				t.Fatalf("bounded Sass overview unavailable: err=%v result=%+v", err, overview)
			}
			for _, target := range tc.targets {
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "sass",
					Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
					FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("source target not found %s: err=%v result=%+v", target.name, err, found)
				}
				var symbolID string
				for _, sym := range found.Symbols {
					if sym.Name == target.name && sym.Path == path && sym.NameRange.Start.Line == target.line {
						symbolID = sym.ID
						break
					}
				}
				if symbolID == "" {
					t.Fatalf("original declaration missing %s:%d %s, matches=%+v", path, target.line, target.name, found.Symbols)
				}
				f := found.Files[0]
				if f.SourceFingerprint == "" || f.Encoding == "" {
					t.Fatalf("unsafe source identity: %+v", f)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "sass", SymbolID: symbolID,
					SourceFingerprint: f.SourceFingerprint, Encoding: f.Encoding, MaxBytes: 8192,
				})
				// Sass currently exposes an exact declaration line. Selector
				// properties belong to the independently read source oracle;
				// do not misrepresent this narrow show range as a full body.
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.name) {
					t.Fatalf("fingerprint-bound declaration missing %s: err=%v result=%+v", target.name, err, shown.Show)
				}
				_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "sass", Query: target.name, Match: "exact", MaxFiles: 1, MaxResults: 64,
				})
				linked := false
				if searched.Search != nil {
					for _, hit := range searched.Search.Matches {
						if hit.SymbolID == symbolID && hit.SourceFingerprint == f.SourceFingerprint && hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || searched.Search == nil || searched.Coverage.ResultsTruncated || !linked {
					t.Fatalf("structural original source query not linked %s: err=%v result=%+v", target.name, err, searched)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "sass",
				Query: tc.negativeName, Match: "exact", MaxFiles: 1, MaxSymbols: 24,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("non-declaration incorrectly indexed %q: err=%v symbols=%+v", tc.negativeName, err, negative.Symbols)
			}
			t.Logf("source-verified Sass original navigation: %d lines, %d targets, complete=%v", len(lines), len(tc.targets), overview.CoverageComplete)
		})
	}
}
