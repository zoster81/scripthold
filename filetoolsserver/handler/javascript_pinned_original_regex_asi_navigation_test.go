package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestJavaScriptPinnedWholeExpressRegexASINavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JAVASCRIPT_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole JavaScript originals not configured")
	}
	path := filepath.Join(root, "express-generator-test-utils.js")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "javascript", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 11 {
		t.Fatalf("whole original Express eleven supported declarations: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
	var regexpID string
	for _, s := range outlined.Symbols {
		if s.Name == "WARNINGS_REGEXP" {
			if string(s.Kind) != "variable" || s.NativeKind != "var" ||
				s.DeclarationRange.Start.Line != 8 || s.DeclarationRange.End.Line != 8 ||
				s.DeclarationRange.End.Column != 73 {
				t.Errorf("real original regex declaration includes unrelated export: %+v", s)
			}
			regexpID = s.ID
		}
	}
	if regexpID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("source-backed regexp identity/fingerprint missing")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "javascript", Encoding: "utf-8",
		Query: "WARNINGS_REGEXP", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 ||
		found.Symbols[0].DeclarationRange.End.Line != 8 {
		t.Fatalf("source name find range: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: regexpID, Language: "javascript", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 2048,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.HasPrefix(shown.Show.Text, "var WARNINGS_REGEXP = /") ||
		strings.Contains(shown.Show.Text, "module.exports") {
		t.Fatalf("regexp show leaks following export: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "javascript", Encoding: "utf-8",
		Query: "WARNINGS_REGEXP", Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 {
		t.Fatalf("regexp structural source search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestJavaScriptPinnedIndependentWholeGeekmagicNavigationControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JAVASCRIPT_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole JavaScript originals not configured")
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{filepath.Join(root, "geekmagic-utils.js")},
		Presentation: "detailed", Language: "javascript", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated ||
		len(outlined.Symbols) != 4 {
		t.Fatalf("independent Geekmagic four complete function declarations: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
}
