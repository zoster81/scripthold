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

// Two complete upstream JavaScript sources with ASI, regular expressions,
// top-level var declarations and function bodies; independent physical oracle.
func TestJavaScriptTwoWholePinnedOriginalInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JS_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("two independently pinned JS complete sources not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "geekmagic-utils.js", blob: "dfcb62f65415d9782f4194f666f82eaac7077785", size: 1583, count: 4, rows: "humanFileSize\u001ffunction\u001ffunction-definition\u001f\u001fhumanFileSize\u001f1\u001f10\u001f1\u001f23\u001f1\u001f1\u001f11\u001f2\u001f1\u001f1\u001f1\u001f31\u001f1\u001f31\u001f11\u001f2\u001ffunction humanFileSize(bytes) \u001eapiFetch\u001ffunction\u001ffunction-definition\u001f\u001fapiFetch\u001f13\u001f10\u001f13\u001f18\u001f13\u001f1\u001f29\u001f2\u001f13\u001f1\u001f13\u001f38\u001f13\u001f38\u001f29\u001f2\u001ffunction apiFetch(url, options = {}) \u001eincludeHTML\u001ffunction\u001ffunction-definition\u001f\u001fincludeHTML\u001f31\u001f10\u001f31\u001f21\u001f31\u001f1\u001f38\u001f2\u001f31\u001f1\u001f31\u001f41\u001f31\u001f41\u001f38\u001f2\u001ffunction includeHTML(id, url, callback) \u001esetHeaderTitle\u001ffunction\u001ffunction-definition\u001f\u001fsetHeaderTitle\u001f40\u001f10\u001f40\u001f24\u001f40\u001f1\u001f48\u001f2\u001f40\u001f1\u001f40\u001f32\u001f40\u001f32\u001f48\u001f2\u001ffunction setHeaderTitle(title) "},
		{file: "express-generator-test-utils.js", blob: "9afc44044e807acf7e298fd75a1ab5fb8d249505", size: 1794, count: 11, rows: "fs\u001fvariable\u001fvar\u001f\u001ffs\u001f3\u001f5\u001f3\u001f7\u001f3\u001f1\u001f3\u001f23\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001eos\u001fvariable\u001fvar\u001f\u001fos\u001f4\u001f5\u001f4\u001f7\u001f4\u001f1\u001f4\u001f23\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001epath\u001fvariable\u001fvar\u001f\u001fpath\u001f5\u001f5\u001f5\u001f9\u001f5\u001f1\u001f5\u001f27\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001euid\u001fvariable\u001fvar\u001f\u001fuid\u001f6\u001f5\u001f6\u001f8\u001f6\u001f1\u001f6\u001f30\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001eWARNINGS_REGEXP\u001fvariable\u001fvar\u001f\u001fWARNINGS_REGEXP\u001f8\u001f5\u001f8\u001f20\u001f8\u001f1\u001f8\u001f73\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001echildEnvironment\u001ffunction\u001ffunction-definition\u001f\u001fchildEnvironment\u001f17\u001f10\u001f17\u001f26\u001f17\u001f1\u001f28\u001f2\u001f17\u001f1\u001f17\u001f30\u001f17\u001f30\u001f28\u001f2\u001ffunction childEnvironment () \u001eparseCreatedFiles\u001ffunction\u001ffunction-definition\u001f\u001fparseCreatedFiles\u001f30\u001f10\u001f30\u001f27\u001f30\u001f1\u001f50\u001f2\u001f30\u001f1\u001f30\u001f42\u001f30\u001f42\u001f50\u001f2\u001ffunction parseCreatedFiles (output, dir) \u001eparseWarnings\u001ffunction\u001ffunction-definition\u001f\u001fparseWarnings\u001f52\u001f10\u001f52\u001f23\u001f52\u001f1\u001f64\u001f2\u001f52\u001f1\u001f52\u001f30\u001f52\u001f30\u001f64\u001f2\u001ffunction parseWarnings (str) \u001estripColors\u001ffunction\u001ffunction-definition\u001f\u001fstripColors\u001f66\u001f10\u001f66\u001f21\u001f66\u001f1\u001f69\u001f2\u001f66\u001f1\u001f66\u001f28\u001f66\u001f28\u001f69\u001f2\u001ffunction stripColors (str) \u001estripWarnings\u001ffunction\u001ffunction-definition\u001f\u001fstripWarnings\u001f71\u001f10\u001f71\u001f23\u001f71\u001f1\u001f73\u001f2\u001f71\u001f1\u001f71\u001f30\u001f71\u001f30\u001f73\u001f2\u001ffunction stripWarnings (str) \u001etmpDir\u001ffunction\u001ffunction-definition\u001f\u001ftmpDir\u001f75\u001f10\u001f75\u001f16\u001f75\u001f1\u001f81\u001f2\u001f75\u001f1\u001f75\u001f20\u001f75\u001f20\u001f81\u001f2\u001ffunction tmpDir () "}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("whole JS source changed %+v %v", st, e)
			}
			b, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(b), 0)
			_, _ = sha.Write(b)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("whole original JS raw Git blob differs")
			}
			fail, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "javascript", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("full original JS source-first expected=%d got=%d complete=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, fail)
			}
			physical := map[string]int{}
			seen := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, exists := physical[loc]; exists {
					t.Errorf("duplicate JS declaration %s", loc)
				}
				physical[loc] = i
				if s.ID == "" || seen[s.ID] {
					t.Errorf("duplicate/missing JS ID %+v", s)
				}
				seen[s.ID] = true
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 22 {
					t.Fatalf("bad original oracle row %q", row)
				}
				n := make([]int, 16)
				for i := range n {
					v, err := strconv.Atoi(f[i+5])
					if err != nil {
						t.Fatal(err)
					}
					n[i] = v
				}
				loc := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, exists := physical[loc]
				if !exists {
					t.Errorf("missing JS original authored declaration %q", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] || s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] || s.ParentID != "" ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] || s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
					s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] || s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] {
					t.Errorf("JS authored metadata mismatch original=%q actual=%+v", row, s)
				}
				if n[8] > 0 {
					if s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] || s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] || s.SignatureRange.End.Column != n[11] || s.Signature != f[21] {
						t.Errorf("JS authored function signature mismatch %+v", s)
					}
				} else if s.SignatureRange != nil || s.Signature != "" {
					t.Errorf("JS phantom signature %+v", s)
				}
				if n[12] > 0 {
					if s.BodyRange == nil || s.BodyRange.Start.Line != n[12] || s.BodyRange.Start.Column != n[13] || s.BodyRange.End.Line != n[14] || s.BodyRange.End.Column != n[15] {
						t.Errorf("JS body mismatch original=%q actual=%+v", row, s)
					}
				} else if s.BodyRange != nil {
					t.Errorf("JS phantom body %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("full JS fingerprint/size missing")
			}
			name := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "javascript", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].Name != name {
				t.Fatalf("JS original exact find %v %+v %+v", e, fail, found.Symbols)
			}
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "javascript", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 15000})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, name) {
				t.Fatalf("JS fingerprint-bound show %v %+v %+v", e, fail, shown.Show)
			}
			fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "javascript", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxResults: 120})
			matched := false
			if q.Search != nil {
				for _, m := range q.Search.Matches {
					if m.SymbolID == found.Symbols[0].ID && m.Name == name {
						matched = true
					}
				}
			}
			if e != nil || fail != nil || !matched {
				t.Fatalf("JS original structural search %v %+v %+v", e, fail, q)
			}
			for _, neg := range []string{"return", "require", "console", "module", "ghostSymbol"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "javascript", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("JS reference invented as authored symbol %q", neg)
				}
			}
		})
	}
}
