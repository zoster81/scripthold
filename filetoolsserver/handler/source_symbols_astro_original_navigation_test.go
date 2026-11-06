package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type astroOriginalOracle struct{ file, text string }
type astroOriginalTarget struct {
	file, name string
	line       int
}
type astroOriginalTask struct {
	project, query, primary, unrelated string
	files                              []string
	oracles                            []astroOriginalOracle
	targets                            []astroOriginalTarget
	neighbor                           astroOriginalTarget
}

func TestSourceSymbolsOriginalAstroTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Astro corpus unavailable")
	}
	cases := []astroOriginalTask{
		{
			project: "astro-ink", query: "getStaticPaths",
			primary:   "src/pages/blog/[...page].astro",
			unrelated: "src/components/PostDraftPreviewList.astro",
			files: []string{
				"src/pages/blog/[...page].astro",
				"src/components/Paginator.astro",
				"src/components/PostPreviewList.astro",
				"src/components/PostDraftPreviewList.astro",
			},
			oracles: []astroOriginalOracle{
				{"src/config.ts", "export const PAGE_SIZE = 8;"},
				{"src/pages/blog/[...page].astro", "import { PAGE_SIZE } from '$/config'"},
				{"src/pages/blog/[...page].astro", "pageSize: PAGE_SIZE"},
				{"src/pages/blog/[...page].astro", "const sortedPosts = allPosts.sort"},
				{"src/pages/blog/[...page].astro", "<PostPreviewList posts={page.data} />"},
				{"src/pages/blog/[...page].astro", "<Paginator page={page} />"},
				{"src/components/Paginator.astro", "href={page.url.prev}"},
				{"src/components/Paginator.astro", "href={page.url.next}"},
				{"src/components/PostPreviewList.astro", "const sortedPosts = posts.sort"},
				{"src/components/PostDraftPreviewList.astro", "PostDraftPreview post={post}"},
			},
			targets: []astroOriginalTarget{
				{"src/pages/blog/[...page].astro", "getStaticPaths", 13},
				{"src/components/PostPreviewList.astro", "sortedPosts", 12},
			},
			neighbor: astroOriginalTarget{"src/config.ts", "PAGE_SIZE", 51},
		},
		{
			project: "astro-astrowind", query: "getStaticPaths",
			primary:   "src/pages/[...blog]/[...page].astro",
			unrelated: "src/components/blog/Grid.astro",
			files: []string{
				"src/pages/[...blog]/[...page].astro",
				"src/components/blog/Pagination.astro",
				"src/components/blog/List.astro",
				"src/components/blog/Grid.astro",
			},
			oracles: []astroOriginalOracle{
				{"src/pages/[...blog]/[...page].astro", "getStaticPathsBlogList({ paginate })"},
				{"src/pages/[...blog]/[...page].astro", "<BlogList posts={page.data} />"},
				{"src/pages/[...blog]/[...page].astro", "<Pagination prevUrl={page.url.prev} nextUrl={page.url.next} />"},
				{"src/utils/blog.ts", "export const blogPostsPerPage = APP_BLOG?.postsPerPage;"},
				{"src/utils/blog.ts", "export const getStaticPathsBlogList = async"},
				{"src/utils/blog.ts", "pageSize: blogPostsPerPage"},
				{"src/components/blog/Pagination.astro", "prevText = 'Newer posts'"},
				{"src/components/blog/Pagination.astro", "nextText = 'Older posts'"},
				{"src/components/blog/Pagination.astro", "href={getPermalink(prevUrl)}"},
				{"src/components/blog/List.astro", "posts.map((post) => ("},
				{"src/components/blog/Grid.astro", "grid gap-6"},
			},
			targets: []astroOriginalTarget{
				{"src/pages/[...blog]/[...page].astro", "getStaticPaths", 14},
				{"src/components/blog/Pagination.astro", "prevText", 9},
				{"src/components/blog/List.astro", "Props", 5},
			},
			neighbor: astroOriginalTarget{"src/utils/blog.ts", "getStaticPathsBlogList", 177},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				requireAstroOriginalFile(t, path)
				paths = append(paths, path)
			}
			for _, oracle := range tc.oracles {
				p := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireAstroOriginalFile(t, p)
				source, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(source), oracle.text) {
					t.Fatalf("original source oracle %s %q err=%v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "astro", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("compact digest bytes=%d err=%v", len(data), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, target := range tc.targets {
				checkAstroOriginalTarget(t, h, root, target, "astro")
			}
			checkAstroOriginalTarget(t, h, root, tc.neighbor, "typescript")
			failed, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural",
				Language: "astro", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || failed != nil || query.Search == nil {
				t.Fatalf("structural query %s err=%v failed=%+v query=%+v", tc.query, err, failed, query.Search)
			}
			saw := false
			primary := filepath.Join(root, filepath.FromSlash(tc.primary))
			for _, match := range query.Search.Matches {
				if match.Name == tc.query && filepath.Clean(match.Path) == primary {
					saw = true
				}
			}
			if !saw {
				t.Fatalf("structural query omitted primary %s: %+v", tc.primary, query.Search.Matches)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.query, Match: "exact", Language: "astro", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated source err=%v failed=%+v symbols=%+v", err, failed, negative.Symbols)
			}
		})
	}
}

func checkAstroOriginalTarget(t *testing.T, h *Handler, root string, target astroOriginalTarget, language string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(target.file))
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{p}, Query: target.name, Match: "exact",
		Language: language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
	})
	if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
		t.Fatalf("find %s err=%v failed=%+v result=%+v", target.name, err, failed, found)
	}
	id := ""
	for _, symbol := range found.Symbols {
		if symbol.Name == target.name && symbol.NameRange.Start.Line == target.line {
			id = symbol.ID
			break
		}
	}
	if id == "" {
		t.Fatalf("missing original declaration %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: p, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
		Language: language, Encoding: "utf-8", MaxBytes: 16384,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.name) {
		t.Fatalf("fingerprint show %s err=%v failed=%+v shown=%+v", target.name, err, failed, shown.Show)
	}
}

func requireAstroOriginalFile(t *testing.T, p string) {
	t.Helper()
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("original source %s missing or non-regular: %v", p, err)
	}
}
