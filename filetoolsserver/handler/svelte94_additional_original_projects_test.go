package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type svelte94Target struct {
	file, name, excerpt string
	line                int
}

// Two additional original project maintenance questions, distinct from the
// historical Flowbite, Cobalt, Neohtop and Bits project origins. Each input
// was independently read and byte-verified against a pinned upstream Git blob
// before development source_* tools were invoked.
func TestSvelte94AdditionalOriginalProjectNavigation(t *testing.T) {
	skeleton := os.Getenv("SCRIPTHOLD_REAL_SVELTE94_SKELETON_ROOT")
	shadcn := os.Getenv("SCRIPTHOLD_REAL_SVELTE94_SHADCN_ROOT")
	if skeleton == "" && shadcn == "" {
		t.Skip("both independent Svelte original source roots not configured")
	}
	if skeleton == "" || shadcn == "" {
		t.Fatal("both pinned Svelte original projects required together")
	}
	cases := []struct {
		name, root                        string
		files                             []string
		targets                           []svelte94Target
		negativeFile, negativeQuery, edge string
	}{
		{
			name:  "Skeleton accordion content versus trigger and root attribute providers",
			root:  skeleton,
			files: []string{"item-content.svelte", "item-trigger.svelte", "root.svelte"},
			targets: []svelte94Target{
				{"item-content.svelte", "attributes", "getItemContentProps(itemProps())", 20},
				{"item-trigger.svelte", "attributes", "getItemTriggerProps(itemProps())", 20},
				{"root.svelte", "attributes", "getRootProps()", 26},
			},
			negativeFile: "item-content.svelte", negativeQuery: "AccordionItemTriggerProps",
			edge: "AccordionRootContext.provide(() => accordion())",
		},
		{
			name:  "Shadcn block viewer file fallback and independent copy actions",
			root:  shadcn,
			files: []string{"block-viewer.svelte", "block-viewer-toolbar.svelte", "block-viewer-copy-code-button.svelte", "announcement.svelte"},
			targets: []svelte94Target{
				{"block-viewer.svelte", "getFirstFileTargetInTree", "if (!_tree?.length) return null;", 36},
				{"block-viewer-toolbar.svelte", "command", "addCommand.command + \" \" + addCommand.args.join", 31},
				{"block-viewer-copy-code-button.svelte", "clipboard", "new UseClipboard()", 12},
			},
			negativeFile: "announcement.svelte", negativeQuery: "getFirstFileTargetInTree",
			edge: "clipboard.copy(ctx.activeFileCodeToCopy)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			paths := make([]string, 0, len(tc.files))
			sources := map[string]string{}
			for _, f := range tc.files {
				path := filepath.Join(tc.root, f)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original file unavailable/bounded: %s info=%v err=%v", f, info, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				sources[f] = string(data)
				paths = append(paths, path)
			}
			// File and exact line evidence comes only from the original source.
			for _, tg := range tc.targets {
				lines := strings.Split(sources[tg.file], "\n")
				if tg.line < 1 || tg.line > len(lines) || !strings.Contains(lines[tg.line-1], tg.name) ||
					!strings.Contains(sources[tg.file], tg.excerpt) {
					t.Fatalf("independent original declaration mismatch %s:%d %s", tg.file, tg.line, tg.name)
				}
			}
			sawEdge := false
			for _, body := range sources {
				if strings.Contains(body, tc.edge) {
					sawEdge = true
				}
			}
			if !sawEdge {
				t.Fatalf("original cross-component behavior evidence absent %q", tc.edge)
			}
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: paths, Language: "svelte", MaxFiles: len(paths),
			})
			if err != nil || digest.Truncated || digest.FilesParsed != len(paths) || len(digest.OverviewFiles) != len(paths) {
				t.Fatalf("bounded original Svelte project overview failed err=%v digest=%+v", err, digest)
			}
			for _, tg := range tc.targets {
				path := filepath.Join(tc.root, tg.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "svelte",
					Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("source symbol lookup failed %s:%s err=%v result=%+v", tg.file, tg.name, err, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == tg.name && sym.Path == path && sym.NameRange.Start.Line == tg.line {
						id = sym.ID
					}
				}
				if id == "" {
					t.Fatalf("original declaration missing %s:%d %s results=%+v", tg.file, tg.line, tg.name, found.Symbols)
				}
				file := found.Files[0]
				if file.SourceFingerprint == "" || file.Encoding == "" {
					t.Fatalf("fingerprint or encoding missing: %+v", file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "svelte", SymbolID: id,
					Encoding: file.Encoding, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tg.excerpt) {
					t.Fatalf("original fingerprint-verified source excerpt missing: %s:%s err=%v show=%+v", tg.file, tg.name, err, shown.Show)
				}
				_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path}, Language: "svelte",
					Query: tg.name, Match: "exact", MaxFiles: 1, MaxResults: 64,
				})
				linked := false
				if searched.Search != nil {
					for _, m := range searched.Search.Matches {
						if m.SymbolID == id && m.SourceFingerprint == file.SourceFingerprint && m.Range.Start.Line == tg.line {
							linked = true
						}
					}
				}
				if err != nil || searched.Search == nil || !linked || searched.Coverage.ResultsTruncated {
					t.Fatalf("exact structural navigation missing for %s:%s err=%v query=%+v", tg.file, tg.name, err, searched)
				}
			}
			_, irrelevant, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(tc.root, tc.negativeFile)},
				Language: "svelte", Query: tc.negativeQuery, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || irrelevant.Truncated || len(irrelevant.Symbols) != 0 {
				t.Fatalf("unrelated declaration fabricated: err=%v symbols=%+v", err, irrelevant.Symbols)
			}
			t.Logf("source-grounded original Svelte task passed: %d files, %d targets", len(paths), len(tc.targets))
		})
	}
}
