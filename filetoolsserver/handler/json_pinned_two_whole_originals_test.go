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

// Every supported JSON object key from two complete pinned original files was
// enumerated independently; array elements and keys inside array objects are
// intentionally outside this contract.
func TestJSONTwoWholePinnedOriginalKeyInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_JSON_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete pinned JSON originals not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		records    string
	}{{file: "sindresorhus-package.json", size: 1635, blob: "d7a386f1d2e7d3b4a02634e83e81feb502803750", records: "name|name|key|key||2|3|7|2|2|28\nversion|version|key|key||3|3|10|2|3|20\ndescription|description|key|key||4|3|14|2|4|36\nlicense|license|key|key||5|3|10|2|5|18\nrepository|repository|key|key||6|3|13|2|6|33\nfunding|funding|key|key||7|3|10|2|7|59\nauthor|author|key|key||8|3|9|2|12|3\nauthor.name|name|key|key|author|9|4|8|3|9|26\nauthor.email|email|key|key|author|10|4|9|3|10|36\nauthor.url|url|key|key|author|11|4|7|3|11|36\ntype|type|key|key||13|3|7|2|13|18\nexports|exports|key|key||14|3|10|2|17|3\nexports.types|types|key|key|exports|15|4|9|3|15|39\nexports.default|default|key|key|exports|16|4|11|3|16|39\nsideEffects|sideEffects|key|key||18|3|14|2|18|22\nengines|engines|key|key||19|3|10|2|21|3\nengines.node|node|key|key|engines|20|4|8|3|20|17\nscripts|scripts|key|key||22|3|10|2|26|3\nscripts.build|build|key|key|scripts|23|4|9|3|23|37\nscripts.test|test|key|key|scripts|24|4|8|3|24|135\nscripts.prepare|prepare|key|key|scripts|25|4|11|3|25|29\nfiles|files|key|key||27|3|8|2|29|3\nkeywords|keywords|key|key||30|3|11|2|53|3\nxo|xo|key|key||54|3|5|2|61|3\nxo.rules|rules|key|key|xo|55|4|9|3|60|4\nxo.rules.@typescript-eslint/no-unsafe-enum-comparison|@typescript-eslint/no-unsafe-enum-comparison|key|key|xo.rules|56|5|49|4|56|57\nxo.rules.@typescript-eslint/no-confusing-void-expression|@typescript-eslint/no-confusing-void-expression|key|key|xo.rules|57|5|52|4|57|60\nxo.rules.@typescript-eslint/no-unsafe-type-assertion|@typescript-eslint/no-unsafe-type-assertion|key|key|xo.rules|58|5|48|4|58|56\nxo.rules.@stylistic/operator-linebreak|@stylistic/operator-linebreak|key|key|xo.rules|59|5|34|4|59|42\ndevDependencies|devDependencies|key|key||62|3|18|2|75|3\ndevDependencies.@sindresorhus/tsconfig|@sindresorhus/tsconfig|key|key|devDependencies|63|4|26|3|63|37\ndevDependencies.@types/jsdom|@types/jsdom|key|key|devDependencies|64|4|16|3|64|28\ndevDependencies.@types/node|@types/node|key|key|devDependencies|65|4|15|3|65|27\ndevDependencies.@types/zen-observable|@types/zen-observable|key|key|devDependencies|66|4|25|3|66|36\ndevDependencies.del-cli|del-cli|key|key|devDependencies|67|4|11|3|67|22\ndevDependencies.expect-type|expect-type|key|key|devDependencies|68|4|15|3|68|26\ndevDependencies.jsdom|jsdom|key|key|devDependencies|69|4|9|3|69|21\ndevDependencies.rxjs|rxjs|key|key|devDependencies|70|4|8|3|70|19\ndevDependencies.tempy|tempy|key|key|devDependencies|71|4|9|3|71|20\ndevDependencies.typescript|typescript|key|key|devDependencies|72|4|14|3|72|24\ndevDependencies.xo|xo|key|key|devDependencies|73|4|6|3|73|17\ndevDependencies.zen-observable|zen-observable|key|key|devDependencies|74|4|18|3|74|30"},
		{file: "chalk-package.json", size: 1753, blob: "0ad44b2c825a7fae6da866d2572846dbee2d01ef", records: "name|name|key|key||2|3|7|2|2|17\nversion|version|key|key||3|3|10|2|3|20\ndescription|description|key|key||4|3|14|2|4|53\nlicense|license|key|key||5|3|10|2|5|18\nrepository|repository|key|key||6|3|13|2|6|29\nfunding|funding|key|key||7|3|10|2|7|55\ntype|type|key|key||8|3|7|2|8|18\nexports|exports|key|key||9|3|10|2|12|3\nexports.types|types|key|key|exports|10|4|9|3|10|33\nexports.default|default|key|key|exports|11|4|11|3|11|33\nimports|imports|key|key||13|3|10|2|19|3\nimports.#ansi-styles|#ansi-styles|key|key|imports|14|4|16|3|14|57\nimports.#supports-color|#supports-color|key|key|imports|15|4|19|3|18|4\nimports.#supports-color.node|node|key|key|imports.#supports-color|16|5|9|4|16|53\nimports.#supports-color.default|default|key|key|imports.#supports-color|17|5|12|4|17|58\nsideEffects|sideEffects|key|key||20|3|14|2|20|22\nengines|engines|key|key||21|3|10|2|23|3\nengines.node|node|key|key|engines|22|4|8|3|22|17\nscripts|scripts|key|key||24|3|10|2|27|3\nscripts.test|test|key|key|scripts|25|4|8|3|25|96\nscripts.bench|bench|key|key|scripts|26|4|9|3|26|60\nfiles|files|key|key||28|3|8|2|30|3\nkeywords|keywords|key|key||31|3|11|2|52|3\ndevDependencies|devDependencies|key|key||53|3|18|2|63|3\ndevDependencies.@types/node|@types/node|key|key|devDependencies|54|4|15|3|54|27\ndevDependencies.ansi-styles|ansi-styles|key|key|devDependencies|55|4|15|3|55|26\ndevDependencies.c8|c8|key|key|devDependencies|56|4|6|3|56|18\ndevDependencies.color-convert|color-convert|key|key|devDependencies|57|4|17|3|57|28\ndevDependencies.execa|execa|key|key|devDependencies|58|4|9|3|58|21\ndevDependencies.log-update|log-update|key|key|devDependencies|59|4|14|3|59|25\ndevDependencies.typescript|typescript|key|key|devDependencies|60|4|14|3|60|25\ndevDependencies.xo|xo|key|key|devDependencies|61|4|6|3|61|17\ndevDependencies.yoctodelay|yoctodelay|key|key|devDependencies|62|4|14|3|62|25\nxo|xo|key|key||64|3|5|2|80|3\nc8|c8|key|key||81|3|5|2|89|3\nc8.reporter|reporter|key|key|c8|82|4|12|3|85|4\nc8.exclude|exclude|key|key|c8|86|4|11|3|88|4"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("original JSON file changed: %+v %v", st, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha1.New()
			_, _ = fmt.Fprintf(sum, "blob %d%c", len(b), 0)
			_, _ = sum.Write(b)
			if hex.EncodeToString(sum.Sum(nil)) != tc.blob {
				t.Fatal("original raw Git blob mismatch")
			}
			rows := strings.Split(tc.records, "\n")
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "json", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 180})
			if err != nil || failed != nil || out.Truncated || out.CoverageComplete != (tc.file != "chalk-package.json") || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != len(rows) {
				t.Fatalf("complete whole-original JSON key inventory want=%d actual=%d parsed=%d files=%d coverage=%v truncated=%v status=%+v err=%v failed=%+v", len(rows), len(out.Symbols), out.FilesParsed, len(out.Files), out.CoverageComplete, out.Truncated, out.Files, err, failed)
			}
			byQualified := make(map[string]int, len(out.Symbols))
			ids := make(map[string]string, len(out.Symbols))
			for i, s := range out.Symbols {
				if _, dupe := byQualified[s.QualifiedName]; dupe {
					t.Errorf("duplicate original key %s", s.QualifiedName)
				}
				byQualified[s.QualifiedName] = i
				if s.ID == "" {
					t.Errorf("missing key ID %+v", s)
				}
				ids[s.QualifiedName] = s.ID
			}
			for _, entry := range rows {
				p := strings.Split(entry, "|")
				if len(p) != 11 {
					t.Fatalf("invalid frozen oracle row %q", entry)
				}
				ints := make([]int, 6)
				for j, k := range []int{5, 6, 7, 8, 9, 10} {
					n, e := strconv.Atoi(p[k])
					if e != nil {
						t.Fatal(e)
					}
					ints[j] = n
				}
				idx, ok := byQualified[p[0]]
				if !ok {
					t.Errorf("missing original JSON key %s", entry)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != p[1] || string(s.Kind) != p[2] || s.NativeKind != p[3] ||
					s.ParentQualifiedName != p[4] || s.QualifiedName != p[0] ||
					s.NameRange.Start.Line != ints[0] || s.NameRange.Start.Column != ints[1] ||
					s.NameRange.End.Line != ints[0] || s.NameRange.End.Column != ints[2] ||
					s.DeclarationRange.Start.Line != ints[0] || s.DeclarationRange.Start.Column != ints[3] ||
					s.DeclarationRange.End.Line != ints[4] || s.DeclarationRange.End.Column != ints[5] {
					t.Errorf("original full JSON key declaration mismatch want=%q got=%+v", entry, s)
				}
				if p[4] == "" && s.ParentID != "" {
					t.Errorf("unexpected root JSON key parent %+v", s)
				}
				if p[4] != "" && s.ParentID != ids[p[4]] {
					t.Errorf("broken JSON parent ID %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("whole original source fingerprint/byte count missing")
			}
			failed, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "json", Encoding: "utf-8", Query: "version", Match: "exact", MaxFiles: 1, MaxSymbols: 180})
			if err != nil || failed != nil || found.Truncated || len(found.Symbols) != 1 || found.Symbols[0].ID != ids["version"] {
				t.Fatalf("original version key find: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "json", Encoding: "utf-8", SymbolID: ids["version"], SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "version") {
				t.Fatalf("original fingerprint-bound JSON show: err=%v failed=%+v show=%+v", err, failed, shown.Show)
			}
			failed, query, err := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "json", Encoding: "utf-8", Query: "version", Match: "exact", MaxFiles: 1, MaxResults: 180})
			if err != nil || failed != nil || query.Search == nil || len(query.Search.Matches) != 1 || query.Search.Matches[0].SymbolID != ids["version"] {
				t.Fatalf("original structural JSON key search: err=%v failed=%+v query=%+v", err, failed, query)
			}
			failed, negative, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "json", Encoding: "utf-8", Query: "distribution", Match: "exact", MaxFiles: 1, MaxSymbols: 180})
			if err != nil || failed != nil || len(negative.Symbols) != 0 {
				t.Errorf("JSON array value must not become key: err=%v failed=%+v", err, failed)
			}
		})
	}
}
