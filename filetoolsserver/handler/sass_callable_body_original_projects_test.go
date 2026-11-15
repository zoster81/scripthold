package handler

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The two independent original sources are never executed. Compass is read
// from its pinned Git object because the corpus keeps that checkout Git-only.
func TestSassOriginalCallableBodyNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Sass sources not configured")
	}
	cases := []struct {
		name, project, source, symbol, header, body, nested, excluded, native string
		start, end                                                            int
		gitObject                                                             bool
	}{
		{
			name: "Christos rem unit conversion", project: "sass-christos-gist-sass-selected",
			source: "files/sass/main.sass", symbol: "rem",
			header: "@function rem($pixels, $context: $browser-context)",
			body:   "@return $pixels / $context * 1rem", nested: "$pixels: $pixels * 1px",
			excluded: "$browser-context: 16", native: "function", start: 3, end: 10,
		},
		{
			name: "Compass top corner mixin", project: "sass-compass",
			source: "compass-style.org/content/stylesheets/core/_base-classes.sass",
			symbol: "round-top-corners", header: "=round-top-corners($num: $default-rounded-corner)",
			body: "@extend .clear-bottom-right-corner", nested: "+round-corners($num)",
			excluded: "=round-bottom-corners", native: "mixin", start: 21, end: 24, gitObject: true,
		},
		{
			name: "Compass independent single-line mixin body", project: "sass-compass",
			source: "compass-style.org/content/stylesheets/core/_base-classes.sass",
			symbol: "round-corners", header: "=round-corners($num: $default-rounded-corner)",
			body: "@extend .round-corners-#{$num}", nested: "@extend .round-corners-#{$num}",
			excluded: "=round-top-corners", native: "mixin", start: 18, end: 19, gitObject: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			originalRoot := filepath.Join(corpus, tc.project)
			root := originalRoot
			path := filepath.Join(root, filepath.FromSlash(tc.source))
			var raw []byte
			var err error
			if tc.gitObject {
				info, statErr := os.Lstat(originalRoot)
				if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					t.Fatalf("original Compass Git checkout unavailable: %v", statErr)
				}
				object := "4de01475c98449c4435359544db443fd3da6e9d3:" + tc.source
				sizeOutput, sizeErr := exec.Command("git", "-C", originalRoot, "cat-file", "-s", object).Output()
				size, parseErr := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
				if sizeErr != nil || parseErr != nil || size < 1 || size > 1<<20 {
					t.Fatalf("original Compass blob unavailable or oversized: size=%d, git=%v, parse=%v", size, sizeErr, parseErr)
				}
				raw, err = exec.Command("git", "-C", originalRoot, "show", object).Output()
				if err != nil || int64(len(raw)) != size {
					t.Fatalf("original Compass blob changed or unavailable: %v", err)
				}
				root = t.TempDir()
				path = filepath.Join(root, filepath.Base(tc.source))
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				info, statErr := os.Lstat(path)
				if statErr != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original Sass file unavailable or unsafe: %v", statErr)
				}
				raw, err = os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
			if tc.start < 1 || tc.end > len(lines) || lines[tc.start-1] != tc.header {
				t.Fatalf("original declaration header or range changed: %s:%d-%d", tc.source, tc.start, tc.end)
			}
			expected := strings.Join(lines[tc.start-1:tc.end], "\n")
			if !strings.Contains(expected, tc.body) || !strings.Contains(expected, tc.nested) ||
				strings.Contains(expected, tc.excluded) {
				t.Fatalf("original independent source oracle changed: %s:%d-%d", tc.source, tc.start, tc.end)
			}

			h := NewHandler([]string{root})
			ctx := context.Background()
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "sass", MaxFiles: 1,
			})
			if err != nil || !digest.CoverageComplete || digest.Truncated || digest.FilesParsed != 1 {
				t.Fatalf("original Sass digest incomplete: %v %+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{path}, Language: "sass",
				Presentation: "detailed", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || !outline.CoverageComplete || outline.Truncated {
				t.Fatalf("original Sass outline incomplete: %v %+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "sass",
				Query: tc.symbol, Match: "exact", FileDetails: "relevant",
				MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || !found.CoverageComplete || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original Sass find incomplete: %v %+v", err, found)
			}
			id := ""
			for _, s := range found.Symbols {
				if s.Name == tc.symbol && s.NativeKind == tc.native &&
					s.DeclarationRange.Start.Line == tc.start && s.DeclarationRange.End.Line == tc.end {
					id = s.ID
				}
			}
			if id == "" {
				t.Fatalf("original callable symbol/range missing: %s:%d %+v", tc.source, tc.start, found.Symbols)
			}
			seen := false
			for _, s := range outline.Symbols {
				if s.ID == id {
					seen = true
					if s.Signature != "" && s.Signature != tc.header {
						t.Fatalf("outline signature must not contain the body: %q", s.Signature)
					}
				}
			}
			if !seen {
				t.Fatalf("callable absent from original outline: %s:%d", tc.source, tc.start)
			}
			f := found.Files[0]
			if f.Encoding == "" || f.SourceFingerprint == "" {
				t.Fatalf("missing source identity: %+v", f)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "sass", Encoding: f.Encoding,
				SymbolID: id, SourceFingerprint: f.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || strings.ReplaceAll(shown.Show.Text, "\r\n", "\n") != expected {
				t.Fatalf("original Sass show must exactly match authored body, without peers: %s:%d err=%v show=%+v",
					tc.source, tc.start, err, shown.Show)
			}

			// Verify lexical variable ownership through the real handler API,
			// including exact find, detailed outline and fingerprint-bound show.
			var variables []struct {
				name   string
				line   int
				parent string
			}
			switch tc.symbol {
			case "rem":
				variables = []struct {
					name   string
					line   int
					parent string
				}{{"browser-context", 2, ""}, {"pixels", 5, id}, {"context", 8, id}}
			case "round-top-corners":
				variables = []struct {
					name   string
					line   int
					parent string
				}{{"default-rounded-corner", 1, ""}}
			}
			for _, want := range variables {
				_, varsFound, findErr := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "sass",
					Query: want.name, Match: "exact", FileDetails: "relevant",
					MaxFiles: 1, MaxSymbols: 128,
				})
				if findErr != nil || !varsFound.CoverageComplete || varsFound.Truncated ||
					len(varsFound.Symbols) != 1 || len(varsFound.Files) != 1 {
					t.Fatalf("original Sass variable lookup failed for %s: %v %+v", want.name, findErr, varsFound)
				}
				v := varsFound.Symbols[0]
				if v.Name != want.name || v.NativeKind != "variable" ||
					v.ParentID != want.parent || v.DeclarationRange.Start.Line != want.line {
					t.Errorf("wrong original Sass variable owner or position for %s: %+v", want.name, v)
				}
				inOutline := false
				for _, outlined := range outline.Symbols {
					if outlined.ID == v.ID {
						inOutline = true
						if outlined.ParentID != want.parent {
							t.Errorf("original variable outline parent for %s: %+v", want.name, outlined)
						}
					}
				}
				if !inOutline {
					t.Errorf("original Sass variable missing from outline: %s", want.name)
				}
				vFile := varsFound.Files[0]
				_, varShow, showErr := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "sass", Encoding: vFile.Encoding,
					SymbolID: v.ID, SourceFingerprint: vFile.SourceFingerprint, MaxBytes: 2048,
				})
				if showErr != nil || varShow.Show == nil ||
					strings.ReplaceAll(varShow.Show.Text, "\r\n", "\n") != lines[want.line-1] {
					t.Errorf("original Sass variable show mismatch for %s: %v %+v", want.name, showErr, varShow.Show)
				}
			}
		})
	}
}
