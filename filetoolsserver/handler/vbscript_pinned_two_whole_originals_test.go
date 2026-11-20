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

type vbscriptOriginalDeclaration struct {
	name, kind, native, qualified, owner              string
	line, nameStart, nameEnd, declEndLine, declEndCol int
	signature                                         string
}

func vbscriptOriginalKey(name string, line int) string {
	return fmt.Sprintf("%s@%d", name, line)
}

// Entire independent Git originals are authoritative here. Source-distinct
// Let/Get accessors share qualified names but must retain separate identities.
func TestVBScriptTwoWholePinnedOriginalInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VBSCRIPT_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two whole pinned VBScript upstream sources not configured")
	}
	cases := []struct {
		file, blob, target string
		size               int64
		want               []vbscriptOriginalDeclaration
	}{
		{
			file: "EA-Resolve.vbs", blob: "5d11760971375d171a0c606ff67d7d2fbc14fca1", size: 927, target: "resolve",
			want: []vbscriptOriginalDeclaration{
				{"main", "function", "sub", "main", "", 12, 5, 9, 14, 8, "sub main"},
				{"resolve", "function", "function", "resolve", "", 16, 10, 17, 42, 13, "function resolve (itemGuid)"},
				{"item", "variable", "variable", "resolve.item", "resolve", 18, 6, 10, 18, 10, ""},
				{"itemName", "variable", "variable", "resolve.itemName", "resolve", 23, 6, 14, 23, 14, ""},
				{"itemNotes", "variable", "variable", "resolve.itemNotes", "resolve", 25, 6, 15, 25, 15, ""},
				{"invalidChars", "variable", "variable", "resolve.invalidChars", "resolve", 27, 6, 18, 27, 18, ""},
				{"invalidChar", "variable", "variable", "resolve.invalidChar", "resolve", 29, 6, 17, 29, 17, ""},
			},
		},
		{
			file: "cavo-Debug.vbs", blob: "4366fbc4af2522082bbd3f3ab98a5a214e02a8b2", size: 1531, target: "Class_Initialize",
			want: []vbscriptOriginalDeclaration{
				{"clsDebug", "class", "class", "clsDebug", "", 12, 7, 15, 12, 15, "Class clsDebug"},
				{"objFSO", "field", "variable", "clsDebug.objFSO", "clsDebug", 14, 6, 12, 14, 21, ""},
				{"objFile", "field", "variable", "clsDebug.objFile", "clsDebug", 14, 14, 21, 14, 21, ""},
				{"bEnable", "field", "variable", "clsDebug.bEnable", "clsDebug", 15, 6, 13, 15, 41, ""},
				{"bOnlyConsole", "field", "variable", "clsDebug.bOnlyConsole", "clsDebug", 15, 15, 27, 15, 41, ""},
				{"bConsoleMode", "field", "variable", "clsDebug.bConsoleMode", "clsDebug", 15, 29, 41, 15, 41, ""},
				{"Enable", "property", "property", "clsDebug.Enable", "clsDebug", 18, 22, 28, 20, 14, "Public Property Let Enable(bYesNo)"},
				{"Enable", "property", "property", "clsDebug.Enable", "clsDebug", 22, 22, 28, 24, 14, "Public Property Get Enable"},
				{"onlyConsole", "property", "property", "clsDebug.onlyConsole", "clsDebug", 29, 22, 33, 31, 14, "Public Property Let onlyConsole(bYesNo)"},
				{"onlyConsole", "property", "property", "clsDebug.onlyConsole", "clsDebug", 33, 22, 33, 35, 14, "Public Property Get onlyConsole"},
				{"Class_Initialize", "method", "sub", "clsDebug.Class_Initialize", "clsDebug", 37, 14, 30, 45, 9, "Private Sub Class_Initialize()"},
				{"Class_Terminate", "method", "sub", "clsDebug.Class_Terminate", "clsDebug", 47, 14, 29, 48, 9, "Private Sub Class_Terminate()"},
				{"echo", "method", "function", "clsDebug.echo", "clsDebug", 52, 18, 22, 62, 14, "Public Function echo(sLine)"},
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
				t.Fatalf("whole pinned original absent or altered: info=%+v err=%v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(data), 0)
			_, _ = hash.Write(data)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("full Git-pinned original blob mismatch")
			}
			failed, outlined, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "vbscript", Encoding: "utf-8", IncludeSignatures: true,
				MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || outlined.Truncated || !outlined.CoverageComplete ||
				outlined.FilesParsed != 1 || len(outlined.Files) != 1 || len(outlined.Symbols) != len(tc.want) {
				t.Fatalf("complete VBScript original inventory: err=%v failed=%+v result=%+v", err, failed, outlined)
			}
			byKey := make(map[string]int, len(outlined.Symbols))
			ids := make(map[string]string, len(outlined.Symbols))
			for i, sym := range outlined.Symbols {
				key := vbscriptOriginalKey(sym.QualifiedName, sym.NameRange.Start.Line)
				if _, duplicate := byKey[key]; duplicate {
					t.Errorf("duplicate physical declaration %s", key)
				}
				byKey[key] = i
				if sym.ID == "" {
					t.Errorf("empty physical symbol identity for %s", key)
				}
				if _, duplicate := ids[sym.ID]; duplicate {
					t.Errorf("two physical declarations share symbol ID %s", key)
				}
				ids[sym.ID] = key
			}
			for _, exp := range tc.want {
				key := vbscriptOriginalKey(exp.qualified, exp.line)
				idx, exists := byKey[key]
				if !exists {
					t.Errorf("missing authored source declaration %+v", exp)
					continue
				}
				sym := outlined.Symbols[idx]
				if sym.Name != exp.name || string(sym.Kind) != exp.kind ||
					sym.NativeKind != exp.native || sym.QualifiedName != exp.qualified ||
					sym.ParentQualifiedName != exp.owner ||
					sym.NameRange.Start.Line != exp.line ||
					sym.NameRange.Start.Column != exp.nameStart ||
					sym.NameRange.End.Line != exp.line ||
					sym.NameRange.End.Column != exp.nameEnd ||
					sym.DeclarationRange.Start.Line != exp.line ||
					sym.DeclarationRange.End.Line != exp.declEndLine ||
					sym.DeclarationRange.End.Column != exp.declEndCol ||
					(exp.signature != "" && sym.Signature != exp.signature) {
					t.Errorf("source-first VBScript physical declaration mismatch: want=%+v got=%+v", exp, sym)
				}
				if exp.owner == "" && sym.ParentID != "" {
					t.Errorf("unexpected owner of root declaration %+v", sym)
				}
				if exp.owner != "" {
					found := false
					for _, possibleParent := range outlined.Symbols {
						if possibleParent.QualifiedName == exp.owner && possibleParent.ID == sym.ParentID {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("missing direct lexical parent link for %+v", sym)
					}
				}
			}
			if tc.file == "cavo-Debug.vbs" {
				for _, pair := range [][2]int{{18, 22}, {29, 33}} {
					left := outlined.Symbols[byKey[vbscriptOriginalKey("clsDebug."+map[int]string{18: "Enable", 29: "onlyConsole"}[pair[0]], pair[0])]]
					right := outlined.Symbols[byKey[vbscriptOriginalKey(left.QualifiedName, pair[1])]]
					if left.ID == right.ID || left.Name != right.Name || left.NameRange.Start.Line == right.NameRange.Start.Line {
						t.Errorf("VBScript Let/Get accessors collapsed: %+v vs %+v", left, right)
					}
				}
			}
			fingerprint := outlined.Files[0].SourceFingerprint
			if fingerprint == "" || outlined.Files[0].SourceBytes != tc.size {
				t.Fatal("original bytes/fingerprint absent")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "vbscript", Encoding: "utf-8",
				Query: tc.target, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 {
				t.Fatalf("whole original VBScript exact find: err=%v failed=%+v result=%+v", err, failed, found)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vbscript", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(strings.ToLower(shown.Show.Text), strings.ToLower(tc.target)) {
				t.Fatalf("original fingerprint-bound VBScript show: err=%v failed=%+v result=%+v", err, failed, shown.Show)
			}
			failed, queried, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: "vbscript", Encoding: "utf-8", Query: tc.target, Match: "exact",
				MaxFiles: 1, MaxResults: 64,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].SymbolID != found.Symbols[0].ID {
				t.Fatalf("original VBScript structural search: err=%v failed=%+v result=%+v", err, failed, queried)
			}
		})
	}
}
