package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBladePinnedWholeBagistoQuotedGreaterThanTagSpan(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_BLADE_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned full Blade originals not configured")
	}
	b, err := os.ReadFile(filepath.Join(root, "bagisto-account-edit.blade.php"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 23616 {
		t.Fatalf("Bagisto pinned original differs: %d bytes", len(b))
	}
	for _, tc := range []struct{ name, src string }{
		{"original", string(b)}, {"crlf", strings.ReplaceAll(string(b), "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (BladeAnalyzer{}).Analyze(context.Background(), testSourceDocument("account-edit.blade.php", tc.src), testAnalyzeOptions(true, 96))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Analysis.CoverageComplete || got.Analysis.Truncated || len(got.Analysis.Symbols) != 3 {
				t.Fatalf("complete Bagisto three symbols: count=%d complete=%v truncated=%v", len(got.Analysis.Symbols), got.Analysis.CoverageComplete, got.Analysis.Truncated)
			}
			found := map[string]bool{}
			for _, s := range got.Analysis.Symbols {
				found[s.Name] = true
				if s.Name == "email" {
					if s.Kind != SymbolKindEntity || s.NativeKind != "id-element" ||
						s.NameRange.Start.Line != 91 ||
						s.DeclarationRange.Start.Line != 88 || s.DeclarationRange.End.Line != 95 ||
						s.DeclarationRange.End.Column != 35 ||
						s.SignatureRange == nil || s.SignatureRange.End.Line != 95 ||
						s.SignatureRange.End.Column != 35 ||
						!strings.Contains(s.Signature, "$user->email") ||
						!strings.Contains(s.Signature, ":label=") ||
						!strings.HasSuffix(strings.TrimSpace(s.Signature), "/>") {
						t.Errorf("complete authored HTML component opening tag truncated at quoted PHP object arrow: %+v", s)
					}
				}
			}
			for _, name := range []string{"v-two-factor-setup-template", "email", "code"} {
				if !found[name] {
					t.Errorf("missing whole original entity %q", name)
				}
			}
		})
	}
}

func TestBladePinnedSevenIndependentWholeOriginalControls(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_BLADE_OUTLINE_ROOT")
	if root == "" {
		t.Skip("pinned full Blade originals not configured")
	}
	tests := []struct {
		name          string
		length, count int
	}{
		{"framework-401.blade.php", 131, 3},
		{"spatie-can.blade.php", 88, 0},
		{"livewire-actions-view.blade.php", 1826, 0},
		{"framework-badge-php.blade.php", 1986, 0},
		{"livewire-pagination-php.blade.php", 3508, 0},
		{"volt-class-multiple-fragments.blade.php", 351, 3},
		{"volt-functional-php-fragment.blade.php", 179, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join(root, tc.name))
			if err != nil {
				t.Fatal(err)
			}
			if len(b) != tc.length {
				t.Fatalf("original length changed %d != %d", len(b), tc.length)
			}
			got, err := (BladeAnalyzer{}).Analyze(context.Background(), testSourceDocument(tc.name, string(b)), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if !got.Analysis.CoverageComplete || got.Analysis.Truncated || len(got.Analysis.Symbols) != tc.count {
				t.Fatalf("independent whole original symbols count=%d expected=%d complete=%v truncated=%v", len(got.Analysis.Symbols), tc.count, got.Analysis.CoverageComplete, got.Analysis.Truncated)
			}
		})
	}
}

func TestMarkupQuotedGreaterThanTagBoundaries(t *testing.T) {
	samples := []struct {
		name, src string
		count     int
		id        string
	}{
		{"double-quoted-php-arrow", "<x:input id=\"email\" :value=\"old('email') ?: $user->email\" :label=\"Email\" />\n", 1, "email"},
		{"single-quoted-condition", "<x:input id='email' data-test='left > right' title=\"E\" />\n", 1, "email"},
		{"both-quotes", "<x:input id=\"email\" data-test=\"left > right's\" title='ok > still'/>\n", 1, "email"},
		{"unquoted-greater-than", "<x:input id='email' data-test=1>\n", 1, "email"},
		{"unclosed-quoted-attribute", "<x:input id=\"email\" :value=\"wrong >\n", 0, ""},
		{"normal-following-tag", "<x:input id=\"email\" :value=\"a > b\" />\n<div id=\"after\"></div>\n", 2, "email"},
	}
	for _, tc := range samples {
		t.Run(tc.name, func(t *testing.T) {
			got, err := (HTMLAnalyzer{}).Analyze(context.Background(), testSourceDocument("sample.html", tc.src), testAnalyzeOptions(true, 32))
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Analysis.Symbols) != tc.count {
				t.Fatalf("expected %d symbols, got %+v", tc.count, got.Analysis.Symbols)
			}
			if tc.id != "" && len(got.Analysis.Symbols) > 0 {
				s := got.Analysis.Symbols[0]
				firstTag := strings.SplitN(tc.src, "\n", 2)[0]
				if s.Name != tc.id || s.DeclarationRange.Start.Line != 1 ||
					s.DeclarationRange.End.Line != 1 ||
					s.DeclarationRange.End.Column != len(firstTag)+1 || s.Signature != firstTag {
					t.Errorf("full quoted-attribute tag must preserve original source: %+v, expected %q", s, firstTag)
				}
			}
		})
	}
}
