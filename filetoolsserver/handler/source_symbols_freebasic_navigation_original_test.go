package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These task-navigation expectations were independently checked against the original
// FreeBASIC sources. Source Intelligence reports facts; it does not select changes.
func TestSourceSymbolsOriginalFreeBasicProjectNavigation(t *testing.T) {
	check := func(t *testing.T, h *Handler, root, rel, name, qualified, kind string, line int, excerpt string) {
		t.Helper()
		path := filepath.Join(root, filepath.FromSlash(rel))
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{path}, Language: "freebasic", Encoding: "utf-8",
			Query: name, Match: "exact", MaxFiles: 1, MaxSymbols: 16,
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated {
			t.Fatalf("%s search failure: err=%v tool=%+v out=%+v", name, err, failed, out)
		}
		var symbolID string
		for _, sym := range out.Symbols {
			if sym.Name == name && sym.QualifiedName == qualified &&
				string(sym.Kind) == kind && sym.DeclarationRange.Start.Line == line &&
				sym.NameRange.Start.Line == line {
				symbolID = sym.ID
				break
			}
		}
		if symbolID == "" {
			t.Fatalf("missing %s (%s) kind=%s line=%d: %+v", name, qualified, kind, line, out.Symbols)
		}
		if excerpt == "" {
			return
		}
		if len(out.Files) != 1 || out.Files[0].SourceFingerprint == "" {
			t.Fatalf("source fingerprint missing for %s: %+v", name, out.Files)
		}
		failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "show", Path: path, SymbolID: symbolID,
			SourceFingerprint: out.Files[0].SourceFingerprint,
			Language:          "freebasic", Encoding: "utf-8", MaxBytes: 8192,
		})
		if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, excerpt) {
			t.Fatalf("source body or signature missing for %s: err=%v tool=%+v out=%+v", name, err, failed, shown.Show)
		}
	}
	digest := func(t *testing.T, h *Handler, root string, rels ...string) []string {
		t.Helper()
		paths := make([]string, 0, len(rels))
		for _, rel := range rels {
			path := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("original project file is missing/not regular: %s err=%v", rel, err)
			}
			paths = append(paths, path)
		}
		failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: paths, Language: "freebasic", Encoding: "utf-8",
			Presentation: "compact", MaxFiles: len(paths),
		})
		if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || out.FilesParsed != len(paths) {
			t.Fatalf("project digest error: err=%v tool=%+v out=%+v", err, failed, out)
		}
		data, err := json.Marshal(out)
		if err != nil || len(data) > 8192 {
			t.Fatalf("project digest exceeds 8 KiB (%d bytes): %v", len(data), err)
		}
		return paths
	}
	t.Run("fbjson-null-value-and-declaration", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_FREEBASIC_NAV_JSON_ROOT")
		if root == "" {
			t.Skip("original FreeBASIC JSON project not configured")
		}
		h := NewHandler([]string{root})
		paths := digest(t, h, root, "fbJson/JsonItem.bas", "fbJson/JsonItem.bi", "examples/test.bas")
		check(t, h, root, "fbJson/JsonItem.bas", "Value", "JsonItem.Value", "property", 73, "case \"null\"")
		check(t, h, root, "fbJson/JsonItem.bas", "AddItem", "JsonItem.AddItem", "function", 132, "this.AppendChild")
		check(t, h, root, "fbJson/JsonItem.bi", "Value", "JsonItem.Value", "property", 22, "declare property Value")
		check(t, h, root, "examples/test.bas", "AssertEqual", "AssertEqual", "function", 7, "")
		check(t, h, root, "fbJson/JsonItem.bi", "ToString", "JsonItem.ToString", "function", 33, "")
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "freebasic", Encoding: "utf-8",
			Query: "JsonItem.Value", Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) < 3 {
			t.Fatalf("missing related property declarations/implementations: err=%v tool=%+v out=%+v", err, failed, out)
		}
		data, err := os.ReadFile(paths[2])
		if err != nil || !strings.Contains(string(data), "item = jsonItem(\"null\")") ||
			!strings.Contains(string(data), "assertEqual(jsonNull, item.Datatype)") {
			t.Fatal("independent original project test oracle changed or missing")
		}
	})
	t.Run("raylib-fps-example-and-binding", func(t *testing.T) {
		root := os.Getenv("SCRIPTHOLD_FREEBASIC_NAV_RAYLIB_ROOT")
		if root == "" {
			t.Skip("original FreeBASIC raylib project not configured")
		}
		h := NewHandler([]string{root})
		paths := digest(t, h, root, "raylib.bi", "examples/core/core_basic_window.bas", "examples/core/core_3d_camera_first_person.bas")
		check(t, h, root, "raylib.bi", "SetTargetFPS", "SetTargetFPS", "function", 952, "declare sub SetTargetFPS")
		check(t, h, root, "raylib.bi", "GetFPS", "GetFPS", "function", 953, "")
		check(t, h, root, "raylib.bi", "DrawText", "DrawText", "function", 1225, "")
		check(t, h, root, "examples/core/core_basic_window.bas", "screen_width", "screen_width", "variable", 3, "")
		check(t, h, root, "examples/core/core_3d_camera_first_person.bas", "cam", "cam", "variable", 10, "")
		// Source Symbols indexes declarations, not arbitrary call sites. Real uses
		// are established independently from the original files instead of guessed.
		for _, path := range paths[1:] {
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), "SetTargetFPS(60)") ||
				!strings.Contains(string(data), "#include \"../../raylib.bi\"") {
				t.Fatalf("original example binding/use oracle changed: %s err=%v", path, err)
			}
		}
		failed, out, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
			Operation: "search", Paths: paths, Language: "freebasic", Encoding: "utf-8",
			Query: "SetTargetFPS", Match: "exact", Mode: "structural", MaxResults: 8,
		})
		if err != nil || failed != nil || out.Search == nil || len(out.Search.Matches) != 1 ||
			out.Search.Matches[0].QualifiedName != "SetTargetFPS" {
			t.Fatalf("binding declaration missing: err=%v tool=%+v out=%+v", err, failed, out)
		}
	})
}
