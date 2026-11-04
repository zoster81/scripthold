package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These project questions and expected source locations come from independent
// reads of original QBasic sources. Third-party project code is never executed.
// The original QBasic function is spelled getfileinfo&. An exact lookup of
// that spelling must find the same two declarations as the normalized name,
// but must not accept the different return-type suffix getfileinfo%.
func TestSourceSymbolsOriginalQBasicTypeSuffixLookup(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_QBASIC_NAV_GIF_ROOT")
	if root == "" {
		t.Skip("original GIF decoder project not configured")
	}
	path := filepath.Join(root, "GIF.BAS")
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		query string
		want  int
	}{
		{"getfileinfo", 2},
		{"getfileinfo&", 2},
		{"getfileinfo%", 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "qbasic",
				Encoding: "utf-8", Query: tc.query, Match: "exact", MaxFiles: 1, MaxSymbols: 8,
			})
			if err != nil || failed != nil || !out.CoverageComplete || len(out.Symbols) != tc.want {
				t.Fatalf("find %q: want=%d got=%d err=%v tool=%+v", tc.query, tc.want, len(out.Symbols), err, failed)
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: []string{path}, Language: "qbasic",
				Encoding: "utf-8", Query: tc.query, Match: "exact",
				Mode: "structural", MaxResults: 8,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != tc.want {
				t.Fatalf("search %q: want=%d err=%v tool=%+v search=%+v", tc.query, tc.want, err, failed, search.Search)
			}
		})
	}
}

func TestSourceSymbolsOriginalQBasicTaskNavigation(t *testing.T) {
	type target struct {
		name, qualified, kind string
		line                  int
		excerpt               string
	}
	check := func(t *testing.T, h *Handler, path string, e target) {
		t.Helper()
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "qbasic", Encoding: "utf-8",
			Query: e.name, Match: "exact", MaxFiles: 1, MaxSymbols: 12,
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated {
			t.Fatalf("find %s failed: err=%v tool=%+v output=%+v", e.name, err, failed, out)
		}
		var selected string
		for _, symbol := range out.Symbols {
			if symbol.Name == e.name && symbol.QualifiedName == e.qualified &&
				string(symbol.Kind) == e.kind && symbol.DeclarationRange.Start.Line == e.line &&
				symbol.NameRange.Start.Line == e.line {
				selected = symbol.ID
				break
			}
		}
		if selected == "" {
			t.Fatalf("missing %s qualified=%s line=%d; candidates=%+v", e.name, e.qualified, e.line, out.Symbols)
		}
		if e.excerpt == "" {
			return
		}
		if len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
			t.Fatalf("missing fingerprint for %s", e.name)
		}
		failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: path, SymbolID: selected,
			SourceFingerprint: out.Files[0].SourceFingerprint,
			Language:          "qbasic", Encoding: "utf-8", MaxBytes: 8192,
		})
		if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, e.excerpt) {
			t.Fatalf("fingerprint-bound source for %s unavailable: err=%v tool=%+v show=%+v",
				e.name, err, failed, shown.Show)
		}
	}
	digest := func(t *testing.T, h *Handler, path string) {
		t.Helper()
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("original source missing/not regular: %v", err)
		}
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: []string{path}, Language: "qbasic",
			Encoding: "utf-8", Presentation: "compact", MaxFiles: 1,
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || out.FilesParsed != 1 {
			t.Fatalf("compact digest failed: err=%v tool=%+v out=%+v", err, failed, out)
		}
		data, err := json.Marshal(out)
		if err != nil || len(data) > 8192 {
			t.Fatalf("compact digest too large: bytes=%d err=%v", len(data), err)
		}
		t.Logf("one source: symbols=%d digestBytes=%d covered=%v", out.SymbolCount, len(data), out.CoverageComplete)
	}
	t.Run("gif-header-validation-versus-palette-and-lzw", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_QBASIC_NAV_GIF_ROOT")
		if root == "" {
			t.Skip("original GIF decoder project not configured")
		}
		path := filepath.Join(root, "GIF.BAS")
		h := NewHandler([]string{root})
		digest(t, h, path)
		for _, e := range []target{
			{"getfileinfo", "getfileinfo", "function", 3, ""},
			{"getfileinfo", "getfileinfo", "function", 48, `"Invalid LZW bit depth."`},
			{"lzwdecode", "lzwdecode", "function", 100, "dictrefs(4095)"},
			{"setpalette", "setpalette", "function", 228, "PALETTE i"},
			{"dictrefs", "lzwdecode.dictrefs", "variable", 105, ""},
			{"file", "file", "constant", 9, ""},
		} {
			check(t, h, path, e)
		}
		content, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(content), "IF LEFT$(headlsd$, 3) <> \"GIF\"") ||
			!strings.Contains(string(content), "CALL setpalette(palbits)") ||
			!strings.Contains(string(content), "CALL lzwdecode(") {
			t.Fatalf("original GIF header and call-site oracle changed: %v", err)
		}
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Language: "qbasic",
			Encoding: "utf-8", Query: "getfileinfo", Match: "exact",
			Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) != 2 {
			t.Fatalf("GIF declaration/definition pair missing: err=%v tool=%+v out=%+v", err, failed, out)
		}
	})
	t.Run("starman-viewport-camera-and-map-bounds", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_QBASIC_NAV_STARMAN_ROOT")
		if root == "" {
			t.Skip("original STARMAN project not configured")
		}
		path := filepath.Join(root, "STARMAN", "STARMAN.BAS")
		h := NewHandler([]string{root})
		digest(t, h, path)
		for _, e := range []target{
			{"CAMWIDTH", "CAMWIDTH", "constant", 27, ""},
			{"CAMHEIGHT", "CAMHEIGHT", "constant", 28, ""},
			{"mapwidth", "mapwidth", "variable", 66, ""},
			{"mapheight", "mapheight", "variable", 66, ""},
			{"numscripts", "numscripts", "variable", 66, ""},
			{"LoadMap", "LoadMap", "function", 124, "GET file,,mapheight"},
			{"CameraFollow", "CameraFollow", "function", 208, "IF camera.y > mapheight - CAMHEIGHT"},
			{"DrawScreen", "DrawScreen", "function", 223, "FOR rely = 0 TO CAMHEIGHT - 1"},
			{"DrawPlayer", "DrawPlayer", "function", 236, "player.x - camera.x"},
			{"MovePlayer", "MovePlayer", "function", 242, "MASKWALK"},
		} {
			check(t, h, path, e)
		}
		content, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(content), "CALL CameraFollow") ||
			!strings.Contains(string(content), "CALL DrawScreen") ||
			!strings.Contains(string(content), "CALL DrawPlayer") ||
			!strings.Contains(string(content), "DIM SHARED mapwidth, mapheight, numscripts AS INTEGER") {
			t.Fatalf("STARMAN source call-site and declaration oracle changed: %v", err)
		}
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: []string{path}, Language: "qbasic",
			Encoding: "utf-8", Query: "mapheight", Match: "exact",
			Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) != 1 ||
			out.Search.Matches[0].QualifiedName != "mapheight" {
			t.Fatalf("map height declaration missing: err=%v tool=%+v out=%+v", err, failed, out)
		}
	})
}
