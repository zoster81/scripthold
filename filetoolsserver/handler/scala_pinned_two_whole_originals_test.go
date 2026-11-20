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

type scalaOriginalDeclaration struct {
	name, kind, native, qualified, owner, signature string
	line, nameCol, nameEnd, declarationEnd          int
}

// The complete Git originals from two independent upstream Scala projects
// were inventoried before asking Scripthold for their supported declarations.
func TestScalaTwoWholePinnedOriginalDeclarations(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SCALA_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete pinned upstream Scala originals not configured")
	}
	cases := []struct {
		file, blob, target string
		bytes              int64
		expected           []scalaOriginalDeclaration
	}{
		{
			file:   "scala-companionObject.scala",
			blob:   "dc817b138707839a8f293b37c66e4584477bfb33",
			bytes:  545,
			target: "companionObject",
			expected: []scalaOriginalDeclaration{
				{"scala.annotation.meta", "package", "package", "scala.annotation.meta", "", "package scala.annotation.meta", 13, 9, 30, 30},
				{"companionObject", "class", "class", "scala.annotation.meta.companionObject", "scala.annotation.meta", "final class companionObject extends scala.annotation.StaticAnnotation", 19, 13, 28, 70},
			},
		},
		{
			file:   "play-PortNumber.scala",
			blob:   "6b1e1971f848347a3b250e21d5dc78d619278d0d",
			bytes:  1447,
			target: "PortNumber",
			expected: []scalaOriginalDeclaration{
				{"org.scalatestplus.play", "package", "package", "org.scalatestplus.play", "", "package org.scalatestplus.play", 17, 9, 31, 31},
				{"PortNumber", "class", "class", "org.scalatestplus.play.PortNumber", "org.scalatestplus.play", "case class PortNumber(value: Int)", 31, 12, 22, 34},
			},
		},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.bytes {
				t.Fatalf("pinned complete Scala original unavailable or changed: info=%+v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hasher := sha1.New()
			_, _ = fmt.Fprintf(hasher, "blob %d%c", len(data), 0)
			_, _ = hasher.Write(data)
			if hex.EncodeToString(hasher.Sum(nil)) != tc.blob {
				t.Fatal("complete original Git blob differs from pinned upstream")
			}
			failed, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "scala", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outlined.Truncated || !outlined.CoverageComplete ||
				outlined.FilesParsed != 1 || len(outlined.Files) != 1 || len(outlined.Symbols) != len(tc.expected) {
				t.Fatalf("full Scala source inventory: err=%v failed=%+v result=%+v", err, failed, outlined)
			}
			byQualified := make(map[string]int, len(outlined.Symbols))
			ids := make(map[string]string, len(outlined.Symbols))
			for i, sym := range outlined.Symbols {
				if _, duplicate := byQualified[sym.QualifiedName]; duplicate {
					t.Errorf("duplicate authored declaration %q", sym.QualifiedName)
				}
				byQualified[sym.QualifiedName] = i
				ids[sym.QualifiedName] = sym.ID
				if sym.ID == "" {
					t.Errorf("empty identity for %q", sym.QualifiedName)
				}
			}
			for _, exp := range tc.expected {
				idx, found := byQualified[exp.qualified]
				if !found {
					t.Errorf("missing Scala authored declaration %+v", exp)
					continue
				}
				sym := outlined.Symbols[idx]
				if sym.Name != exp.name || string(sym.Kind) != exp.kind ||
					sym.NativeKind != exp.native || sym.QualifiedName != exp.qualified ||
					sym.ParentQualifiedName != exp.owner ||
					sym.NameRange.Start.Line != exp.line || sym.NameRange.Start.Column != exp.nameCol ||
					sym.NameRange.End.Line != exp.line || sym.NameRange.End.Column != exp.nameEnd ||
					sym.DeclarationRange.Start.Line != exp.line ||
					sym.DeclarationRange.Start.Column != 1 ||
					sym.DeclarationRange.End.Line != exp.line ||
					sym.DeclarationRange.End.Column != exp.declarationEnd ||
					sym.Signature != exp.signature ||
					sym.SignatureRange == nil ||
					sym.SignatureRange.End.Line != exp.line ||
					sym.SignatureRange.End.Column != exp.declarationEnd {
					t.Errorf("source-first Scala identity/range mismatch want=%+v got=%+v", exp, sym)
				}
				if exp.owner == "" && sym.ParentID != "" {
					t.Errorf("package unexpectedly owned: %+v", sym)
				}
				if exp.owner != "" && sym.ParentID != ids[exp.owner] {
					t.Errorf("missing direct package parent: %+v", sym)
				}
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if fingerprint == "" || outlined.Files[0].SourceBytes != tc.bytes {
				t.Fatal("original source fingerprint/size absent")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "scala", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 ||
				found.Symbols[0].ID != ids[found.Symbols[0].QualifiedName] {
				t.Fatalf("whole original Scala exact find: err=%v failed=%+v result=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "scala", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.target) {
				t.Fatalf("fingerprint-verified Scala show: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "scala", Encoding: "utf-8", Query: tc.target,
				Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("Scala whole source structural search: err=%v failed=%+v queried=%+v", err, failed, queried)
			}
		})
	}
}
