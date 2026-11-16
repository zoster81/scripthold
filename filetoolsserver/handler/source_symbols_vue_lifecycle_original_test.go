package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original Vue 2 lifecycle callbacks must be independently findable and
// fingerprint-bound, rather than appearing only as unindexed source text.
func TestVueOriginalLifecycleHandlerNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_SORTGROUPCITY_ROOT")
	if root == "" {
		t.Skip("independent original Vue 2 files not configured")
	}
	ctx, h := context.Background(), NewHandler([]string{root})
	for _, tc := range []struct {
		filename, hook, declaration, body string
		line                              int
	}{
		{"vue2-elm-home.vue", "mounted", "mounted(){", "cityGuess().then(res => {", 57},
		{"vue-weather-citylist.vue", "created", "created() {", "this.$root.eventHub.$on(", 143},
	} {
		t.Run(tc.hook, func(t *testing.T) {
			path := filepath.Join(root, tc.filename)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Vue file unavailable: %v %v", info, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(b), "\n")
			if tc.line < 1 || tc.line > len(lines) || strings.TrimSpace(lines[tc.line-1]) != tc.declaration ||
				!strings.Contains(string(b), tc.body) {
				t.Fatal("original lifecycle source oracle changed")
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "vue",
				Query: tc.hook, Match: "exact", MaxFiles: 1, MaxSymbols: 128, FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("lifecycle find failed: %v %+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tc.hook && sym.Kind == "method" &&
					sym.NativeKind == "vue-options-lifecycle" && sym.RegionID != "" &&
					sym.NameRange.Start.Line == tc.line {
					id = sym.ID
				}
			}
			if id == "" || found.Files[0].SourceFingerprint == "" || found.Files[0].Encoding == "" {
				t.Fatalf("original lifecycle declaration absent: %+v %+v", found.Symbols, found.Files)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vue",
				Encoding: found.Files[0].Encoding, SymbolID: id,
				SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.declaration) ||
				!strings.Contains(shown.Show.Text, tc.body) {
				t.Fatalf("fingerprint-bound lifecycle show failed: %v %+v", err, shown.Show)
			}
		})
	}
}
