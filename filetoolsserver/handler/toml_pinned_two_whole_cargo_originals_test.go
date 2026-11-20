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

// Fixed, independently source-authored declarations from complete Cargo TOML
// originals in two distinct projects, including table arrays and multi-line values.
func TestTOMLTwoWholePinnedCargoOriginalInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_TOML_OUTLINE_ROOT")
	if root == "" {
		t.Skip("complete pinned TOML sources not configured")
	}
	cases := []struct {
		file, blob string
		size       int64
		rows       string
	}{{file: "zoxide-Cargo.toml", size: 2499, blob: "bfcf4086408c4aeef2a9e1c8c4ba11063cbee852", rows: "package|package||section|section|1|2|9|1\u001eauthors|package.authors|package|key|key|2|1|8|2\u001ecategories|package.categories|package|key|key|3|1|11|3\u001edescription|package.description|package|key|key|4|1|12|4\u001eedition|package.edition|package|key|key|5|1|8|5\u001ehomepage|package.homepage|package|key|key|6|1|9|6\u001ekeywords|package.keywords|package|key|key|7|1|9|7\u001elicense|package.license|package|key|key|8|1|8|8\u001ename|package.name|package|key|key|9|1|5|9\u001ereadme|package.readme|package|key|key|10|1|7|10\u001erepository|package.repository|package|key|key|11|1|11|11\u001erust-version|package.rust-version|package|key|key|12|1|13|12\u001eversion|package.version|package|key|key|13|1|8|13\u001ebadges|badges||section|section|15|2|8|15\u001emaintenance|badges.maintenance|badges|key|key|16|1|12|16\u001edependencies|dependencies||section|section|18|2|14|18\u001eanyhow|dependencies.anyhow|dependencies|key|key|19|1|7|19\u001easkama|dependencies.askama|dependencies|key|key|20|1|7|20\u001ebincode|dependencies.bincode|dependencies|key|key|24|1|8|24\u001eclap|dependencies.clap|dependencies|key|key|25|1|5|25\u001ecolor-print|dependencies.color-print|dependencies|key|key|26|1|12|26\u001edirs|dependencies.dirs|dependencies|key|key|27|1|5|27\u001edunce|dependencies.dunce|dependencies|key|key|28|1|6|28\u001efastrand|dependencies.fastrand|dependencies|key|key|29|1|9|29\u001eglob|dependencies.glob|dependencies|key|key|30|1|5|30\u001eouroboros|dependencies.ouroboros|dependencies|key|key|31|1|10|31\u001eserde|dependencies.serde|dependencies|key|key|32|1|6|32\u001etime|dependencies.time|dependencies|key|key|33|1|5|33\u001etarget.'cfg(windows)'.dependencies|target.'cfg(windows)'.dependencies||section|section|35|2|36|35\u001ewhich|target.'cfg(windows)'.dependencies.which|target.'cfg(windows)'.dependencies|key|key|36|1|6|36\u001ebuild-dependencies|build-dependencies||section|section|38|2|20|38\u001eclap|build-dependencies.clap|build-dependencies|key|key|39|1|5|39\u001eclap_complete|build-dependencies.clap_complete|build-dependencies|key|key|40|1|14|40\u001eclap_complete_fig|build-dependencies.clap_complete_fig|build-dependencies|key|key|41|1|18|41\u001eclap_complete_nushell|build-dependencies.clap_complete_nushell|build-dependencies|key|key|42|1|22|42\u001ecolor-print|build-dependencies.color-print|build-dependencies|key|key|43|1|12|43\u001edev-dependencies|dev-dependencies||section|section|45|2|18|45\u001eassert_cmd|dev-dependencies.assert_cmd|dev-dependencies|key|key|46|1|11|46\u001erstest|dev-dependencies.rstest|dev-dependencies|key|key|47|1|7|47\u001erstest_reuse|dev-dependencies.rstest_reuse|dev-dependencies|key|key|48|1|13|48\u001etempfile|dev-dependencies.tempfile|dev-dependencies|key|key|49|1|9|49\u001efeatures|features||section|section|51|2|10|51\u001edefault|features.default|features|key|key|52|1|8|52\u001enix-dev|features.nix-dev|features|key|key|53|1|8|53\u001eprofile.release|profile.release||section|section|55|2|17|55\u001ecodegen-units|profile.release.codegen-units|profile.release|key|key|56|1|14|56\u001edebug|profile.release.debug|profile.release|key|key|57|1|6|57\u001elto|profile.release.lto|profile.release|key|key|58|1|4|58\u001estrip|profile.release.strip|profile.release|key|key|59|1|6|59\u001epackage.metadata.deb|package.metadata.deb||section|section|61|2|22|61\u001eassets|package.metadata.deb.assets|package.metadata.deb|key|key|62|1|7|62\u001eextended-description|package.metadata.deb.extended-description|package.metadata.deb|key|key|104|1|21|107\u001epriority|package.metadata.deb.priority|package.metadata.deb|key|key|108|1|9|108\u001esection|package.metadata.deb.section|package.metadata.deb|key|key|109|1|8|109"},
		{file: "ripgrep-Cargo.toml", size: 3544, blob: "9bf95826e625f3be5694a8881511707876851520", rows: "package|package||section|section|1|2|9|1\u001ename|package.name|package|key|key|2|1|5|2\u001eversion|package.version|package|key|key|3|1|8|3\u001eauthors|package.authors|package|key|key|4|1|8|4\u001edescription|package.description|package|key|key|5|1|12|9\u001edocumentation|package.documentation|package|key|key|10|1|14|10\u001ehomepage|package.homepage|package|key|key|11|1|9|11\u001erepository|package.repository|package|key|key|12|1|11|12\u001ekeywords|package.keywords|package|key|key|13|1|9|13\u001ecategories|package.categories|package|key|key|14|1|11|14\u001elicense|package.license|package|key|key|15|1|8|15\u001eexclude|package.exclude|package|key|key|16|1|8|16\u001ebuild|package.build|package|key|key|25|1|6|25\u001eautotests|package.autotests|package|key|key|26|1|10|26\u001eedition.workspace|package.edition.workspace|package|key|key|27|1|18|27\u001erust-version.workspace|package.rust-version.workspace|package|key|key|28|1|23|28\u001ebin|bin||section|array-table|30|3|6|30\u001ebench|bin.bench|bin|key|key|31|1|6|31\u001epath|bin.path|bin|key|key|32|1|5|32\u001ename|bin.name|bin|key|key|33|1|5|33\u001etest|test||section|array-table|35|3|7|35\u001ename|test.name|test|key|key|36|1|5|36\u001epath|test.path|test|key|key|37|1|5|37\u001eworkspace|workspace||section|section|39|2|11|39\u001emembers|workspace.members|workspace|key|key|40|1|8|40\u001epackage|workspace.package|workspace|section|section|53|12|19|53\u001eedition|workspace.package.edition|workspace.package|key|key|54|1|8|54\u001erust-version|workspace.package.rust-version|workspace.package|key|key|55|1|13|55\u001edependencies|dependencies||section|section|57|2|14|57\u001eanyhow|dependencies.anyhow|dependencies|key|key|58|1|7|58\u001ebstr|dependencies.bstr|dependencies|key|key|59|1|5|59\u001egrep|dependencies.grep|dependencies|key|key|60|1|5|60\u001egrep-index|dependencies.grep-index|dependencies|key|key|61|1|11|61\u001eignore|dependencies.ignore|dependencies|key|key|62|1|7|62\u001elexopt|dependencies.lexopt|dependencies|key|key|63|1|7|63\u001elog|dependencies.log|dependencies|key|key|64|1|4|64\u001eserde_json|dependencies.serde_json|dependencies|key|key|65|1|11|65\u001etermcolor|dependencies.termcolor|dependencies|key|key|66|1|10|66\u001etextwrap|dependencies.textwrap|dependencies|key|key|67|1|9|67\u001etarget.'cfg(all(target_env = \"musl\", target_pointer_width = \"64\"))'.dependencies.tikv-jemallocator|target.'cfg(all(target_env = \"musl\", target_pointer_width = \"64\"))'.dependencies.tikv-jemallocator||section|section|69|2|100|69\u001eversion|target.'cfg(all(target_env = \"musl\", target_pointer_width = \"64\"))'.dependencies.tikv-jemallocator.version|target.'cfg(all(target_env = \"musl\", target_pointer_width = \"64\"))'.dependencies.tikv-jemallocator|key|key|70|1|8|70\u001edev-dependencies|dev-dependencies||section|section|72|2|18|72\u001eserde|dev-dependencies.serde|dev-dependencies|key|key|73|1|6|73\u001eserde_derive|dev-dependencies.serde_derive|dev-dependencies|key|key|74|1|13|74\u001ewalkdir|dev-dependencies.walkdir|dev-dependencies|key|key|75|1|8|75\u001efeatures|features||section|section|77|2|10|77\u001epcre2|features.pcre2|features|key|key|78|1|6|78\u001eunstable-index|features.unstable-index|features|key|key|81|1|15|81\u001eprofile.release|profile.release||section|section|83|2|17|83\u001edebug|profile.release.debug|profile.release|key|key|84|1|6|84\u001eprofile.release-lto|profile.release-lto||section|section|86|2|21|86\u001einherits|profile.release-lto.inherits|profile.release-lto|key|key|87|1|9|87\u001eopt-level|profile.release-lto.opt-level|profile.release-lto|key|key|88|1|10|88\u001edebug|profile.release-lto.debug|profile.release-lto|key|key|89|1|6|89\u001estrip|profile.release-lto.strip|profile.release-lto|key|key|90|1|6|90\u001edebug-assertions|profile.release-lto.debug-assertions|profile.release-lto|key|key|91|1|17|91\u001eoverflow-checks|profile.release-lto.overflow-checks|profile.release-lto|key|key|92|1|16|92\u001elto|profile.release-lto.lto|profile.release-lto|key|key|93|1|4|93\u001epanic|profile.release-lto.panic|profile.release-lto|key|key|94|1|6|94\u001eincremental|profile.release-lto.incremental|profile.release-lto|key|key|95|1|12|95\u001ecodegen-units|profile.release-lto.codegen-units|profile.release-lto|key|key|96|1|14|96\u001eprofile.deb|profile.deb||section|section|98|2|13|98\u001einherits|profile.deb.inherits|profile.deb|key|key|99|1|9|99\u001epackage.metadata.deb|package.metadata.deb||section|section|101|2|22|101\u001efeatures|package.metadata.deb.features|package.metadata.deb|key|key|102|1|9|102\u001esection|package.metadata.deb.section|package.metadata.deb|key|key|103|1|8|103\u001eassets|package.metadata.deb.assets|package.metadata.deb|key|key|104|1|7|104\u001eextended-description|package.metadata.deb.extended-description|package.metadata.deb|key|key|122|1|21|126"}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			p := filepath.Join(root, tc.file)
			st, err := os.Lstat(p)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("original complete TOML file changed %+v %v", st, err)
			}
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(src), 0)
			_, _ = sha.Write(src)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("complete original raw Git blob mismatch")
			}
			rows := strings.Split(tc.rows, "\x1e")
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{p}, Language: "toml", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 180})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != len(rows) {
				t.Fatalf("original TOML complete key+table inventory expected=%d actual=%d coverage=%v err=%v fail=%+v", len(rows), len(out.Symbols), out.CoverageComplete, err, failed)
			}
			physical := make(map[string]int, len(out.Symbols))
			ids := map[string]string{}
			idSeen := map[string]bool{}
			for i, s := range out.Symbols {
				key := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dupe := physical[key]; dupe {
					t.Errorf("duplicate physical declaration at %s", key)
				}
				physical[key] = i
				if s.ID == "" || idSeen[s.ID] {
					t.Errorf("missing or duplicate declaration ID %+v", s)
				}
				idSeen[s.ID] = true
				ids[s.QualifiedName] = s.ID
			}
			for _, row := range rows {
				f := strings.Split(row, "|")
				if len(f) != 9 {
					t.Fatalf("broken source-first TOML oracle %s", row)
				}
				n := make([]int, 4)
				for i := range n {
					v, e := strconv.Atoi(f[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = v
				}
				key := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, ok := physical[key]
				if !ok {
					t.Errorf("missing physical TOML declaration %s", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || s.QualifiedName != f[1] || s.ParentQualifiedName != f[2] ||
					string(s.Kind) != f[3] || s.NativeKind != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[0] || s.NameRange.End.Column != n[2] ||
					s.DeclarationRange.Start.Line != n[0] || s.DeclarationRange.End.Line != n[3] {
					t.Errorf("source-first TOML key/table mismatch want=%q got=%+v", row, s)
				}
				if f[2] == "" && s.ParentID != "" {
					t.Errorf("unexpected root parent %+v", s)
				}
				if f[2] != "" && s.ParentID != ids[f[2]] {
					t.Errorf("incorrect source-first parent ID %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("missing full original fingerprint")
			}
			// Verify a known root [package] table and exact source excerpt navigation.
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{p}, Language: "toml", Encoding: "utf-8", Query: "package", Match: "exact", MaxFiles: 1, MaxSymbols: 180})
			expectedPackageMatches := 1
			if tc.file == "ripgrep-Cargo.toml" {
				expectedPackageMatches = 2
			} // [package] and [workspace.package]
			if e != nil || fail != nil || len(found.Symbols) != expectedPackageMatches || found.Symbols[0].ID != ids["package"] {
				t.Fatalf("exact original TOML package find: %v %+v %+v", e, fail, found.Symbols)
			}
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: p, Language: "toml", Encoding: "utf-8", SymbolID: ids["package"], SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "[package]") {
				t.Fatalf("TOML package fingerprint excerpt %v %+v %+v", e, fail, shown.Show)
			}
			fail, query, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{p}, Language: "toml", Encoding: "utf-8", Query: "package", Match: "exact", MaxFiles: 1, MaxResults: 180})
			if e != nil || fail != nil || query.Search == nil || len(query.Search.Matches) != expectedPackageMatches || query.Search.Matches[0].SymbolID != ids["package"] {
				t.Fatalf("TOML package structural query %v %+v %+v", e, fail, query)
			}
			for _, neg := range []string{"#", "target/release/rg", "command-line-utilities"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{p}, Language: "toml", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 180})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("TOML source value/comment invented declaration %s", neg)
				}
			}
		})
	}
}
