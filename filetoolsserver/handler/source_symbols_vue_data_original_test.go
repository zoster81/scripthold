package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Vue Options data() initializer is an authored method, not the template
// reference or an inferred declaration for each returned state field.
func TestVueOriginalDataHandlerNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_SORTGROUPCITY_ROOT")
	if root == "" {
		t.Skip("independent original Vue 2 source not configured")
	}
	h, ctx := NewHandler([]string{root}), context.Background()
	for _, tc := range []struct {
		filename, header, body string
		line                   int
	}{
		{"vue2-elm-home.vue", "data(){", "guessCity: ''", 48},
		{"vue-weather-citylist.vue", "data () {", "filterKey: ''", 61},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			path := filepath.Join(root, tc.filename)
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("independent original unavailable: %v %v", info, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(b), "\n")
			if tc.line < 1 || tc.line > len(lines) ||
				strings.TrimSpace(lines[tc.line-1]) != tc.header ||
				!strings.Contains(string(b), tc.body) {
				t.Fatal("original Vue data() source oracle changed")
			}
			failure, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "vue",
				Query: "data", Match: "exact", MaxFiles: 1, MaxSymbols: 128, FileDetails: "relevant",
			})
			if err != nil || failure != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Vue data() find failed: %v %+v %+v", err, failure, found)
			}
			count, id := 0, ""
			for _, sym := range found.Symbols {
				if sym.Name != "data" {
					continue
				}
				count++
				if sym.Kind == "method" && sym.NativeKind == "vue-options-data" &&
					sym.RegionID != "" && sym.NameRange.Start.Line == tc.line &&
					sym.BodyRange != nil {
					id = sym.ID
				}
			}
			if count != 1 || id == "" || found.Files[0].Encoding == "" || found.Files[0].SourceFingerprint == "" {
				t.Fatalf("Vue data() identity missing or ambiguous: count=%d symbols=%+v", count, found.Symbols)
			}
			failure, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "vue", Encoding: found.Files[0].Encoding,
				SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || failure != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.header) ||
				!strings.Contains(shown.Show.Text, tc.body) {
				t.Fatalf("Vue data() fingerprint-bound show failed: %v %+v %+v", err, failure, shown.Show)
			}
		})
	}
}
