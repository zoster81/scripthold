package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/sourceintelligence"
)

// Four exact Git-pinned full XAML originals from two independent upstream
// projects. The oracle was authored from the XML attributes before the local
// source-symbol output; ordinary attributes/references are not declarations.
func TestXAMLPinnedFourWholeOriginalAttributeOwners(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_XAML_R30_ROOT")
	if root == "" {
		t.Skip("four independent original XAML files not configured")
	}
	type declaration struct {
		name, native, parent string
		line                 int
	}
	cases := []struct {
		file     string
		bytes    int64
		expected []declaration
	}{
		{"blazor-App.xaml", 1109, []declaration{
			{"MauiBlazorWeb.App", "x:Class", "", 5},
			{"PageBackgroundColor", "x:Key", "MauiBlazorWeb.App", 9},
			{"PrimaryTextColor", "x:Key", "MauiBlazorWeb.App", 10},
		}},
		{"blazor-MainPage.xaml", 717, []declaration{
			{"MauiBlazorWeb.MainPage", "x:Class", "", 6},
			{"blazorWebView", "x:Name", "MauiBlazorWeb.MainPage", 9},
		}},
		{"windows-App.xaml", 322, []declaration{
			{"DesktopToastsSample.App", "x:Class", "", 1},
		}},
		{"windows-MainWindow.xaml", 538, []declaration{
			{"DesktopToastsSample.MainWindow", "x:Class", "", 1},
			{"ShowToastButton", "x:Name", "DesktopToastsSample.MainWindow", 6},
			{"Output", "x:Name", "DesktopToastsSample.MainWindow", 7},
		}},
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.bytes {
				t.Fatalf("pinned original byte identity changed: %v %v", info, err)
			}
			rejected, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "xaml", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || rejected != nil || outlined.FilesParsed != 1 ||
				!outlined.CoverageComplete || outlined.Truncated ||
				len(outlined.Symbols) != len(tc.expected) {
				t.Fatalf("independent whole-original inventory mismatch: %v %+v", err, outlined)
			}
			byName := make(map[string]sourceintelligence.NormalizedSymbol, len(outlined.Symbols))
			for _, sym := range outlined.Symbols {
				if _, exists := byName[sym.Name]; exists {
					t.Errorf("duplicate source declaration: %+v", sym)
				}
				byName[sym.Name] = sym
			}
			for _, expected := range tc.expected {
				s, ok := byName[expected.name]
				if !ok || s.NativeKind != expected.native || s.NameRange.Start.Line != expected.line ||
					s.ParentQualifiedName != expected.parent ||
					s.DeclarationRange.Start.Line != expected.line || s.ID == "" {
					t.Errorf("missing/wrong authored XAML class/name/key: want %+v got %+v", expected, s)
					continue
				}
				if expected.parent != "" {
					parent, ok := byName[expected.parent]
					if !ok || s.ParentID != parent.ID {
						t.Errorf("source owner link invalid for %s: %+v", s.Name, s)
					}
				}
				rejected, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "xaml",
					Query: expected.name, Match: "exact", MaxFiles: 1,
					MaxSymbols: 16, FileDetails: "relevant",
				})
				if err != nil || rejected != nil || len(found.Symbols) != 1 || len(found.Files) != 1 ||
					found.Symbols[0].ID != s.ID {
					t.Fatalf("source owner find wrong %s: %v %+v", expected.name, err, found)
				}
				file := found.Files[0]
				rejected, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: path, Language: "xaml", Encoding: file.Encoding,
					SymbolID: s.ID, SourceFingerprint: file.SourceFingerprint, MaxBytes: 2048,
				})
				if err != nil || rejected != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, expected.name) {
					t.Fatalf("fingerprint-verified source excerpt %s missing: %v %+v", expected.name, err, shown.Show)
				}
				rejected, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{path},
					Language: "xaml", Query: expected.name, Match: "exact", MaxFiles: 1, MaxResults: 16,
				})
				if err != nil || rejected != nil || queried.Search == nil ||
					len(queried.Search.Matches) != 1 ||
					queried.Search.Matches[0].SymbolID != s.ID ||
					queried.Search.Matches[0].SourceFingerprint != file.SourceFingerprint {
					t.Errorf("original structural query mismatched source identity %s: %v %+v", expected.name, err, queried)
				}
			}
		})
	}
}
