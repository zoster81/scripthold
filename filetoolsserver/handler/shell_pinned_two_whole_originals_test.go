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

// Source-first full Git originals from two independent shell projects.
// Bash's "function name()" is supported by the current shell provider and
// must not be excluded from the original declaration oracle.
func TestShellTwoWholePinnedOriginalFunctionInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SHELL_OUTLINE_ROOT")
	if root == "" {
		t.Skip("whole original Shell files not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "vitoris-config_dns.sh", blob: "1529025c4759b3c866f3faf3367e57b22f7343ae", size: 5404, rows: "print_help\u001ffunction\u001ffunction\u001f\u001fprint_help\u001f21\u001f10\u001f21\u001f20\u001f21\u001f10\u001f27\u001f2\u001f21\u001f10\u001f21\u001f23\u001f21\u001f23\u001f27\u001f2\u001fprint_help() \u001echeckForProgram\u001ffunction\u001ffunction\u001f\u001fcheckForProgram\u001f111\u001f10\u001f111\u001f25\u001f111\u001f10\u001f119\u001f2\u001f111\u001f10\u001f111\u001f28\u001f111\u001f28\u001f119\u001f2\u001fcheckForProgram() \u001echeckDomain\u001ffunction\u001ffunction\u001f\u001fcheckDomain\u001f126\u001f10\u001f126\u001f21\u001f126\u001f10\u001f141\u001f2\u001f126\u001f10\u001f126\u001f24\u001f126\u001f24\u001f141\u001f2\u001fcheckDomain() \u001echeckRecord\u001ffunction\u001ffunction\u001f\u001fcheckRecord\u001f144\u001f10\u001f144\u001f21\u001f144\u001f10\u001f157\u001f2\u001f144\u001f10\u001f144\u001f24\u001f144\u001f24\u001f157\u001f2\u001fcheckRecord() \u001edeleteRecord\u001ffunction\u001ffunction\u001f\u001fdeleteRecord\u001f159\u001f10\u001f159\u001f22\u001f159\u001f10\u001f162\u001f2\u001f159\u001f10\u001f159\u001f25\u001f159\u001f25\u001f162\u001f2\u001fdeleteRecord() \u001ewriteDNS\u001ffunction\u001ffunction\u001f\u001fwriteDNS\u001f165\u001f10\u001f165\u001f18\u001f165\u001f10\u001f168\u001f2\u001f165\u001f10\u001f165\u001f21\u001f165\u001f21\u001f168\u001f2\u001fwriteDNS() ", count: 6},
		{file: "arduino-wifi-upgrade.sh", blob: "e3a7d7f7ab82d0ee72e3bb947bb19224e04d3c09", size: 3259, rows: "usage\u001ffunction\u001ffunction\u001f\u001fusage\u001f11\u001f1\u001f11\u001f6\u001f11\u001f1\u001f21\u001f2\u001f11\u001f1\u001f11\u001f10\u001f11\u001f10\u001f21\u001f2\u001fusage () \u001eupgradeHDmodule\u001ffunction\u001ffunction\u001f\u001fupgradeHDmodule\u001f23\u001f1\u001f23\u001f16\u001f23\u001f1\u001f37\u001f2\u001f23\u001f1\u001f23\u001f20\u001f23\u001f20\u001f37\u001f2\u001fupgradeHDmodule () \u001eupgradeShield\u001ffunction\u001ffunction\u001f\u001fupgradeShield\u001f39\u001f1\u001f39\u001f14\u001f39\u001f1\u001f53\u001f2\u001f39\u001f1\u001f39\u001f18\u001f39\u001f18\u001f53\u001f2\u001fupgradeShield () ", count: 3}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("original Shell file changed stat=%+v err=%v", st, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(bytes), 0)
			_, _ = hash.Write(bytes)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("complete source Git blob mismatch")
			}
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "shell", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 80})
			if err != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("full Shell source-first inventory expected=%d got=%d coverage=%v err=%v failed=%+v", tc.count, len(out.Symbols), out.CoverageComplete, err, fail)
			}
			positions := map[string]int{}
			ids := map[string]string{}
			used := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := positions[loc]; dup {
					t.Errorf("repeated physical Shell location %s", loc)
				}
				positions[loc] = i
				if s.ID == "" || used[s.ID] {
					t.Errorf("missing or duplicate function ID %+v", s)
				}
				used[s.ID] = true
				ids[s.Name] = s.ID
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 22 {
					t.Fatalf("invalid frozen shell source-first row %q", row)
				}
				n := make([]int, 16)
				for i := range n {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				loc := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, found := positions[loc]
				if !found {
					t.Errorf("missing source-first full Shell function %s", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] || s.ParentID != "" ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
					s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
					s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
					s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
					s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
					s.SignatureRange.End.Column != n[11] || s.Signature != f[21] ||
					s.BodyRange == nil || s.BodyRange.Start.Line != n[12] ||
					s.BodyRange.Start.Column != n[13] || s.BodyRange.End.Line != n[14] ||
					s.BodyRange.End.Column != n[15] {
					t.Errorf("source-first original Shell metadata mismatch expected=%q got=%+v", row, s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("full original Shell fingerprint/bytes missing")
			}
			first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			fail, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "shell", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 80})
			if err != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != ids[first] {
				t.Fatalf("Shell original function find %v %+v %+v", err, fail, found.Symbols)
			}
			fail, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "shell", Encoding: "utf-8", SymbolID: ids[first], SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("Shell fingerprint show %v %+v %+v", err, fail, shown.Show)
			}
			fail, q, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "shell", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 80})
			if err != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != ids[first] {
				t.Fatalf("Shell original structural search %v %+v %+v", err, fail, q)
			}
			for _, neg := range []string{"DO_PAT", "TARGET_MICRO", "dfu-programmer", "EOF", "exit"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "shell", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 80})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("shell expression/assignment invented as declaration %s", neg)
				}
			}
		})
	}
}
