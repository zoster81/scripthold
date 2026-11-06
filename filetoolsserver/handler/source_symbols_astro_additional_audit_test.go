package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type astroExtraTarget struct {
	file, name, language string
	line                 int
}
type astroExtraScenario struct {
	project, query, primary, unrelated string
	files                              []string
	oracles                            []astroOriginalOracle
	targets                            []astroExtraTarget
}

func TestSourceSymbolsOriginalAstroAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Astro corpus unavailable")
	}
	scenarios := []astroExtraScenario{
		{
			project: "astro-nano", query: "getStaticPaths",
			primary:   "src/pages/blog/[...slug].astro",
			unrelated: "src/components/ArrowCard.astro",
			files: []string{
				"src/pages/blog/[...slug].astro", "src/pages/blog/index.astro",
				"src/pages/projects/[...slug].astro", "src/components/ArrowCard.astro",
			},
			oracles: []astroOriginalOracle{
				{"src/pages/blog/[...slug].astro", "getCollection(\"blog\")"},
				{"src/pages/blog/[...slug].astro", ".filter(post => !post.data.draft)"},
				{"src/pages/blog/[...slug].astro", "params: { slug: post.slug }"},
				{"src/pages/blog/index.astro", "getCollection(\"blog\")"},
				{"src/pages/blog/index.astro", ".filter(post => !post.data.draft)"},
				{"src/pages/blog/index.astro", "posts[year].map((post) => ("},
				{"src/pages/blog/index.astro", "<ArrowCard entry={post}/>"},
				{"src/pages/projects/[...slug].astro", "getCollection(\"projects\")"},
				{"src/pages/projects/[...slug].astro", ".filter(post => !post.data.draft)"},
				{"src/components/ArrowCard.astro", "entry.collection"},
				{"src/components/ArrowCard.astro", "entry.data.title"},
			},
			targets: []astroExtraTarget{
				{"src/pages/blog/[...slug].astro", "getStaticPaths", "astro", 9},
				{"src/pages/blog/index.astro", "data", "astro", 8},
				{"src/pages/projects/[...slug].astro", "getStaticPaths", "astro", 10},
			},
		},
		{
			project: "astro-paper", query: "getStaticPaths",
			primary:   "src/pages/posts/[...page].astro",
			unrelated: "src/components/Pagination.astro",
			files: []string{
				"src/pages/posts/[...page].astro", "src/pages/tags/[tag]/[...page].astro",
				"src/components/Pagination.astro", "src/pages/posts/[...slug]/index.astro",
			},
			oracles: []astroOriginalOracle{
				{"astro-paper.config.ts", "perPage: 4,"},
				{"src/config.ts", "perPage: userConfig.posts?.perPage ?? 4,"},
				{"src/pages/posts/[...page].astro", "getCollection(\"posts\", ({ data }) => !data.draft)"},
				{"src/pages/posts/[...page].astro", "pageSize: config.posts.perPage"},
				{"src/pages/posts/[...page].astro", "<Pagination {page} />"},
				{"src/pages/tags/[tag]/[...page].astro", "pageSize: config.posts.perPage"},
				{"src/pages/tags/[tag]/[...page].astro", "<Pagination {page} />"},
				{"src/components/Pagination.astro", "page.lastPage > 1"},
				{"src/components/Pagination.astro", "href={page.url.prev as string}"},
				{"src/components/Pagination.astro", "href={page.url.next as string}"},
				{"src/pages/posts/[...slug]/index.astro", "params: { slug: getPostSlug(post.id, post.filePath) }"},
			},
			targets: []astroExtraTarget{
				{"src/pages/posts/[...page].astro", "getStaticPaths", "astro", 15},
				{"src/pages/tags/[tag]/[...page].astro", "getStaticPaths", "astro", 17},
				{"src/pages/posts/[...slug]/index.astro", "getStaticPaths", "astro", 19},
				{"src/components/Pagination.astro", "Props", "astro", 9},
				{"src/config.ts", "config", "typescript", 13},
			},
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.project, func(t *testing.T) {
			root := filepath.Join(corpus, sc.project)
			paths := make([]string, 0, len(sc.files))
			for _, rel := range sc.files {
				p := filepath.Join(root, filepath.FromSlash(rel))
				requireAstroOriginalFile(t, p)
				paths = append(paths, p)
			}
			for _, oracle := range sc.oracles {
				p := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireAstroOriginalFile(t, p)
				source, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(source), oracle.text) {
					t.Fatalf("independent source oracle %s %q missing: %v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failure, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "astro", Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failure != nil || digest.Truncated || digest.FilesParsed != len(paths) || !digest.CoverageComplete {
				t.Fatalf("digest incomplete err=%v failure=%+v digest=%+v", err, failure, digest)
			}
			compact, err := json.Marshal(digest)
			if err != nil || len(compact) > 8192 {
				t.Fatalf("compact digest bytes=%d err=%v", len(compact), err)
			}
			t.Logf("files=%d symbols=%d bytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(compact), digest.CoverageComplete)
			for _, target := range sc.targets {
				checkAstroOriginalTarget(t, h, root, astroOriginalTarget{file: target.file, name: target.name, line: target.line}, target.language)
			}
			failure, query, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: sc.query, Match: "exact",
				Mode: "structural", Language: "astro", Encoding: "utf-8", MaxResults: 30,
			})
			if err != nil || failure != nil || query.Search == nil {
				t.Fatalf("structural search err=%v failed=%+v response=%+v", err, failure, query)
			}
			primary := filepath.Join(root, filepath.FromSlash(sc.primary))
			foundPrimary := false
			for _, m := range query.Search.Matches {
				if m.Name == sc.query && filepath.Clean(m.Path) == primary {
					foundPrimary = true
				}
			}
			if !foundPrimary {
				t.Fatalf("structural search missing primary declaration %s: %+v", sc.primary, query.Search.Matches)
			}
			failure, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(sc.unrelated))},
				Query: sc.query, Match: "exact", Language: "astro", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failure != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated source unexpectedly matches %s err=%v failure=%+v symbols=%+v",
					sc.query, err, failure, negative.Symbols)
			}
		})
	}
}
