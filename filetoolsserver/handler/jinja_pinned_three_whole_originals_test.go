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

// Source-first inventory over complete original Jinja documents from Flask
// and Trellis. A control-only J2 original must never invent macro or block symbols.
func TestJinjaThreeWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JINJA_OUTLINE_ROOT")
	if root == "" {
		t.Skip("complete pinned original Jinja files not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "flask-base.html", size: 752, blob: "f09e92687c7acf29f0068ae485859dca43bfb5c3", rows: "title|section|block||title|2|17|2|22|2|8|2|39|2|25|2|25\u001eheader|section|block||header|18|14|18|20|18|5|18|37|18|23|18|23\u001econtent|section|block||content|23|12|23|19|23|3|23|36|23|22|23|22", count: 3},
		{file: "flask-macro.html", size: 55, blob: "3460ae2e09de73a52bc617ccf77b4c4a11821a48", rows: "hello|function|macro||hello|1|10|1|15|1|1|1|55|1|24|1|41", count: 1},
		{file: "trellis-wordpress-sites.j2", size: 400, blob: "07e4136cb69024cc066a9ae5530d202b469fa5c1", rows: "", count: 0}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("original complete Jinja source changed %+v %v", st, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(b), 0)
			_, _ = hash.Write(b)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("original Jinja Git blob identity changed")
			}
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "jinja", Encoding: "utf-8",
				IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 70,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 ||
				len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("full original source-first Jinja count want=%d got=%d coverage=%v err=%v failed=%+v", tc.count, len(out.Symbols), out.CoverageComplete, err, failed)
			}
			physical := map[string]int{}
			ids := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := physical[loc]; dup {
					t.Errorf("duplicate physical macro/block %s", loc)
				}
				physical[loc] = i
				if s.ID == "" || ids[s.ID] {
					t.Errorf("missing/duplicate Jinja symbol ID %+v", s)
				}
				ids[s.ID] = true
			}
			if tc.count > 0 {
				for _, row := range strings.Split(tc.rows, "\x1e") {
					f := strings.Split(row, "|")
					if len(f) != 17 {
						t.Fatalf("invalid frozen Jinja inventory row %q", row)
					}
					n := make([]int, 12)
					for i := range n {
						v, e := strconv.Atoi(f[i+5])
						if e != nil {
							t.Fatal(e)
						}
						n[i] = v
					}
					loc := fmt.Sprintf("%d:%d", n[0], n[1])
					idx, exists := physical[loc]
					if !exists {
						t.Errorf("missing source-first Jinja declaration %s", row)
						continue
					}
					s := out.Symbols[idx]
					if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
						s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] || s.ParentID != "" ||
						s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
						s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
						s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
						s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
						s.BodyRange == nil || s.BodyRange.Start.Line != n[8] || s.BodyRange.Start.Column != n[9] ||
						s.BodyRange.End.Line != n[10] || s.BodyRange.End.Column != n[11] {
						t.Errorf("complete Jinja source-first range or owner mismatch want=%q got=%+v", row, s)
					}
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("Jinja source identity missing")
			}
			if tc.count > 0 {
				first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "|")[0]
				fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "jinja", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 70})
				if e != nil || fail != nil || len(found.Symbols) != 1 {
					t.Fatalf("Jinja original find %v %+v %+v", e, fail, found.Symbols)
				}
				fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "jinja", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
				if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
					t.Fatalf("Jinja fingerprint show %v %+v %+v", e, fail, shown.Show)
				}
				fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "jinja", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 70})
				if e != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != found.Symbols[0].ID {
					t.Fatalf("Jinja structural search %v %+v %+v", e, fail, q)
				}
			}
			for _, neg := range []string{"url_for", "endblock", "endmacro", "wordpress_sites", "get_flashed_messages"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "jinja", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 70})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("Jinja control/reference incorrectly declared %s", neg)
				}
			}
		})
	}
}
