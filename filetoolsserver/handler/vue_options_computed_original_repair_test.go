package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Check original Vue Options API computed getters rather than identifying the
// identically named script-setup variables as an adequate replacement.
func TestVueOptionsComputedOriginalSourceNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_VUE_OPTIONS_VUETIFY_ROOT")
	if root == "" {
		t.Skip("immutable original Vuetify Options API source not configured")
	}

	type target struct {
		file, name, excerpt string
		line                int
	}
	targets := []target{
		{"packages/docs/src/examples/transitions/misc-todo.vue", "completedTasks", "completedTasks () {", 138},
		{"packages/docs/src/examples/transitions/misc-todo.vue", "progress", "progress () {", 141},
		{"packages/docs/src/examples/transitions/misc-todo.vue", "remainingTasks", "remainingTasks () {", 144},
		{"packages/docs/src/examples/grid/misc-row-and-column-breakpoints.vue", "cols", "cols () {", 35},
	}
	neighbors := []target{
		{"packages/docs/src/examples/transitions/misc-todo.vue", "create", "create () {", 150},
		{"packages/docs/src/examples/v-autocomplete/prop-filter.vue", "customFilter", "customFilter (itemTitle, queryText, item) {", 110},
		{"packages/docs/src/examples/v-autocomplete/prop-filter.vue", "save", "save () {", 118},
	}
	sources := make(map[string]string)
	for _, tg := range append(append([]target{}, targets...), neighbors...) {
		if _, seen := sources[tg.file]; !seen {
			path := filepath.Join(root, filepath.FromSlash(tg.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
				t.Fatalf("original Vue source not a bounded regular file %s: info=%v err=%v", tg.file, info, err)
			}
			bytes, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sources[tg.file] = string(bytes)
		}
		lines := strings.Split(sources[tg.file], "\n")
		if tg.line < 1 || tg.line > len(lines) ||
			!strings.Contains(lines[tg.line-1], tg.excerpt) {
			t.Fatalf("independent original source oracle changed %s:%d %s", tg.file, tg.line, tg.name)
		}
	}

	// Template, Options and setup sources are independently read before the
	// handler: the task is to change a computed getter, not its setup counterpart.
	if !strings.Contains(sources[targets[0].file], "{{ remainingTasks }}") ||
		!strings.Contains(sources[targets[0].file], "const remainingTasks = computed(") ||
		!strings.Contains(sources[targets[3].file], "const cols = computed(") {
		t.Fatal("independently reviewed Vue template/setup/Options maintenance task changed")
	}

	ctx, h := context.Background(), NewHandler([]string{root})
	for _, tg := range targets {
		t.Run(tg.name, func(t *testing.T) {
			full := filepath.Join(root, filepath.FromSlash(tg.file))
			_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{full}, Language: "vue", MaxFiles: 1,
			})
			if err != nil || digest.Truncated || digest.FilesParsed != 1 ||
				len(digest.Digests) != 1 || digest.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Vue digest unavailable: err=%v digest=%+v", err, digest)
			}
			_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{full},
				Language: "vue", MaxFiles: 1, MaxSymbols: 256,
			})
			if err != nil || outline.Truncated || outline.FilesParsed != 1 || len(outline.Files) != 1 {
				t.Fatalf("original Vue outline unavailable: err=%v outline=%+v", err, outline)
			}
			_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{full}, Language: "vue",
				Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
				FileDetails: "relevant",
			})
			if err != nil || found.Truncated || len(found.Files) != 1 {
				t.Fatalf("Vue original computed property find failed: err=%v found=%+v", err, found)
			}
			id := ""
			for _, sym := range found.Symbols {
				if sym.Name == tg.name && sym.Kind == "property" &&
					sym.NativeKind == "vue-options-computed" &&
					sym.NameRange.Start.Line == tg.line && sym.RegionID != "" {
					id = sym.ID
				}
			}
			inOutline := false
			for _, sym := range outline.Symbols {
				if sym.ID == id {
					inOutline = true
				}
			}
			if id == "" || !inOutline {
				t.Fatalf("original Vue computed getter not navigable at %s:%d: %+v", tg.file, tg.line, found.Symbols)
			}
			file := found.Files[0]
			if file.Encoding == "" || file.SourceFingerprint != digest.Digests[0].SourceFingerprint {
				t.Fatalf("Vue original fingerprint/encoding mismatch: %+v", file)
			}
			_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "show", Path: full, Language: "vue", Encoding: file.Encoding,
				SymbolID: id, SourceFingerprint: file.SourceFingerprint, MaxBytes: 8192,
			})
			if err != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, tg.excerpt) {
				t.Fatalf("Vue original computed source excerpt missing: err=%v shown=%+v", err, shown.Show)
			}
			_, search, err := h.SourceQuery(ctx, nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{full},
				Language: "vue", Query: tg.name, Match: "exact",
				MaxFiles: 1, MaxResults: 128,
			})
			linked := false
			if search.Search != nil {
				for _, hit := range search.Search.Matches {
					if hit.SymbolID == id && hit.SourceFingerprint == file.SourceFingerprint &&
						hit.Range.Start.Line == tg.line {
						linked = true
					}
				}
			}
			if err != nil || search.Search == nil || search.Coverage.ResultsTruncated || !linked {
				t.Fatalf("Vue original computed search not source-linked: err=%v search=%+v", err, search)
			}
		})
	}

	// Existing Options API methods remain available, and unrelated examples
	// must not fabricate a getter named for a different component.
	for _, tg := range neighbors {
		full := filepath.Join(root, filepath.FromSlash(tg.file))
		_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
			Operation: "find", Paths: []string{full}, Language: "vue",
			Query: tg.name, Match: "exact", MaxFiles: 1, MaxSymbols: 128,
		})
		ok := false
		for _, sym := range found.Symbols {
			if sym.Name == tg.name && sym.NameRange.Start.Line == tg.line {
				ok = true
			}
		}
		if err != nil || !ok {
			t.Fatalf("existing Options API method changed %s:%d: err=%v found=%+v", tg.file, tg.line, err, found.Symbols)
		}
	}
	unrelated := filepath.Join(root, filepath.FromSlash(neighbors[1].file))
	_, negatives, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{unrelated}, Language: "vue",
		Query: "completedTasks", Match: "exact", MaxFiles: 1, MaxSymbols: 32,
	})
	if err != nil || negatives.Truncated || len(negatives.Symbols) != 0 {
		t.Fatalf("unrelated Vue Options source fabricated computed getter: err=%v symbols=%+v", err, negatives.Symbols)
	}
}
