package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRubyPinnedWholeKamalNestedMemberNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_RUBY_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole Ruby originals not configured")
	}
	path := filepath.Join(root, "ruby-kamal-proxy-eee0083.rb")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "ruby", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 3 {
		t.Fatalf("whole pinned Kamal three original source declarations: err=%v failed=%+v symbols=%+v", err, failed, outlined.Symbols)
	}
	var validateID string
	for _, s := range outlined.Symbols {
		switch s.Name {
		case "validate!":
			if string(s.Kind) != "method" || s.DeclarationRange.Start.Line != 2 ||
				s.DeclarationRange.End.Line != 36 ||
				s.ParentQualifiedName != "Kamal::Configuration::Validator::Proxy" {
				t.Errorf("whole original nested validate range: %+v", s)
			}
			validateID = s.ID
		case "ensure_valid_bind_ips":
			if string(s.Kind) != "method" || s.DeclarationRange.Start.Line != 39 ||
				s.DeclarationRange.End.Line != 44 ||
				s.ParentQualifiedName != "Kamal::Configuration::Validator::Proxy" ||
				s.Visibility != "private" {
				t.Errorf("whole original do-block private method range: %+v", s)
			}
		}
	}
	if validateID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("original method ID or fingerprint missing")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "ruby", Encoding: "utf-8",
		Query: "validate!", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 ||
		found.Symbols[0].DeclarationRange.End.Line != 36 {
		t.Fatalf("nested method find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: validateID, Language: "ruby", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "unless config.nil?") ||
		!strings.Contains(shown.Show.Text, "private_key_pem") ||
		!strings.HasSuffix(strings.TrimSpace(shown.Show.Text), "end") ||
		strings.Contains(shown.Show.Text, "def ensure_valid_bind_ips") {
		t.Fatalf("complete bounded source-backed method show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "ruby", Encoding: "utf-8",
		Query: "validate!", Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 {
		t.Fatalf("nested method structural search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestRubyPinnedWholeIndependentSidekiqNavigationControl(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_RUBY_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned whole Ruby originals not configured")
	}
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{filepath.Join(root, "ruby-sidekiq-ring-1bb4aa0.rb")},
		Presentation: "detailed", Language: "ruby", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated || len(outlined.Symbols) != 6 {
		t.Fatalf("whole independent Sidekiq original six declarations: err=%v failed=%+v count=%d", err, failed, len(outlined.Symbols))
	}
}
