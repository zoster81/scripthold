package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type astro95Target struct {
	file, name, excerpt string
	line                int
}

// Two further independent upstream Astro projects. Selected Git source blobs
// were completely read and independently annotated before querying source_*.
func TestAstro95AdditionalOriginalProjectNavigation(t *testing.T) {
	starlight := os.Getenv("SCRIPTHOLD_REAL_ASTRO95_STARLIGHT_ROOT")
	official := os.Getenv("SCRIPTHOLD_REAL_ASTRO95_OFFICIAL_ROOT")
	if starlight == "" && official == "" {
		t.Skip("further independently pinned Astro source roots unavailable")
	}
	if starlight == "" || official == "" {
		t.Fatal("both independent Astro original source roots must be configured")
	}
	scenarios := []struct {
		name, root                         string
		files                              []string
		targets                            []astro95Target
		evidence                           []struct{ file, excerpt string }
		negativeFile, negativeName, search string
	}{
		{
			name:  "Starlight theme persistence versus language selection",
			root:  starlight,
			files: []string{"ThemeSelect.astro", "ThemeProvider.astro", "Header.astro", "LanguageSelect.astro"},
			targets: []astro95Target{
				{"ThemeSelect.astro", "storeTheme", "localStorage.setItem(storageKey, theme === 'light' || theme === 'dark' ? theme : '');", 39},
				{"ThemeSelect.astro", "onThemeChange", "StarlightThemeProvider.updatePickers(theme);", 50},
				{"LanguageSelect.astro", "localizedPathname", "return localizedUrl(Astro.url, locale, context.trailingSlash).pathname;", 10},
			},
			evidence: []struct{ file, excerpt string }{
				{"ThemeProvider.astro", "updatePickers(theme = storedTheme || 'auto')"},
				{"ThemeSelect.astro", "StarlightThemeProvider.updatePickers();"},
				{"Header.astro", "<ThemeSelect />"},
				{"Header.astro", "<LanguageSelect />"},
			},
			negativeFile: "LanguageSelect.astro", negativeName: "onThemeChange", search: "onThemeChange",
		},
		{
			name:  "Astro hybrid benchmark prerender policy and shared cards",
			root:  official,
			files: []string{"Card.astro", "BaseLayout.astro", "index.astro", "about.astro", "dashboard.astro", filepath.Join("blog", "index.astro")},
			targets: []astro95Target{
				{"index.astro", "prerender", "export const prerender = true;", 5},
				{"dashboard.astro", "prerender", "export const prerender = false;", 4},
				{filepath.Join("blog", "index.astro"), "posts", "const posts = Array.from({ length: 100 }", 7},
				{"index.astro", "posts", "const posts = Array.from({ length: 50 }", 7},
			},
			evidence: []struct{ file, excerpt string }{
				{"Card.astro", "<a href={href}>Read more</a>"},
				{"BaseLayout.astro", "<slot />"},
				{"index.astro", "<Card title={post.title} description={post.description} href={post.href} />"},
				{filepath.Join("blog", "index.astro"), "<Card title={post.title} description={post.description} href={post.href} />"},
				{"dashboard.astro", "Dashboard (Server-Rendered)"},
			},
			negativeFile: "BaseLayout.astro", negativeName: "prerender", search: "prerender",
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			ctx := context.Background()
			h := NewHandler([]string{sc.root})
			allPaths := make([]string, 0, len(sc.files))
			source := map[string]string{}
			for _, rel := range sc.files {
				path := filepath.Join(sc.root, rel)
				stat, err := os.Lstat(path)
				if err != nil || !stat.Mode().IsRegular() || stat.Size() > 1<<20 {
					t.Fatalf("pinned original file unavailable/bounded: %s stat=%v err=%v", rel, stat, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				source[rel] = string(data)
				allPaths = append(allPaths, path)
			}
			for _, e := range sc.evidence {
				if !strings.Contains(source[e.file], e.excerpt) {
					t.Fatalf("original project edge/evidence missing %s %q", e.file, e.excerpt)
				}
			}
			for _, tg := range sc.targets {
				lines := strings.Split(source[tg.file], "\n")
				if tg.line < 1 || tg.line > len(lines) || !strings.Contains(lines[tg.line-1], tg.name) ||
					!strings.Contains(source[tg.file], tg.excerpt) {
					t.Fatalf("independent source oracle changed %s:%d %q", tg.file, tg.line, tg.name)
				}
			}
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: allPaths, Language: "astro", MaxFiles: len(allPaths),
			})
			if err != nil || digest.Truncated || digest.FilesParsed != len(allPaths) ||
				len(digest.OverviewFiles) != len(allPaths) {
				t.Fatalf("Astro original project digest missing files: err=%v digest=%+v", err, digest)
			}
			for _, tg := range sc.targets {
				path := filepath.Join(sc.root, tg.file)
				_, find, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "astro",
					Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64, FileDetails: "relevant",
				})
				if err != nil || find.Truncated || len(find.Files) != 1 {
					t.Fatalf("original target lookup incomplete %s:%s err=%v result=%+v", tg.file, tg.name, err, find)
				}
				var id string
				for _, sym := range find.Symbols {
					if sym.Path == path && sym.Name == tg.name && sym.NameRange.Start.Line == tg.line {
						id = sym.ID
					}
				}
				if id == "" {
					t.Fatalf("original source symbol omitted %s:%d %s symbols=%+v", tg.file, tg.line, tg.name, find.Symbols)
				}
				f := find.Files[0]
				if f.SourceFingerprint == "" || f.Encoding == "" {
					t.Fatalf("unsafe missing original identity: %+v", f)
				}
				_, show, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "astro", SymbolID: id,
					SourceFingerprint: f.SourceFingerprint, Encoding: f.Encoding, MaxBytes: 8192,
				})
				if err != nil || show.Show == nil || !strings.Contains(show.Show.Text, tg.excerpt) {
					t.Fatalf("fingerprint-bound original source excerpt missing %s:%s err=%v shown=%+v", tg.file, tg.name, err, show.Show)
				}
				_, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path}, Language: "astro",
					Query: tg.name, Match: "exact", MaxFiles: 1, MaxResults: 64,
				})
				linked := false
				if query.Search != nil {
					for _, hit := range query.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == f.SourceFingerprint &&
							hit.Range.Start.Line == tg.line {
							linked = true
						}
					}
				}
				if err != nil || query.Search == nil || query.Coverage.ResultsTruncated || !linked {
					t.Fatalf("structural original source query missing %s:%d %s err=%v result=%+v", tg.file, tg.line, tg.name, err, query)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(sc.root, sc.negativeFile)},
				Language: "astro", Query: sc.negativeName, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated Astro component falsely declares %s err=%v result=%+v", sc.negativeName, err, negative.Symbols)
			}
			_, multi, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: allPaths, Language: "astro",
				Query: sc.search, Match: "exact", MaxFiles: len(allPaths), MaxResults: 64,
			})
			if err != nil || multi.Search == nil || multi.Coverage.ResultsTruncated || len(multi.Search.Matches) == 0 {
				t.Fatalf("cross-file structural query not usable: err=%v result=%+v", err, multi)
			}
			t.Logf("original task navigation PASS, %d files and %d independent source targets", len(allPaths), len(sc.targets))
		})
	}
}
