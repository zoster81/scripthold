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

type haskellPinnedDeclaration struct {
	name, kind, native, qualified, signature string
	line, nameStart, nameEnd, declarationEnd int
}

// Two full independent Git upstream sources were inventoried before consulting
// Scripthold. Identifier-headed module/functions only: a symbolic (!!!) binder
// is separately documented as outside the current native indexing contract.
func TestHaskellTwoWholePinnedOriginalDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_HASKELL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete upstream Haskell sources not configured")
	}
	cases := []struct {
		file, blob, target string
		bytes              int64
		expected           []haskellPinnedDeclaration
	}{
		{
			file: "shellcheck-Prelude.hs", blob: "7610c466f5f816abad41d41ec8a6963963c801a8", bytes: 1540,
			target: "sconcat1",
			expected: []haskellPinnedDeclaration{
				{"ShellCheck.Prelude", "module", "module", "ShellCheck.Prelude", "module ShellCheck.Prelude where", 22, 8, 26, 32},
				{"headOrDefault", "function", "function-binding", "ShellCheck.Prelude.headOrDefault", "headOrDefault _ (a:_) = a", 28, 1, 14, 26},
				{"lastOrDefault", "function", "function-binding", "ShellCheck.Prelude.lastOrDefault", "lastOrDefault def [] = def", 32, 1, 14, 27},
				{"sconcat1", "function", "type-signature", "ShellCheck.Prelude.sconcat1", "sconcat1 :: (Semigroup t) => [t] -> t", 43, 1, 9, 38},
				{"sconcatOrDefault", "function", "function-binding", "ShellCheck.Prelude.sconcatOrDefault", "sconcatOrDefault def [] = def", 47, 1, 17, 30},
				{"pleaseReport", "function", "function-binding", "ShellCheck.Prelude.pleaseReport", "pleaseReport str = \"ShellCheck internal error, please report: \" ++ str", 51, 1, 13, 71},
			},
		},
		{
			file: "random-Setup.hs", blob: "00bfe1fe44155953e866958b155c10612bb88a76", bytes: 61,
			target: "main",
			expected: []haskellPinnedDeclaration{
				{"main", "function", "type-signature", "main", "main :: IO ()", 3, 1, 5, 14},
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
				t.Fatalf("original upstream source type or size changed: info=%+v err=%v", info, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(b), 0)
			_, _ = hash.Write(b)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("pinned upstream source Git blob changed")
			}
			failed, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "haskell", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outlined.Truncated || !outlined.CoverageComplete ||
				outlined.FilesParsed != 1 || len(outlined.Files) != 1 ||
				len(outlined.Symbols) != len(tc.expected) {
				t.Fatalf("whole original identifier-headed Haskell inventory mismatch: err=%v failed=%+v outlined=%+v", err, failed, outlined)
			}
			symbols := make(map[string]int, len(outlined.Symbols))
			ids := make(map[string]string, len(outlined.Symbols))
			for i, sym := range outlined.Symbols {
				if _, dup := symbols[sym.QualifiedName]; dup {
					t.Errorf("duplicated Haskell authored symbol: %s", sym.QualifiedName)
				}
				symbols[sym.QualifiedName] = i
				ids[sym.QualifiedName] = sym.ID
				if sym.ID == "" {
					t.Errorf("missing identity for %s", sym.QualifiedName)
				}
			}
			for _, exp := range tc.expected {
				idx, found := symbols[exp.qualified]
				if !found {
					t.Errorf("missing supported authored Haskell symbol %+v", exp)
					continue
				}
				sym := outlined.Symbols[idx]
				owner := ""
				if exp.qualified != "ShellCheck.Prelude" && strings.HasPrefix(exp.qualified, "ShellCheck.Prelude.") {
					owner = "ShellCheck.Prelude"
				}
				if sym.Name != exp.name || string(sym.Kind) != exp.kind ||
					sym.NativeKind != exp.native || sym.QualifiedName != exp.qualified ||
					sym.ParentQualifiedName != owner || sym.NameRange.Start.Line != exp.line ||
					sym.NameRange.Start.Column != exp.nameStart || sym.NameRange.End.Line != exp.line ||
					sym.NameRange.End.Column != exp.nameEnd ||
					sym.DeclarationRange.Start.Line != exp.line ||
					sym.DeclarationRange.Start.Column != 1 ||
					sym.DeclarationRange.End.Line != exp.line ||
					sym.DeclarationRange.End.Column != exp.declarationEnd ||
					sym.Signature != exp.signature ||
					sym.SignatureRange == nil || sym.SignatureRange.End.Line != exp.line ||
					sym.SignatureRange.End.Column != exp.declarationEnd {
					t.Errorf("supported source-first Haskell identity/range mismatch: want=%+v got=%+v", exp, sym)
				}
				if owner == "" && sym.ParentID != "" {
					t.Errorf("root declaration has parent %+v", sym)
				}
				if owner != "" && sym.ParentID != ids[owner] {
					t.Errorf("function missing module parent link %+v", sym)
				}
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if fingerprint == "" || outlined.Files[0].SourceBytes != tc.bytes {
				t.Fatal("original source fingerprint/size missing")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "haskell", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated ||
				len(found.Symbols) != 1 || found.Symbols[0].ID != ids[found.Symbols[0].QualifiedName] {
				t.Fatalf("full original Haskell find mismatch: err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "haskell", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.target) {
				t.Fatalf("full original Haskell fingerprint show mismatch: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "haskell", Encoding: "utf-8", Query: tc.target, Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) != 1 ||
				query.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original Haskell structural search mismatch: err=%v failed=%+v query=%+v", err, failed, query)
			}
		})
	}
}
