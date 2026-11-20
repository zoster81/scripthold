package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The oracles are based on independent original-source reads and bounded grep.
// Third-party Kotlin projects are never executed or changed.
func TestSourceSymbolsOriginalKotlinTaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Kotlin corpus not configured")
	}
	type expected struct {
		file, name string
		line       int
	}
	cases := []struct {
		project, primary, query, unrelated, partial string
		files, evidence                             []string
		expected                                    []expected
	}{
		{
			project: "project-kotlin-jakewharton-timber",
			files: []string{
				"timber/src/androidMain/kotlin/timber/log/Timber.kt",
				"timber/src/commonMain/kotlin/timber/log/Timber.kt",
				"timber/src/test/java/timber/log/TimberTest.kt",
				"timber-lint/src/main/java/timber/lint/WrongTimberUsageDetector.kt",
			},
			evidence: []string{
				"actual fun plant(tree: Tree)", "actual fun plant(vararg trees: Tree)",
				"require(tree !== this) { \"Cannot plant Timber into itself.\" }",
				"actual fun uproot(tree: Tree)", "fun plant(tree: Tree)",
				"fun recursion()", "fun uprootThrowsIfMissing()", "fun forestReturnsAllTreesPlanted()",
			},
			expected: []expected{
				{"timber/src/androidMain/kotlin/timber/log/Timber.kt", "plant", 431},
				{"timber/src/androidMain/kotlin/timber/log/Timber.kt", "plant", 441},
				{"timber/src/commonMain/kotlin/timber/log/Timber.kt", "plant", 95},
				{"timber/src/commonMain/kotlin/timber/log/Timber.kt", "plant", 98},
				{"timber/src/test/java/timber/log/TimberTest.kt", "recursion", 58},
				{"timber/src/test/java/timber/log/TimberTest.kt", "uprootThrowsIfMissing", 105},
			},
			primary: "timber/src/androidMain/kotlin/timber/log/Timber.kt",
			query:   "plant", unrelated: "timber-lint/src/main/java/timber/lint/WrongTimberUsageDetector.kt",
			partial: "",
		},
		{
			project: "project-kotlin-square-moshi",
			files: []string{
				"moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterFactory.kt",
				"moshi-kotlin-tests/src/test/kotlin/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterTest.kt",
				"moshi-kotlin/src/main/java/com/squareup/moshi/KotlinJsonAdapterFactory.kt",
			},
			evidence: []string{
				"override fun fromJson(reader: JsonReader): T",
				"if (values[i] === ABSENT_VALUE)",
				"if (!param.declaresDefaultValue)",
				"throw missingProperty(constructor.parameters[i].name, allBindings[i]?.jsonName, reader)",
				"fun constructorDefaults()", "fun requiredTransientConstructorParameterFails()",
				"public class KotlinJsonAdapterFactory : JsonAdapter.Factory by KotlinJsonAdapterFactory()",
			},
			expected: []expected{
				{"moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterFactory.kt", "fromJson", 70},
				{"moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterFactory.kt", "fromJson", 195},
				{"moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterFactory.kt", "create", 215},
				{"moshi-kotlin-tests/src/test/kotlin/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterTest.kt", "constructorDefaults", 127},
				{"moshi-kotlin-tests/src/test/kotlin/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterTest.kt", "requiredTransientConstructorParameterFails", 292},
			},
			primary: "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterFactory.kt",
			query:   "fromJson", unrelated: "moshi-kotlin/src/main/java/com/squareup/moshi/KotlinJsonAdapterFactory.kt",
			partial: "moshi-kotlin/src/main/java/com/squareup/moshi/kotlin/reflect/KotlinJsonAdapterFactory.kt",
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			h := NewHandler([]string{root})
			paths := make([]string, 0, len(tc.files))
			var contents strings.Builder
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("missing original %s: %v", rel, err)
				}
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				contents.Write(data)
				paths = append(paths, path)
			}
			original := contents.String()
			for _, phrase := range tc.evidence {
				if !strings.Contains(original, phrase) {
					t.Fatalf("original source oracle changed %q", phrase)
				}
			}
			fail, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "kotlin", Encoding: "utf-8",
				Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || fail != nil || digest.Truncated || digest.FilesParsed != len(paths) ||
				len(digest.OverviewFiles) != len(paths) {
				t.Fatalf("digest error=%v fail=%+v result=%+v", err, fail, digest)
			}
			partial := filepath.Join(root, filepath.FromSlash(tc.partial))
			if digest.CoverageComplete != (tc.partial == "") {
				t.Fatalf("unexpected aggregate completeness %v expectedPartial=%q", digest.CoverageComplete, tc.partial)
			}
			for _, file := range digest.OverviewFiles {
				if file.Status != "parsed" || file.CoverageComplete != (file.Path != partial) {
					t.Fatalf("incorrect file coverage: %s status=%s complete=%v", file.Path, file.Status, file.CoverageComplete)
				}
			}
			raw, err := json.Marshal(digest)
			if err != nil || len(raw) > 8192 {
				t.Fatalf("unbounded compact map bytes=%d err=%v", len(raw), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d reportedComplete=%v partial=%s",
				digest.FilesParsed, digest.SymbolCount, len(raw), digest.CoverageComplete, tc.partial)
			for _, want := range tc.expected {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				fail, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact",
					Language: "kotlin", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 25,
				})
				if err != nil || fail != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: %v fail=%+v result=%+v", want.name, err, fail, found)
				}
				id := ""
				for _, s := range found.Symbols {
					if s.Name == want.name && s.NameRange.Start.Line == want.line {
						id = s.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("missing %s at %s:%d: %+v", want.name, want.file, want.line, found.Symbols)
				}
				fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: id,
					SourceFingerprint: found.Files[0].SourceFingerprint,
					Language:          "kotlin", Encoding: "utf-8", MaxBytes: 16384,
				})
				if err != nil || fail != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.name) {
					t.Fatalf("fingerprint-bound excerpt %s: %v fail=%+v", want.name, err, fail)
				}
			}
			fail, q, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact",
				Mode: "structural", Language: "kotlin", Encoding: "utf-8", MaxResults: 25,
			})
			if err != nil || fail != nil || q.Search == nil || len(q.Search.Matches) == 0 {
				t.Fatalf("structural search %s: %v fail=%+v", tc.query, err, fail)
			}
			fail, unrelated, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.query, Match: "exact", Language: "kotlin", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 10,
			})
			if err != nil || fail != nil || len(unrelated.Symbols) != 0 {
				t.Fatalf("false declaration in alternative file %s: %v fail=%+v matches=%+v",
					tc.unrelated, err, fail, unrelated.Symbols)
			}
		})
	}
}

