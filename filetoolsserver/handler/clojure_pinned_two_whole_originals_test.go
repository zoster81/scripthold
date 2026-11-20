package handler

import (
	"bytes"
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

// Complete independently archived upstream Clojure originals are stored with
// CRLF. The pinned upstream Git blob identities use canonical LF; normalize
// only for identity checking, never mutate the original source text.
func TestClojureTwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CLOJURE_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("two independent pinned complete Clojure originals not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "kde-clojure.clj", blob: "b2f3768851fe5086752013d4696e074b393b7472", size: 1772, rows: "highlighting\u001fnamespace\u001fns\u001f\u001fhighlighting\u001f4\u001f5\u001f4\u001f17\u001f2\u001f1\u001f4\u001f18\u001f2\u001f1\u001f4\u001f17\u001f(ns ^{:doc \"Syntax highlighting test file\"\r\n      :author \"Markus Brenneis\"}\r\n    highlighting\u001esomething-else\u001ffunction\u001fdefn\u001fhighlighting\u001fhighlighting.something-else\u001f6\u001f7\u001f6\u001f21\u001f6\u001f1\u001f10\u001f6\u001f6\u001f1\u001f6\u001f21\u001f(defn something-else\u001efoo\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.foo\u001f12\u001f6\u001f12\u001f9\u001f12\u001f1\u001f13\u001f54\u001f12\u001f1\u001f12\u001f9\u001f(def foo\u001efizz\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.fizz\u001f15\u001f6\u001f15\u001f10\u001f15\u001f1\u001f16\u001f28\u001f15\u001f1\u001f15\u001f10\u001f(def fizz\u001efizz'\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.fizz'\u001f18\u001f6\u001f18\u001f11\u001f18\u001f1\u001f19\u001f29\u001f18\u001f1\u001f18\u001f11\u001f(def fizz'\u001ebar\u001ffunction\u001fdefn\u001fhighlighting\u001fhighlighting.bar\u001f21\u001f7\u001f21\u001f10\u001f21\u001f1\u001f25\u001f52\u001f21\u001f1\u001f21\u001f10\u001f(defn bar\u001ex-2-y\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.x-2-y\u001f27\u001f6\u001f27\u001f11\u001f27\u001f1\u001f32\u001f15\u001f27\u001f1\u001f27\u001f11\u001f(def x-2-y\u001ebases\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.bases\u001f34\u001f6\u001f34\u001f11\u001f34\u001f1\u001f40\u001f25\u001f34\u001f1\u001f34\u001f11\u001f(def bases\u001es\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.s\u001f45\u001f6\u001f45\u001f7\u001f45\u001f1\u001f45\u001f18\u001f45\u001f1\u001f45\u001f7\u001f(def s\u001er\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.r\u001f46\u001f6\u001f46\u001f7\u001f46\u001f1\u001f46\u001f16\u001f46\u001f1\u001f46\u001f7\u001f(def r\u001estuff!\u001ffunction\u001fdefn-\u001fhighlighting\u001fhighlighting.stuff!\u001f48\u001f8\u001f48\u001f14\u001f48\u001f1\u001f52\u001f36\u001f48\u001f1\u001f48\u001f14\u001f(defn- stuff!\u001ethrowIllegalArgumentException!\u001ffunction\u001fdefn-\u001fhighlighting\u001fhighlighting.throwIllegalArgumentException!\u001f54\u001f8\u001f54\u001f38\u001f54\u001f1\u001f59\u001f41\u001f54\u001f1\u001f54\u001f38\u001f(defn- throwIllegalArgumentException!\u001elet-fn\u001ffunction\u001fdefmacro\u001fhighlighting\u001fhighlighting.let-fn\u001f61\u001f11\u001f61\u001f17\u001f61\u001f1\u001f65\u001f20\u001f61\u001f1\u001f61\u001f17\u001f(defmacro let-fn\u001estate\u001fvariable\u001fdef\u001fhighlighting\u001fhighlighting.state\u001f67\u001f6\u001f67\u001f11\u001f67\u001f1\u001f67\u001f40\u001f67\u001f1\u001f67\u001f11\u001f(def state\u001esomething-cool\u001ffunction\u001fdefn\u001fhighlighting\u001fhighlighting.something-cool\u001f69\u001f7\u001f69\u001f21\u001f69\u001f1\u001f69\u001f40\u001f69\u001f1\u001f69\u001f21\u001f(defn something-cool\u001esomething-different\u001ffunction\u001fdefn-\u001fhighlighting\u001fhighlighting.something-different\u001f71\u001f8\u001f71\u001f27\u001f71\u001f1\u001f71\u001f58\u001f71\u001f1\u001f71\u001f27\u001f(defn- something-different\u001einstance-getfield\u001ffunction\u001fdefn-\u001fhighlighting\u001fhighlighting.instance-getfield\u001f73\u001f8\u001f73\u001f25\u001f73\u001f1\u001f73\u001f54\u001f73\u001f1\u001f73\u001f25\u001f(defn- instance-getfield", count: 17},
		{file: "ctags-metadata.clj", blob: "9284a0ca7acc0d59c050b838d325b9de66c29dc7", size: 257, rows: "my-fn\u001ffunction\u001fdefn\u001f\u001fmy-fn\u001f1\u001f7\u001f1\u001f12\u001f1\u001f1\u001f1\u001f16\u001f1\u001f1\u001f1\u001f12\u001f(defn my-fn\u001emy-private-fn\u001ffunction\u001fdefn\u001f\u001fmy-private-fn\u001f3\u001f17\u001f3\u001f30\u001f3\u001f1\u001f3\u001f34\u001f3\u001f1\u001f3\u001f30\u001f(defn ^:private my-private-fn\u001emy-public-fn\u001ffunction\u001fdefn\u001f\u001fmy-public-fn\u001f5\u001f20\u001f5\u001f32\u001f5\u001f1\u001f5\u001f36\u001f5\u001f1\u001f5\u001f32\u001f(defn ^{:foo :bar} my-public-fn\u001emy-hinted-fn\u001ffunction\u001fdefn\u001f\u001fmy-hinted-fn\u001f7\u001f7\u001f7\u001f19\u001f7\u001f1\u001f7\u001f37\u001f7\u001f1\u001f7\u001f19\u001f(defn my-hinted-fn\u001emy-private-hinted-fn\u001ffunction\u001fdefn\u001f\u001fmy-private-hinted-fn\u001f9\u001f17\u001f9\u001f37\u001f9\u001f1\u001f9\u001f55\u001f9\u001f1\u001f9\u001f37\u001f(defn ^:private my-private-hinted-fn\u001eso-many-hints\u001ffunction\u001fdefn\u001f\u001fso-many-hints\u001f11\u001f17\u001f11\u001f30\u001f11\u001f1\u001f12\u001f18\u001f11\u001f1\u001f11\u001f30\u001f(defn ^:private so-many-hints", count: 6}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("complete Clojure original changed stat=%+v err=%v", st, e)
			}
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Contains(raw, []byte("\r\n")) {
				t.Fatal("expected original CRLF source representation changed")
			}
			canonical := bytes.ReplaceAll(raw, []byte("\r\n"), []byte("\n"))
			blobHash := sha1.New()
			_, _ = fmt.Fprintf(blobHash, "blob %d%c", len(canonical), 0)
			_, _ = blobHash.Write(canonical)
			if hex.EncodeToString(blobHash.Sum(nil)) != tc.blob {
				t.Fatal("whole Clojure original canonical Git blob mismatch")
			}
			failed, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "clojure", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("complete Clojure source-first inventory expected=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, failed)
			}
			locations := map[string]int{}
			ids := map[string]string{}
			seen := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := locations[loc]; dup {
					t.Errorf("duplicate Clojure original declaration position %s", loc)
				}
				locations[loc] = i
				if s.ID == "" || seen[s.ID] {
					t.Errorf("missing/duplicate Clojure symbol ID %+v", s)
				}
				seen[s.ID] = true
				ids[s.QualifiedName] = s.ID
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 18 {
					t.Fatalf("invalid Clojure frozen oracle row %q", row)
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
				idx, exists := locations[loc]
				if !exists {
					t.Errorf("missing authored original Clojure definition %q", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
					s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
					s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
					s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
					s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
					s.SignatureRange.End.Column != n[11] || s.Signature != f[17] {
					t.Errorf("whole Clojure authored source declaration mismatch expected=%q got=%+v", row, s)
				}
				if f[3] == "" && s.ParentID != "" {
					t.Errorf("unexpected namespace owner ID %+v", s)
				}
				if f[3] != "" && s.ParentID != "" && ids[f[3]] != "" && s.ParentID != ids[f[3]] {
					t.Errorf("Clojure namespace owner mismatch %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("complete Clojure source fingerprint/bytes missing")
			}
			first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			failed, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "clojure", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].Name != first {
				t.Fatalf("Clojure original exact find %v %+v %+v", e, failed, found.Symbols)
			}
			failed, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "clojure", Encoding: "utf-8", SymbolID: found.Symbols[0].ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 14000})
			if e != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("Clojure source-bound show %v %+v %+v", e, failed, shown.Show)
			}
			failed, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "clojure", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 120})
			matched := false
			if q.Search != nil {
				for _, m := range q.Search.Matches {
					if m.SymbolID == found.Symbols[0].ID && m.Name == first {
						matched = true
					}
				}
			}
			if e != nil || failed != nil || !matched {
				t.Fatalf("original Clojure structural declaration missing %v %+v %+v", e, failed, q)
			}
			for _, neg := range []string{"def", "println", "let", "ghostSymbol", "readerFake"} {
				failed, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "clojure", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || failed != nil || len(none.Symbols) != 0 {
					t.Errorf("Clojure reader form/reference invented as declaration %q", neg)
				}
			}
		})
	}
}
