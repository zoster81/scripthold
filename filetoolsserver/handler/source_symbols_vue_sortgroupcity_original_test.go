package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Independently verify source-linked navigation of Vue 2's city-sorting
// computed property, not merely a matching expression in a template.
func TestVueOriginalSortgroupcityHandlerNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_SORTGROUPCITY_ROOT")
	if root == "" {
		t.Skip("independent original Vue 2 city-list files not configured")
	}
	h, ctx := NewHandler([]string{root}), context.Background()
	for _, tc := range []struct {
		name, filename, declaration, body string
		line                              int
	}{
		{"vue2-elm", "vue2-elm-home.vue", "sortgroupcity(){", "return sortobj", 81},
		{"vue-weather", "vue-weather-citylist.vue", "sortgroupcity() {", "return sortobj", 81},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, tc.filename)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("independent original file unavailable: %v %v", info, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			if tc.line < 1 || tc.line > len(lines) ||
				strings.TrimSpace(lines[tc.line-1]) != tc.declaration ||
				!strings.Contains(string(data), "in sortgroupcity") ||
				!strings.Contains(string(data), tc.body) {
				t.Fatalf("original source oracle changed for %s", tc.name)
			}
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "vue", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || len(digest.Digests) != 1 ||
				digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original digest failure: %v %+v", err, digest)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "vue",
				Query: "sortgroupcity", Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("original find failure: %v %+v", err, found)
			}
			id, count := "", 0
			for _, sym := range found.Symbols {
				if sym.Name != "sortgroupcity" {
					continue
				}
				count++
				if sym.Kind == "property" && sym.NativeKind == "vue-options-computed" &&
					sym.NameRange.Start.Line == tc.line && sym.RegionID != "" {
					id = sym.ID
				}
			}
			if count != 1 || id == "" ||
				found.Files[0].SourceFingerprint != digest.Digests[0].SourceFingerprint ||
				found.Files[0].Encoding == "" {
				t.Fatalf("computed source identity invalid, count=%d symbols=%+v files=%+v", count, found.Symbols, found.Files)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vue",
				Encoding: found.Files[0].Encoding, SymbolID: id,
				SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.declaration) ||
				!strings.Contains(shown.Show.Text, tc.body) {
				t.Fatalf("fingerprint-bound show failed: %v %+v", err, shown.Show)
			}
		})
	}
}
