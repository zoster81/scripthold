package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The opt-in original corpus stays outside the tracked repository.
func TestSourceSymbolsRealPythonRequestsDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_PYTHON_REQUESTS_PROJECT")
	if root == "" {
		t.Skip("original Python Requests project not provided")
	}
	paths := []string{
		filepath.Join(root, "src", "requests", "exceptions.py"),
		filepath.Join(root, "src", "requests", "status_codes.py"),
		filepath.Join(root, "src", "requests", "hooks.py"),
		filepath.Join(root, "src", "requests", "_internal_utils.py"),
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Presentation: "compact", MaxFiles: 4, MaxSymbols: 80,
	})
	if err != nil || failed != nil || !out.CoverageComplete || out.FilesParsed != 4 || out.SymbolCount != 46 {
		t.Fatalf("original Python outline: err=%v failed=%+v count=%d parsed=%d coverage=%v", err, failed, out.SymbolCount, out.FilesParsed, out.CoverageComplete)
	}

	type expected struct {
		kind   string
		line   int
		parent string
	}
	expectedByFile := map[string]map[string]expected{
		"exceptions.py": {
			"RequestException":           {"class", 20, ""},
			"RequestException.response":  {"field", 25, "RequestException"},
			"RequestException.request":   {"field", 26, "RequestException"},
			"RequestException.__init__":  {"method", 28, "RequestException"},
			"InvalidJSONError":           {"class", 38, ""},
			"JSONDecodeError":            {"class", 42, ""},
			"JSONDecodeError.__init__":   {"method", 45, "JSONDecodeError"},
			"JSONDecodeError.__reduce__": {"method", 55, "JSONDecodeError"},
			"HTTPError":                  {"class", 66, ""},
			"ConnectionError":            {"class", 70, ""},
			"ProxyError":                 {"class", 74, ""},
			"SSLError":                   {"class", 78, ""},
			"Timeout":                    {"class", 82, ""},
			"ConnectTimeout":             {"class", 91, ""},
			"ReadTimeout":                {"class", 98, ""},
			"URLRequired":                {"class", 102, ""},
			"TooManyRedirects":           {"class", 106, ""},
			"MissingSchema":              {"class", 110, ""},
			"InvalidSchema":              {"class", 114, ""},
			"InvalidURL":                 {"class", 118, ""},
			"InvalidHeader":              {"class", 122, ""},
			"InvalidProxyURL":            {"class", 126, ""},
			"ChunkedEncodingError":       {"class", 130, ""},
			"ContentDecodingError":       {"class", 134, ""},
			"StreamConsumedError":        {"class", 138, ""},
			"RetryError":                 {"class", 142, ""},
			"UnrewindableBodyError":      {"class", 146, ""},
			"RequestsWarning":            {"class", 153, ""},
			"FileModeWarning":            {"class", 157, ""},
			"RequestsDependencyWarning":  {"class", 161, ""},
		},
		"status_codes.py": {
			"_codes":    {"variable", 23, ""},
			"codes":     {"variable", 106, ""},
			"_init":     {"function", 109, ""},
			"_init.doc": {"function", 116, "_init"},
		},
		"hooks.py": {
			"HOOKS":         {"variable", 22, ""},
			"default_hooks": {"function", 25, ""},
			"dispatch_hook": {"function", 32, ""},
		},
		"_internal_utils.py": {
			"_VALID_HEADER_NAME_RE_BYTE":  {"variable", 13, ""},
			"_VALID_HEADER_NAME_RE_STR":   {"variable", 14, ""},
			"_VALID_HEADER_VALUE_RE_BYTE": {"variable", 15, ""},
			"_VALID_HEADER_VALUE_RE_STR":  {"variable", 16, ""},
			"_HEADER_VALIDATORS_STR":      {"variable", 18, ""},
			"_HEADER_VALIDATORS_BYTE":     {"variable", 19, ""},
			"HEADER_VALIDATORS":           {"variable", 20, ""},
			"to_native_string":            {"function", 26, ""},
			"unicode_is_ascii":            {"function", 39, ""},
		},
	}
	for _, f := range out.OutlineFiles {
		want, exists := expectedByFile[filepath.Base(f.Path)]
		if !exists || !f.CoverageComplete || len(f.Symbols) != len(want) {
			t.Errorf("unexpected file or incomplete inventory %s: count=%d expected=%d", f.Path, len(f.Symbols), len(want))
			continue
		}
		byName := make(map[string]*SourceOutlineNode, len(f.Symbols))
		seen := make(map[string]int, len(f.Symbols))
		for _, sym := range f.Symbols {
			byName[sym.QualifiedName] = sym
			seen[sym.QualifiedName]++
		}
		for name := range want {
			if seen[name] != 1 {
				t.Errorf("missing or duplicated Python declaration %s: %d", name, seen[name])
			}
		}
		for _, sym := range f.Symbols {
			expected, ok := want[sym.QualifiedName]
			if !ok || string(sym.Kind) != expected.kind || sym.Line != expected.line {
				t.Errorf("incorrect Python declaration %+v; expected=%+v exists=%v", sym, expected, ok)
				continue
			}
			parentID := ""
			if expected.parent != "" {
				if byName[expected.parent] == nil {
					t.Errorf("missing parent %s of %s", expected.parent, sym.QualifiedName)
					continue
				}
				parentID = byName[expected.parent].SymbolID
			}
			if sym.ParentID != parentID {
				t.Errorf("wrong owner for %s: parentID=%s, want=%s", sym.QualifiedName, sym.ParentID, parentID)
			}
		}
	}
	// A project map must agree with the independently checked file inventories.
	failed, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "digest", Paths: paths, Presentation: "compact", MaxFiles: 4,
	})
	if err != nil || failed != nil || !digest.CoverageComplete || digest.FilesParsed != 4 ||
		digest.SymbolCount != 46 || len(digest.OverviewFiles) != 4 {
		t.Fatalf("original Python digest: err=%v failed=%+v output=%+v", err, failed, digest)
	}
	fingerprints := make(map[string]string, len(out.OutlineFiles))
	for _, f := range out.OutlineFiles {
		fingerprints[filepath.Base(f.Path)] = f.SourceFingerprint
	}
	for _, f := range digest.OverviewFiles {
		name := filepath.Base(f.Path)
		want := expectedByFile[name]
		if !f.CoverageComplete || f.Status != "parsed" || f.Language != "python" ||
			f.SourceFingerprint == "" || f.SourceFingerprint != fingerprints[name] {
			t.Errorf("inconsistent Python digest file %+v", f)
		}
		counts := make(map[string]int)
		for _, declaration := range f.DeclarationCounts {
			counts[string(declaration.Kind)] += declaration.Count
		}
		wantCounts := make(map[string]int)
		for _, expected := range want {
			wantCounts[expected.kind]++
		}
		if len(counts) != len(wantCounts) {
			t.Errorf("digest kind groups for %s: got=%v want=%v", name, counts, wantCounts)
		}
		for kind, count := range wantCounts {
			if counts[kind] != count {
				t.Errorf("digest %s %s count=%d want=%d", name, kind, counts[kind], count)
			}
		}
	}
	failed, tree, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Presentation: "tree", MaxFiles: 4, MaxSymbols: 80,
	})
	if err != nil || failed != nil || !tree.CoverageComplete || tree.SymbolCount != 46 ||
		len(tree.OutlineFiles) != 4 {
		t.Fatalf("original Python tree: err=%v failed=%+v output=%+v", err, failed, tree)
	}
	for _, file := range tree.OutlineFiles {
		name := filepath.Base(file.Path)
		want := expectedByFile[name]
		seen := make(map[string]int)
		var walk func([]*SourceOutlineNode, string)
		walk = func(nodes []*SourceOutlineNode, parent string) {
			for _, node := range nodes {
				expected, ok := want[node.QualifiedName]
				if !ok || string(node.Kind) != expected.kind || node.Line != expected.line ||
					expected.parent != parent {
					t.Errorf("wrong Python tree node %s parent=%s node=%+v", name, parent, node)
				}
				seen[node.QualifiedName]++
				walk(node.Children, node.QualifiedName)
			}
		}
		walk(file.Symbols, "")
		if len(seen) != len(want) {
			t.Errorf("Python tree %s has %d unique names, want %d", name, len(seen), len(want))
		}
		for expected := range want {
			if seen[expected] != 1 {
				t.Errorf("Python tree %s missing/duplicate %s: %d", name, expected, seen[expected])
			}
		}
	}
	failed, found, err := h.SourceQuery(ctx, nil, SourceQueryInput{
		Operation: "search", Paths: paths, Query: "RequestException.response",
		Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || found.Search == nil || len(found.Search.Matches) != 1 ||
		string(found.Search.Matches[0].Kind) != "field" {
		t.Fatalf("Python field search: err=%v failed=%+v search=%+v", err, failed, found.Search)
	}
	for _, f := range out.OutlineFiles {
		if filepath.Base(f.Path) != "exceptions.py" {
			continue
		}
		for _, sym := range f.Symbols {
			if sym.QualifiedName != "JSONDecodeError.__reduce__" {
				continue
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: f.Path, Encoding: "utf-8", Language: "python",
				SymbolID: sym.SymbolID, SourceFingerprint: f.SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.HasPrefix(shown.Show.Text, "def __reduce__(") ||
				!strings.HasSuffix(shown.Show.Text, "return CompatJSONDecodeError.__reduce__(self)") {
				t.Fatalf("Python method show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			return
		}
	}
	t.Fatal("verified Python method missing from outline")
}
