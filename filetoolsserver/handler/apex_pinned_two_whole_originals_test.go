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

// Complete source-first declaration inventory of two Git-pinned upstream Apex
// originals. Original authored ranges distinguish overloaded constructors,
// overloaded methods, nested classes, and source fields lacking signatures.
func TestApexTwoWholePinnedOriginalDeclarationInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_APEX_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete independently pinned Apex original sources not configured")
	}
	cases := []struct {
		file, blob, rows string
		size             int64
		count            int
	}{{file: "dlrs-BypassHandler.cls", blob: "efd0c72ef7dec90c326c7add1d825c58c11cdd4c", size: 3791, rows: "BypassHandler\u001fclass\u001fclass\u001f\u001fBypassHandler\u001f33\u001f30\u001f33\u001f43\u001f33\u001f1\u001f104\u001f2\u001f33\u001f1\u001f33\u001f44\u001f33\u001f44\u001f104\u001f2\u001fpublic without sharing class BypassHandler \u001fpublic\u001fpublic,sharing,without\u001ebypassedRollups\u001ffield\u001ffield\u001fBypassHandler\u001fBypassHandler.bypassedRollups\u001f34\u001f30\u001f34\u001f45\u001f34\u001f3\u001f34\u001f66\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001fprivate\u001fprivate,static\u001ebypassAll\u001ffield\u001ffield\u001fBypassHandler\u001fBypassHandler.bypassAll\u001f35\u001f26\u001f35\u001f35\u001f35\u001f3\u001f35\u001f44\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001fprivate\u001fprivate,static\u001eisBypassed\u001fmethod\u001fmethod\u001fBypassHandler\u001fBypassHandler.isBypassed\u001f41\u001f25\u001f41\u001f35\u001f41\u001f3\u001f46\u001f4\u001f41\u001f3\u001f41\u001f56\u001f41\u001f56\u001f46\u001f4\u001fpublic static Boolean isBypassed(String handlerName) \u001fpublic\u001fpublic,static\u001esetBypassAll\u001fmethod\u001fmethod\u001fBypassHandler\u001fBypassHandler.setBypassAll\u001f51\u001f22\u001f51\u001f34\u001f51\u001f3\u001f53\u001f4\u001f51\u001f3\u001f51\u001f48\u001f51\u001f48\u001f53\u001f4\u001fpublic static void setBypassAll(Boolean val) \u001fpublic\u001fpublic,static\u001ebypass\u001fmethod\u001fmethod\u001fBypassHandler\u001fBypassHandler.bypass\u001f58\u001f25\u001f58\u001f31\u001f58\u001f3\u001f73\u001f4\u001f58\u001f3\u001f58\u001f52\u001f58\u001f52\u001f73\u001f4\u001fpublic static Boolean bypass(String handlerName) \u001fpublic\u001fpublic,static\u001eclearBypass\u001fmethod\u001fmethod\u001fBypassHandler\u001fBypassHandler.clearBypass\u001f78\u001f25\u001f78\u001f36\u001f78\u001f3\u001f93\u001f4\u001f78\u001f3\u001f78\u001f57\u001f78\u001f57\u001f93\u001f4\u001fpublic static Boolean clearBypass(String handlerName) \u001fpublic\u001fpublic,static\u001eclearAllBypasses\u001fmethod\u001fmethod\u001fBypassHandler\u001fBypassHandler.clearAllBypasses\u001f98\u001f22\u001f98\u001f38\u001f98\u001f3\u001f103\u001f4\u001f98\u001f3\u001f98\u001f41\u001f98\u001f41\u001f103\u001f4\u001fpublic static void clearAllBypasses() \u001fpublic\u001fpublic,static", count: 8},
		{file: "fflib-StringBuilder.cls", blob: "5bb97f3b719473739719ab99dedd3e27f3dbeb8e", size: 4887, rows: "fflib_StringBuilder\u001fclass\u001fclass\u001f\u001ffflib_StringBuilder\u001f32\u001f22\u001f32\u001f41\u001f32\u001f1\u001f157\u001f2\u001f32\u001f1\u001f33\u001f1\u001f33\u001f1\u001f157\u001f2\u001fpublic virtual class fflib_StringBuilder\n\u001fpublic\u001fpublic,virtual\u001ebuffer\u001ffield\u001ffield\u001ffflib_StringBuilder\u001ffflib_StringBuilder.buffer\u001f34\u001f25\u001f34\u001f31\u001f34\u001f2\u001f34\u001f53\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001fprotected\u001fprotected\u001efflib_StringBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder\u001ffflib_StringBuilder.fflib_StringBuilder\u001f39\u001f9\u001f39\u001f28\u001f39\u001f2\u001f39\u001f33\u001f39\u001f2\u001f39\u001f31\u001f39\u001f31\u001f39\u001f33\u001fpublic fflib_StringBuilder() \u001fpublic\u001fpublic\u001efflib_StringBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder\u001ffflib_StringBuilder.fflib_StringBuilder\u001f44\u001f9\u001f44\u001f28\u001f44\u001f2\u001f47\u001f3\u001f44\u001f2\u001f45\u001f2\u001f45\u001f2\u001f47\u001f3\u001fpublic fflib_StringBuilder(List<String> values)\n\t\u001fpublic\u001fpublic\u001eadd\u001fmethod\u001fmethod\u001ffflib_StringBuilder\u001ffflib_StringBuilder.add\u001f52\u001f22\u001f52\u001f25\u001f52\u001f2\u001f55\u001f3\u001f52\u001f2\u001f53\u001f2\u001f53\u001f2\u001f55\u001f3\u001fpublic virtual void add(List<String> values)\n\t\u001fpublic\u001fpublic,virtual\u001eadd\u001fmethod\u001fmethod\u001ffflib_StringBuilder\u001ffflib_StringBuilder.add\u001f60\u001f22\u001f60\u001f25\u001f60\u001f2\u001f63\u001f3\u001f60\u001f2\u001f61\u001f2\u001f61\u001f2\u001f63\u001f3\u001fpublic virtual void add(String value)\n\t\u001fpublic\u001fpublic,virtual\u001etoString\u001fmethod\u001fmethod\u001ffflib_StringBuilder\u001ffflib_StringBuilder.toString\u001f65\u001f33\u001f65\u001f41\u001f65\u001f2\u001f68\u001f3\u001f65\u001f2\u001f66\u001f2\u001f66\u001f2\u001f68\u001f3\u001fpublic virtual override String toString()\n\t\u001fpublic\u001foverride,public,virtual\u001egetStringValue\u001fmethod\u001fmethod\u001ffflib_StringBuilder\u001ffflib_StringBuilder.getStringValue\u001f73\u001f24\u001f73\u001f38\u001f73\u001f2\u001f76\u001f3\u001f73\u001f2\u001f74\u001f2\u001f74\u001f2\u001f76\u001f3\u001fpublic virtual String getStringValue()\n\t\u001fpublic\u001fpublic,virtual\u001eCommaDelimitedListBuilder\u001fclass\u001fclass\u001ffflib_StringBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001f81\u001f36\u001f81\u001f61\u001f81\u001f2\u001f113\u001f3\u001f81\u001f2\u001f82\u001f2\u001f82\u001f2\u001f113\u001f3\u001fpublic virtual with sharing class CommaDelimitedListBuilder extends fflib_StringBuilder\n\t\u001fpublic\u001fpublic,sharing,virtual,with\u001eitemPrefix\u001ffield\u001ffield\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.itemPrefix\u001f83\u001f10\u001f83\u001f20\u001f83\u001f3\u001f83\u001f26\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001f\u001f\u001edelimiter\u001ffield\u001ffield\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.delimiter\u001f84\u001f10\u001f84\u001f19\u001f84\u001f3\u001f84\u001f26\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f0\u001f\u001f\u001f\u001eCommaDelimitedListBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.CommaDelimitedListBuilder\u001f86\u001f10\u001f86\u001f35\u001f86\u001f3\u001f86\u001f40\u001f86\u001f3\u001f86\u001f38\u001f86\u001f38\u001f86\u001f40\u001fpublic CommaDelimitedListBuilder() \u001fpublic\u001fpublic\u001eCommaDelimitedListBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.CommaDelimitedListBuilder\u001f88\u001f10\u001f88\u001f35\u001f88\u001f3\u001f91\u001f4\u001f88\u001f3\u001f89\u001f3\u001f89\u001f3\u001f91\u001f4\u001fpublic CommaDelimitedListBuilder(List<String> values)\n\t\t\u001fpublic\u001fpublic\u001esetItemPrefix\u001fmethod\u001fmethod\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.setItemPrefix\u001f93\u001f15\u001f93\u001f28\u001f93\u001f3\u001f96\u001f4\u001f93\u001f3\u001f94\u001f3\u001f94\u001f3\u001f96\u001f4\u001fpublic void setItemPrefix(String itemPrefix)\n\t\t\u001fpublic\u001fpublic\u001esetDelimiter\u001fmethod\u001fmethod\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.setDelimiter\u001f98\u001f15\u001f98\u001f27\u001f98\u001f3\u001f101\u001f4\u001f98\u001f3\u001f99\u001f3\u001f99\u001f3\u001f101\u001f4\u001fpublic void setDelimiter(String delimiter)\n\t\t\u001fpublic\u001fpublic\u001egetStringValue\u001fmethod\u001fmethod\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.getStringValue\u001f103\u001f17\u001f103\u001f31\u001f103\u001f3\u001f107\u001f4\u001f103\u001f3\u001f104\u001f3\u001f104\u001f3\u001f107\u001f4\u001fpublic String getStringValue(String itemPrefix)\n\t\t\u001fpublic\u001fpublic\u001etoString\u001fmethod\u001fmethod\u001ffflib_StringBuilder.CommaDelimitedListBuilder\u001ffflib_StringBuilder.CommaDelimitedListBuilder.toString\u001f109\u001f26\u001f109\u001f34\u001f109\u001f3\u001f112\u001f4\u001f109\u001f3\u001f110\u001f3\u001f110\u001f3\u001f112\u001f4\u001fpublic override String toString()\n\t\t\u001fpublic\u001foverride,public\u001eFieldListBuilder\u001fclass\u001fclass\u001ffflib_StringBuilder\u001ffflib_StringBuilder.FieldListBuilder\u001f118\u001f36\u001f118\u001f52\u001f118\u001f2\u001f136\u001f3\u001f118\u001f2\u001f119\u001f2\u001f119\u001f2\u001f136\u001f3\u001fpublic virtual with sharing class FieldListBuilder extends CommaDelimitedListBuilder\n\t\u001fpublic\u001fpublic,sharing,virtual,with\u001eFieldListBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder.FieldListBuilder\u001ffflib_StringBuilder.FieldListBuilder.FieldListBuilder\u001f120\u001f10\u001f120\u001f26\u001f120\u001f3\u001f123\u001f4\u001f120\u001f3\u001f121\u001f3\u001f121\u001f3\u001f123\u001f4\u001fpublic FieldListBuilder(List<Schema.SObjectField> values)\n\t\t\u001fpublic\u001fpublic\u001eFieldListBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder.FieldListBuilder\u001ffflib_StringBuilder.FieldListBuilder.FieldListBuilder\u001f125\u001f10\u001f125\u001f26\u001f125\u001f3\u001f135\u001f4\u001f125\u001f3\u001f126\u001f3\u001f126\u001f3\u001f135\u001f4\u001fpublic FieldListBuilder(List<Schema.SObjectField> values, List<Schema.FieldSet> fieldSets)\n\t\t\u001fpublic\u001fpublic\u001eMultiCurrencyFieldListBuilder\u001fclass\u001fclass\u001ffflib_StringBuilder\u001ffflib_StringBuilder.MultiCurrencyFieldListBuilder\u001f141\u001f28\u001f141\u001f57\u001f141\u001f2\u001f156\u001f3\u001f141\u001f2\u001f142\u001f2\u001f142\u001f2\u001f156\u001f3\u001fpublic with sharing class MultiCurrencyFieldListBuilder extends FieldListBuilder\n\t\u001fpublic\u001fpublic,sharing,with\u001eMultiCurrencyFieldListBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder.MultiCurrencyFieldListBuilder\u001ffflib_StringBuilder.MultiCurrencyFieldListBuilder.MultiCurrencyFieldListBuilder\u001f143\u001f10\u001f143\u001f39\u001f143\u001f3\u001f146\u001f4\u001f143\u001f3\u001f144\u001f3\u001f144\u001f3\u001f146\u001f4\u001fpublic MultiCurrencyFieldListBuilder(List<Schema.SObjectField> values)\n\t\t\u001fpublic\u001fpublic\u001eMultiCurrencyFieldListBuilder\u001fconstructor\u001fconstructor\u001ffflib_StringBuilder.MultiCurrencyFieldListBuilder\u001ffflib_StringBuilder.MultiCurrencyFieldListBuilder.MultiCurrencyFieldListBuilder\u001f148\u001f10\u001f148\u001f39\u001f148\u001f3\u001f155\u001f4\u001f148\u001f3\u001f149\u001f3\u001f149\u001f3\u001f155\u001f4\u001fpublic MultiCurrencyFieldListBuilder(List<Schema.SObjectField> values, List<Schema.FieldSet> fieldSets)\n\t\t\u001fpublic\u001fpublic", count: 23}}
	h := NewHandler([]string{root})
	ctx := context.Background()
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			st, e := os.Lstat(path)
			if e != nil || !st.Mode().IsRegular() || st.Size() != tc.size {
				t.Fatalf("Apex complete original changed stat=%+v err=%v", st, e)
			}
			bytes, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			sha := sha1.New()
			_, _ = fmt.Fprintf(sha, "blob %d%c", len(bytes), 0)
			_, _ = sha.Write(bytes)
			if hex.EncodeToString(sha.Sum(nil)) != tc.blob {
				t.Fatal("Apex complete source raw Git blob differs from independently archived original")
			}
			fail, out, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "apex", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 120})
			if e != nil || fail != nil || out.Truncated || !out.CoverageComplete || out.FilesParsed != 1 ||
				len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("complete Apex original source-first inventory want=%d got=%d coverage=%v err=%v fail=%+v", tc.count, len(out.Symbols), out.CoverageComplete, e, fail)
			}
			sourcePosition := map[string]int{}
			classes := map[string]string{}
			usedIDs := map[string]bool{}
			for i, s := range out.Symbols {
				pos := fmt.Sprintf("%d:%d", s.NameRange.Start.Line, s.NameRange.Start.Column)
				if _, dup := sourcePosition[pos]; dup {
					t.Errorf("duplicate Apex source location %s", pos)
				}
				sourcePosition[pos] = i
				if s.ID == "" || usedIDs[s.ID] {
					t.Errorf("missing or duplicate Apex symbol ID %+v", s)
				}
				usedIDs[s.ID] = true
				if string(s.Kind) == "class" {
					classes[s.QualifiedName] = s.ID
				}
			}
			for _, row := range strings.Split(tc.rows, "\x1e") {
				f := strings.Split(row, "\x1f")
				if len(f) != 24 {
					t.Fatalf("invalid immutable Apex oracle row %q", row)
				}
				n := make([]int, 16)
				for i := range n {
					v, err := strconv.Atoi(f[i+5])
					if err != nil {
						t.Fatal(err)
					}
					n[i] = v
				}
				pos := fmt.Sprintf("%d:%d", n[0], n[1])
				idx, present := sourcePosition[pos]
				if !present {
					t.Errorf("original supported Apex declaration missing %q", row)
					continue
				}
				s := out.Symbols[idx]
				if s.Name != f[0] || string(s.Kind) != f[1] || s.NativeKind != f[2] ||
					s.ParentQualifiedName != f[3] || s.QualifiedName != f[4] ||
					s.NameRange.Start.Line != n[0] || s.NameRange.Start.Column != n[1] ||
					s.NameRange.End.Line != n[2] || s.NameRange.End.Column != n[3] ||
					s.DeclarationRange.Start.Line != n[4] || s.DeclarationRange.Start.Column != n[5] ||
					s.DeclarationRange.End.Line != n[6] || s.DeclarationRange.End.Column != n[7] ||
					string(s.Visibility) != f[22] || strings.Join(s.Modifiers, ",") != f[23] {
					t.Errorf("Apex full original general metadata mismatch source=%q got=%+v", row, s)
				}
				if f[3] == "" && s.ParentID != "" {
					t.Errorf("unexpected Apex root parent %+v", s)
				}
				if f[3] != "" && s.ParentID != classes[f[3]] {
					t.Errorf("Apex nested class/field/method ownership mismatch expected=%s got=%+v", f[3], s)
				}
				if n[8] > 0 {
					if s.SignatureRange == nil || s.SignatureRange.Start.Line != n[8] ||
						s.SignatureRange.Start.Column != n[9] || s.SignatureRange.End.Line != n[10] ||
						s.SignatureRange.End.Column != n[11] || s.Signature != f[21] {
						t.Errorf("Apex complete original signature mismatch source=%q got=%+v", row, s)
					}
				} else if s.SignatureRange != nil || s.Signature != "" {
					t.Errorf("unsupported Apex field signature emitted %+v", s)
				}
				if n[12] > 0 {
					if s.BodyRange == nil || s.BodyRange.Start.Line != n[12] ||
						s.BodyRange.Start.Column != n[13] || s.BodyRange.End.Line != n[14] ||
						s.BodyRange.End.Column != n[15] {
						t.Errorf("Apex authored body mismatch source=%q got=%+v", row, s)
					}
				} else if s.BodyRange != nil {
					t.Errorf("unwanted Apex field body %+v", s)
				}
			}
			if out.Files[0].SourceFingerprint == "" || out.Files[0].SourceBytes != tc.size {
				t.Fatal("Apex whole source fingerprint/size missing")
			}
			name := strings.Split(strings.Split(tc.rows, "\x1e")[0], "\x1f")[0]
			fail, found, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "apex", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
			classID := classes[name]
			inFind := false
			for _, s := range found.Symbols {
				if s.ID == classID && string(s.Kind) == "class" {
					inFind = true
				}
			}
			if e != nil || fail != nil || classID == "" || !inFind {
				t.Fatalf("Apex whole source class find (including overloaded constructors) %v %+v %+v", e, fail, found.Symbols)
			}
			fail, shown, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "show", Path: path, Language: "apex", Encoding: "utf-8", SymbolID: classID, SourceFingerprint: out.Files[0].SourceFingerprint, MaxBytes: 8192})
			if e != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, name) {
				t.Fatalf("Apex fingerprint show %v %+v %+v", e, fail, shown.Show)
			}
			fail, q, e := h.SourceQuery(ctx, nil, SourceQueryInput{Operation: "search", Mode: "structural", Paths: []string{path}, Language: "apex", Encoding: "utf-8", Query: name, Match: "exact", MaxFiles: 1, MaxResults: 120})
			inSearch := false
			if q.Search != nil {
				for _, match := range q.Search.Matches {
					if match.SymbolID == classID {
						inSearch = true
					}
				}
			}
			if e != nil || fail != nil || !inSearch {
				t.Fatalf("Apex structural search must include original class among constructor overloads %v %+v %+v", e, fail, q)
			}
			for _, negative := range []string{"System", "return", "new", "if", "for", "String"} {
				fail, none, e := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "find", Paths: []string{path}, Language: "apex", Encoding: "utf-8", Query: negative, Match: "exact", MaxFiles: 1, MaxSymbols: 120})
				if e != nil || fail != nil || len(none.Symbols) != 0 {
					t.Errorf("Apex statement/reference invented as declaration %q", negative)
				}
			}
		})
	}
}
