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

type pbOriginal struct {
	name, qualified, kind, native, owner string
	line, nameCol, nameEnd, scopeEnd     int
}

// Independently inventoried complete upstream PureBasic sources. DeclareModule,
// Module, typed prototypes and authored procedure implementations are distinct.
func TestPureBasicTwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_PUREBASIC_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete independent PureBasic originals not configured")
	}
	cases := []struct {
		file, blob, target string
		size               int64
		matchCount         int
		want               []pbOriginal
	}{
		{file: "GetRequiredSize.pb", blob: "9109753707c5cfa791ec29d70cc85187f8e80508", size: 3194, target: "GetRequiredSize", matchCount: 1, want: []pbOriginal{
			{"Max", "Max", "function", "procedure", "", 21, 13, 16, 27},
			{"GetRequiredSize", "GetRequiredSize", "function", "procedure", "", 30, 11, 26, 48},
			{"GetRequiredWidth", "GetRequiredWidth", "function", "procedure", "", 52, 11, 27, 56},
			{"GetRequiredHeight", "GetRequiredHeight", "function", "procedure", "", 58, 11, 28, 62},
			{"Frame3DTopOffset", "Frame3DTopOffset", "function", "procedure", "", 71, 11, 27, 96},
		}},
		{file: "Henry-StringConverter.pb", blob: "b5c3a321837ded7a3c71b18ce683c68cfa42087f", size: 2673, target: "ToAscii", matchCount: 2, want: []pbOriginal{
			{"StringConverter", "StringConverter", "module", "declaremodule", "", 8, 15, 30, 8},
			{"ToAscii", "StringConverter.ToAscii", "function", "declare-procedure", "StringConverter", 9, 13, 20, 9},
			{"ToUtf8", "StringConverter.ToUtf8", "function", "declare-procedure", "StringConverter", 10, 13, 19, 10},
			{"FromAsciiPtr", "StringConverter.FromAsciiPtr", "function", "declare-procedure", "StringConverter", 11, 12, 24, 11},
			{"FromUtf8Ptr", "StringConverter.FromUtf8Ptr", "function", "declare-procedure", "StringConverter", 12, 12, 23, 12},
			{"FromAscii", "StringConverter.FromAscii", "function", "declare-procedure", "StringConverter", 13, 12, 21, 13},
			{"FromUtf8", "StringConverter.FromUtf8", "function", "declare-procedure", "StringConverter", 14, 12, 20, 14},
			{"StringConverter", "StringConverter", "module", "module", "", 17, 8, 23, 17},
			{"ToAscii", "StringConverter.ToAscii", "function", "procedure", "StringConverter", 21, 15, 22, 32},
			{"ToUtf8", "StringConverter.ToUtf8", "function", "procedure", "StringConverter", 35, 15, 21, 43},
			{"FromAsciiPtr", "StringConverter.FromAsciiPtr", "function", "procedure", "StringConverter", 46, 14, 26, 48},
			{"FromUtf8Ptr", "StringConverter.FromUtf8Ptr", "function", "procedure", "StringConverter", 51, 14, 25, 53},
			{"FromAscii", "StringConverter.FromAscii", "function", "procedure", "StringConverter", 56, 14, 23, 58},
			{"FromUtf8", "StringConverter.FromUtf8", "function", "procedure", "StringConverter", 61, 14, 22, 63},
		}},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() != tc.size {
				t.Fatalf("pinned original changed: info=%+v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(data), 0)
			_, _ = hash.Write(data)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob ||
				!bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
				t.Fatal("whole original Git blob or UTF-8 BOM mismatch")
			}
			failed, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "purebasic", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete ||
				outline.FilesParsed != 1 || len(outline.Files) != 1 || len(outline.Symbols) != len(tc.want) {
				t.Fatalf("whole original PureBasic source inventory: err=%v failed=%+v result=%+v", err, failed, outline)
			}
			keys := map[string]int{}
			ids := map[string]bool{}
			for i, s := range outline.Symbols {
				key := fmt.Sprintf("%s@%d", s.QualifiedName, s.NameRange.Start.Line)
				if _, ok := keys[key]; ok {
					t.Errorf("duplicate physical symbol %s", key)
				}
				keys[key] = i
				if s.ID == "" || ids[s.ID] {
					t.Errorf("missing/reused physical ID for %s", key)
				}
				ids[s.ID] = true
			}
			for _, w := range tc.want {
				key := fmt.Sprintf("%s@%d", w.qualified, w.line)
				i, ok := keys[key]
				if !ok {
					t.Errorf("original authored declaration missing %+v", w)
					continue
				}
				s := outline.Symbols[i]
				if s.Name != w.name || s.QualifiedName != w.qualified ||
					string(s.Kind) != w.kind || s.NativeKind != w.native ||
					s.ParentQualifiedName != w.owner ||
					s.NameRange.Start.Line != w.line || s.NameRange.Start.Column != w.nameCol ||
					s.NameRange.End.Line != w.line || s.NameRange.End.Column != w.nameEnd ||
					s.DeclarationRange.Start.Line != w.line ||
					s.DeclarationRange.End.Line != w.scopeEnd {
					t.Errorf("original authored declaration mismatch want=%+v got=%+v", w, s)
				}
				if w.owner == "" && s.ParentID != "" {
					t.Errorf("root symbol has parent %+v", s)
				}
				if w.owner != "" {
					moduleLine := 8
					if w.native == "procedure" {
						moduleLine = 17
					}
					pi, ok := keys[fmt.Sprintf("%s@%d", w.owner, moduleLine)]
					if !ok || s.ParentID != outline.Symbols[pi].ID {
						t.Errorf("wrong physical module owner %+v", s)
					}
				}
			}
			fingerprint := outline.Files[0].SourceFingerprint
			if fingerprint == "" || outline.Files[0].SourceBytes != tc.size {
				t.Fatal("original fingerprint/bytes absent")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "purebasic", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != tc.matchCount {
				t.Fatalf("exact original PureBasic find: err=%v failed=%+v result=%+v", err, failed, found)
			}
			targetIDs := map[string]bool{}
			for _, s := range outline.Symbols {
				if s.Name == tc.target {
					targetIDs[s.ID] = true
				}
			}
			for _, s := range found.Symbols {
				if !targetIDs[s.ID] {
					t.Errorf("unknown found ID %+v", s)
				}
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: found.Symbols[0].ID,
				SourceFingerprint: fingerprint, Language: "purebasic", Encoding: "utf-8", MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.target) {
				t.Fatalf("fingerprint show original PureBasic: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "purebasic", Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || failed != nil || query.Search == nil ||
				len(query.Search.Matches) != tc.matchCount {
				t.Fatalf("whole original PureBasic structural match count: err=%v failed=%+v result=%+v", err, failed, query)
			}
			for _, m := range query.Search.Matches {
				if !targetIDs[m.SymbolID] {
					t.Errorf("query invented symbol %+v", m)
				}
			}
		})
	}
}
