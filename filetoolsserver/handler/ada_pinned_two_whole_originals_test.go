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

// Every supported declaration in two independent complete Ada Git originals:
// specification generics and nested implementation package/functions.
func TestAdaTwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ADA_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("two complete independently pinned Ada originals not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "ctags-input2.ads", blob: "f86b57cc8781d7acb1abe21bba3a369e21618d48", size: 242, count: 5, rows: "input_2\u001fpackage\u001fpackage\u001f\u001finput_2\u001f1\u001f9\u001f16\u001f1\u001f1\u001f19\u001fpackage input_2 is\u001eGenerator\u001ftype\u001ftype\u001finput_2\u001finput_2.Generator\u001f3\u001f8\u001f17\u001f3\u001f3\u001f33\u001ftype Generator is null record;\u001eUnsigned\u001ftype\u001ftype\u001finput_2\u001finput_2.Unsigned\u001f5\u001f10\u001f18\u001f5\u001f5\u001f29\u001ftype Unsigned is mod <>;\u001eReal\u001ftype\u001ftype\u001finput_2\u001finput_2.Real\u001f6\u001f10\u001f14\u001f5\u001f6\u001f28\u001ftype Real is digits <>;\u001eRandom\u001ffunction\u001ffunction\u001finput_2\u001finput_2.Random\u001f8\u001f12\u001f18\u001f3\u001f8\u001f46\u001ffunction Random (G: Generator) return Real;"},
		{file: "sample-binary-search.adb", blob: "51738a3a485f99140b718b0cabcd40d22456039f", size: 3205, count: 6, rows: "Binary_Search\u001ffunction\u001fprocedure\u001f\u001fBinary_Search\u001f6\u001f11\u001f24\u001f1\u001f6\u001f27\u001fprocedure Binary_Search is\u001eInt_Vec\u001fpackage\u001fpackage\u001fBinary_Search\u001fBinary_Search.Int_Vec\u001f10\u001f12\u001f19\u001f4\u001f11\u001f78\u001fpackage Int_Vec is new\n     Ada.Containers.Vectors (Index_Type => Natural, Element_Type => Integer);\u001eContains\u001ffunction\u001ffunction\u001fBinary_Search\u001fBinary_Search.Contains\u001f15\u001f13\u001f21\u001f4\u001f15\u001f69\u001ffunction Contains (V : Vector; Value : Integer) return Boolean is\u001eParse_Int\u001ffunction\u001ffunction\u001fBinary_Search\u001fBinary_Search.Parse_Int\u001f47\u001f13\u001f22\u001f4\u001f47\u001f53\u001ffunction Parse_Int (S : String) return Integer is\u001eParse_Sorted_List\u001ffunction\u001ffunction\u001fBinary_Search\u001fBinary_Search.Parse_Sorted_List\u001f60\u001f13\u001f30\u001f4\u001f60\u001f60\u001ffunction Parse_Sorted_List (S : String) return Vector is\u001ePrint_Usage\u001ffunction\u001fprocedure\u001fBinary_Search\u001fBinary_Search.Print_Usage\u001f101\u001f14\u001f25\u001f4\u001f101\u001f28\u001fprocedure Print_Usage is"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("Ada original changed stat=%+v err=%v", st, e)
			}
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(b), 0)
			_, _ = sha.Write(b)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("complete Ada original Git blob mismatch")
			}
			failed, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "ada", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("Ada whole original inventory want=%d got=%d coverage=%v err=%v failed=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, failed)
			}
			locations := map[string]int{}
			seenIDs := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, found := locations[loc]; found {
					t.Errorf("duplicate Ada physical source position %s", loc)
				}
				locations[loc] = i
				if s.ID == "" || seenIDs[s.ID] {
					t.Errorf("missing or duplicate Ada symbol ID %+v", s)
				}
				seenIDs[s.ID] = true
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 12 {
					t.Fatalf("invalid Ada source-first oracle row %q", row)
				}
				n := make([]int, 6)
				for i := range n {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				loc := fmt.Sprintf("%d:%d", n[0], n[1])
				index, found := locations[loc]
				if !found {
					t.Errorf("missing authored Ada declaration %q", row)
					continue
				}
				s := out.Symbols[index]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[0] || s.NameRange.End.Column != n[2] ||
					s.DeclarationRange.Start.Line != n[0] || s.DeclarationRange.Start.Column != n[3] ||
					s.DeclarationRange.End.Line != n[4] || s.DeclarationRange.End.Column != n[5] ||
					s.SignatureRange == nil || s.Signature != f[11] {
					t.Errorf("Ada original declaration name/ownership/signature/ranges mismatch expected=%q got=%+v", row, s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("Ada full source fingerprint/bytes missing")
			}
			first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			failed, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "ada", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != out.Symbols[0].ID {
				t.Fatalf("original Ada package find %v %+v %+v", e, failed, found.Symbols)
			}
			failed, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "ada", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 14000})
			if e != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("Ada original show %v %+v %+v", e, failed, shown.Show)
			}
			failed, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "ada", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 120})
			matched := false
			if q.Search != nil {
				for _, m := range q.Search.Matches {
					if m.SymbolID == found.Symbols[0].ID && m.Name == first {
						matched = true
					}
				}
			}
			if e != nil || failed != nil || !matched {
				t.Fatalf("Ada original structural search %v %+v %+v", e, failed, q)
			}
			for _, negative := range []string{"with", "use", "begin", "return", "declare", "ghostSymbol"} {
				failed, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "ada", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || failed != nil || len(none.Symbols) != 0 {
					t.Errorf("Ada nondeclaration invented as symbol %q", negative)
				}
			}
		})
	}
}
