package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Maintenance targets, adjacent statements and distractors were chosen by
// reading the original source before consulting Source Intelligence.
// These tests do not execute the original programs.
func TestSourceSymbolsOriginalClassicBasicTaskNavigation(t *testing.T) {
	type target struct {
		rel, name, kind string
		line            int
		excerpt         string
	}
	check := func(t *testing.T, h *Handler, root string, e target) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(e.rel))
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "classic-basic",
			Encoding: "utf-8", Query: e.name, Match: "exact", MaxFiles: 1, MaxSymbols: 12,
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated {
			t.Fatalf("find %q failed: err=%v tool=%+v output=%+v", e.name, err, failed, out)
		}
		var selected string
		var fingerprint string
		for _, s := range out.Symbols {
			if s.Name == e.name && s.QualifiedName == e.name && string(s.Kind) == e.kind &&
				s.DeclarationRange.Start.Line == e.line && s.NameRange.Start.Line == e.line {
				selected = s.ID
				if len(out.Files) == 1 {
					fingerprint = out.Files[0].SourceFingerprint
				}
			}
		}
		if selected == "" {
			t.Fatalf("missing source target %s line %d; found=%+v", e.name, e.line, out.Symbols)
		}
		if e.excerpt == "" {
			return
		}
		if fingerprint == "" {
			t.Fatalf("no fingerprint for %s", e.name)
		}
		failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: path, SymbolID: selected,
			SourceFingerprint: fingerprint, Language: "classic-basic", Encoding: "utf-8",
			MaxBytes: 8192,
		})
		if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, e.excerpt) {
			t.Fatalf("fingerprinted show %s incomplete: err=%v tool=%+v show=%+v",
				e.name, err, failed, shown.Show)
		}
	}
	digest := func(t *testing.T, h *Handler, root string, rels ...string) []string {
		t.Helper()
		paths := make([]string, 0, len(rels))
		for _, rel := range rels {
			path := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("original file not regular: %s: %v", rel, err)
			}
			paths = append(paths, path)
		}
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: paths, Language: "classic-basic",
			Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || out.FilesParsed != len(paths) {
			t.Fatalf("compact original-project digest failed: err=%v tool=%+v out=%+v", err, failed, out)
		}
		data, err := json.Marshal(out)
		if err != nil || len(data) > 8192 {
			t.Fatalf("digest too large: %d bytes err=%v", len(data), err)
		}
		t.Logf("files=%d symbols=%d digestBytes=%d complete=%v", out.FilesParsed, out.SymbolCount, len(data), out.CoverageComplete)
		return paths
	}

	t.Run("bagels-guess-limit-clues-vs-hangman", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_CLASSIC_NAV_GAMES_ROOT")
		if root == "" {
			t.Skip("original games collection not configured")
		}
		h := NewHandler([]string{root})
		paths := digest(t, h, root, "bagels.bas", "hang.bas")
		for _, e := range []target{
			{"bagels.bas", "220", "label", 22, "FOR I=1 TO 20"},
			{"bagels.bas", "615", "label", 65, "THAT'S 20 GUESSES"},
			{"bagels.bas", "350", "label", 38, "C=0:D=0"},
			{"bagels.bas", "500", "label", 53, "IF D=3 THEN 680"},
			{"bagels.bas", "630", "label", 67, "TRY GUESSING A THREE-DIGIT NUMBER"},
			{"bagels.bas", "A1", "variable", 6, ""},
			{"bagels.bas", "A", "variable", 6, ""},
			{"bagels.bas", "B", "variable", 6, ""},
			{"hang.bas", "400", "label", 40, "SORRY, THAT LETTER ISN'T IN THE WORD"},
			{"hang.bas", "600", "label", 66, "SORRY, YOU LOSE"},
		} {
			check(t, h, root, e)
		}
		main, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(main), "220 FOR I=1 TO 20") ||
			!strings.Contains(string(main), "615 PRINT \"THAT'S 20 GUESSES") ||
			!strings.Contains(string(main), "590 PRINT \"BAGLES\"") {
			t.Fatalf("original guesses/clues oracle changed: %v", err)
		}
		alternate, err := os.ReadFile(paths[1])
		if err != nil || !strings.Contains(string(alternate), "IF M=10 THEN 600") {
			t.Fatalf("independent Hangman loss threshold oracle changed: %v", err)
		}
		failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "classic-basic", Encoding: "utf-8",
			Query: "220", Match: "exact", Mode: "structural", MaxResults: 12,
		})
		if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 2 {
			t.Fatalf("independent numbered targets must remain distinguishable: err=%v tool=%+v search=%+v", err, failed, search.Search)
		}
	})
	t.Run("connect4-diagonal-defense-vs-tictactoe", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_CLASSIC_NAV_HOARD_ROOT")
		if root == "" {
			t.Skip("original GW-BASIC collection not configured")
		}
		h := NewHandler([]string{root})
		main := "AllBasicCode/Connect4.bas"
		other := "AllBasicCode/TICTAC.BAS"
		paths := digest(t, h, root, main, other)
		for _, e := range []target{
			{main, "100", "label", 21, "DIM A$(6)"},
			{main, "110", "label", 23, "WR = 0"},
			{main, "250", "label", 38, "COLOR 7, 0"},
			{main, "450", "label", 60, "IF Y < 4 THEN 490"},
			{main, "520", "label", 69, "NEXT X: GOSUB 1980"},
			{main, "630", "label", 82, "Z$ = LEFT$(Z$"},
			{main, "A$", "variable", 21, ""},
			{main, "B$", "variable", 21, ""},
			{main, "M$", "variable", 21, ""},
			{other, "320", "label", 30, "REM MAGIC"},
			{other, "450", "label", 42, "CHOOSE AN UNFORCED MOVE"},
		} {
			check(t, h, root, e)
		}
		data, err := os.ReadFile(paths[0])
		if err != nil || !strings.Contains(string(data), `Z$ = "       "`) ||
			!strings.Contains(string(data), `IF A$ = "222" THEN 610`) ||
			!strings.Contains(string(data), "520 NEXT X: GOSUB 1980") {
			t.Fatalf("original Connect4 priority/diagonal oracle changed: %v", err)
		}
		data, err = os.ReadFile(paths[1])
		if err != nil || !strings.Contains(string(data), "320 REM MAGIC") {
			t.Fatalf("unrelated TicTacToe strategy oracle changed: %v", err)
		}
		failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "classic-basic", Encoding: "utf-8",
			Query: "450", Match: "exact", Mode: "structural", MaxResults: 12,
		})
		if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 2 {
			t.Fatalf("two independently named line targets must remain distinguishable: err=%v tool=%+v search=%+v", err, failed, search.Search)
		}
	})
}
