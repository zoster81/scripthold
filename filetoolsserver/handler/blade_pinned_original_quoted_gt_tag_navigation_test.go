package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBladePinnedWholeBagistoQuotedGreaterThanNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_BLADE_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Blade originals not configured")
	}
	path := filepath.Join(root, "bagisto-account-edit.blade.php")
	h := NewHandler([]string{root})
	failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "detailed",
		Language: "blade", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128, IncludeSignatures: true,
	})
	if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated ||
		len(outlined.Symbols) != 3 {
		t.Fatalf("complete Bagisto original three HTML IDs: err=%v failed=%+v symbols=%+v", err, failed, outlined.Symbols)
	}
	var emailID string
	names := map[string]bool{}
	for _, s := range outlined.Symbols {
		names[s.Name] = true
		if s.Name == "email" {
			if string(s.Kind) != "entity" || s.NativeKind != "id-element" ||
				s.NameRange.Start.Line != 91 || s.DeclarationRange.Start.Line != 88 ||
				s.DeclarationRange.End.Line != 95 || s.DeclarationRange.End.Column != 35 ||
				s.SignatureRange == nil || s.SignatureRange.End.Line != 95 ||
				s.SignatureRange.End.Column != 35 || !strings.Contains(s.Signature, "$user->email") ||
				!strings.Contains(s.Signature, ":label=") ||
				!strings.HasSuffix(strings.TrimSpace(s.Signature), "/>") {
				t.Errorf("source-authored quoted PHP arrow tag not whole: %+v", s)
			}
			emailID = s.ID
		}
	}
	for _, name := range []string{"v-two-factor-setup-template", "email", "code"} {
		if !names[name] {
			t.Errorf("missing original Bagisto ID %q", name)
		}
	}
	if emailID == "" || len(outlined.Files) != 1 || outlined.Files[0].SourceFingerprint == "" {
		t.Fatal("complete source identity/fingerprint missing")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "blade", Encoding: "utf-8",
		Query: "email", Match: "exact", MaxFiles: 1, MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 ||
		found.Symbols[0].DeclarationRange.End.Line != 95 {
		t.Fatalf("local MCP find truncated: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: emailID, Language: "blade", Encoding: "utf-8",
		SourceFingerprint: outlined.Files[0].SourceFingerprint, MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, ":value=\"old('email') ?: $user->email\"") ||
		!strings.Contains(shown.Show.Text, ":label=") ||
		!strings.HasSuffix(strings.TrimSpace(shown.Show.Text), "/>") {
		t.Fatalf("whole quoted source show incomplete: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, searched, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Paths: []string{path}, Language: "blade", Encoding: "utf-8",
		Query: "email", Match: "exact", Mode: "structural", MaxResults: 8,
	})
	if err != nil || failed != nil || searched.Search == nil || len(searched.Search.Matches) != 1 {
		t.Fatalf("source structural ID search: err=%v failed=%+v search=%+v", err, failed, searched.Search)
	}
}

func TestBladePinnedSevenWholeIndependentOriginalNavigationControls(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_BLADE_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned complete Blade originals not configured")
	}
	h := NewHandler([]string{root})
	controls := []struct {
		name  string
		count int
	}{
		{"framework-401.blade.php", 3}, {"spatie-can.blade.php", 0},
		{"livewire-actions-view.blade.php", 0}, {"framework-badge-php.blade.php", 0},
		{"livewire-pagination-php.blade.php", 0},
		{"volt-class-multiple-fragments.blade.php", 3},
		{"volt-functional-php-fragment.blade.php", 1},
	}
	for _, tc := range controls {
		t.Run(tc.name, func(t *testing.T) {
			failed, outlined, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Paths: []string{filepath.Join(root, tc.name)},
				Presentation: "detailed", Language: "blade", Encoding: "utf-8", MaxFiles: 1,
				MaxSymbols: 128, IncludeSignatures: true,
			})
			if err != nil || failed != nil || !outlined.CoverageComplete || outlined.Truncated ||
				len(outlined.Symbols) != tc.count {
				t.Fatalf("complete independent original count=%d expected=%d err=%v failed=%+v",
					len(outlined.Symbols), tc.count, err, failed)
			}
		})
	}
}
