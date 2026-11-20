package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type phpHTML98Target struct {
	file, language, name, excerpt string
	line                          int
}

func TestPHPHTML98AdditionalOriginalProjectNavigation(t *testing.T) {
	codeigniter := os.Getenv("SCRIPTHOLD_REAL_PHPHTML98_CODEIGNITER_ROOT")
	joomla := os.Getenv("SCRIPTHOLD_REAL_PHPHTML98_JOOMLA_ROOT")
	if codeigniter == "" && joomla == "" {
		t.Skip("independent original PHP/HTML projects are not configured")
	}
	if codeigniter == "" || joomla == "" {
		t.Fatal("both independent original PHP/HTML projects must be configured together")
	}

	cases := []struct {
		name, root string
		files      []string
		edges      []struct{ file, text string }
		targets    []phpHTML98Target
		negative   struct{ file, name string }
	}{
		{
			name:  "CodeIgniter welcome-menu control versus framework error views",
			root:  codeigniter,
			files: []string{"welcome_message.php", "Home.php", "error_404.php", "production.php"},
			edges: []struct{ file, text string }{
				{"Home.php", "return view('welcome_message');"},
				{"welcome_message.php", "id=\"menuToggle\""},
				{"welcome_message.php", "document.getElementById(\"menuToggle\")"},
				{"welcome_message.php", "function toggleMenu()"},
				{"error_404.php", "ENVIRONMENT !== 'production'"},
				{"production.php", "debug.css"},
			},
			targets: []phpHTML98Target{
				{"Home.php", "php", "Home", "class Home extends BaseController", 5},
				{"Home.php", "php", "index", "return view('welcome_message');", 7},
				{"welcome_message.php", "php-html", "menuToggle", "id=\"menuToggle\"", 214},
			},
			negative: struct{ file, name string }{"production.php", "debug.css"},
		},
		{
			name:  "Joomla contact form and address template selection",
			root:  joomla,
			files: []string{"default.php", "default_form.php", "default_address.php"},
			edges: []struct{ file, text string }{
				{"default.php", "$this->loadTemplate('address')"},
				{"default.php", "$this->loadTemplate('form')"},
				{"default.php", "id=\"selectForm\""},
				{"default_form.php", "id=\"contact-form\""},
				{"default_form.php", "value=\"contact.submit\""},
				{"default_form.php", "HTMLHelper::_('form.token')"},
				{"default_address.php", "class=\"contact-street\""},
				{"default_address.php", "class=\"contact-telephone\""},
				{"default_address.php", "class=\"contact-mobile\""},
			},
			targets: []phpHTML98Target{
				{"default.php", "php-html", "selectForm", "id=\"selectForm\"", 77},
				{"default_form.php", "php-html", "contact-form", "id=\"contact-form\"", 25},
			},
			negative: struct{ file, name string }{"default_address.php", "contact-street"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := NewHandler([]string{tc.root})
			ctx := context.Background()
			sources := make(map[string]string, len(tc.files))
			for _, name := range tc.files {
				path := filepath.Join(tc.root, name)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("missing or oversized original source %s: info=%v err=%v", name, info, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				sources[name] = string(raw)
				language := "php-html"
				if name == "Home.php" {
					language = "php"
				}
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Presentation: "compact", Paths: []string{path},
					Language: language, MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 || len(digest.OverviewFiles) != 1 {
					t.Fatalf("source overview failed for %s: err=%v digest=%+v", name, err, digest)
				}
			}
			// Cross-file calls and DOM references are independently confirmed
			// source facts, not inferred source_query semantic relationships.
			for _, edge := range tc.edges {
				if !strings.Contains(sources[edge.file], edge.text) {
					t.Fatalf("original source evidence missing %s: %q", edge.file, edge.text)
				}
			}
			for _, target := range tc.targets {
				lines := strings.Split(sources[target.file], "\n")
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], target.name) ||
					!strings.Contains(sources[target.file], target.excerpt) {
					t.Fatalf("independent original oracle mismatch %s:%d %s", target.file, target.line, target.name)
				}
				path := filepath.Join(tc.root, target.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: target.language,
					Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
					FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("source find failed %s:%s err=%v result=%+v", target.file, target.name, err, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Path == path && sym.Name == target.name && sym.NameRange.Start.Line == target.line {
						id = sym.ID
					}
				}
				if id == "" {
					t.Fatalf("original target missing %s:%d %s matches=%+v", target.file, target.line, target.name, found.Symbols)
				}
				identity := found.Files[0]
				if identity.SourceFingerprint == "" || identity.Encoding == "" {
					t.Fatalf("unsafe source identity %s: %+v", target.file, identity)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: target.language,
					SymbolID: id, SourceFingerprint: identity.SourceFingerprint,
					Encoding: identity.Encoding, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("fingerprint-bound source excerpt missing %s:%s err=%v show=%+v", target.file, target.name, err, shown.Show)
				}
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{path},
					Language: target.language, MaxFiles: 1, MaxSymbols: 128,
				})
				outlined := false
				for _, symbol := range outline.Symbols {
					if symbol.ID == id && symbol.Name == target.name &&
						symbol.NameRange.Start.Line == target.line {
						outlined = true
					}
				}
				if err != nil || outline.Truncated || !outlined {
					t.Fatalf("detailed original outline missing %s:%d %s err=%v", target.file, target.line, target.name, err)
				}
				_, searched, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: target.language, Query: target.name, Match: "exact",
					MaxFiles: 1, MaxResults: 64,
				})
				linked := false
				if searched.Search != nil {
					for _, hit := range searched.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == identity.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || searched.Search == nil || searched.Coverage.ResultsTruncated || !linked {
					t.Fatalf("original structural search not linked %s:%d %s err=%v searched=%+v", target.file, target.line, target.name, err, searched)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(tc.root, tc.negative.file)},
				Language: "php-html", Query: tc.negative.name, Match: "exact",
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("non-declaration treated as symbol %s err=%v found=%+v", tc.negative.name, err, negative.Symbols)
			}
			t.Logf("source-verified PHP/HTML navigation: %d files, %d targets", len(tc.files), len(tc.targets))
		})
	}
}
