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
