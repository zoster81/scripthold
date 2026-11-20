package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type ada104Target struct {
	file, name, nativeKind, owner, lineText string
	line                                    int
}

// Original Ada project navigation must expose supported declarations without
// claiming that task/protected scopes or program behavior are fully understood.
func TestAda104AdditionalOriginalProjectNavigation(t *testing.T) {
	ctags := os.Getenv("SCRIPTHOLD_REAL_ADA104_CTAGS_ROOT")
	samples := os.Getenv("SCRIPTHOLD_REAL_ADA104_SAMPLES_ROOT")
	if ctags == "" && samples == "" {
		t.Skip("independent original Ada projects not configured")
	}
	if ctags == "" || samples == "" {
		t.Fatal("both original Ada project roots are required")
	}

	type originalText struct{ file, excerpt string }
	cases := []struct {
		name, root, negativeFile, negativeName string
		code, selected                         []string
		partial                                []string
		original                               []originalText
		targets                                []ada104Target
	}{
		{
			name: "Universal Ctags Ada package specification, body and generic fixtures",
			root: ctags,
			code: []string{
				"Units/parser-ada.r/ada-entry.d/input_1.ads",
				"Units/parser-ada.r/ada-entry.d/input_1.adb",
				"Units/parser-ada.r/ada-generic-in-package.d/input_1.adb",
				"Units/parser-ada.r/ada-generic-in-package.d/input_2.ads",
			},
			selected: []string{
				"Units/parser-ada.r/ada-entry.d/input_1.ads",
				"Units/parser-ada.r/ada-entry.d/input_1.adb",
				"Units/parser-ada.r/ada-generic-in-package.d/input_1.adb",
				"Units/parser-ada.r/ada-generic-in-package.d/input_2.ads",
			},
			partial: []string{
				"Units/parser-ada.r/ada-entry.d/input_1.ads",
				"Units/parser-ada.r/ada-entry.d/input_1.adb",
			},
			negativeFile: "Units/parser-ada.r/ada-entry.d/input_1.ads",
			negativeName: "Variable",
			original: []originalText{
				{"Units/parser-ada.r/ada-entry.d/input_1.ads", "task Ma_Tasche is"},
				{"Units/parser-ada.r/ada-entry.d/input_1.ads", "protected Objet_Protege is"},
				{"Units/parser-ada.r/ada-entry.d/input_1.ads", "function Tester return Boolean;"},
				{"Units/parser-ada.r/ada-entry.d/input_1.adb", "package body Input_1 is"},
				{"Units/parser-ada.r/ada-entry.d/input_1.adb", "protected body Objet_Protege is"},
				{"Units/parser-ada.r/ada-entry.d/input_1.adb", "function Tester"},
				{"Units/parser-ada.r/ada-generic-in-package.d/input_1.adb", "procedure Generic_Reverse_Array (X : in out Array_T) is"},
				{"Units/parser-ada.r/ada-generic-in-package.d/input_2.ads", "with function Random (G: Generator) return Unsigned is <>;"},
			},
			targets: []ada104Target{
				{"Units/parser-ada.r/ada-entry.d/input_1.ads", "Input_1", "package", "", "package Input_1 is", 2},
				{"Units/parser-ada.r/ada-entry.d/input_1.adb", "Input_1", "package", "", "package body Input_1 is", 2},
				{"Units/parser-ada.r/ada-generic-in-package.d/input_1.adb", "Input_1", "procedure", "", "procedure Input_1 is", 1},
				{"Units/parser-ada.r/ada-generic-in-package.d/input_2.ads", "input_2", "package", "", "package input_2 is", 1},
			},
		},
		{
			name: "Sample Programs Ada nested binary search and shortest-path navigation",
			root: samples,
			code: []string{
				"archive/a/ada/binary-search.adb",
				"archive/a/ada/dijkstra.adb",
			},
			selected: []string{
				"archive/a/ada/binary-search.adb",
				"archive/a/ada/dijkstra.adb",
				"archive/a/ada/README.md",
			},
			negativeFile: "archive/a/ada/binary-search.adb",
			negativeName: "To_Int_List",
			original: []originalText{
				{"archive/a/ada/binary-search.adb", "Put_Line (if Contains (Haystack, Needle) then \"true\" else \"false\");"},
				{"archive/a/ada/binary-search.adb", "package Int_Vec is new"},
				{"archive/a/ada/dijkstra.adb", "Result := Dijkstra (Adj, N, Src, Dst);"},
				{"archive/a/ada/dijkstra.adb", "function Min_Node return Natural is"},
				{"archive/a/ada/README.md", "This page is auto-generated. DO NOT EDIT!"},
				{"archive/a/ada/README.md", "Binary Search in Ada"},
				{"archive/a/ada/README.md", "Dijkstra in Ada"},
			},
			targets: []ada104Target{
				{"archive/a/ada/binary-search.adb", "Binary_Search", "procedure", "", "procedure Binary_Search is", 6},
				{"archive/a/ada/binary-search.adb", "Contains", "function", "Binary_Search", "function Contains (V : Vector; Value : Integer) return Boolean is", 15},
				{"archive/a/ada/binary-search.adb", "Parse_Sorted_List", "function", "Binary_Search", "function Parse_Sorted_List (S : String) return Vector is", 60},
				{"archive/a/ada/dijkstra.adb", "Dijkstra", "procedure", "", "procedure Dijkstra is", 8},
				{"archive/a/ada/dijkstra.adb", "To_Int_List", "function", "Dijkstra", "function To_Int_List (Raw : String) return Int_List is", 24},
				{"archive/a/ada/dijkstra.adb", "Dijkstra", "function", "Dijkstra", "function Dijkstra (Adj : Int_List; N, Src, Dst : Natural) return Integer is", 54},
				{"archive/a/ada/dijkstra.adb", "Min_Node", "function", "Dijkstra.Dijkstra", "function Min_Node return Natural is", 58},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := make(map[string]string, len(tc.selected))
			for _, relative := range tc.selected {
				full := filepath.Join(tc.root, filepath.FromSlash(relative))
				info, err := os.Lstat(full)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("Ada original source not a bounded regular file %s: info=%v err=%v", relative, info, err)
				}
				raw, err := os.ReadFile(full)
				if err != nil {
					t.Fatal(err)
				}
				files[relative] = string(raw)
			}
			for _, expected := range tc.original {
				if !strings.Contains(files[expected.file], expected.excerpt) {
					t.Fatalf("original Ada source evidence absent %s: %q", expected.file, expected.excerpt)
				}
			}
			for _, target := range tc.targets {
				lines := strings.Split(files[target.file], "\n")
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], target.lineText) {
					t.Fatalf("original Ada maintenance declaration changed %s:%d %s", target.file, target.line, target.name)
				}
			}

			h, ctx := NewHandler([]string{tc.root}), context.Background()
			fingerprints := make(map[string]string, len(tc.code))
			ids := make(map[string]map[string]bool, len(tc.code))
			partiallySupported := make(map[string]bool, len(tc.partial))
			for _, name := range tc.partial {
				partiallySupported[name] = true
			}
			for _, relative := range tc.code {
				full := filepath.Join(tc.root, filepath.FromSlash(relative))
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Paths: []string{full}, Language: "ada", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
					len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
					t.Fatalf("Ada original digest %s: err=%v result=%+v", relative, err, digest)
				}
				fingerprints[relative] = digest.Digests[0].SourceFingerprint
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{full},
					Language: "ada", MaxFiles: 1, MaxSymbols: 256,
				})
				if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
					t.Fatalf("Ada original outline %s: err=%v result=%+v", relative, err, outline)
				}
				if partiallySupported[relative] && outline.CoverageComplete {
					t.Fatalf("unsupported Ada task/protected scope incorrectly called complete: %s", relative)
				}
				symbolIDs := make(map[string]bool, len(outline.Symbols))
				for _, sym := range outline.Symbols {
					symbolIDs[sym.ID] = true
				}
				ids[relative] = symbolIDs
			}

			for _, target := range tc.targets {
				full := filepath.Join(tc.root, filepath.FromSlash(target.file))
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{full}, Language: "ada",
					Query: target.name, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
					FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("Ada exact find %s:%d: err=%v result=%+v", target.file, target.line, err, found)
				}
				id := ""
				for _, sym := range found.Symbols {
					if sym.Name == target.name && sym.NativeKind == target.nativeKind &&
						sym.ParentQualifiedName == target.owner && sym.NameRange.Start.Line == target.line {
						id = sym.ID
					}
				}
				if id == "" || !ids[target.file][id] {
					t.Fatalf("Ada expected symbol absent/wrong owner %s:%d (%s): %+v", target.file, target.line, target.name, found.Symbols)
				}
				file := found.Files[0]
				if file.Encoding == "" || file.SourceFingerprint != fingerprints[target.file] {
					t.Fatalf("Ada original fingerprint mismatch %s: %+v", target.file, file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: full, Language: "ada",
					Encoding: file.Encoding, SymbolID: id,
					SourceFingerprint: file.SourceFingerprint, MaxBytes: 16384,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.lineText) {
					t.Fatalf("Ada protected exact excerpt %s:%d: err=%v result=%+v", target.file, target.line, err, shown.Show)
				}
				_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{full},
					Language: "ada", Query: target.name, Match: "exact",
					MaxFiles: 1, MaxResults: 128,
				})
				linked := false
				if search.Search != nil {
					for _, match := range search.Search.Matches {
						if match.SymbolID == id && match.SourceFingerprint == file.SourceFingerprint &&
							match.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || search.Search == nil || search.Coverage.ResultsTruncated || !linked {
					t.Fatalf("Ada structural search identity %s:%d: err=%v result=%+v", target.file, target.line, err, search)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find",
				Paths:     []string{filepath.Join(tc.root, filepath.FromSlash(tc.negativeFile))},
				Language:  "ada", Query: tc.negativeName, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("Ada unrelated field or other example incorrectly indexed %s: err=%v symbols=%+v", tc.negativeName, err, negative.Symbols)
			}
			t.Logf("Ada original navigation: %d selected files, %d independently annotated targets", len(tc.selected), len(tc.targets))
		})
	}
}
