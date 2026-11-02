package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestCQualifiedOriginalCJSONExportWrapper(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_CJSON_SOURCE")
	if path == "" {
		t.Skip("set qualified original cJSON source path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		t.Fatal("original cJSON source must be a regular file no larger than 1 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc := sourceDocumentForScanner(string(data))
	doc.Path = path
	res, err := (CAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 2048))
	if err != nil {
		t.Fatal(err)
	}
	if res.Analysis.Truncated || !res.Analysis.CoverageComplete {
		t.Fatalf("original cJSON incomplete: %+v", res.Analysis)
	}
	byName := symbolsByQualifiedName(res.Analysis.Symbols)
	for name, line := range map[string]int{"cJSON_GetErrorPtr": 94, "cJSON_Version": 124} {
		sym, ok := byName[name]
		if !ok || sym.Kind != SymbolKindFunction || sym.NativeKind != "function-definition" || sym.NameRange.Start.Line != line {
			t.Errorf("%s: found=%v symbol=%+v, want C function definition at line %d", name, ok, sym, line)
		}
	}
}

func TestCHeaderConditionalLinkageBlockDoesNotInventMacroFunction(t *testing.T) {
	const code = "#ifdef __cplusplus\nextern \"C\"\n{\n#endif\n" +
		"CJSON_PUBLIC(char *) get_one(void);\n" +
		"CJSON_PUBLIC(int) get_two(void);\n" +
		"#ifdef __cplusplus\n}\n#endif\n"
	for _, language := range []string{"c", "cpp"} {
		for _, tc := range []struct{ name, text string }{
			{"lf", code}, {"crlf", strings.ReplaceAll(code, "\n", "\r\n")},
		} {
			t.Run(language+"/"+tc.name, func(t *testing.T) {
				var analyzer SourceAnalyzer = CAnalyzer{}
				if language == "cpp" {
					analyzer = CPPAnalyzer{}
				}
				out, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if !out.Analysis.CoverageComplete || out.Analysis.Truncated {
					t.Fatalf("unexpected incomplete header: %+v", out.Analysis)
				}
				want := map[string]int{"get_one": 5, "get_two": 6}
				actual := symbolsByQualifiedName(out.Analysis.Symbols)
				for name, line := range want {
					s, found := actual[name]
					if !found || s.Kind != SymbolKindFunction || s.NameRange.Start.Line != line {
						t.Errorf("missing/wrong %s line=%d: found=%v got=%+v", name, line, found, s)
					}
				}
				if len(out.Analysis.Symbols) != len(want) {
					t.Errorf("unexpected declarations: %v", sortedSymbolQualifiedNames(out.Analysis.Symbols))
				}
			})
		}
	}
}

func TestCUppercaseAPIReturnMacroAcrossLines(t *testing.T) {
	const source = "#define LIBAPI(type) type\n" +
		"LIBAPI(void)\ndo_work(int count);\n" +
		"LIBAPI(int)\nanother(int x) { return x; }\n" +
		"LIBAPI(char *) make_name(void);\n" +
		"LIBAPI(int, void) malformed(void);\n"
	for _, language := range []string{"c", "cpp"} {
		for _, tc := range []struct{ name, text string }{
			{"lf", source}, {"crlf", strings.ReplaceAll(source, "\n", "\r\n")},
		} {
			t.Run(language+"/"+tc.name, func(t *testing.T) {
				var analyzer SourceAnalyzer = CAnalyzer{}
				if language == "cpp" {
					analyzer = CPPAnalyzer{}
				}
				out, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				if !out.Analysis.CoverageComplete || out.Analysis.Truncated {
					t.Fatalf("unexpected partial API declarations: %+v", out.Analysis)
				}
				byName := symbolsByQualifiedName(out.Analysis.Symbols)
				for name, line := range map[string]int{"do_work": 3, "another": 5, "make_name": 6} {
					sym, found := byName[name]
					if !found || sym.Kind != SymbolKindFunction || sym.NameRange.Start.Line != line {
						t.Errorf("missing/wrong %s at %d: found=%v sym=%+v", name, line, found, sym)
					}
				}
				for _, forbidden := range []string{"LIBAPI", "malformed"} {
					if sym, found := byName[forbidden]; found {
						t.Errorf("wrong macro-based symbol %s: %+v", forbidden, sym)
					}
				}
			})
		}
	}
}

func TestCExportWrapperFunctionAndNoPhantoms(t *testing.T) {
	const source = "#define CJSON_PUBLIC(type) type\n" +
		"CJSON_PUBLIC(const char *) cJSON_Version(void) { return \"1.0\"; }\n" +
		"CJSON_PUBLIC(int) cJSON_GetNumber(void);\n" +
		"#define DECLARE_FAKE(name) void name()\n" +
		"DECLARE_FAKE(macro_generated);\n" +
		"static int actual(void) { return 1; }\n" +
		"CJSON_PUBLIC(int, void) badArgumentCount(void) { return 0; }\n" +
		"CJSON_PUBLIC(123) badReturnType(void) { return 0; }\n"
	for _, tc := range []struct{ name, code string }{{"LF", source}, {"CRLF", strings.ReplaceAll(source, "\n", "\r\n")}} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := (CAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(tc.code), testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if res.Analysis.Truncated || !res.Analysis.CoverageComplete {
				t.Fatalf("coverage incomplete: %+v", res.Analysis)
			}
			byName := symbolsByQualifiedName(res.Analysis.Symbols)
			for name, line := range map[string]int{"cJSON_Version": 2, "cJSON_GetNumber": 3, "actual": 6} {
				sym, ok := byName[name]
				if !ok || sym.Kind != SymbolKindFunction || sym.NameRange.Start.Line != line {
					t.Errorf("missing %s at line %d (exists=%v sym=%+v)", name, line, ok, sym)
				}
			}
			for _, name := range []string{"macro_generated", "badArgumentCount", "badReturnType"} {
				if sym, ok := byName[name]; ok {
					t.Errorf("unsupported macro form %q emitted as function: %+v", name, sym)
				}
			}
		})
	}
}
