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

// Source-first declarations in two complete independently Git-pinned Erlang
// originals. Attribute directives and call sites are not function definitions;
// the Erlang terminal dot is excluded from the supported declaration span.
func TestErlangTwoWholePinnedOriginalSourceInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_ERLANG_OUTLINE_ROOT")
	if root == "" {
		t.Skip("complete original Erlang sources not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		rows       string
	}{{file: "poolboy-worker.erl", size: 341, blob: "4a5bfae4f00a6cbe3e70ee37620e983b2374d93c", rows: "poolboy_worker\u001fmodule\u001fmodule\u001f\u001fpoolboy_worker\u001f3\u001f9\u001f23\u001f3\u001f1\u001f3\u001f24\u001f3\u001f1\u001f3\u001f24\u001f-module(poolboy_worker)"},
		{file: "ranch-acceptor.erl", size: 2694, blob: "a43c09bc0c76a58718bbca6ee481985a7f7ae469", rows: "ranch_acceptor\u001fmodule\u001fmodule\u001f\u001franch_acceptor\u001f15\u001f9\u001f23\u001f15\u001f1\u001f15\u001f24\u001f15\u001f1\u001f15\u001f24\u001f-module(ranch_acceptor)\u001estart_link\u001ffunction\u001ffunction\u001franch_acceptor\u001franch_acceptor.start_link\u001f23\u001f1\u001f11\u001f23\u001f1\u001f26\u001f11\u001f23\u001f1\u001f23\u001f56\u001fstart_link(Ref, AcceptorId, LSocket, Transport, Logger)\u001einit\u001ffunction\u001ffunction\u001franch_acceptor\u001franch_acceptor.init\u001f29\u001f1\u001f5\u001f29\u001f1\u001f31\u001f56\u001f29\u001f1\u001f29\u001f43\u001finit(LSocket, Transport, Logger, ConnsSup)\u001eloop\u001ffunction\u001ffunction\u001franch_acceptor\u001franch_acceptor.loop\u001f34\u001f1\u001f5\u001f34\u001f1\u001f62\u001f64\u001f34\u001f1\u001f34\u001f55\u001floop(LSocket, Transport, Logger, ConnsSup, MonitorRef)\u001eflush\u001ffunction\u001ffunction\u001franch_acceptor\u001franch_acceptor.flush\u001f64\u001f1\u001f6\u001f64\u001f1\u001f72\u001f5\u001f64\u001f1\u001f64\u001f14\u001fflush(Logger)"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("original Erlang source changed: %+v %v", st, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(bytes), 0)
			_, _ = sha.Write(bytes)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("whole original raw Git blob mismatch")
			}
			rows := strings.Split(tc.rows, "\x1e")
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "erlang", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 80})
			if err != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != len(rows) {
				t.Fatalf("source-first whole-original Erlang inventory want=%d got=%d coverage=%v err=%v fail=%+v", len(rows), len(out.Symbols), out.CoverageComplete, err, fail)
			}
			physical := map[string]int{}
			ids := map[string]string{}
			uniqueIDs := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, duplicate := physical[loc]; duplicate {
					t.Errorf("duplicate declaration location %s", loc)
				}
				physical[loc] = i
				if s.ID == "" || uniqueIDs[s.ID] {
					t.Errorf("empty/duplicate Erlang ID %+v", s)
				}
				uniqueIDs[s.ID] = true
				ids[s.QualifiedName] = s.ID
			}
			for _, row := range rows {
				f := strings.Split(row, "\x1f")
				if len(f) != 17 {
					t.Fatalf("broken independent original oracle %q", row)
				}
				n := make([]int, 11)
				for i := range n {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				loc := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, ok := physical[loc]
				if !ok {
					t.Errorf("missing complete original function/module %s", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[0] || s.NameRange.End.Column != n[2] ||
					s.DeclarationRange.Start.Line != n[3] || s.DeclarationRange.Start.Column != n[4] ||
					s.DeclarationRange.End.Line != n[5] || s.DeclarationRange.End.Column != n[6] ||
					s.SignatureRange == nil || s.SignatureRange.Start.Line != n[7] ||
					s.SignatureRange.Start.Column != n[8] || s.SignatureRange.End.Line != n[9] ||
					s.SignatureRange.End.Column != n[10] || s.Signature != f[16] {
					t.Errorf("source-first Erlang mismatch expected=%q got=%+v", row, s)
				}
				if f[3] == "" && s.ParentID != "" {
					t.Errorf("unexpected module parent %+v", s)
				}
				if f[3] != "" && s.ParentID != ids[f[3]] {
					t.Errorf("wrong function module owner %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("source identity missing")
			}
			name := strings.Split(rows[0], "\x1f")[0]
			fail, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "erlang", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxSymbols: 80})
			if err != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != ids[name] {
				t.Fatalf("module find err=%v fail=%+v symbols=%+v", err, fail, found.Symbols)
			}
			fail, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "erlang", Encoding: "utf-8", SymbolID: ids[name], SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, name) {
				t.Fatalf("module fingerprint show err=%v fail=%+v show=%+v", err, fail, shown.Show)
			}
			fail, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "erlang", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxResults: 80})
			if err != nil || fail != nil || query.Search == nil || len(query.Search.Matches) != 1 || query.Search.Matches[0].SymbolID != ids[name] {
				t.Fatalf("module structural search err=%v fail=%+v matches=%+v", err, fail, query)
			}
			for _, neg := range []string{"-callback", "-export", "-spec", "?MODULE"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "erlang", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 80})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("unsupported attribute/reference as symbol %s", neg)
				}
			}
		})
	}
}
