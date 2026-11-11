package handler

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Original optparse-applicative Haskell files are independently read in full.
// This inventory covers module declarations, top-level type signatures and
// data declarations; local let/where bindings and module reexports are not
// top-level declarations and must not masquerade as such.
func TestHaskellOptparseSelectedOriginalCompleteTopLevelInventory(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("original optparse-applicative source corpus not configured")
	}
	project := filepath.Join(root, "haskell59-optparse-audit-selected")
	type decl struct {
		file                       string
		line                       int
		name, kind, native, parent string
	}
	inventory := []decl{
		{"Levenshtein.hs", 3, "Options.Applicative.Help.Levenshtein", "module", "module", ""},
		{"Extra.hs", 2, "Options.Applicative.Extra", "module", "module", ""},
		{"Common.hs", 3, "Options.Applicative.Common", "module", "module", ""},
		{"Help.hs", 1, "Options.Applicative.Help", "module", "module", ""},
		{"Levenshtein.hs", 22, "editDistance", "function", "type-signature", "Options.Applicative.Help.Levenshtein"},
		{"Extra.hs", 51, "helper", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 69, "helperWith", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 89, "hsubparser", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 102, "simpleVersioner", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 115, "execParser", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 119, "customExecParser", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 125, "handleParseResult", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 147, "getParseResult", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 152, "execParserPure", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 172, "parserFailure", "function", "type-signature", "Options.Applicative.Extra"},
		{"Extra.hs", 345, "renderFailure", "function", "type-signature", "Options.Applicative.Extra"},
		{"Common.hs", 125, "OptWord", "type", "data", "Options.Applicative.Common"},
		{"Common.hs", 68, "showOption", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 72, "optionNames", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 77, "isOptionPrefix", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 83, "liftOpt", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 86, "optMatches", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 121, "isArg", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 127, "parseWord", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 140, "searchParser", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 160, "searchOpt", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 172, "searchArg", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 204, "lookupCmd", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 212, "stepParser", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 227, "runParser", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 247, "runParserStep", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 254, "parseError", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 257, "runParserInfo", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 260, "runParserFully", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 269, "evalParser", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 278, "mapParser", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 288, "treeMapParser", "function", "type-signature", "Options.Applicative.Common"},
		{"Common.hs", 333, "simplify", "function", "type-signature", "Options.Applicative.Common"},
	}
	files := []struct{ file, relative string }{
		{"Levenshtein.hs", "src/Options/Applicative/Help/Levenshtein.hs"},
		{"Extra.hs", "src/Options/Applicative/Extra.hs"},
		{"Common.hs", "src/Options/Applicative/Common.hs"},
		{"Help.hs", "src/Options/Applicative/Help.hs"},
	}
	sources := map[string][]string{}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		path := filepath.Join(project, filepath.FromSlash(f.relative))
		requireAdditionalOriginalFile(t, path)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(bytes) > 1<<20 {
			t.Fatalf("oversized original %s", f.relative)
		}
		sources[f.file] = strings.Split(strings.ReplaceAll(string(bytes), "\r\n", "\n"), "\n")
		paths = append(paths, path)
	}
	expected := map[string]decl{}
	for _, d := range inventory {
		lines, ok := sources[d.file]
		if !ok || d.line < 1 || d.line > len(lines) || !strings.Contains(lines[d.line-1], d.name) {
			t.Fatalf("bad independently inventoried declaration %s:%d %q", d.file, d.line, d.name)
		}
		key := d.file + ":" + strconv.Itoa(d.line)
		if _, found := expected[key]; found {
			t.Fatalf("duplicate independent key %s", key)
		}
		expected[key] = d
	}
	h := NewHandler([]string{project})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Language: "haskell", Encoding: "utf-8",
		Presentation: "detailed", MaxFiles: len(files), MaxSymbols: 128,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != len(files) {
		t.Fatalf("original outline incomplete: err=%v failed=%+v result=%+v", err, failed, out)
	}
	actual := map[string]bool{}
	for _, s := range out.Symbols {
		file := filepath.Base(s.Path)
		key := file + ":" + strconv.Itoa(s.NameRange.Start.Line)
		if filepath.Clean(s.Path) != filepath.Join(project, filepath.FromSlash(map[string]string{
			"Levenshtein.hs": "src/Options/Applicative/Help/Levenshtein.hs",
			"Extra.hs":       "src/Options/Applicative/Extra.hs",
			"Common.hs":      "src/Options/Applicative/Common.hs",
			"Help.hs":        "src/Options/Applicative/Help.hs",
		}[file])) {
			t.Errorf("unexpected original path: %+v", s)
		}
		if actual[key] {
			t.Errorf("duplicate original position %s: %+v", key, s)
		}
		actual[key] = true
		d, found := expected[key]
		if !found {
			t.Errorf("false top-level declaration at %s: %s %s", key, s.Kind, s.Name)
			continue
		}
		if s.Name != d.name || string(s.Kind) != d.kind || s.NativeKind != d.native || s.ParentQualifiedName != d.parent {
			t.Errorf("incorrect %s: got %s %s native=%q owner=%q; want %+v", key, s.Kind, s.Name, s.NativeKind, s.ParentQualifiedName, d)
		}
	}
	for key, d := range expected {
		if !actual[key] {
			t.Errorf("missing source declaration at %s: %s", key, d.name)
		}
	}
	if len(out.Symbols) != len(inventory) {
		t.Errorf("expected %d declarations, got %d", len(inventory), len(out.Symbols))
	}
	t.Logf("original complete files=%d supported top-level declarations=%d returned=%d complete=%v", len(files), len(inventory), len(out.Symbols), out.CoverageComplete)
}
