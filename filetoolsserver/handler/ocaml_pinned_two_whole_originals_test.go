package handler

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Two complete independent OCaml Git originals: one compiler stub and one
// Exercism source with multiline pipeline and binding-only declarations.
// Every supported authored binding has an independently pinned physical span.
func TestOCamlTwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_OCAML_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("two independently pinned complete OCaml originals not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "ocaml-compiler-scheduling.ml", blob: "b50749aeb29b1f5b733cdd1c54bce5e37cdfb43b", size: 1644, count: 1, rows: "fundecl\u001ffunction\u001ffunction-binding\u001f\u001ffundecl\u001f30\u001f5\u001f30\u001f12\u001f30\u001f1\u001f30\u001f18\u001f30\u001f1\u001f30\u001f18\u001flet fundecl f = f"},
		{file: "exercism-acronym.ml", blob: "3c1ae95117fa7f29046d8f9a0f5d68338474742a", size: 378, count: 3, rows: "delimiters\u001fvariable\u001fvalue-binding\u001f\u001fdelimiters\u001f3\u001f5\u001f3\u001f15\u001f3\u001f1\u001f3\u001f33\u001f3\u001f1\u001f3\u001f33\u001flet delimiters = [' '; '-'; '_']\u001eis_relevant\u001ffunction\u001ffunction-binding\u001f\u001fis_relevant\u001f4\u001f5\u001f4\u001f16\u001f4\u001f1\u001f4\u001f80\u001f4\u001f1\u001f4\u001f80\u001flet is_relevant c = Char.is_alpha c || List.exists delimiters ~f:(Char.equal c)\u001eacronym\u001ffunction\u001ffunction-binding\u001f\u001facronym\u001f6\u001f5\u001f6\u001f12\u001f6\u001f1\u001f12\u001f25\u001f6\u001f1\u001f12\u001f25\u001flet acronym input =\n  input\n  |> String.filter ~f:is_relevant\n  |> String.split_on_chars ~on:delimiters\n  |> List.filter ~f:(fun word -> String.length word > 0)\n  |> List.map ~f:(fun word -> String.get word 0 |> Char.uppercase) \n  |> String.of_char_list"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("whole OCaml original source changed stat=%+v err=%v", st, e)
			}
			bytes, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(bytes), 0)
			_, _ = sha.Write(bytes)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("whole original OCaml raw Git blob changed")
			}
			failed, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "ocaml", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("OCaml whole source-first declarations want=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, failed)
			}
			locations := map[string]int{}
			seen := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := locations[loc]; dup {
					t.Errorf("duplicate authored OCaml declaration at %s", loc)
				}
				locations[loc] = i
				if s.ID == "" || seen[s.ID] {
					t.Errorf("missing or duplicate OCaml symbol ID %+v", s)
				}
				seen[s.ID] = true
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 18 {
					t.Fatalf("invalid independently sourced OCaml oracle row %q", row)
				}
				n := make([]int, 12)
				for i := range n {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				pos := fmt.Sprintf("%d:%d", n[0], n[1])
				index, found := locations[pos]
				if !found {
					t.Errorf("missing original OCaml source binding %q", row)
					continue
				}
				s := out.Symbols[index]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] || s.ParentID != "" ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
					s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
					s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
					s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
					s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
					s.SignatureRange.End.Column != n[11] || s.Signature != f[17] {
					t.Errorf("original authored OCaml binding mismatch expected=%q got=%+v", row, s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("whole original OCaml fingerprint/size missing")
			}
			first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			failed, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "ocaml", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != out.Symbols[0].ID {
				t.Fatalf("OCaml authored binding find %v %+v %+v", e, failed, found.Symbols)
			}
			failed, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "ocaml", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if e != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("OCaml fingerprint-bound show %v %+v %+v", e, failed, shown.Show)
			}
			failed, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "ocaml", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 120})
			foundStructural := false
			if q.Search != nil {
				for _, match := range q.Search.Matches {
					if match.SymbolID == found.Symbols[0].ID && match.Name == first {
						foundStructural = true
					}
				}
			}
			if e != nil || failed != nil || !foundStructural {
				t.Fatalf("OCaml original structural search %v %+v %+v", e, failed, q)
			}
			for _, negative := range []string{"open", "Schedgen", "input", "String", "Char", "ghostSymbol"} {
				failed, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "ocaml", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || failed != nil || len(none.Symbols) != 0 {
					t.Errorf("OCaml reference/comment invented as declaration %q", negative)
				}
			}
		})
	}
}
