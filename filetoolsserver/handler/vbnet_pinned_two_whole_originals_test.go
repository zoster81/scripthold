package handler

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type vbnetPinnedDeclaration struct {
	name, kind, native, qualified, owner, signature              string
	line, nameStart, nameEnd, declStart, declEndLine, declEndCol int
}

// Two independently Git-pinned complete upstream VB.NET originals are source
// oracles. Both physical encodings and all supported authored declarations
// were inventoried before requesting any development handler response.
func TestVBNetTwoWholePinnedOriginalDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VBNET_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two independent complete VB.NET originals not configured")
	}
	cases := []struct {
		file, blob, target string
		size               int64
		bom                bool
		crlf               int
		expected           []vbnetPinnedDeclaration
	}{
		{
			file: "dotnet-Program.vb", blob: "cef60b45f8faa79907aaeacce31c260214a416f8",
			size: 445, bom: true, crlf: 0, target: "Main",
			expected: []vbnetPinnedDeclaration{
				{"Program", "module", "module", "Program", "", "Module Program", 1, 8, 15, 1, 15, 11},
				{"Main", "method", "sub", "Program.Main", "Program", "Sub Main()", 2, 9, 13, 5, 14, 12},
			},
		},
		{
			file: "nunit-IMoney.vb", blob: "41b23bb8e52378eaf942a37e8058e245f99997bf",
			size: 957, bom: false, crlf: 31, target: "IsZero",
			expected: []vbnetPinnedDeclaration{
				{"NUnit.Samples", "namespace", "namespace", "NUnit.Samples", "", "Namespace NUnit.Samples", 1, 11, 24, 1, 31, 14},
				{"IMoney", "interface", "interface", "NUnit.Samples.IMoney", "NUnit.Samples", "Public Interface IMoney", 4, 22, 28, 5, 29, 18},
				{"Add", "method", "function", "NUnit.Samples.IMoney.Add", "NUnit.Samples.IMoney", "Function Add(ByVal m As IMoney) As IMoney", 7, 18, 21, 9, 7, 50},
				{"AddMoney", "method", "function", "NUnit.Samples.IMoney.AddMoney", "NUnit.Samples.IMoney", "Function AddMoney(ByVal m As Money) As IMoney", 11, 18, 26, 9, 11, 54},
				{"AddMoneyBag", "method", "function", "NUnit.Samples.IMoney.AddMoneyBag", "NUnit.Samples.IMoney", "Function AddMoneyBag(ByVal s As MoneyBag) As IMoney", 15, 18, 29, 9, 15, 60},
				{"IsZero", "property", "property", "NUnit.Samples.IMoney.IsZero", "NUnit.Samples.IMoney", "ReadOnly Property IsZero() As Boolean", 18, 27, 33, 9, 18, 46},
				{"Multiply", "method", "function", "NUnit.Samples.IMoney.Multiply", "NUnit.Samples.IMoney", "Function Multiply(ByVal factor As Int32) As IMoney", 21, 18, 26, 9, 21, 59},
				{"Negate", "method", "function", "NUnit.Samples.IMoney.Negate", "NUnit.Samples.IMoney", "Function Negate() As IMoney", 24, 18, 24, 9, 24, 36},
				{"Subtract", "method", "function", "NUnit.Samples.IMoney.Subtract", "NUnit.Samples.IMoney", "Function Subtract(ByVal m As IMoney) As IMoney", 27, 18, 26, 9, 27, 55},
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
				t.Fatalf("complete pinned original absent or modified: info=%+v err=%v", info, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			blob := sha1.New()
			_, _ = fmt.Fprintf(blob, "blob %d%c", len(original), 0)
			_, _ = blob.Write(original)
			if hex.EncodeToString(blob.Sum(nil)) != tc.blob {
				t.Fatal("raw original Git blob identity mismatch")
			}
			hasBOM := bytes.HasPrefix(original, []byte{0xef, 0xbb, 0xbf})
			if hasBOM != tc.bom || bytes.Count(original, []byte("\r\n")) != tc.crlf {
				t.Fatal("authored original BOM or line-ending representation changed")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "vbnet", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 ||
				len(outline.Symbols) != len(tc.expected) {
				t.Fatalf("complete whole-original VB.NET outline: err=%v failed=%+v result=%+v", err, failed, outline)
			}
			byName := make(map[string]int, len(outline.Symbols))
			ids := make(map[string]string, len(outline.Symbols))
			for i, sym := range outline.Symbols {
				if _, duplicate := byName[sym.QualifiedName]; duplicate {
					t.Errorf("duplicate declaration %s", sym.QualifiedName)
				}
				byName[sym.QualifiedName] = i
				ids[sym.QualifiedName] = sym.ID
				if sym.ID == "" {
					t.Errorf("empty symbol identity for %s", sym.QualifiedName)
				}
			}
			for _, exp := range tc.expected {
				idx, ok := byName[exp.qualified]
				if !ok {
					t.Errorf("authored VB.NET declaration missing %+v", exp)
					continue
				}
				sym := outline.Symbols[idx]
				if sym.Name != exp.name || sym.QualifiedName != exp.qualified ||
					string(sym.Kind) != exp.kind || sym.NativeKind != exp.native ||
					sym.ParentQualifiedName != exp.owner ||
					sym.NameRange.Start.Line != exp.line || sym.NameRange.Start.Column != exp.nameStart ||
					sym.NameRange.End.Line != exp.line || sym.NameRange.End.Column != exp.nameEnd ||
					sym.DeclarationRange.Start.Line != exp.line ||
					sym.DeclarationRange.Start.Column != exp.declStart ||
					sym.DeclarationRange.End.Line != exp.declEndLine ||
					sym.DeclarationRange.End.Column != exp.declEndCol ||
					sym.Signature != exp.signature ||
					sym.SignatureRange == nil ||
					sym.SignatureRange.End.Line != exp.line {
					t.Errorf("source-first VB.NET identity/scope mismatch want=%+v got=%+v", exp, sym)
				}
				if exp.owner == "" && sym.ParentID != "" {
					t.Errorf("unexpected declaration owner %+v", sym)
				}
				if exp.owner != "" && sym.ParentID != ids[exp.owner] {
					t.Errorf("broken VB.NET direct parent link %+v", sym)
				}
				if exp.owner == "NUnit.Samples.IMoney" && sym.DeclarationRange.End.Line != exp.line {
					t.Errorf("bodyless interface member invented a body: %+v", sym)
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != tc.size {
				t.Fatal("original source fingerprint or length absent")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "vbnet", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 ||
				found.Symbols[0].ID != ids[found.Symbols[0].QualifiedName] {
				t.Fatalf("whole original VB.NET exact find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vbnet", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.target) {
				t.Fatalf("original fingerprint-bound VB.NET show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "vbnet", Encoding: "utf-8", Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original VB.NET structural query mismatch: err=%v failed=%+v result=%+v", err, failed, queried)
			}
		})
	}
}
