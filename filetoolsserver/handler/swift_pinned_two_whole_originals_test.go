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

// Independently source-first enumerated declarations for two complete Git-pinned
// Swift originals. Nested local functions are physical declarations, not flattened.
func TestSwiftTwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_SWIFT_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two independently pinned whole Swift originals not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		records    string
	}{
		{file: "stdlib-ByteOrder.swift", size: 1129, blob: "6cd5980baa3409bf62b6f3fc64e9a612298dc73d", records: "ByteOrder\u001fenum\u001fenum\u001f\u001fByteOrder\u001f16\u001f13\u001f22\u001f14\u001f1\u001f36\u001f2\u001f14\u001f1\u001f16\u001f54\u001f16\u001f54\u001f36\u001f2\u001f@frozen\n@available(SwiftStdlib 6.4, *)\npublic enum ByteOrder: Equatable, Hashable, Sendable \u001ebigEndian\u001ffield\u001fcase\u001fByteOrder\u001fByteOrder.bigEndian\u001f19\u001f8\u001f17\u001f19\u001f8\u001f19\u001f17\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001elittleEndian\u001ffield\u001fcase\u001fByteOrder\u001fByteOrder.littleEndian\u001f23\u001f8\u001f20\u001f23\u001f8\u001f23\u001f20\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001enative\u001fproperty\u001fvar\u001fByteOrder\u001fByteOrder.native\u001f27\u001f21\u001f27\u001f26\u001f3\u001f35\u001f4\u001f0\u001f0\u001f0\u001f0\u001f27\u001f34\u001f35\u001f4\u001f"},
		{file: "exercism-HighScoreBoardExemplar.swift", size: 951, blob: "d7b986d0181dbcf3495156224d4f8d99d0fa5a49", records: "newScoreBoard\u001ffunction\u001ffunc-definition\u001f\u001fnewScoreBoard\u001f1\u001f6\u001f19\u001f1\u001f1\u001f3\u001f2\u001f1\u001f1\u001f1\u001f39\u001f1\u001f39\u001f3\u001f2\u001ffunc newScoreBoard() -> [String: Int] \u001eaddPlayer\u001ffunction\u001ffunc-definition\u001f\u001faddPlayer\u001f5\u001f6\u001f15\u001f5\u001f1\u001f7\u001f2\u001f5\u001f1\u001f5\u001f81\u001f5\u001f81\u001f7\u001f2\u001ffunc addPlayer(_ scores: inout [String: Int], _ name: String, _ score: Int = 0) \u001eremovePlayer\u001ffunction\u001ffunc-definition\u001f\u001fremovePlayer\u001f9\u001f6\u001f18\u001f9\u001f1\u001f11\u001f2\u001f9\u001f1\u001f9\u001f66\u001f9\u001f66\u001f11\u001f2\u001ffunc removePlayer(_ scores: inout [String: Int], _ name: String) \u001eresetScore\u001ffunction\u001ffunc-definition\u001f\u001fresetScore\u001f13\u001f6\u001f16\u001f13\u001f1\u001f17\u001f2\u001f13\u001f1\u001f13\u001f64\u001f13\u001f64\u001f17\u001f2\u001ffunc resetScore(_ scores: inout [String: Int], _ name: String) \u001eupdateScore\u001ffunction\u001ffunc-definition\u001f\u001fupdateScore\u001f19\u001f6\u001f17\u001f19\u001f1\u001f23\u001f2\u001f19\u001f1\u001f19\u001f79\u001f19\u001f79\u001f23\u001f2\u001ffunc updateScore(_ scores: inout [String: Int], _ name: String, _ delta: Int) \u001eorderByPlayers\u001ffunction\u001ffunc-definition\u001f\u001forderByPlayers\u001f25\u001f6\u001f20\u001f25\u001f1\u001f30\u001f2\u001f25\u001f1\u001f25\u001f65\u001f25\u001f65\u001f30\u001f2\u001ffunc orderByPlayers(_ scores: [String: Int]) -> [(String, Int)] \u001eascendingKeys\u001ffunction\u001ffunc-definition\u001forderByPlayers\u001forderByPlayers.ascendingKeys\u001f26\u001f8\u001f21\u001f26\u001f3\u001f28\u001f4\u001f26\u001f3\u001f26\u001f70\u001f26\u001f70\u001f28\u001f4\u001ffunc ascendingKeys(_ a: (String, Int), _ b: (String, Int)) -> Bool \u001eorderByScores\u001ffunction\u001ffunc-definition\u001f\u001forderByScores\u001f32\u001f6\u001f19\u001f32\u001f1\u001f37\u001f2\u001f32\u001f1\u001f32\u001f64\u001f32\u001f64\u001f37\u001f2\u001ffunc orderByScores(_ scores: [String: Int]) -> [(String, Int)] \u001edescendingValues\u001ffunction\u001ffunc-definition\u001forderByScores\u001forderByScores.descendingValues\u001f33\u001f8\u001f24\u001f33\u001f3\u001f35\u001f4\u001f33\u001f3\u001f33\u001f73\u001f33\u001f73\u001f35\u001f4\u001ffunc descendingValues(_ a: (String, Int), _ b: (String, Int)) -> Bool "},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("full pinned Swift source changed: stat=%+v err=%v", st, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(b), 0)
			_, _ = hash.Write(b)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("full original Git blob mismatch")
			}
			records := strings.Split(tc.records, "\x1e")
			fail, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "swift", Encoding: "utf-8",
				IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 100,
			})
			if err != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 ||
				len(out.Files) != 1 || len(out.Symbols) != len(records) {
				t.Fatalf("whole Swift source-first declaration count want=%d got=%d coverage=%v err=%v fail=%+v", len(records), len(out.Symbols), out.CoverageComplete, err, fail)
			}
			byPosition := map[string]int{}
			ids := map[string]string{}
			idUsed := map[string]bool{}
			for i, s := range out.Symbols {
				key := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := byPosition[key]; dup {
					t.Errorf("duplicate Swift source position %s", key)
				}
				byPosition[key] = i
				if s.ID == "" || idUsed[s.ID] {
					t.Errorf("missing/duplicate Swift ID %+v", s)
				}
				idUsed[s.ID] = true
				ids[s.QualifiedName] = s.ID
			}
			for _, record := range records {
				f := strings.Split(record, "\x1f")
				if len(f) != 21 {
					t.Fatalf("malformed independent Swift oracle row %q", record)
				}
				n := make([]int, 15)
				for i := 0; i < 15; i++ {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				physical := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, ok := byPosition[physical]
				if !ok {
					t.Errorf("original supported declaration missing %s", record)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[0] || s.NameRange.End.Column != n[2] ||
					s.DeclarationRange.Start.Line != n[3] || s.DeclarationRange.Start.Column != n[4] ||
					s.DeclarationRange.End.Line != n[5] || s.DeclarationRange.End.Column != n[6] {
					t.Errorf("independent Swift source declaration mismatch want=%q got=%+v", record, s)
				}
				if f[3] == "" && s.ParentID != "" {
					t.Errorf("unexpected root Swift owner %+v", s)
				}
				if f[3] != "" && s.ParentID != ids[f[3]] {
					t.Errorf("broken local/enum Swift owner %+v", s)
				}
				if n[7] > 0 {
					if s.SignatureRange == nil || s.SignatureRange.Start.Line != n[7] ||
						s.SignatureRange.Start.Column != n[8] || s.SignatureRange.End.Line != n[9] ||
						s.SignatureRange.End.Column != n[10] || s.Signature != f[20] {
						t.Errorf("original Swift signature/range mismatch want=%q got=%+v", record, s)
					}
				} else if s.Signature != "" || s.SignatureRange != nil {
					t.Errorf("unexpected source signature %+v", s)
				}
				if n[11] > 0 {
					if s.BodyRange == nil || s.BodyRange.Start.Line != n[11] ||
						s.BodyRange.Start.Column != n[12] || s.BodyRange.End.Line != n[13] ||
						s.BodyRange.End.Column != n[14] {
						t.Errorf("Swift body range mismatch want=%q got=%+v", record, s)
					}
				} else if s.BodyRange != nil {
					t.Errorf("unexpected body range %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("whole original Swift source fingerprint/size missing")
			}
			target := strings.Split(records[0], "\x1f")[0]
			fail, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "swift", Encoding: "utf-8", Query: target, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
			if err != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != ids[target] {
				t.Fatalf("original Swift exact find err=%v fail=%+v found=%+v", err, fail, found.Symbols)
			}
			fail, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "swift", Encoding: "utf-8", SymbolID: ids[target], SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, target) {
				t.Fatalf("Swift original fingerprint-bound show err=%v fail=%+v show=%+v", err, fail, shown.Show)
			}
			fail, q, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "swift", Encoding: "utf-8", Query: target, Match: "exact", MaxFiles: 1, MaxResults: 100})
			if err != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != ids[target] {
				t.Fatalf("Swift original structural search err=%v fail=%+v found=%+v", err, fail, q)
			}
			for _, neg := range []string{"#if", "#else", "#error", "scores"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "swift", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 100})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("unsupported conditional/local ref inventoried: %s %v %+v", neg, e, fail)
				}
			}
		})
	}
}
