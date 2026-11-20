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

type mql5OriginalDeclaration struct {
	name, kind, native                                                  string
	line, nameColumn, nameEnd, declarationEndLine, declarationEndColumn int
	signaturePrefix                                                     string
}

// These two complete Git originals were independently inventoried before
// running development. Top-level input variables and event definitions count;
// #property/#define/#include, local variables and called methods do not.
func TestMQL5TwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_MQL5_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete upstream pinned MQL5 originals not configured")
	}
	cases := []struct {
		file, blob, target string
		bytes              int64
		expected           []mql5OriginalDeclaration
	}{
		{
			file: "metatrader-Grid.mq5", blob: "0035aa6dd63395d29ebc1aec34f645d543730e47", bytes: 3077, target: "OnCalculate",
			expected: []mql5OriginalDeclaration{
				{"InpP1", "variable", "input-variable", 13, 11, 16, 13, 22, ""},
				{"InpColor", "variable", "input-variable", 14, 13, 21, 14, 32, ""},
				{"InpStyle", "variable", "input-variable", 15, 23, 31, 15, 44, ""},
				{"InpMaxBars", "variable", "input-variable", 16, 11, 21, 16, 30, ""},
				{"OnInit", "function", "function-definition", 21, 5, 11, 53, 2, "int OnInit()"},
				{"OnDeinit", "function", "function-definition", 58, 6, 14, 60, 2, "void OnDeinit(const int reason)"},
				{"OnCalculate", "function", "function-definition", 65, 5, 16, 77, 2, "int OnCalculate(const int rates_total,"},
			},
		},
		{
			file: "alfredo-code9.mq5", blob: "c80fe33ebeb532fbd53834b8300d63cab46f70fe", bytes: 520, target: "OnTick",
			expected: []mql5OriginalDeclaration{
				{"trade", "variable", "variable", 2, 10, 15, 2, 16, ""},
				{"OnTick", "function", "function-definition", 4, 6, 12, 26, 4, "void OnTick()"},
			},
		},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.bytes {
				t.Fatalf("complete MQL5 pinned original changed: info=%+v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			blob := sha1.New()
			_, _ = fmt.Fprintf(blob, "blob %d%c", len(data), 0)
			_, _ = blob.Write(data)
			if hex.EncodeToString(blob.Sum(nil)) != tc.blob {
				t.Fatal("whole original Git blob identity changed")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "mql5", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 ||
				len(outline.Symbols) != len(tc.expected) {
				t.Fatalf("whole original MQL5 inventory expected=%d err=%v failed=%+v result=%+v", len(tc.expected), err, failed, outline)
			}
			actual := make(map[string]int, len(outline.Symbols))
			for i, sym := range outline.Symbols {
				if _, duplicate := actual[sym.Name]; duplicate {
					t.Errorf("duplicate authored declaration %s", sym.Name)
				}
				actual[sym.Name] = i
				if sym.ID == "" {
					t.Errorf("empty structural identity for %+v", sym)
				}
			}
			for _, want := range tc.expected {
				i, found := actual[want.name]
				if !found {
					t.Errorf("missing original authored MQL5 declaration %+v", want)
					continue
				}
				sym := outline.Symbols[i]
				if sym.Name != want.name || sym.QualifiedName != want.name ||
					string(sym.Kind) != want.kind || sym.NativeKind != want.native ||
					sym.ParentID != "" || sym.ParentQualifiedName != "" ||
					sym.NameRange.Start.Line != want.line ||
					sym.NameRange.Start.Column != want.nameColumn ||
					sym.NameRange.End.Line != want.line ||
					sym.NameRange.End.Column != want.nameEnd ||
					sym.DeclarationRange.Start.Line != want.line ||
					sym.DeclarationRange.Start.Column != 1 ||
					sym.DeclarationRange.End.Line != want.declarationEndLine ||
					sym.DeclarationRange.End.Column != want.declarationEndColumn {
					t.Errorf("MQL5 source-first name/kind/range/ownership mismatch want=%+v got=%+v", want, sym)
				}
				if want.signaturePrefix != "" {
					if !strings.HasPrefix(sym.Signature, want.signaturePrefix) ||
						sym.SignatureRange == nil ||
						sym.SignatureRange.Start.Line != want.line {
						t.Errorf("MQL5 original event signature mismatch want-prefix=%q got=%q", want.signaturePrefix, sym.Signature)
					}
					if want.name == "OnCalculate" &&
						(sym.SignatureRange.End.Line != 74 || sym.SignatureRange.End.Column != 38 ||
							!strings.Contains(sym.Signature, "const int &spread[]")) {
						t.Errorf("MQL5 multiline function signature lost source extent %+v", sym)
					}
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != tc.bytes {
				t.Fatal("whole original source fingerprint/bytes absent")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "mql5", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 ||
				found.Symbols[0].ID != outline.Symbols[actual[tc.target]].ID {
				t.Fatalf("whole original exact event find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "mql5", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.target) {
				t.Fatalf("fingerprint-bound original MQL5 show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "mql5", Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || query.Search == nil ||
				len(query.Search.Matches) != 1 ||
				query.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original MQL5 structural search mismatch: err=%v failed=%+v result=%+v", err, failed, query)
			}
		})
	}
}
