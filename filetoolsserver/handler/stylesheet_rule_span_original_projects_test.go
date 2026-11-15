package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The original Less sources are read to establish independent rule boundaries;
// they are never built, run or modified by this test.
func TestLessOriginalRuleShowIncludesPropertiesAndGroup(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original stylesheet snapshots not configured")
	}
	cases := []struct {
		name, project, file, selector, opening, required, negative string
		nameLine, startLine, endLine                               int
	}{
		{
			name:    "Ant Design footer color and background",
			project: "less-antdesign-less-selected", file: "files/less/global.less",
			selector: ".rc-footer", opening: ".rc-footer {",
			required: "background-color: #000;", negative: ".rc-footer a {",
			nameLine: 29, startLine: 29, endLine: 36,
		},
		{
			name:    "Ant Design shared grouped footer icon",
			project: "less-antdesign-less-selected", file: "files/less/global.less",
			selector: ".rc-footer-column-icon > svg",
			opening:  ".rc-footer-column-icon > span,",
			required: "width: 100%;", negative: ".rc-footer-item {",
			nameLine: 75, startLine: 74, endLine: 79,
		},
		{
			name:    "Semantic UI breadcrumb divider layout",
			project: "less-semantic-less-selected", file: "files/less/breadcrumb.less",
			selector: ".ui.breadcrumb .divider", opening: ".ui.breadcrumb .divider {",
			required: "color: @dividerColor;", negative: ".ui.breadcrumb a {",
			nameLine: 43, startLine: 43, endLine: 51,
		},
		{
			name:    "Semantic UI breadcrumb hover link",
			project: "less-semantic-less-selected", file: "files/less/breadcrumb.less",
			selector: ".ui.breadcrumb a:hover", opening: ".ui.breadcrumb a:hover {",
			required: "color: @linkHoverColor;", negative: ".ui.breadcrumb .icon.divider {",
			nameLine: 57, startLine: 57, endLine: 59,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original source unavailable or unsafe: %v %+v", err, info)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.nameLine < 1 || tc.startLine < 1 || tc.endLine >= len(lines) ||
				strings.TrimSpace(lines[tc.startLine-1]) != tc.opening ||
				!strings.Contains(lines[tc.nameLine-1], tc.selector) ||
				!strings.Contains(strings.Join(lines[tc.startLine-1:tc.endLine], "\n"), tc.required) ||
				strings.TrimSpace(lines[tc.endLine-1]) != "}" {
				t.Fatalf("independent original selector oracle changed: %s:%d-%d", tc.file, tc.startLine, tc.endLine)
			}
			ctx, h := context.Background(), NewHandler([]string{root})
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "less", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.FilesParsed != 1 || digest.Truncated {
				t.Fatalf("source digest invalid: %v %+v", err, digest)
			}
			_, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "less",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || !outlined.CoverageComplete || outlined.Truncated {
				t.Fatalf("source outline incomplete: %v %+v", err, outlined)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "less",
				Query: tc.selector, Match: "exact", FileDetails: "relevant",
				MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("source find incomplete: %v %+v", err, found)
			}
			selectorID := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.selector && sym.NameRange.Start.Line == tc.nameLine &&
					sym.DeclarationRange.Start.Line == tc.startLine &&
					sym.DeclarationRange.End.Line == tc.endLine &&
					string(sym.Kind) == "selector" {
					selectorID = sym.ID
				}
			}
			if selectorID == "" {
				t.Fatalf("selector range missing in original source: %s:%d %+v", tc.file, tc.nameLine, found.Symbols)
			}
			seen := false
			for _, sym := range outlined.Symbols {
				if sym.ID == selectorID {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("selector not in detailed outline: %s:%d", tc.file, tc.nameLine)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint == "" {
				t.Fatalf("unsafe selector evidence: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "less",
				Encoding: file.Encoding, SymbolID: selectorID,
				SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.opening) ||
				!strings.Contains(shown.Show.Text, tc.required) ||
				strings.Contains(shown.Show.Text, tc.negative) {
				t.Fatalf("original selector source is incomplete or crosses a sibling: %s:%d err=%v show=%+v",
					tc.file, tc.nameLine, err, shown.Show)
			}
		})
	}
}
