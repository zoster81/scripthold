package handler

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Exact original WPF and Rubberduck XAML snapshots have distinct maintenance
// tasks. The code-behind/runtime behavior is not executed or inferred.
func TestXAMLOriginalProjectNavigation(t *testing.T) {
	cases := []struct {
		name, env, class, target, native, excerpt, oracle, sibling, unrelated string
		line, siblingLine                                                     int
	}{
		{
			name:   "WPF named list control and click handler",
			env:    "SCRIPTHOLD_REAL_XAML_WPF",
			class:  "WpfSample.SampleContent",
			target: "listBox1", native: "Name", line: 16,
			excerpt: `Name="listBox1"`, oracle: `Click="button1_Click"`,
			sibling: "textBox1", siblingLine: 17,
			unrelated: "GitHubIssueUrl",
		},
		{
			name:   "Rubberduck help hyperlink and binding",
			env:    "SCRIPTHOLD_REAL_XAML_RUBBERDUCK",
			class:  "Rubberduck.UI.About.AboutControl",
			target: "GitHubIssueUrl", native: "Name", line: 121,
			excerpt: `Name="GitHubIssueUrl"`,
			oracle:  `ElementName=GitHubIssueUrl`,
			sibling: "Version", siblingLine: 96,
			unrelated: "listBox1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, path := os.Getenv("SCRIPTHOLD_REAL_XAML_ROOT"), os.Getenv(tc.env)
			if root == "" && path == "" {
				t.Skip("independent original XAML project snapshots not configured")
			}
			if root == "" || path == "" {
				t.Fatal("original XAML project fixture only partially configured")
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("original XAML source unavailable: %v", err)
			}
			if !strings.Contains(string(raw), tc.excerpt) ||
				!strings.Contains(string(raw), tc.oracle) || !strings.Contains(string(raw), tc.class) {
				t.Fatal("original XAML source did not match independent source oracle")
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "xaml", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("XAML original digest incomplete: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "xaml",
				MaxFiles: 1, MaxSymbols: 80, Presentation: "detailed",
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("XAML original outline incomplete: err=%v outline=%+v", err, outline)
			}
			classFound, targetFound, siblingFound := false, false, false
			for _, sym := range outline.Symbols {
				if sym.Name == tc.class && string(sym.Kind) == "class" {
					classFound = true
				}
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line &&
					sym.ParentQualifiedName == tc.class && sym.NativeKind == tc.native {
					targetFound = true
				}
				if sym.Name == tc.sibling && sym.NameRange.Start.Line == tc.siblingLine &&
					sym.ParentQualifiedName == tc.class {
					siblingFound = true
				}
			}
			if !classFound || !targetFound || !siblingFound {
				t.Fatalf("original XAML class/target/related field absent: class=%v target=%v sibling=%v symbols=%+v",
					classFound, targetFound, siblingFound, outline.Symbols)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "xaml",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
				FileDetails: "relevant",
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("XAML original find incomplete: err=%v found=%+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.target && sym.NameRange.Start.Line == tc.line &&
					sym.ParentQualifiedName == tc.class {
					id = sym.ID
				}
			}
			if id == "" || found.Files[0].Encoding == "" ||
				found.Files[0].SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("XAML safe exact-source selector missing: %+v", found)
			}
			file := found.Files[0]
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "xaml",
				Encoding: file.Encoding, SymbolID: id,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("verified XAML original source range unavailable: err=%v shown=%+v", err, shown.Show)
			}
			_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "xaml", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || query.Search == nil || !query.Coverage.CoverageComplete ||
				query.Coverage.ResultsTruncated {
				t.Fatalf("XAML original structural search incomplete: err=%v query=%+v", err, query)
			}
			linked := false
			for _, match := range query.Search.Matches {
				if match.Name == tc.target && match.SymbolID == id &&
					match.SourceFingerprint == file.SourceFingerprint {
					linked = true
				}
			}
			if !linked {
				t.Fatalf("XAML structural search lacks matched source selector for %q", tc.target)
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "xaml",
				Query: tc.unrelated, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || !negative.CoverageComplete || len(negative.Symbols) != 0 {
				t.Fatalf("XAML unrelated symbol fabricated: err=%v result=%+v", err, negative.Symbols)
			}
		})
	}
}
