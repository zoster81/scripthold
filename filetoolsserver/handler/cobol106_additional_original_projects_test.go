package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type cobol106Target struct {
	file, name, kind, native, owner, excerpt string
	line                                     int
}

// Original COBOL navigation does not prove embedded SQL semantics or execution.
func TestCOBOL106AdditionalOriginalProjectNavigation(t *testing.T) {
	ctags := os.Getenv("SCRIPTHOLD_REAL_COBOL106_CTAGS_ROOT")
	rosetta := os.Getenv("SCRIPTHOLD_REAL_COBOL106_ROSETTA_ROOT")
	if ctags == "" && rosetta == "" {
		t.Skip("additional original COBOL Git projects not configured")
	}
	if ctags == "" || rosetta == "" {
		t.Fatal("both independent original COBOL project roots are required")
	}
	type evidence struct{ file, excerpt string }
	cases := []struct {
		name, root, negativeFile, negativeName string
		selected                               []string
		evidence                               []evidence
		targets                                []cobol106Target
	}{
		{
			name: "Universal Ctags fixed-format printer and SQL parser fixture navigation",
			root: ctags,
			selected: []string{
				"Units/parser-cobol-to-review.r/TEST-PRINTER.d/input.cbl",
				"Units/parser-cobol-to-review2.r/FETCHTBL.d/input.cbl",
				"Units/parser-cobol-to-review2.r/INSERTTBL.d/input.cbl",
				"Units/parser-cobol-to-review2.r/sqlca.d/input.cbl",
			},
			negativeFile: "Units/parser-cobol-to-review2.r/FETCHTBL.d/input.cbl",
			negativeName: "SQLCA",
			evidence: []evidence{
				{"Units/parser-cobol-to-review.r/TEST-PRINTER.d/input.cbl", "\"VIRTUAL-PRINTER\" using PRINTER-PARAM"},
				{"Units/parser-cobol-to-review2.r/FETCHTBL.d/input.cbl", "EXEC SQL INCLUDE SQLCA END-EXEC."},
				{"Units/parser-cobol-to-review2.r/FETCHTBL.d/input.cbl", "FETCH C1 INTO :EMP-NO, :EMP-NAME, :EMP-SALARY"},
				{"Units/parser-cobol-to-review2.r/INSERTTBL.d/input.cbl", "EXEC SQL INCLUDE SQLCA END-EXEC."},
				{"Units/parser-cobol-to-review2.r/INSERTTBL.d/input.cbl", "INSERT INTO EMP VALUES"},
				{"Units/parser-cobol-to-review2.r/sqlca.d/input.cbl", "05  SQLCODE"},
				{"Units/parser-cobol-to-review2.r/sqlca.d/input.cbl", "05  SQLSTATE"},
			},
			targets: []cobol106Target{
				{"Units/parser-cobol-to-review.r/TEST-PRINTER.d/input.cbl", "TEST-PRINTER", "module", "program-id", "", "PROGRAM-ID. TEST-PRINTER.", 10},
				{"Units/parser-cobol-to-review.r/TEST-PRINTER.d/input.cbl", "WORKING-STORAGE", "section", "section", "TEST-PRINTER", "WORKING-STORAGE SECTION.", 23},
				{"Units/parser-cobol-to-review.r/TEST-PRINTER.d/input.cbl", "MAIN-PROCEDURE", "function", "paragraph", "TEST-PRINTER", "MAIN-PROCEDURE.", 35},
				{"Units/parser-cobol-to-review.r/TEST-PRINTER.d/input.cbl", "HELLO-WORLD", "function", "paragraph", "TEST-PRINTER", "HELLO-WORLD.", 70},
				{"Units/parser-cobol-to-review2.r/FETCHTBL.d/input.cbl", "FETCHTBL", "module", "program-id", "", "PROGRAM-ID.                 FETCHTBL.", 12},
				{"Units/parser-cobol-to-review2.r/FETCHTBL.d/input.cbl", "MAIN-RTN", "function", "paragraph", "FETCHTBL", "MAIN-RTN.", 42},
				{"Units/parser-cobol-to-review2.r/FETCHTBL.d/input.cbl", "ERROR-RTN", "function", "paragraph", "FETCHTBL", "ERROR-RTN.", 111},
				{"Units/parser-cobol-to-review2.r/INSERTTBL.d/input.cbl", "INSERTTBL", "module", "program-id", "", "PROGRAM-ID.                 INSERTTBL.", 11},
				{"Units/parser-cobol-to-review2.r/INSERTTBL.d/input.cbl", "MAIN-RTN", "function", "paragraph", "INSERTTBL", "MAIN-RTN.", 63},
				{"Units/parser-cobol-to-review2.r/INSERTTBL.d/input.cbl", "ERROR-RTN", "function", "paragraph", "INSERTTBL", "ERROR-RTN.", 137},
			},
		},
		{
			name: "Rosetta Code standalone CSV processing and graph search examples",
			root: rosetta,
			selected: []string{
				"Task/Binary-search/COBOL/binary-search.cob",
				"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob",
				"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob",
			},
			negativeFile: "Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob",
			negativeName: "CSV-File",
			evidence: []evidence{
				{"Task/Binary-search/COBOL/binary-search.cob", "SEARCH ALL nums"},
				{"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob", "SELECT CSV-File ASSIGN TO \"csv.txt\""},
				{"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob", "PERFORM Process-a-Record"},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "PERFORM GET-OR-ADD-VERTEX"},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "PERFORM DIJKSTRA-ALGO"},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "MOVE VERT-PREV(PATH-IDX) TO TEMP-IDX"},
			},
			targets: []cobol106Target{
				{"Task/Binary-search/COBOL/binary-search.cob", "binary-search", "module", "program-id", "", "PROGRAM-ID. binary-search.", 3},
				{"Task/Binary-search/COBOL/binary-search.cob", "WORKING-STORAGE", "section", "section", "binary-search", "WORKING-STORAGE SECTION.", 6},
				{"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob", "CSV", "module", "program-id", "", "PROGRAM-ID. CSV.", 2},
				{"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob", "Main-Program", "function", "paragraph", "CSV", "Main-Program.", 56},
				{"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob", "Read-a-Record", "function", "paragraph", "CSV", "Read-a-Record.", 69},
				{"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob", "Build-Header", "function", "paragraph", "CSV", "Build-Header.", 74},
				{"Task/CSV-data-manipulation/COBOL/csv-data-manipulation.cob", "Process-a-Record", "function", "paragraph", "CSV", "Process-a-Record.", 81},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "DIJKSTRA", "module", "program-id", "", "PROGRAM-ID. DIJKSTRA.", 2},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "BUILD-GRAPH", "function", "paragraph", "DIJKSTRA", "BUILD-GRAPH.", 74},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "GET-OR-ADD-VERTEX", "function", "paragraph", "DIJKSTRA", "GET-OR-ADD-VERTEX.", 149},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "DIJKSTRA-ALGO", "function", "paragraph", "DIJKSTRA", "DIJKSTRA-ALGO.", 170},
				{"Task/Dijkstras-algorithm/COBOL/dijkstras-algorithm.cob", "PRINT-RESULT", "function", "paragraph", "DIJKSTRA", "PRINT-RESULT.", 214},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			original := make(map[string]string, len(tc.selected))
			for _, file := range tc.selected {
				full := filepath.Join(tc.root, filepath.FromSlash(file))
				info, err := os.Lstat(full)
				if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
					t.Fatalf("original COBOL source missing or unbounded %s: info=%v err=%v", file, info, err)
				}
				raw, err := os.ReadFile(full)
				if err != nil {
					t.Fatal(err)
				}
				original[file] = string(raw)
			}
			for _, e := range tc.evidence {
				if !strings.Contains(original[e.file], e.excerpt) {
					t.Fatalf("original COBOL source evidence missing %s: %q", e.file, e.excerpt)
				}
			}
			for _, target := range tc.targets {
				lines := strings.Split(original[target.file], "\n")
				if target.line < 1 || target.line > len(lines) ||
					!strings.Contains(lines[target.line-1], target.excerpt) {
					t.Fatalf("original COBOL declaration changed %s:%d %s", target.file, target.line, target.name)
				}
			}
			ctx, h := context.Background(), NewHandler([]string{tc.root})
			digestIDs := make(map[string]string)
			outlineIDs := make(map[string]map[string]bool)
			for _, file := range tc.selected {
				// SQLCA holds data items rather than a program/paragraph: direct-read evidence only.
				if strings.Contains(file, "/sqlca.d/") {
					continue
				}
				full := filepath.Join(tc.root, filepath.FromSlash(file))
				_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "digest", Paths: []string{full}, Language: "cobol", MaxFiles: 1,
				})
				if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
					len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
					t.Fatalf("COBOL digest failed %s: err=%v result=%+v", file, err, digest)
				}
				digestIDs[file] = digest.Digests[0].SourceFingerprint
				_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "outline", Presentation: "detailed", Paths: []string{full},
					Language: "cobol", MaxFiles: 1, MaxSymbols: 256,
				})
				if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
					t.Fatalf("COBOL detailed outline failed %s: err=%v result=%+v", file, err, outline)
				}
				outlineIDs[file] = make(map[string]bool)
				for _, sym := range outline.Symbols {
					outlineIDs[file][sym.ID] = true
					switch sym.Name {
					case "SQLCA", "SQLCODE", "PERFORM", "CSV-File", "SEARCH":
						t.Fatalf("COBOL include/field/instruction invented as a declaration: %+v", sym)
					}
				}
			}
			for _, target := range tc.targets {
				full := filepath.Join(tc.root, filepath.FromSlash(target.file))
				_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{full}, Language: "cobol",
					Query: target.name, Match: "exact", MaxFiles: 1,
					MaxSymbols: 256, FileDetails: "relevant",
				})
				if err != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("COBOL exact find failed %s:%d: err=%v found=%+v", target.file, target.line, err, found)
				}
				id := ""
				for _, sym := range found.Symbols {
					if sym.Name == target.name && string(sym.Kind) == target.kind &&
						sym.NativeKind == target.native && sym.ParentQualifiedName == target.owner &&
						sym.NameRange.Start.Line == target.line {
						id = sym.ID
					}
				}
				if id == "" || !outlineIDs[target.file][id] {
					t.Fatalf("COBOL source target missing or misowned %s:%d %s: %+v", target.file, target.line, target.name, found.Symbols)
				}
				file := found.Files[0]
				if file.Encoding == "" || file.SourceFingerprint != digestIDs[target.file] {
					t.Fatalf("COBOL original source identity mismatch %s: %+v", target.file, file)
				}
				_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
					Operation: "show", Path: full, Language: "cobol",
					Encoding: file.Encoding, SymbolID: id,
					SourceFingerprint: file.SourceFingerprint, MaxBytes: 16384,
				})
				if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target.excerpt) {
					t.Fatalf("COBOL fingerprint-bound source excerpt missing %s:%d: err=%v shown=%+v", target.file, target.line, err, shown.Show)
				}
				_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
					Operation: "search", Mode: "structural", Paths: []string{full},
					Language: "cobol", Query: target.name, Match: "exact",
					MaxFiles: 1, MaxResults: 128,
				})
				linked := false
				if search.Search != nil {
					for _, hit := range search.Search.Matches {
						if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
							hit.Range.Start.Line == target.line {
							linked = true
						}
					}
				}
				if err != nil || search.Search == nil || search.Coverage.ResultsTruncated || !linked {
					t.Fatalf("COBOL structural search identity missing %s:%d: err=%v search=%+v", target.file, target.line, err, search)
				}
			}
			_, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find",
				Paths:     []string{filepath.Join(tc.root, filepath.FromSlash(tc.negativeFile))},
				Language:  "cobol", Query: tc.negativeName, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("COBOL include/field fabricated %q: err=%v symbols=%+v", tc.negativeName, err, negative.Symbols)
			}
			t.Logf("original COBOL project: %d selected files, %d exact supported declarations", len(tc.selected), len(tc.targets))
		})
	}
}
