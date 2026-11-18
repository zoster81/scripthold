package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original C++ task questions come from source reads and adjacent original tests.
// Third-party projects remain immutable and are not executed.
// Complete immutable GoogleTest and Catch2 sources guard C++ authored
// declarations against function-like macro calls with a brace body.
func TestCppPinnedGoogleTestDestructorAuthoredIdentityNavigation(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("complete pinned GoogleTest original not configured")
	}
	path := filepath.Join(root, "googletest-sample2.h")
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > 1<<20 {
		t.Fatalf("original missing/invalid: %v", err)
	}
	h := NewHandler([]string{root})
	failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "cpp", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 32,
	})
	if err != nil || failed != nil || outline.Truncated || !outline.CoverageComplete || outline.FilesParsed != 1 || len(outline.Symbols) != 11 {
		t.Fatalf("complete original GoogleTest outline failed: err=%v failed=%+v symbols=%d", err, failed, len(outline.Symbols))
	}
	foundDestructor := 0
	for _, sym := range outline.Symbols {
		if sym.Kind != "destructor" {
			continue
		}
		foundDestructor++
		if sym.Name != "~MyString" || sym.QualifiedName != "MyString.~MyString" ||
			sym.ParentQualifiedName != "MyString" || sym.NativeKind != "destructor-definition" ||
			sym.NameRange.Start.Line != 68 || sym.NameRange.Start.Column != 3 ||
			sym.NameRange.End.Column != 12 {
			t.Errorf("whole original destructor not authored correctly: %+v", sym)
		}
	}
	if foundDestructor != 1 {
		t.Errorf("destructor count=%d", foundDestructor)
	}
	fp := outline.Files[0].SourceFingerprint
	if fp == "" {
		t.Fatal("original source fingerprint missing")
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "cpp", Encoding: "utf-8",
		Query: "~MyString", Match: "exact", MaxFiles: 1, MaxSymbols: 16,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].Name != "~MyString" {
		t.Fatalf("authored destructor not findable: err=%v failure=%+v count=%d", err, failed, len(found.Symbols))
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, Language: "cpp", Encoding: "utf-8", SymbolID: found.Symbols[0].ID,
		SourceFingerprint: fp, MaxBytes: 8192,
	})
	if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "~MyString() { delete[] c_string_; }") {
		t.Fatalf("destructor excerpt not authenticated: err=%v failure=%+v", err, failed)
	}
	failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Paths: []string{path}, Language: "cpp",
		Encoding: "utf-8", Query: "~MyString", Match: "exact", MaxFiles: 1, MaxResults: 16,
	})
	if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 1 {
		t.Fatalf("destructor structural search incorrect: err=%v failure=%+v", err, failed)
	}
}

func TestCppPinnedWholeOriginalMacroIsNotSourceFunction(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_REAL_CPP_OUTLINE_ROOT")
	if root == "" {
		t.Skip("two complete original C++ snapshots not configured")
	}
	h := NewHandler([]string{root})
	for _, tc := range []struct {
		file  string
		count int
	}{
		{"googletest-sample2.h", 11}, {"catch2-fixture.cpp", 9},
	} {
		t.Run(tc.file, func(t *testing.T) {
			path := filepath.Join(root, tc.file)
			data, err := os.ReadFile(path)
			if err != nil || len(data) == 0 || len(data) > 1<<20 {
				t.Fatalf("complete original missing: %v", err)
			}
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: "cpp", Encoding: "utf-8", IncludeSignatures: true, MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || out.Truncated || out.FilesParsed != 1 || len(out.Files) != 1 || len(out.Symbols) != tc.count {
				t.Fatalf("full source navigation invalid: err=%v failed=%+v count=%d want=%d", err, failed, len(out.Symbols), tc.count)
			}
			if tc.file == "googletest-sample2.h" {
				return
			}
			if out.CoverageComplete {
				t.Error("unknown macro expansion incorrectly marked fully complete")
			}
			for _, sym := range out.Symbols {
				if sym.Name == "TEST_CASE_METHOD" || sym.NameRange.Start.Line == 54 || sym.NameRange.Start.Line == 58 {
					t.Errorf("Catch2 invocation invented callable: %+v", sym)
				}
			}
			fp := out.Files[0].SourceFingerprint
			if fp == "" {
				t.Fatal("missing exact source fingerprint")
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: "cpp", Encoding: "utf-8",
				Query: "getID", Match: "exact", MaxFiles: 1, MaxSymbols: 32,
			})
			if err != nil || failed != nil || len(found.Symbols) != 1 || found.Symbols[0].NameRange.Start.Line != 41 {
				t.Fatalf("real authored method missing: err=%v failed=%+v", err, failed)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: "cpp", Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: fp, MaxBytes: 8192,
			})
			if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, "++uniqueID") {
				t.Fatalf("real authored method excerpt missing err=%v failed=%+v", err, failed)
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path}, Language: "cpp", Encoding: "utf-8",
				Query: "getID", Match: "exact", MaxFiles: 1, MaxResults: 32,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) != 1 {
				t.Fatalf("real authored method structural search failed: err=%v failed=%+v", err, failed)
			}
		})
	}
}

