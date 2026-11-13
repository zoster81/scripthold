package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independent, original Vue project evidence for two practical maintenance
// questions. Every selected source was fully read and pinned to its upstream
// Git blob before any development handler queries. This verifies navigation,
// not arbitrary Vue syntax, runtime execution, or automatic tool selection.
func TestVue93AdditionalIndependentOriginalProjectNavigation(t *testing.T) {
	core := os.Getenv("SCRIPTHOLD_REAL_VUE93_CORE_ROOT")
	vuetify := os.Getenv("SCRIPTHOLD_REAL_VUE93_VUETIFY_ROOT")
	if core == "" && vuetify == "" {
		t.Skip("both further independent original Vue project roots are not configured")
	}
	if core == "" || vuetify == "" {
		t.Fatal("both independent pinned Vue original project roots must be provided together")
	}
	type target struct {
		file, name, excerpt, neighbor string
		line                          int
	}
	cases := []struct {
		name, root, unrelated, separateName, sourceEdge string
		files                                           []string
		primary, related                                target
	}{
		{
			name:         "Vue playground dark-theme persistence and parent event wiring",
			root:         core,
			files:        []string{"Header.vue", "App.vue", "VersionSelect.vue"},
			primary:      target{"Header.vue", "toggleDark", "localStorage.setItem(", "", 58},
			related:      target{"App.vue", "toggleTheme", "theme.value = isDark ? 'dark' : 'light'", "", 109},
			unrelated:    "VersionSelect.vue",
			separateName: "setVersion",
			sourceEdge:   "@toggle-theme=\"toggleTheme\"",
		},
		{
			name:         "Vuetify API filtering and props table slot contract",
			root:         vuetify,
			files:        []string{"ApiTable.vue", "PropsTable.vue", "Alert.vue"},
			primary:      target{"ApiTable.vue", "filtered", "user.one.devmode || item.description !== '**FOR INTERNAL USE ONLY**'", "", 83},
			related:      target{"PropsTable.vue", "headers", "const headers = ['name', 'type', 'default']", "", 20},
			unrelated:    "Alert.vue",
			separateName: "icon",
			sourceEdge:   "<ApiApiTable :headers=\"headers\">",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h := NewHandler([]string{tc.root})
			paths := make([]string, 0, len(tc.files))
			texts := map[string]string{}
			for _, rel := range tc.files {
				path := filepath.Join(tc.root, rel)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("pinned original Vue file missing or not bounded regular file %q: info=%v err=%v", rel, info, err)
				}
				bytes, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				texts[rel] = string(bytes)
				paths = append(paths, path)
			}
			for _, tg := range []target{tc.primary, tc.related} {
				source := strings.Split(texts[tg.file], "\n")
				if tg.line < 1 || tg.line > len(source) || !strings.Contains(source[tg.line-1], tg.name) ||
					!strings.Contains(texts[tg.file], tg.excerpt) {
					t.Fatalf("independent original code-oracle changed %s:%d %q", tg.file, tg.line, tg.name)
				}
			}
			if !strings.Contains(texts[tc.related.file], tc.sourceEdge) {
				t.Fatalf("original component event/slot connection missing %q", tc.sourceEdge)
			}
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: paths, Language: "vue", MaxFiles: len(paths),
			})
			if err != nil || digest.Truncated || !digest.CoverageComplete || digest.FilesParsed != len(paths) ||
				len(digest.OverviewFiles) != len(paths) {
				t.Fatalf("original selected project overview incomplete: err=%v digest=%+v", err, digest)
			}
			for _, tg := range []target{tc.primary, tc.related} {
				path := filepath.Join(tc.root, tg.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "vue",
					Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || !found.CoverageComplete || len(found.Files) != 1 {
					t.Fatalf("cannot find original Vue task target %q: err=%v result=%+v", tg.name, err, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == tg.name && sym.NameRange.Start.Line == tg.line && sym.Path == path {
						id = sym.ID
					}
				}
				if id == "" {
					t.Fatalf("original declaration not indexed %s:%d %s symbols=%+v", tg.file, tg.line, tg.name, found.Symbols)
				}
				file := found.Files[0]
				if file.SourceFingerprint == "" || file.Encoding == "" {
					t.Fatalf("cannot safely drill down into original source: %+v", file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "vue", SymbolID: id,
					SourceFingerprint: file.SourceFingerprint, Encoding: file.Encoding, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tg.excerpt) {
					t.Fatalf("fingerprint-bound original excerpt missing for %q: err=%v show=%+v", tg.name, err, shown.Show)
				}
				_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "vue", Query: tg.name, Match: "exact", MaxFiles: 1, MaxResults: 64,
				})
				linked := false
				if search.Search != nil {
					for _, hit := range search.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == tg.line {
							linked = true
						}
					}
				}
				if err != nil || search.Search == nil || search.Coverage.ResultsTruncated || !linked {
					t.Fatalf("original Vue structural source query not bound to target: err=%v search=%+v", err, search)
				}
			}
			unrelated := filepath.Join(tc.root, tc.unrelated)
			_, irrelevant, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{unrelated}, Language: "vue",
				Query: tc.primary.name, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || irrelevant.Truncated || len(irrelevant.Symbols) != 0 {
				t.Fatalf("irrelevant Vue sibling falsely declares task target: err=%v symbols=%+v", err, irrelevant.Symbols)
			}
			_, other, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{unrelated}, Language: "vue",
				Query: tc.separateName, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || len(other.Symbols) == 0 {
				t.Fatalf("unrelated Vue sibling not distinguishable: err=%v symbols=%+v", err, other.Symbols)
			}
			t.Logf("source-verified original Vue task PASS: selectedFiles=%d primary=%s related=%s",
				len(paths), tc.primary.name, tc.related.name)
		})
	}
}