func TestSourceSymbolsOriginalKotlinConstructorEnumEntries(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Kotlin corpus not configured")
	}
	cases := []struct {
		project, file, owner, sample, excerpt, method string
		firstLine                                     int
		entries                                       []string
	}{
		{
			project: "project-kotlin-zhanghai-materialfiles",
			file:    "app/src/main/java/me/zhanghai/android/files/file/MimeTypeIcon.kt",
			owner:   "me.zhanghai.android.files.file.MimeTypeIcon",
			sample:  "APK", excerpt: "APK(R.drawable.file_apk_icon)", firstLine: 11,
			entries: []string{
				"APK", "ARCHIVE", "AUDIO", "CALENDAR", "CERTIFICATE", "CODE",
				"CONTACT", "DIRECTORY", "DOCUMENT", "EBOOK", "EMAIL", "FONT",
				"GENERIC", "IMAGE", "PDF", "PRESENTATION", "SPREADSHEET",
				"TEXT", "VIDEO", "WORD", "EXCEL", "POWERPOINT",
			},
		},
		{
			project: "project-kotlin-lysine-dev-okio",
			file:    "okio/src/jvmTest/kotlin/okio/CipherAlgorithm.kt",
			owner:   "okio.CipherAlgorithm", firstLine: 28,
			sample:  "AesCbcNopadding",
			excerpt: "AesCbcNopadding(\"AES/CBC/NoPadding\", false, 16, 16)",
			method:  "createCipherFactory",
			entries: []string{
				"AesCbcNopadding", "AesCbcPkcs5padding", "AesEcbNopadding",
				"AesEcbPkcs5padding", "DesCbcNopadding", "DesCbcPkcs5padding",
				"DesEcbNopadding", "DesEcbPkcs5padding", "DesedeCbcNopadding",
				"DesedeCbcPkcs5padding", "DesedeEcbNopadding", "DesedeEcbPkcs5padding",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() {
				t.Fatalf("original source missing or not regular: %v", err)
			}
			original, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(original), tc.excerpt) {
				t.Fatalf("original source oracle changed or unreadable: %v", err)
			}
			h := NewHandler([]string{root})
			fail, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "kotlin", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || fail != nil || outline.Truncated || outline.FilesParsed != 1 {
				t.Fatalf("original Kotlin outline: %v fail=%+v result=%+v", err, fail, outline)
			}
			byName := map[string]int{}
			for i, symbol := range outline.Symbols {
				byName[symbol.QualifiedName] = i
			}
			for i, entry := range tc.entries {
				qualified := tc.owner + "." + entry
				index, ok := byName[qualified]
				if !ok {
					t.Errorf("missing original enum entry %s", qualified)
					continue
				}
				symbol := outline.Symbols[index]
				if symbol.Kind != "constant" || symbol.NativeKind != "enum-value" ||
					symbol.ParentQualifiedName != tc.owner ||
					symbol.DeclarationRange.Start.Line != tc.firstLine+i ||
					symbol.DeclarationRange.End.Line != tc.firstLine+i ||
					symbol.DeclarationRange.End.Column <= symbol.NameRange.End.Column {
					t.Errorf("incorrect original enum entry %s: %+v", qualified, symbol)
				}
			}
			if tc.method != "" {
				index, ok := byName[tc.owner+"."+tc.method]
				if !ok {
					t.Error("method after enum semicolon missing")
				} else if method := outline.Symbols[index]; method.Kind != "method" ||
					method.ParentQualifiedName != tc.owner || method.DeclarationRange.Start.Line != 42 {
					t.Errorf("incorrect method after enum semicolon: %+v", method)
				}
			}
			fail, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Query: tc.sample, Match: "exact",
				Language: "kotlin", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 8,
			})
			if err != nil || fail != nil || len(found.Files) != 1 || len(found.Symbols) != 1 ||
				found.Symbols[0].QualifiedName != tc.owner+"."+tc.sample {
				t.Fatalf("find original enum %s: %v fail=%+v result=%+v", tc.sample, err, fail, found)
			}
			fail, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, SymbolID: found.Symbols[0].ID,
				SourceFingerprint: found.Files[0].SourceFingerprint,
				Language:          "kotlin", Encoding: "utf-8", MaxBytes: 4096,
			})
			if err != nil || fail != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.excerpt) {
				t.Fatalf("source-exact original enum excerpt: %v fail=%+v", err, fail)
			}
			fail, q, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: []string{path}, Query: tc.sample, Match: "exact",
				Mode: "structural", Language: "kotlin", Encoding: "utf-8", MaxResults: 8,
			})
			if err != nil || fail != nil || q.Search == nil || len(q.Search.Matches) == 0 {
				t.Fatalf("structural search %s: %v fail=%+v", tc.sample, err, fail)
			}
		})
	}
}
