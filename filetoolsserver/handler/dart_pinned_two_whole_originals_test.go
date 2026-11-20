package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two immutable, independent Git upstream Dart originals are source oracles,
// not executable project dependencies. The complete supported inventories are
// independently annotated before querying the development handler.
func TestDartPinnedTwoWholeOriginalDeclarationsAndNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_DART_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two independent complete upstream Dart originals not configured")
	}
	type declaration struct {
		name, kind, owner          string
		line, col, endLine, endCol int
	}
	cases := []struct {
		file            string
		bytes           int64
		want            []declaration
		target, excerpt string
	}{
		{
			"flutter-main.dart", 339,
			[]declaration{{"main", "function", "", 7, 6, 11, 3}},
			"main", "void main() => runApp(",
		},
		{
			"http-exception.dart", 645,
			[]declaration{
				{"ClientException", "class", "", 6, 7, 22, 2},
				{"message", "field", "ClientException", 7, 16, 7, 24},
				{"uri", "field", "ClientException", 10, 14, 10, 18},
				{"ClientException", "constructor", "ClientException", 12, 3, 12, 45},
				{"toString", "method", "ClientException", 15, 10, 21, 4},
			},
			"toString", "return 'ClientException: $message, uri=$uri';",
		},
	}
	ctx := context.Background()
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatalf("original fixture unavailable: %v", err)
			}
			if !info.Mode().IsRegular() || info.Size() != tc.bytes {
				t.Fatalf("original fixture changed type/size: %+v", info)
			}
			failed, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "dart", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || outlined.Truncated ||
				!outlined.CoverageComplete || outlined.FilesParsed != 1 ||
				len(outlined.Files) != 1 || len(outlined.Symbols) != len(tc.want) {
				t.Fatalf("full original Dart inventory mismatch: err=%v failed=%+v result=%+v", err, failed, outlined)
			}
			ids := make(map[string]string, len(tc.want))
			for _, sym := range outlined.Symbols {
				key := sym.Name + ":" + string(sym.Kind)
				if ids[key] != "" || sym.ID == "" {
					t.Errorf("duplicate or empty symbol identity: %+v", sym)
				}
				ids[key] = sym.ID
			}
			for _, expected := range tc.want {
				matched := 0
				for _, sym := range outlined.Symbols {
					if sym.Name != expected.name || string(sym.Kind) != expected.kind {
						continue
					}
					matched++
					if sym.NativeKind != expected.kind ||
						sym.NameRange.Start.Line != expected.line ||
						sym.NameRange.Start.Column != expected.col ||
						sym.DeclarationRange.End.Line != expected.endLine ||
						sym.DeclarationRange.End.Column != expected.endCol ||
						sym.ParentQualifiedName != expected.owner ||
						sym.QualifiedName != func() string {
							if expected.owner == "" {
								return expected.name
							}
							return expected.owner + "." + expected.name
						}() {
						t.Errorf("authored Dart identity/range mismatch want=%+v got=%+v", expected, sym)
					}
					if expected.owner != "" && sym.ParentID != ids[expected.owner+":class"] {
						t.Errorf("Dart direct class owner linkage wrong: %+v", sym)
					}
				}
				if matched != 1 {
					t.Errorf("expected exactly one full-source Dart declaration %+v; matches=%d", expected, matched)
				}
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if fingerprint == "" {
				t.Fatal("original source fingerprint absent")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "dart",
				Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || found.Truncated ||
				len(found.Symbols) != 1 || found.Symbols[0].ID != ids[tc.target+":"+func() string {
				if tc.target == "main" {
					return "function"
				}
				return "method"
			}()] {
				t.Fatalf("full original Dart exact find failed: err=%v failed=%+v found=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "dart", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("original fingerprint-bound show failed: err=%v failed=%+v", err, failed)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "dart", Encoding: "utf-8", Query: tc.target,
				Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original Dart structural query mismatch: err=%v failed=%+v result=%+v", err, failed, queried)
			}
		})
	}
}
