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

type scssPinnedDeclaration struct {
	name, kind, native                                    string
	line, nameColumn, nameEnd, declEndLine, declEndColumn int
	signature                                             string
}

// Three complete original SCSS sources were inventoried independently.
// Mixin authoring is intentionally outside the lexical variable/selector
// contract; a reference to a mixin does not create a named declaration.
func TestSCSSThreeWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SCSS_OUTLINE_ROOT")
	if root == "" {
		t.Skip("three complete pinned SCSS original files not configured")
	}
	cases := []struct {
		file, blob, target string
		size               int64
		expected           []scssPinnedDeclaration
	}{
		{
			file: "bootstrap-lists.scss", blob: "acc3b53d61b50bbbf8d0781d91d46b7bd2177d9c", size: 179, target: "list-unstyled",
		},
		{
			file: "primer-typography.scss", blob: "7beaad43320990748c8d6116d87009f170a9bf41", size: 3074, target: "hide-text",
		},
		{
			file: "bootstrap-form-text.scss", blob: "604e1684afe49c0b58aa98efcee457e726f300ef", size: 682, target: "form-text-tokens",
			expected: []scssPinnedDeclaration{
				{"form-text-tokens", "variable", "variable", 4, 2, 18, 4, 19, "$form-text-tokens:"},
				{"form-text-tokens", "variable", "variable", 8, 2, 18, 8, 19, "$form-text-tokens:"},
				{".form-text", "selector", "selector", 20, 3, 13, 27, 4, ".form-text {"},
			},
		},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("whole pinned SCSS source changed: info=%+v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			blob := sha1.New()
			_, _ = fmt.Fprintf(blob, "blob %d%c", len(data), 0)
			_, _ = blob.Write(data)
			if hex.EncodeToString(blob.Sum(nil)) != tc.blob {
				t.Fatal("whole original Git blob identity mismatch")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "scss", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != len(tc.expected) {
				t.Fatalf("SCSS complete source lexical inventory expected=%d err=%v failed=%+v outline=%+v", len(tc.expected), err, failed, outline)
			}
			byPhysical := make(map[string]int, len(outline.Symbols))
			byID := make(map[string]bool, len(outline.Symbols))
			for i, sym := range outline.Symbols {
				key := fmt.Sprintf("%s@%d", sym.Name, sym.NameRange.Start.Line)
				if _, already := byPhysical[key]; already {
					t.Errorf("duplicate original physical symbol %s", key)
				}
				byPhysical[key] = i
				if sym.ID == "" || byID[sym.ID] {
					t.Errorf("missing/reused authored SCSS symbol ID %+v", sym)
				}
				byID[sym.ID] = true
			}
			for _, want := range tc.expected {
				key := fmt.Sprintf("%s@%d", want.name, want.line)
				idx, ok := byPhysical[key]
				if !ok {
					t.Errorf("missing authored whole-original SCSS declaration %+v", want)
					continue
				}
				sym := outline.Symbols[idx]
				if sym.Name != want.name || sym.QualifiedName != want.name ||
					string(sym.Kind) != want.kind || sym.NativeKind != want.native ||
					sym.ParentID != "" || sym.ParentQualifiedName != "" ||
					sym.NameRange.Start.Line != want.line ||
					sym.NameRange.Start.Column != want.nameColumn ||
					sym.NameRange.End.Line != want.line ||
					sym.NameRange.End.Column != want.nameEnd ||
					sym.DeclarationRange.Start.Line != want.line ||
					sym.DeclarationRange.End.Line != want.declEndLine ||
					sym.DeclarationRange.End.Column != want.declEndColumn ||
					sym.Signature != want.signature {
					t.Errorf("SCSS authored source-first identity/range mismatch want=%+v got=%+v", want, sym)
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != tc.size {
				t.Fatal("whole original fingerprint/bytes absent")
			}
			if len(tc.expected) == 0 {
				failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Language: "scss", Encoding: "utf-8",
					Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 0 {
					t.Fatalf("unsupported SCSS mixin must not appear as lexical symbol err=%v failed=%+v found=%+v", err, failed, found)
				}
				return
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "scss", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 2 {
				t.Fatalf("source-distinct duplicate SCSS variable exact find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			for _, sym := range found.Symbols {
				if !byID[sym.ID] || sym.Name != "form-text-tokens" {
					t.Errorf("incorrect duplicate variable identity %+v", sym)
				}
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "scss", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, "form-text-tokens") {
				t.Fatalf("fingerprint-bound SCSS original show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "scss", Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || query.Search == nil ||
				len(query.Search.Matches) != 2 {
				t.Fatalf("original SCSS duplicate variable structural search: err=%v failed=%+v result=%+v", err, failed, query)
			}
			for _, match := range query.Search.Matches {
				if !byID[match.SymbolID] {
					t.Errorf("structural query invented original symbol %+v", match)
				}
			}
		})
	}
}
