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
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Paths: paths, Presentation: "compact", MaxFiles: 2, MaxSymbols: 80,
	})
	if err != nil || failed != nil || !out.CoverageComplete || out.FilesParsed != 2 || out.SymbolCount != 34 {
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
