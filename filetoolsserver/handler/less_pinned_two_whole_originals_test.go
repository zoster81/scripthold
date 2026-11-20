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

// Two complete Less originals: declarations were counted from source before
// consulting the historical development response. Repeated selectors are
// intentionally distinct physical occurrences, not deduplicated names.
func TestLessTwoWholePinnedOriginalSelectorInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_LESS_OUTLINE_ROOT")
	if root == "" {
		t.Skip("complete Git-pinned Less originals not configured")
	}
	cases := []struct {
		file, blob string
		bytes      int64
		records    string
	}{
		{file: "antdesign-global.less", bytes: 2803, blob: "6297fdcc5f047b40ac78197b77729037037398a4", records: ".demo-logo|.demo-logo||selector|selector|4|1|11|4|1|11|2|4|1|4|13|.demo-logo {\u001e.demo-logo-vertical|.demo-logo-vertical||selector|selector|13|1|20|13|1|18|2|13|1|13|22|.demo-logo-vertical {\u001ehtml|html||selector|selector|20|1|5|20|1|27|2|20|1|20|7|html {\u001e.rc-footer|.rc-footer||selector|selector|29|1|11|29|1|36|2|29|1|29|13|.rc-footer {\u001e.rc-footer a|.rc-footer a||selector|selector|37|1|13|37|1|41|2|37|1|37|15|.rc-footer a {\u001e.rc-footer a:hover|.rc-footer a:hover||selector|selector|42|1|19|42|1|44|2|42|1|42|21|.rc-footer a:hover {\u001e.rc-footer-container|.rc-footer-container||selector|selector|45|1|21|45|1|50|2|45|1|45|23|.rc-footer-container {\u001e.rc-footer-columns|.rc-footer-columns||selector|selector|51|1|19|51|1|54|2|51|1|51|21|.rc-footer-columns {\u001e.rc-footer-column|.rc-footer-column||selector|selector|55|1|18|55|1|57|2|55|1|55|20|.rc-footer-column {\u001e.rc-footer-column h2|.rc-footer-column h2||selector|selector|58|1|21|58|1|64|2|58|1|58|23|.rc-footer-column h2 {\u001e.rc-footer-column-icon|.rc-footer-column-icon||selector|selector|65|1|23|65|1|73|2|65|1|65|25|.rc-footer-column-icon {\u001e.rc-footer-column-icon > span|.rc-footer-column-icon > span||selector|selector|74|1|30|74|1|79|2|74|1|76|29|.rc-footer-column-icon > span,\n.rc-footer-column-icon > svg,\n.rc-footer-column-icon img {\u001e.rc-footer-column-icon > svg|.rc-footer-column-icon > svg||selector|selector|75|1|29|74|1|79|2|74|1|76|29|.rc-footer-column-icon > span,\n.rc-footer-column-icon > svg,\n.rc-footer-column-icon img {\u001e.rc-footer-column-icon img|.rc-footer-column-icon img||selector|selector|76|1|27|74|1|79|2|74|1|76|29|.rc-footer-column-icon > span,\n.rc-footer-column-icon > svg,\n.rc-footer-column-icon img {\u001e.rc-footer-item|.rc-footer-item||selector|selector|80|1|16|80|1|82|2|80|1|80|18|.rc-footer-item {\u001e.rc-footer-item-icon|.rc-footer-item-icon||selector|selector|83|1|21|83|1|91|2|83|1|83|23|.rc-footer-item-icon {\u001e.rc-footer-item-icon > span|.rc-footer-item-icon > span||selector|selector|92|1|28|92|1|97|2|92|1|94|27|.rc-footer-item-icon > span,\n.rc-footer-item-icon > svg,\n.rc-footer-item-icon img {\u001e.rc-footer-item-icon > svg|.rc-footer-item-icon > svg||selector|selector|93|1|27|92|1|97|2|92|1|94|27|.rc-footer-item-icon > span,\n.rc-footer-item-icon > svg,\n.rc-footer-item-icon img {\u001e.rc-footer-item-icon img|.rc-footer-item-icon img||selector|selector|94|1|25|92|1|97|2|92|1|94|27|.rc-footer-item-icon > span,\n.rc-footer-item-icon > svg,\n.rc-footer-item-icon img {\u001e.rc-footer-item-separator|.rc-footer-item-separator||selector|selector|98|1|26|98|1|100|2|98|1|98|28|.rc-footer-item-separator {\u001e.rc-footer-bottom-container|.rc-footer-bottom-container||selector|selector|101|1|28|101|1|110|2|101|1|101|30|.rc-footer-bottom-container {\u001e.rc-footer-light|.rc-footer-light||selector|selector|111|1|17|111|1|114|2|111|1|111|19|.rc-footer-light {\u001e.rc-footer-light h2|.rc-footer-light h2||selector|selector|115|1|20|115|1|118|2|115|1|116|21|.rc-footer-light h2,\n.rc-footer-light a {\u001e.rc-footer-light a|.rc-footer-light a||selector|selector|116|1|19|115|1|118|2|115|1|116|21|.rc-footer-light h2,\n.rc-footer-light a {\u001e.rc-footer-light .rc-footer-bottom-container|.rc-footer-light .rc-footer-bottom-container||selector|selector|119|1|45|119|1|121|2|119|1|119|47|.rc-footer-light .rc-footer-bottom-container {\u001e.rc-footer-light .rc-footer-item-separator|.rc-footer-light .rc-footer-item-separator||selector|selector|122|1|43|122|1|125|2|122|1|123|47|.rc-footer-light .rc-footer-item-separator,\n.rc-footer-light .rc-footer-item-description {\u001e.rc-footer-light .rc-footer-item-description|.rc-footer-light .rc-footer-item-description||selector|selector|123|1|45|122|1|125|2|122|1|123|47|.rc-footer-light .rc-footer-item-separator,\n.rc-footer-light .rc-footer-item-description {\u001e.rc-footer|.rc-footer||selector|selector|127|3|13|127|3|129|4|127|3|127|15|.rc-footer {\u001e.rc-footer-container|.rc-footer-container||selector|selector|130|3|23|130|3|132|4|130|3|130|25|.rc-footer-container {\u001e.rc-footer-columns|.rc-footer-columns||selector|selector|133|3|21|133|3|135|4|133|3|133|23|.rc-footer-columns {\u001e.rc-footer-column|.rc-footer-column||selector|selector|136|3|20|136|3|139|4|136|3|136|22|.rc-footer-column {\u001e.rc-footer-column:last-child|.rc-footer-column:last-child||selector|selector|140|3|31|140|3|142|4|140|3|140|33|.rc-footer-column:last-child {"},
		{file: "antdv-index.less", bytes: 2260, blob: "fb9e5712ffe7acb06e2dca4207b77782cbcaf094", records: ".drawer > *|.drawer > *||selector|selector|3|1|12|3|1|18|2|3|1|3|14|.drawer > * {\u001e.drawer .drawer-mask|.drawer .drawer-mask||selector|selector|20|1|21|20|1|30|2|20|1|20|23|.drawer .drawer-mask {\u001e.drawer-content-wrapper|.drawer-content-wrapper||selector|selector|32|1|24|32|1|35|2|32|1|32|26|.drawer-content-wrapper {\u001e.drawer-content|.drawer-content||selector|selector|37|1|16|37|1|41|2|37|1|37|18|.drawer-content {\u001e.drawer-handle|.drawer-handle||selector|selector|43|1|15|43|1|66|2|43|1|43|17|.drawer-handle {\u001e.drawer-left|.drawer-left||selector|selector|68|1|13|68|1|72|2|68|1|69|16|.drawer-left,\n.drawer-right {\u001e.drawer-right|.drawer-right||selector|selector|69|1|14|68|1|72|2|68|1|69|16|.drawer-left,\n.drawer-right {\u001e.drawer-left .drawer-content|.drawer-left .drawer-content||selector|selector|74|1|29|74|1|79|2|74|1|77|40|.drawer-left .drawer-content,\n.drawer-left .drawer-content-wrapper,\n.drawer-right .drawer-content,\n.drawer-right .drawer-content-wrapper {\u001e.drawer-left .drawer-content-wrapper|.drawer-left .drawer-content-wrapper||selector|selector|75|1|37|74|1|79|2|74|1|77|40|.drawer-left .drawer-content,\n.drawer-left .drawer-content-wrapper,\n.drawer-right .drawer-content,\n.drawer-right .drawer-content-wrapper {\u001e.drawer-right .drawer-content|.drawer-right .drawer-content||selector|selector|76|1|30|74|1|79|2|74|1|77|40|.drawer-left .drawer-content,\n.drawer-left .drawer-content-wrapper,\n.drawer-right .drawer-content,\n.drawer-right .drawer-content-wrapper {\u001e.drawer-right .drawer-content-wrapper|.drawer-right .drawer-content-wrapper||selector|selector|77|1|38|74|1|79|2|74|1|77|40|.drawer-left .drawer-content,\n.drawer-left .drawer-content-wrapper,\n.drawer-right .drawer-content,\n.drawer-right .drawer-content-wrapper {\u001e.drawer-left|.drawer-left||selector|selector|81|1|13|81|1|84|2|81|1|81|15|.drawer-left {\u001e.drawer-left .ant-drawer-body|.drawer-left .ant-drawer-body||selector|selector|85|1|30|85|1|87|2|85|1|85|32|.drawer-left .ant-drawer-body {"},
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, err := os.Lstat(path)
			if err != nil || !st.Mode().IsRegular() || st.Size() != tc.bytes {
				t.Fatalf("whole pinned Less source changed: %+v %v", st, err)
			}
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha1.New()
			_, _ = fmt.Fprintf(hash, "blob %d%c", len(original), 0)
			_, _ = hash.Write(original)
			if hex.EncodeToString(hash.Sum(nil)) != tc.blob {
				t.Fatal("complete source Git blob differs")
			}
			rows := strings.Split(tc.records, "\x1e")
			failed, out, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "less", Encoding: "utf-8",
				IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120,
			})
			if err != nil || failed != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 ||
				len(out.Files) != 1 || len(out.Symbols) != len(rows) {
				t.Fatalf("whole-original source-first Less inventory expected=%d actual=%d coverage=%v err=%v failed=%+v", len(rows), len(out.Symbols), out.CoverageComplete, err, failed)
			}
			byPosition := map[string]int{}
			ids := map[string]bool{}
			for i, s := range out.Symbols {
				physical := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, exists := byPosition[physical]; exists {
					t.Errorf("duplicate physical selector at %s", physical)
				}
				byPosition[physical] = i
				if s.ID == "" || ids[s.ID] {
					t.Errorf("missing/repeated symbol identity %+v", s)
				}
				ids[s.ID] = true
			}
			for _, row := range rows {
				fields := strings.Split(row, "|")
				if len(fields) != 17 {
					t.Fatalf("invalid independent source row %q", row)
				}
				n := make([]int, 11)
				for i := range n {
					value, e := strconv.Atoi(fields[i+5])
					if e != nil {
						t.Fatal(e)
					}
					n[i] = value
				}
				physical := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, ok := byPosition[physical]
				if !ok {
					t.Errorf("missing physical selector %s: %s", physical, row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != fields[0] || s.QualifiedName != fields[1] || s.ParentQualifiedName != fields[2] ||
					string(s.Kind) != fields[3] || s.NativeKind != fields[4] || s.ParentID != "" ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] || s.NameRange.End.Line != n[0] ||
					s.NameRange.End.Column != n[2] || s.DeclarationRange.Start.Line != n[3] ||
					s.DeclarationRange.Start.Column != n[4] || s.DeclarationRange.End.Line != n[5] ||
					s.DeclarationRange.End.Column != n[6] || s.SignatureRange == nil ||
					s.SignatureRange.Start.Line != n[7] || s.SignatureRange.Start.Column != n[8] ||
					s.SignatureRange.End.Line != n[9] || s.SignatureRange.End.Column != n[10] ||
					s.Signature != fields[16] {
					t.Errorf("source-first selector declaration mismatch want=%q got=%+v", row, s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.bytes {
				t.Fatal("complete original source fingerprint or byte count missing")
			}
			// These inputs are at-rules/properties, not Less selector declarations.
			for _, neg := range []string{"@media", "@import", "background-color", "@supports"} {
				fail, missing, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "less", Encoding: "utf-8", Query: neg, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || fail != nil || len(missing.Symbols) != 0 {
					t.Errorf("non-selector was invented as symbol %s: err=%v failure=%+v", neg, e, fail)
				}
			}
			first := strings.Split(rows[0], "|")[0]
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "less", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || len(found.Symbols) < 1 {
				t.Fatalf("original first selector find: err=%v failure=%+v symbols=%+v", e, fail, found.Symbols)
			}
			one := found.Symbols[0]
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "less", Encoding: "utf-8", SymbolID: one.ID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, first) {
				t.Fatalf("original selector fingerprint show err=%v failure=%+v show=%+v", e, fail, shown.Show)
			}
			fail, query, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "less", Encoding: "utf-8", Query: first, Match: "exact", MaxFiles: 1, MaxResults: 120})
			if e != nil || fail != nil || query.Search == nil || len(query.Search.Matches) < 1 {
				t.Fatalf("original selector structural search err=%v failure=%+v", e, fail)
			}
		})
	}
}
