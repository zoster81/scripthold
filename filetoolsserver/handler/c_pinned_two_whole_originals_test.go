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

// Whole original Git sources from curl and universal-ctags. C function
// declarations inside macro replacement lists are not authored source symbols.
func TestCTwoWholePinnedOriginalFunctionAndMacroInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_C_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("two independently pinned complete C original files not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "curl-get-line.c", blob: "6fcd043c9370c29f8830962cb369333249c2c7f1", size: 2246, rows: "Curl_get_line\u001ffunction\u001ffunction-definition\u001f\u001fCurl_get_line\u001f35\u001f10\u001f35\u001f23\u001f35\u001f1\u001f67\u001f2\u001f35\u001f1\u001f36\u001f1\u001f36\u001f1\u001f67\u001f2\u001fCURLcode Curl_get_line(struct dynbuf *buf, FILE *input, bool *eof)\n", count: 1},
		{file: "ctags-numarray.c", blob: "1de737e8f268dd5a25be11405deec6c9aae59b14", size: 6496, rows: "", count: 0}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("complete original C source changed stat=%+v err=%v", st, e)
			}
			bytes, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(bytes), 0)
			_, _ = sha.Write(bytes)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("whole C source raw original Git blob changed")
			}
			failed, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "c", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 100})
			if e != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("C original author-written declaration inventory expected=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, failed)
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("C original source identity missing")
			}
			if tc.count > 0 {
				for _, row := range strings.Split(tc.rows, "\x1e") {
					f := strings.Split(row, "\x1f")
					if len(f) != 22 {
						t.Fatalf("invalid original C oracle %q", row)
					}
					n := make([]int, 16)
					for i := range n {
						v, e := strconv.Atoi(f[i+5])
						if e != nil {
							t.Fatal(e)
						}
						n[i] = v
					}
					matched := false
					for _, s := range out.Symbols {
						if s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] {
							continue
						}
						matched = true
						if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
							s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
							s.ParentID != "" || s.ID == "" ||
							s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
							s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
							s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
							s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
							s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
							s.SignatureRange.End.Column != n[11] || s.Signature != f[21] ||
							s.BodyRange == nil || s.BodyRange.Start.Line != n[12] ||
							s.BodyRange.Start.Column != n[13] || s.BodyRange.End.Line != n[14] ||
							s.BodyRange.End.Column != n[15] {
							t.Errorf("whole C original authored span mismatch expected=%q got=%+v", row, s)
						}
					}
					if !matched {
						t.Errorf("missing complete C original physical function %q", row)
					}
				}
				name := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
				fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "c", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
				if e != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != out.Symbols[0].ID {
					t.Fatalf("C original source exact find %v %+v %+v", e, fail, found.Symbols)
				}
				fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "c", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
				if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, name) {
					t.Fatalf("C original fingerprint-bound show %v %+v %+v", e, fail, shown.Show)
				}
				fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "c", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxResults: 100})
				if e != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != found.Symbols[0].ID {
					t.Fatalf("C original structural search %v %+v %+v", e, fail, q)
				}
			}
			for _, negative := range []string{"#define", "#include", "return", "struct dynbuf", "Curl_dynbuf"} {
				failed, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "c", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
				if e != nil || failed != nil || len(none.Symbols) != 0 {
					t.Errorf("macro/reference invented as C source declaration %q", negative)
				}
			}
		})
	}
}
