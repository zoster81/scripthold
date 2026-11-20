package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKotlinPinnedWholeTimberMultilinePropertyNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_KOTLIN_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole Kotlin originals not configured")
	}
	path := filepath.Join(root, "kotlin-timber-new-53e1559.kt")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "kotlin", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 6 {
		t.Fatalf("whole original six supported Kotlin symbols: err=%v failed=%+v symbols=%+v", err, failed, outlined.Symbols)
	}
	var vendorID string
	for _, s := range outlined.Symbols {
		if s.Name == "vendor" {
			if string(s.Kind) != "property" || s.NativeKind != "val" ||
				s.DeclarationRange.Start.Line != 22 || s.DeclarationRange.End.Line != 27 ||
				s.ParentQualifiedName != "timber.lint.TimberIssueRegistry" {
				t.Errorf("complete authored vendor initializer range: %+v", s)
			}
			vendorID = s.ID
		}
	}
	if vendorID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("source-authored property ID or source fingerprint missing")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "kotlin", Encoding: "utf-8",
		Query: "vendor", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 ||
		found.Symbols[0].DeclarationRange.End.Line != 27 {
		t.Fatalf("local MCP property find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: vendorID, Language: "kotlin", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "Vendor(") ||
		!strings.Contains(shown.Show.Text, "feedbackUrl") ||
		!strings.HasSuffix(strings.TrimSpace(shown.Show.Text), ")") {
		t.Fatalf("source-backed complete Vendor initializer show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "kotlin", Encoding: "utf-8",
		Query: "vendor", Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 {
		t.Fatalf("structural property search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestKotlinPinnedIndependentWholeMoshiNavigationControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_KOTLIN_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole Kotlin originals not configured")
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{filepath.Join(root, "kotlin-moshi-new-889013e.kt")},
		Presentation: "detailed", Language: "kotlin", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 12 {
		t.Fatalf("whole independent Moshi 12 supported declarations: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
}
