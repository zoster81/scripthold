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

type typescriptOriginalDeclaration struct {
	name, kind, native                                            string
	line, startCol, endCol, declarationEndLine, declarationEndCol int
	signaturePrefix                                               string
}

// These two independent upstream projects were inventoried before calling
// Scripthold: only their seven supported authored top-level declarations count.
// Example code in JSDoc, import dependencies and nested type members do not.
func TestTypeScriptThreeWholePinnedOriginalDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_TYPESCRIPT_OUTLINE_ROOT")
	if root == "" {
		t.Skip("three whole upstream TypeScript originals not configured")
	}
	cases := []struct {
		file, blob, target string
		bytes              int64
		expected           []typescriptOriginalDeclaration
	}{
		{
			file: "ts-generate.ts", blob: "24ce646c3cc4cdb0a7dd94eaf30fecc3d79b11f3", bytes: 384, target: "force",
			expected: []typescriptOriginalDeclaration{
				{"force", "constant", "const", 6, 9, 14, 6, 42, ""},
			},
		},
		{
			file: "ts-nodeProcess.ts", blob: "5d94abf8f5dc6ab8b56ed4ca01a34ad176111044", bytes: 1005, target: "nodeProcess",
			expected: []typescriptOriginalDeclaration{
				{"nodeProcess", "function", "function-definition", 8, 17, 28, 14, 2, "export function nodeProcess(): NodeJS.Process"},
				{"getDefaultExePath", "function", "function-definition", 16, 17, 34, 24, 2, "export function getDefaultExePath(): string"},
			},
		},
		{
			file: "plimit-index.d.ts", blob: "ff71a8037f467ebb9820dff5a0ccf914f00d3b63", bytes: 4645, target: "pLimit",
			expected: []typescriptOriginalDeclaration{
				{"LimitFunction", "alias", "type-alias", 1, 13, 26, 59, 3, "export type LimitFunction = {"},
				{"pLimit", "function", "function-declaration", 91, 25, 31, 91, 78, "export default function pLimit(concurrency: number | Options): LimitFunction"},
				{"Options", "alias", "type-alias", 93, 13, 20, 109, 3, "export type Options = {"},
				{"limitFunction", "function", "function-declaration", 135, 17, 30, 138, 92, "export function limitFunction<Arguments extends unknown[], ReturnType>("},
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
				t.Fatalf("original TypeScript fixture changed: info=%+v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(data), 0)
			_, _ = hash.Write(data)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("full Git-pinned TypeScript source changed")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "typescript", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != len(tc.expected) {
				t.Fatalf("entire TypeScript original supported inventory: err=%v failed=%+v outline=%+v", err, failed, outline)
			}
			actual := make(map[string]int, len(outline.Symbols))
			ids := make(map[string]string, len(outline.Symbols))
			for i, sym := range outline.Symbols {
				if _, exists := actual[sym.Name]; exists {
					t.Errorf("duplicate original top-level declaration: %s", sym.Name)
				}
				actual[sym.Name] = i
				ids[sym.Name] = sym.ID
				if sym.ID == "" {
					t.Errorf("empty original identity: %+v", sym)
				}
			}
			for _, exp := range tc.expected {
				idx, ok := actual[exp.name]
				if !ok {
					t.Errorf("supported authored symbol missing %+v", exp)
					continue
				}
				sym := outline.Symbols[idx]
				if sym.Name != exp.name || sym.QualifiedName != exp.name ||
					string(sym.Kind) != exp.kind || sym.NativeKind != exp.native ||
					sym.ParentID != "" || sym.ParentQualifiedName != "" ||
					sym.NameRange.Start.Line != exp.line ||
					sym.NameRange.Start.Column != exp.startCol ||
					sym.NameRange.End.Line != exp.line ||
					sym.NameRange.End.Column != exp.endCol ||
					sym.DeclarationRange.Start.Line != exp.line ||
					sym.DeclarationRange.Start.Column != 1 ||
					sym.DeclarationRange.End.Line != exp.declarationEndLine ||
					sym.DeclarationRange.End.Column != exp.declarationEndCol {
					t.Errorf("authored TypeScript source range/kind mismatch want=%+v got=%+v", exp, sym)
				}
				if exp.signaturePrefix != "" && (!strings.HasPrefix(sym.Signature, exp.signaturePrefix) ||
					sym.SignatureRange == nil ||
					sym.SignatureRange.Start.Line != exp.line) {
					t.Errorf("authored TypeScript signature mismatch want=%q got=%q", exp.signaturePrefix, sym.Signature)
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != tc.bytes {
				t.Fatal("whole original source identity missing")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "typescript", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 ||
				found.Symbols[0].ID != ids[tc.target] {
				t.Fatalf("whole original TypeScript exact find: err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "typescript", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.target) {
				t.Fatalf("fingerprint-bound original TypeScript show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "typescript", Encoding: "utf-8", Query: tc.target,
				Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != ids[tc.target] {
				t.Fatalf("original TypeScript structural search mismatch: err=%v failed=%+v queried=%+v", err, failed, queried)
			}
		})
	}
}