func TestSourceSymbolsOriginalCppMaintenanceNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original C++ corpus not configured")
	}
	type oracle struct{ file, text string }
	type target struct {
		file, name, excerpt string
		line                int
	}
	cases := []struct {
		project, query, negative, negativeQuery string
		files                                   []string
		oracles                                 []oracle
		targets                                 []target
	}{
		{
			project: "project-cpp-typesense-typesense", query: "get_enable_cors",
			negative: "include/string_utils.h", negativeQuery: "get_enable_cors",
			files: []string{"include/tsconfig.h", "src/tsconfig.cpp", "test/tsconfig_test.cpp", "include/string_utils.h"},
			oracles: []oracle{
				{"include/tsconfig.h", "this->enable_cors = true;"},
				{"include/tsconfig.h", "bool get_enable_cors() const"},
				{"src/tsconfig.cpp", "this->enable_cors = (\"TRUE\" == enable_cors || enable_cors.empty());"},
				{"src/tsconfig.cpp", "this->enable_cors = enable_cors_value == \"true\";"},
				{"test/tsconfig_test.cpp", "TEST(ConfigTest, CorsDefaults)"},
				{"test/tsconfig_test.cpp", "ASSERT_EQ(true, config3.get_enable_cors());"},
			},
			targets: []target{
				{"include/tsconfig.h", "Config", "this->enable_cors = true;", 116},
				{"include/tsconfig.h", "get_enable_cors", "return this->enable_cors;", 393},
				{"include/tsconfig.h", "set_enable_cors", "this->enable_cors = enable_cors;", 269},
				{"src/tsconfig.cpp", "load_config_env", "", 145},
				{"src/tsconfig.cpp", "load_config_file", "", 337},
				{"src/tsconfig.cpp", "load_config_cmd_args", "", 601},
			},
		},
		{
			project: "project-cpp-78-xiaozhi-esp32", query: "SetOutputVolume",
			negative: "main/audio/codecs/dummy_audio_codec.h", negativeQuery: "GetInt",
			files: []string{"main/audio/audio_codec.h", "main/audio/audio_codec.cc", "main/settings.cc", "main/audio/codecs/box_audio_codec.cc", "main/audio/codecs/dummy_audio_codec.h"},
			oracles: []oracle{
				{"main/audio/audio_codec.h", "int output_volume_ = 70;"},
				{"main/audio/audio_codec.cc", "output_volume_ = settings.GetInt(\"output_volume\", output_volume_);"},
				{"main/audio/audio_codec.cc", "output_volume_ = 10;"},
				{"main/audio/audio_codec.cc", "settings.SetInt(\"output_volume\", output_volume_);"},
				{"main/settings.cc", "return default_value;"},
				{"main/audio/codecs/box_audio_codec.cc", "AudioCodec::SetOutputVolume(volume);"},
			},
			targets: []target{
				{"main/audio/audio_codec.h", "output_volume_", "output_volume_ = 70", 64},
				{"main/audio/audio_codec.cc", "Start", "settings.GetInt(\"output_volume\", output_volume_)", 29},
				{"main/audio/audio_codec.cc", "SetOutputVolume", "settings.SetInt(\"output_volume\", output_volume_)", 40},
				{"main/settings.cc", "GetInt", "nvs_get_i32", 49},
				{"main/settings.cc", "SetInt", "nvs_set_i32", 61},
				{"main/audio/codecs/box_audio_codec.cc", "SetOutputVolume", "AudioCodec::SetOutputVolume(volume)", 188},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original C++ file missing %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, o := range tc.oracles {
				path := filepath.Join(root, filepath.FromSlash(o.file))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("oracle file missing %s: %v", o.file, err)
				}
				b, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(b), o.text) {
					t.Fatalf("independent original source %s missing %q: %v", o.file, o.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "cpp", Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("compact digest: err=%v failed=%+v digest=%+v", err, failed, digest)
			}
			b, err := json.Marshal(digest)
			if err != nil || len(b) > 8192 {
				t.Fatalf("compact response %d bytes: %v", len(b), err)
			}
			t.Logf("files=%d symbols=%d compactBytes=%d coverageComplete=%v", digest.FilesParsed, digest.SymbolCount, len(b), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name, Match: "exact", Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 25,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("exact find %s: err=%v failed=%+v result=%+v", want.name, err, failed, found)
				}
				var id string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && sym.NameRange.Start.Line == want.line {
						id = sym.ID
						break
					}
				}
				if id == "" {
					t.Fatalf("source-confirmed C++ declaration %s:%d %s missing: %+v", want.file, want.line, want.name, found.Symbols)
				}
				if want.excerpt != "" {
					failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
						Operation: "show", Path: path, SymbolID: id, SourceFingerprint: found.Files[0].SourceFingerprint, Language: "cpp", Encoding: "utf-8", MaxBytes: 8192,
					})
					if err != nil || failed != nil || shown.Show == nil || !strings.Contains(shown.Show.Text, want.excerpt) {
						t.Fatalf("verified show %s: err=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
					}
				}
			}
			failed, search, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query, Match: "exact", Mode: "structural", Language: "cpp", Encoding: "utf-8", MaxResults: 30,
			})
			if err != nil || failed != nil || search.Search == nil || len(search.Search.Matches) == 0 {
				t.Fatalf("structural search %s: err=%v failed=%+v search=%+v", tc.query, err, failed, search.Search)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.negative))},
				Query: tc.negativeQuery, Match: "exact", Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated C++ file falsely defines %s: err=%v failed=%+v symbols=%+v", tc.negativeQuery, err, failed, negative.Symbols)
			}
		})
	}
}
