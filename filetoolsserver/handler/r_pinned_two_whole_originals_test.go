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

// Whole original source-first R function inventories from two upstream
// projects. Dotted S3 method names are authored symbols, while calls and
// local assignments must not be emitted as top-level functions.
func TestRTwoWholePinnedOriginalFunctionInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_R_TWO_WHOLE_ROOT")
	if root == "" {
		t.Skip("independently pinned complete R source originals not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "cli-ansi.R", blob: "d6fd92939338e34d8a4dd015023e2b927e06e27b", size: 12771, count: 20, rows: "palette_idx\u001ffunction\u001fassigned-function\u001f\u001fpalette_idx\u001f4\u001f1\u001f4\u001f12\u001f4\u001f1\u001f18\u001f2\u001f4\u001f1\u001f4\u001f30\u001fpalette_idx <- function(id) {\u001epalette_color\u001ffunction\u001fassigned-function\u001f\u001fpalette_color\u001f20\u001f1\u001f20\u001f14\u001f20\u001f1\u001f22\u001f2\u001f20\u001f1\u001f20\u001f31\u001fpalette_color <- function(x) {\u001eis_builtin_style\u001ffunction\u001fassigned-function\u001f\u001fis_builtin_style\u001f87\u001f1\u001f87\u001f17\u001f87\u001f1\u001f89\u001f2\u001f87\u001f1\u001f87\u001f34\u001fis_builtin_style <- function(x) {\u001eansi_style_str\u001ffunction\u001fassigned-function\u001f\u001fansi_style_str\u001f118\u001f1\u001f118\u001f15\u001f118\u001f1\u001f120\u001f2\u001f118\u001f1\u001f118\u001f32\u001fansi_style_str <- function(x) {\u001ecreate_ansi_style_tag\u001ffunction\u001fassigned-function\u001f\u001fcreate_ansi_style_tag\u001f122\u001f1\u001f122\u001f22\u001f122\u001f1\u001f127\u001f2\u001f122\u001f1\u001f122\u001f71\u001fcreate_ansi_style_tag <- function(name, open, close, palette = NULL) {\u001ecreate_ansi_style_fun\u001ffunction\u001fassigned-function\u001f\u001fcreate_ansi_style_fun\u001f129\u001f1\u001f129\u001f22\u001f129\u001f1\u001f156\u001f2\u001f129\u001f1\u001f129\u001f44\u001fcreate_ansi_style_fun <- function(styles) {\u001ecreate_ansi_style\u001ffunction\u001fassigned-function\u001f\u001fcreate_ansi_style\u001f158\u001f1\u001f158\u001f18\u001f158\u001f1\u001f164\u001f2\u001f158\u001f1\u001f158\u001f65\u001fcreate_ansi_style <- function(name, open = NULL, close = NULL) {\u001eprint.cli_ansi_string\u001ffunction\u001fassigned-function\u001f\u001fprint.cli_ansi_string\u001f168\u001f1\u001f168\u001f22\u001f168\u001f1\u001f174\u001f2\u001f168\u001f1\u001f168\u001f44\u001fprint.cli_ansi_string <- function(x, ...) {\u001eprint.cli_ansi_style\u001ffunction\u001fassigned-function\u001f\u001fprint.cli_ansi_style\u001f178\u001f1\u001f178\u001f21\u001f178\u001f1\u001f183\u001f2\u001f178\u001f1\u001f178\u001f43\u001fprint.cli_ansi_style <- function(x, ...) {\u001emake_ansi_style\u001ffunction\u001fassigned-function\u001f\u001fmake_ansi_style\u001f229\u001f1\u001f229\u001f16\u001f229\u001f1\u001f298\u001f2\u001f229\u001f1\u001f234\u001f4\u001fmake_ansi_style <- function(\n  ...,\n  bg = FALSE,\n  grey = FALSE,\n  colors = num_ansi_colors()\n) {\u001eis_r_color\u001ffunction\u001fassigned-function\u001f\u001fis_r_color\u001f302\u001f1\u001f302\u001f11\u001f302\u001f1\u001f308\u001f2\u001f302\u001f1\u001f302\u001f28\u001fis_r_color <- function(x) {\u001eis_rgb_matrix\u001ffunction\u001fassigned-function\u001f\u001fis_rgb_matrix\u001f310\u001f1\u001f310\u001f14\u001f310\u001f1\u001f312\u001f2\u001f310\u001f1\u001f310\u001f31\u001fis_rgb_matrix <- function(x) {\u001eansi_style_from_r_color\u001ffunction\u001fassigned-function\u001f\u001fansi_style_from_r_color\u001f314\u001f1\u001f314\u001f24\u001f314\u001f1\u001f316\u001f2\u001f314\u001f1\u001f314\u001f67\u001fansi_style_from_r_color <- function(color, bg, num_colors, grey) {\u001eansi_style_8_from_rgb\u001ffunction\u001fassigned-function\u001f\u001fansi_style_8_from_rgb\u001f318\u001f1\u001f318\u001f22\u001f318\u001f1\u001f324\u001f2\u001f318\u001f1\u001f318\u001f45\u001fansi_style_8_from_rgb <- function(rgb, bg) {\u001eansi_style_from_rgb\u001ffunction\u001fassigned-function\u001f\u001fansi_style_from_rgb\u001f326\u001f1\u001f326\u001f20\u001f326\u001f1\u001f334\u001f2\u001f326\u001f1\u001f326\u001f61\u001fansi_style_from_rgb <- function(rgb, bg, num_colors, grey) {\u001eansi_scale\u001ffunction\u001fassigned-function\u001f\u001fansi_scale\u001f345\u001f1\u001f345\u001f11\u001f345\u001f1\u001f356\u001f2\u001f345\u001f1\u001f345\u001f74\u001fansi_scale <- function(x, from = c(0, 255), to = c(0, 5), round = TRUE) {\u001eansi256\u001ffunction\u001fassigned-function\u001f\u001fansi256\u001f358\u001f1\u001f358\u001f8\u001f358\u001f1\u001f373\u001f2\u001f358\u001f1\u001f358\u001f53\u001fansi256 <- function(rgb, bg = FALSE, grey = FALSE) {\u001eansi256_rgb_index\u001ffunction\u001fassigned-function\u001f\u001fansi256_rgb_index\u001f377\u001f1\u001f377\u001f18\u001f377\u001f1\u001f396\u001f2\u001f377\u001f1\u001f377\u001f50\u001fansi256_rgb_index <- function(red, green, blue) {\u001eansitrue\u001ffunction\u001fassigned-function\u001f\u001fansitrue\u001f398\u001f1\u001f398\u001f9\u001f398\u001f1\u001f410\u001f2\u001f398\u001f1\u001f398\u001f40\u001fansitrue <- function(rgb, bg = FALSE) {\u001ecombine_ansi_styles\u001ffunction\u001fassigned-function\u001f\u001fcombine_ansi_styles\u001f446\u001f1\u001f446\u001f20\u001f446\u001f1\u001f453\u001f2\u001f446\u001f1\u001f446\u001f39\u001fcombine_ansi_styles <- function(...) {"},
		{file: "dplyr-rename.R", blob: "74b94e8a59221af415330adc64b91de0e2be6c3c", size: 3458, count: 4, rows: "rename\u001ffunction\u001fassigned-function\u001f\u001frename\u001f63\u001f1\u001f63\u001f7\u001f63\u001f1\u001f65\u001f2\u001f63\u001f1\u001f63\u001f33\u001frename <- function(.data, ...) {\u001erename.data.frame\u001ffunction\u001fassigned-function\u001f\u001frename.data.frame\u001f68\u001f1\u001f68\u001f18\u001f68\u001f1\u001f75\u001f2\u001f68\u001f1\u001f68\u001f44\u001frename.data.frame <- function(.data, ...) {\u001erename_with\u001ffunction\u001fassigned-function\u001f\u001frename_with\u001f83\u001f1\u001f83\u001f12\u001f83\u001f1\u001f85\u001f2\u001f83\u001f1\u001f83\u001f65\u001frename_with <- function(.data, .fn, .cols = everything(), ...) {\u001erename_with.data.frame\u001ffunction\u001fassigned-function\u001f\u001frename_with.data.frame\u001f88\u001f1\u001f88\u001f23\u001f88\u001f1\u001f112\u001f2\u001f88\u001f1\u001f88\u001f76\u001frename_with.data.frame <- function(.data, .fn, .cols = everything(), ...) {"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("whole R original changed stat=%+v err=%v", st, e)
			}
			bytes, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(bytes), 0)
			_, _ = hash.Write(bytes)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("complete original R raw Git blob mismatch")
			}
			failed, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "r", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("original R 100 percent authored function inventory expected=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, failed)
			}
			positions := map[string]int{}
			ids := map[string]string{}
			seen := map[string]bool{}
			for i, s := range out.Symbols {
				loc := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dupe := positions[loc]; dupe {
					t.Errorf("duplicate R function physical location %s", loc)
				}
				positions[loc] = i
				if s.ID == "" || seen[s.ID] {
					t.Errorf("duplicate/missing original R symbol ID %+v", s)
				}
				seen[s.ID] = true
				ids[s.Name] = s.ID
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 18 {
					t.Fatalf("bad R independent oracle row %q", row)
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
				idx, exists := positions[loc]
				if !exists {
					t.Errorf("missing authored R function %q", row)
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
					s.SignatureRange.End.Column != n[11] || s.Signature != f[17] {
					t.Errorf("full original R function source mismatch expected=%q got=%+v", row, s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("original R source file fingerprint/bytes missing")
			}
			first := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "r", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || len(found.Symbols) != 1 || found.Symbols[0].ID != ids[first] {
				t.Fatalf("R original find %v %+v %+v", e, fail, found.Symbols)
			}
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "r", Encoding: "utf-8", SymbolID: ids[first], SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 18000})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("R fingerprint show %v %+v %+v", e, fail, shown.Show)
			}
			fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "r", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 120})
			if e != nil || fail != nil || q.Search == nil || len(q.Search.Matches) != 1 || q.Search.Matches[0].SymbolID != ids[first] {
				t.Fatalf("R structural search %v %+v %+v", e, fail, q)
			}
			for _, negative := range []string{"function", "return", "local", "if", "length", "ghostSymbol"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "r", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("R reference/keyword invented as function %s", negative)
				}
			}
		})
	}
}
