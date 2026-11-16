package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A protected/task body is not a new package-owned type; supported siblings
// remain navigable after the unsupported scope closes.
func TestAdaOriginalOpaqueBodyHandlerNavigation(t *testing.T) {
	cases := []struct {
		name, env, declaration, wanted string
		line, forbidden                int
		kind                           string
	}{
		{"ctags-task", "SCRIPTHOLD_REAL_ADA_CTAGS_BODY",
			"package body Input_1 is", "Input_1", 2, 4, "package"},
		{"ada-gui-protected", "SCRIPTHOLD_REAL_ADA_GUI_BODY",
			"procedure Activate_Exception_Handler", "Activate_Exception_Handler", 422, 404, "function"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Ada file not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Ada file missing/unbounded: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(body), "\n")
			if tc.line < 1 || tc.line > len(lines) ||
				!strings.Contains(lines[tc.line-1], tc.declaration) ||
				!strings.Contains(string(body), "protected body") {
				t.Fatalf("original Ada oracle drift: %s", tc.name)
			}
			ctx, h := context.Background(), NewHandler([]string{filepath.Dir(path)})
			failure, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "ada",
				Query: tc.wanted, Match: "exact", MaxFiles: 1, MaxSymbols: 100, FileDetails: "relevant",
			})
			if err != nil || failure != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Ada find failed %v %+v %+v", err, failure, found)
			}
			selected := ""
			for _, sym := range found.Symbols {
				if sym.Name != tc.wanted {
					continue
				}
				if sym.NameRange.Start.Line == tc.forbidden && sym.Kind == "type" {
					t.Errorf("false Ada body type returned: %+v", sym)
				}
				if sym.NameRange.Start.Line == tc.line && string(sym.Kind) == tc.kind {
					selected = sym.ID
				}
			}
			if selected == "" || found.Files[0].SourceFingerprint == "" || found.Files[0].Encoding == "" {
				t.Fatalf("source-owned Ada declaration missing: %+v", found.Symbols)
			}
			failure, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "ada", Encoding: found.Files[0].Encoding,
				SymbolID: selected, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 4096,
			})
			if err != nil || failure != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.declaration) {
				t.Fatalf("fingerprint-bound Ada show failed: %v %+v %+v", err, failure, shown.Show)
			}
		})
	}
}
