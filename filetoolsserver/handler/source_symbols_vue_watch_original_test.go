package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Verify that direct Vue Options API watch callbacks remain navigable as
// source declarations through the local MCP handler, not an installed runtime.
func TestVueOriginalWatchHandlerNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_WATCH_ROOT")
	if root == "" {
		t.Skip("independent Vue watch originals not configured")
	}
	h, ctx := NewHandler([]string{root}), context.Background()
	for _, tc := range []struct {
		file, name, anchor, body string
		line                     int
	}{
		{"captcha-App.vue", "show", "    show(newVal) {", "this.$root.captcha.destroy();", 33},
		{"color-picker.vue", "value", "            value(newVal, oldVal) {", "this.setColor(newVal);", 93},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original file unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			if tc.line < 1 || tc.line > len(lines) || lines[tc.line-1] != tc.anchor || !strings.Contains(string(data), tc.body) {
				t.Fatal("independent original source oracle changed")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "vue", Encoding: "utf-8",
				Query: tc.name, Match: "exact", MaxFiles: 1, MaxSymbols: 64,
			})
			if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("MCP find failed: err=%v error=%+v found=%+v", err, failed, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.name && sym.Kind == "method" && sym.NativeKind == "vue-options-watch" &&
					sym.NameRange.Start.Line == tc.line && sym.RegionID != "" {
					id = sym.ID
				}
			}
			if id == "" {
				t.Fatalf("missing original watch callback: %+v", found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vue", Encoding: "utf-8",
				SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tc.body) {
				t.Fatalf("fingerprint-bound show failed: err=%v error=%+v shown=%+v", err, failed, shown.Show)
			}
		})
	}
}
