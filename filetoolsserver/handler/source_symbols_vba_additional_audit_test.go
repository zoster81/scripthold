package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type vbaExtraTarget struct {
	file, symbol string
	line         int
}
type vbaExtraScenario struct {
	project, query, primary, unrelated string
	files                              []string
	oracles                            []astroOriginalOracle
	targets                            []vbaExtraTarget
}

func TestSourceSymbolsOriginalVBAAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original VBA corpus unavailable")
	}
	scenarios := []vbaExtraScenario{
		{
			project: "vba-commonlib", query: "NormalizeArray",
			primary: "VBALib_ArrayUtils.bas", unrelated: "VBALib_List.cls",
			files: []string{"VBALib_ArrayUtils.bas", "VBALib_ExcelTable.cls", "VBALib_ExcelUtils.bas", "VBALib_List.cls"},
			oracles: []astroOriginalOracle{
				{"VBALib_ArrayUtils.bas", "Public Function NormalizeArray(arr As Variant) As Variant"},
				{"VBALib_ArrayUtils.bas", "If ArrayLen(arr) = 0 Then"},
				{"VBALib_ArrayUtils.bas", "Select Case ArrayRank(arr)"},
				{"VBALib_ArrayUtils.bas", "LBound(arr) = NORMALIZE_LBOUND"},
				{"VBALib_ArrayUtils.bas", "Can only normalize a 2-dimensional array"},
				{"VBALib_ExcelTable.cls", "columnSpecs = NormalizeArray(columnSpecs)"},
				{"VBALib_ExcelTable.cls", "If ArrayLen(columnSpecs) = 0 Then"},
				{"VBALib_ExcelUtils.bas", "linkNames = NormalizeArray(ActiveWorkbook.LinkSources(xlExcelLinks))"},
				{"VBALib_List.cls", "Private Sub CheckIndex(i As Integer)"},
				{"VBALib_List.cls", "Private Sub Resize(newCount As Integer)"},
			},
			targets: []vbaExtraTarget{
				{"VBALib_ArrayUtils.bas", "NormalizeArray", 17},
				{"VBALib_ArrayUtils.bas", "ArrayRank", 78},
				{"VBALib_ArrayUtils.bas", "ArrayLen", 118},
				{"VBALib_ExcelTable.cls", "Sort", 202},
				{"VBALib_ExcelUtils.bas", "GetAllExcelLinks", 274},
			},
		},
		{
			project: "vba-macrolibrary", query: "Email_part",
			primary: "Module1.bas", unrelated: "Lib_ReadWriteUtf8.vb",
			files: []string{"Module1.bas", "Lib_ReadWriteUtf8.vb"},
			oracles: []astroOriginalOracle{
				{"Module1.bas", "Public Function Email_part(xstr As String, part As String) As String"},
				{"Module1.bas", "Case \"Fname\""},
				{"Module1.bas", "Case \"Lname\""},
				{"Module1.bas", "Case \"Domain\""},
				{"Module1.bas", "Email_part = ProperX(Mid(xstr, Apos + 1, Len(xstr) - Apos))"},
				{"Module1.bas", "Public Function ProperX(xstr As String) As String"},
				{"Module1.bas", "Public Function ReturnNthPartOfString("},
				{"Lib_ReadWriteUtf8.vb", "Function ReadFromUTFFile(filepath As String) As String"},
				{"Lib_ReadWriteUtf8.vb", "objStream.Charset = \"utf-8\""},
				{"README.md", "=Email_part(B2,\"Domain\")"},
				{"docs/API.md", "Extracts part of an email address."},
			},
			targets: []vbaExtraTarget{
				{"Module1.bas", "ProperX", 31},
				{"Module1.bas", "Email_part", 37},
				{"Module1.bas", "ReturnNthPartOfString", 137},
				{"Lib_ReadWriteUtf8.vb", "ReadFromUTFFile", 1},
				{"Lib_ReadWriteUtf8.vb", "WriteToUTFFile", 16},
			},
		},
	}
	for _, sc := range scenarios {
		t.Run(sc.project, func(t *testing.T) {
			root := filepath.Join(corpus, sc.project)
			paths := make([]string, 0, len(sc.files))
			for _, rel := range sc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				requireAstroOriginalFile(t, path)
				paths = append(paths, path)
			}
			for _, oracle := range sc.oracles {
				p := filepath.Join(root, filepath.FromSlash(oracle.file))
				requireAstroOriginalFile(t, p)
				bytes, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(bytes), oracle.text) {
					t.Fatalf("original source oracle %s %q missing: %v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "vba", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) || !digest.CoverageComplete {
				t.Fatalf("incomplete digest err=%v failed=%+v result=%+v", err, failed, digest)
			}
			compact, err := json.Marshal(digest)
			if err != nil || len(compact) > 8192 {
				t.Fatalf("compact digest bytes=%d err=%v", len(compact), err)
			}
			t.Logf("files=%d symbols=%d bytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(compact), digest.CoverageComplete)
			for _, want := range sc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.symbol, Match: "exact",
					Language: "vba", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s err=%v failed=%+v result=%+v", want.symbol, err, failed, found)
				}
				id := ""
				for _, symbol := range found.Symbols {
					if symbol.Name == want.symbol && symbol.NameRange.Start.Line == want.line {
						id = symbol.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing original declaration %s:%d %s: %+v", want.file, want.line, want.symbol, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint,
					Language: "vba", Encoding: "utf-8", MaxBytes: 16384,
				})
				if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.symbol) {
					t.Fatalf("fingerprint show %s err=%v failed=%+v show=%+v", want.symbol, err, failed, shown.Show)
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: sc.query, Match: "exact", Mode: "structural",
				Language: "vba", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || failed != nil || search.Search == nil {
				t.Fatalf("structural search err=%v failed=%+v result=%+v", err, failed, search)
			}
			primary := filepath.Join(root, filepath.FromSlash(sc.primary))
			seen := false
			for _, match := range search.Search.Matches {
				if match.Name == sc.query && filepath.Clean(match.Path) == primary {
					seen = true
				}
			}
			if !seen {
				t.Fatalf("structural query missed primary %s", sc.primary)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(sc.unrelated))},
				Query: sc.query, Match: "exact", Language: "vba", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated result err=%v failed=%+v symbols=%+v", err, failed, negative.Symbols)
			}
		})
	}
}
