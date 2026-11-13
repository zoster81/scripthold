package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type perl101Target struct {
	file, name, owner, excerpt string
	line                       int
}

// Independent original Git sources were read before the local handler calls.
// Cross-file imports and calls are verified as source evidence, not inferred
// runtime or whole-program relationships.
func TestPerl101AdditionalOriginalProjectNavigation(t *testing.T) {
	mal := os.Getenv("SCRIPTHOLD_REAL_PERL101_MAL_ROOT")
	samples := os.Getenv("SCRIPTHOLD_REAL_PERL101_SAMPLES_ROOT")
	if mal == "" && samples == "" {
		t.Skip("additional independent original Perl projects not configured")
	}
	if mal == "" || samples == "" {
		t.Fatal("both independent original Perl project roots must be configured")
	}
	cases := []struct {
		name, root, directory, absentFile, absentName string
		code, sources                                 []string
		evidence                                      []struct{ file, text string }
		targets                                       []perl101Target
	}{
		{
			name: "MAL interpreter lexical lookup versus core namespace",
			root: mal, directory: filepath.Join("impls", "perl"),
			code:       []string{"Core.pm", "Env.pm", "stepA_mal.pl"},
			sources:    []string{"Core.pm", "Env.pm", "stepA_mal.pl"},
			absentFile: "Env.pm", absentName: "e1",
			evidence: []struct{ file, text string }{
				{"Core.pm", "our %NS = ("},
				{"Env.pm", "while ( not $self->{$key} ) {"},
				{"Env.pm", "#my $e1 = Env->new();"},
				{"stepA_mal.pl", "use Env;"},
				{"stepA_mal.pl", "use Core qw(%NS);"},
				{"stepA_mal.pl", "my $let_env = Env->new($env);"},
			},
			targets: []perl101Target{
				{file: "Env.pm", name: "new", owner: "Env", line: 10, excerpt: "sub new {"},
				{file: "Env.pm", name: "get", owner: "Env", line: 28, excerpt: "sub get {"},
				{file: "Core.pm", name: "assoc", owner: "Core", line: 60, excerpt: "sub assoc {"},
				{file: "stepA_mal.pl", name: "EVAL", line: 85, excerpt: "sub EVAL {"},
				{file: "stepA_mal.pl", name: "special_let", line: 128, excerpt: "sub special_let {"},
			},
		},
		{
			name: "Sample Programs merge and quick sort alternatives",
			root: samples, directory: filepath.Join("archive", "p", "perl"),
			code:       []string{"merge-sort.pl", "quick-sort.pl"},
			sources:    []string{"merge-sort.pl", "quick-sort.pl", "README.md"},
			absentFile: "merge-sort.pl", absentName: "quick_sort",
			evidence: []struct{ file, text string }{
				{"merge-sort.pl", "merge_sort($left);"},
				{"merge-sort.pl", "@$a = _merge( $left, $right );"},
				{"quick-sort.pl", "$a = quick_sort($a);"},
				{"README.md", "Merge Sort in Perl"},
				{"README.md", "Quick Sort in Perl"},
				{"README.md", "This page is auto-generated. DO NOT EDIT!"},
			},
			targets: []perl101Target{
				{file: "merge-sort.pl", name: "parse_list", line: 12, excerpt: "sub parse_list ($s) {"},
				{file: "merge-sort.pl", name: "merge_sort", line: 25, excerpt: "sub merge_sort ($a) {"},
				{file: "merge-sort.pl", name: "_merge", line: 40, excerpt: "sub _merge ( $left, $right ) {"},
				{file: "quick-sort.pl", name: "quick_sort", line: 23, excerpt: "sub quick_sort ($a) {"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sources := make(map[string]string, len(tc.sources))
			for _, name := range tc.sources {
				path := filepath.Join(tc.root, tc.directory, name)
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("missing or oversized original source %s: info=%v err=%v", name, info, err)
				}
				raw, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				sources[name] = string(raw)
			}
			for _, oracle := range tc.evidence {
				if !strings.Contains(sources[oracle.file], oracle.text) {
					t.Fatalf("independent original source evidence absent %s: %q", oracle.file, oracle.text)
				}
			}
			for _, target := range tc.targets {
				lines := strings.Split(sources[target.file], "\n")
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], "sub "+target.name) ||
					!strings.Contains(lines[target.line-1], target.excerpt) {
					t.Fatalf("original Perl declaration oracle changed %s:%d %s", target.file, target.line, target.name)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			outlines := make(map[string]map[string]bool, len(tc.code))
			for _, name := range tc.code {
				path := filepath.Join(tc.root, tc.directory, name)
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Presentation: "compact", Paths: []string{path},
					Language: "perl", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 || len(digest.OverviewFiles) != 1 {
					t.Fatalf("original Perl digest unavailable %s: err=%v result=%+v", name, err, digest)
				}
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{path},
					Language: "perl", MaxFiles: 1, MaxSymbols: 512,
				})
				if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
					t.Fatalf("original Perl outline unavailable %s: err=%v result=%+v", name, err, outline)
				}
				ids := make(map[string]bool, len(outline.Symbols))
				for _, symbol := range outline.Symbols {
					ids[symbol.ID] = true
				}
				outlines[name] = ids
			}
			for _, target := range tc.targets {
				path := filepath.Join(tc.root, tc.directory, target.file)
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "perl",
					Query: target.name, Match: "exact", MaxFiles: 1,
					MaxSymbols: 128, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("Perl find failed %s:%d: err=%v result=%+v", target.file, target.line, err, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Path == path && symbol.Name == target.name && symbol.NativeKind == "sub" &&
						symbol.ParentQualifiedName == target.owner && symbol.NameRange.Start.Line == target.line {
						id = symbol.ID
					}
				}
				if id == "" || !outlines[target.file][id] {
					t.Fatalf("original Perl sub missing or wrong owner %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
				}
				file := found.Files[0]
				if file.SourceFingerprint == "" || file.Encoding == "" {
					t.Fatalf("original Perl safe source selector missing %s: %+v", target.file, file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "perl", Encoding: file.Encoding,
					SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("fingerprint-bound Perl excerpt missing %s:%d: err=%v shown=%+v", target.file, target.line, err, shown.Show)
				}
				_, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "perl", Query: target.name, Match: "exact", MaxFiles: 1, MaxResults: 128,
				})
				linked := false
				if queried.Search != nil {
					for _, hit := range queried.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || queried.Search == nil || queried.Coverage.ResultsTruncated || !linked {
					t.Fatalf("Perl source_query not linked to original %s:%d: err=%v search=%+v", target.file, target.line, err, queried)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(tc.root, tc.directory, tc.absentFile)},
				Language: "perl", Query: tc.absentName, Match: "exact",
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("non-declaration/cross-file Perl symbol fabricated %s: err=%v symbols=%+v", tc.absentName, err, negative.Symbols)
			}
			t.Logf("original Perl maintenance navigation: %d source files, %d targets", len(tc.sources), len(tc.targets))
		})
	}
}
