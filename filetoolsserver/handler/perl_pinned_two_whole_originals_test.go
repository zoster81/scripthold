package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Both independent upstream Git originals were inventoried before querying
// development: the module has exactly two named declarations while the script
// with an anonymous callback and local variables has none.
func TestPerlTwoWholePinnedOriginalDeclarationsAndEmptyScript(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PERL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete pinned upstream Perl original sources not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		count      int
	}{
		{"Ukigumo-Util.pm", "e5486db1ff8fae6ecd77f1ecb9ab85415ba157fb", 631, 2},
		{"CLI-default-prompt.pl", "4a6273e9fd13606c1abdf7590ccd2e95f55e498f", 626, 0},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("complete original source changed: info=%+v err=%v", info, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(original), 0)
			_, _ = hash.Write(original)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("whole Git original blob identity changed")
			}
			failed, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "perl", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outlined.Truncated || !outlined.CoverageComplete ||
				outlined.FilesParsed != 1 || len(outlined.Files) != 1 ||
				len(outlined.Symbols) != tc.count {
				t.Fatalf("whole authored original Perl inventory expected %d: err=%v failed=%+v outline=%+v", tc.count, err, failed, outlined)
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if fingerprint == "" || outlined.Files[0].SourceBytes != tc.size {
				t.Fatal("original source identity/fingerprint missing")
			}
			if tc.count == 0 {
				// The complete script contains a genuine anonymous 'sub { }', lexical
				// locals, imports and function calls, none of which is a named declaration.
				failed, find, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "perl", Encoding: "utf-8",
					Query: "prompt", Match: "exact", MaxFiles: 1, MaxSymbols: 32,
				})
				if err != nil || failed != nil || find.Truncated || len(find.Symbols) != 0 {
					t.Fatalf("empty complete original must not invent prompt declaration err=%v failed=%+v result=%+v", err, failed, find)
				}
				return
			}
			if len(outlined.Symbols) != 2 {
				t.Fatal("expected two source-declared symbols")
			}
			pkg, sub := outlined.Symbols[0], outlined.Symbols[1]
			if pkg.Name != "t::Util" || pkg.QualifiedName != "t::Util" ||
				string(pkg.Kind) != "module" || pkg.NativeKind != "package" ||
				pkg.ParentID != "" || pkg.ParentQualifiedName != "" ||
				pkg.NameRange.Start.Line != 1 || pkg.NameRange.Start.Column != 9 ||
				pkg.NameRange.End.Line != 1 || pkg.NameRange.End.Column != 16 ||
				pkg.DeclarationRange.Start.Line != 1 || pkg.DeclarationRange.Start.Column != 1 ||
				pkg.DeclarationRange.End.Line != 1 || pkg.DeclarationRange.End.Column != 16 ||
				pkg.Signature != "package t::Util" || pkg.ID == "" {
				t.Errorf("independently authored Perl package declaration mismatch %+v", pkg)
			}
			if sub.Name != "dispense_client" || sub.QualifiedName != "t::Util.dispense_client" ||
				string(sub.Kind) != "function" || sub.NativeKind != "sub" ||
				sub.ParentID != pkg.ID || sub.ParentQualifiedName != "t::Util" ||
				sub.NameRange.Start.Line != 11 || sub.NameRange.Start.Column != 5 ||
				sub.NameRange.End.Line != 11 || sub.NameRange.End.Column != 20 ||
				sub.DeclarationRange.Start.Line != 11 || sub.DeclarationRange.Start.Column != 1 ||
				sub.DeclarationRange.End.Line != 11 || sub.DeclarationRange.End.Column != 22 ||
				sub.Signature != "sub dispense_client {" || sub.ID == "" {
				t.Errorf("independently authored Perl named-sub declaration mismatch %+v", sub)
			}
			// A source-first body ending at line 25 is not a supported handler
			// declaration span; the handler deliberately reports the header only.
			failed, find, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "perl", Encoding: "utf-8",
				Query: "dispense_client", Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || find.Truncated ||
				len(find.Symbols) != 1 || find.Symbols[0].ID != sub.ID {
				t.Fatalf("authored original named Perl sub find mismatch err=%v failed=%+v result=%+v", err, failed, find)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "perl", Encoding: "utf-8",
				SymbolID: sub.ID, SourceFingerprint: fingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, "dispense_client") {
				t.Fatalf("original fingerprint-bound Perl show mismatch err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "perl", Encoding: "utf-8", Query: "dispense_client", Match: "exact",
				MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != sub.ID {
				t.Fatalf("original Perl structural search mismatch err=%v failed=%+v result=%+v", err, failed, queried)
			}
		})
	}
}
