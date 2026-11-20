package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// The original Redis sha256.h uses an anonymous typedef struct. It defines
// an alias with member declarations, not global variables.
func TestCFamilyAnonymousAggregateTypedefOwnership(t *testing.T) {
	for _, tc := range []struct {
		name, source, alias, first, second string
	}{
		{"struct", "typedef struct {\n int data[64];\n unsigned long bitlen;\n} SHA256_CTX;\nint after_type(void);\n", "SHA256_CTX", "data", "bitlen"},
		{"union", "typedef union {\n int count;\n double real;\n} Payload;\nint after_type(void);\n", "Payload", "count", "real"},
	} {
		for _, lang := range []struct {
			name     string
			analyzer SourceAnalyzer
		}{{"c", CAnalyzer{}}, {"cpp", CPPAnalyzer{}}} {
			for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
				t.Run(tc.name+"/"+lang.name+"/"+eol.name, func(t *testing.T) {
					doc := sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", eol.value))
					out, err := lang.analyzer.Analyze(context.Background(), doc, testAnalyzeOptions(true, 64))
					if err != nil {
						t.Fatal(err)
					}
					byName := symbolsByQualifiedName(out.Analysis.Symbols)
					alias, ok := byName[tc.alias]
					if !ok || alias.Kind != SymbolKindAlias || alias.NameRange.Start.Line != 4 {
						t.Errorf("anonymous typedef alias missing/wrong: %+v present=%v", alias, ok)
					}
					for _, field := range []struct {
						name string
						line int
					}{{tc.first, 2}, {tc.second, 3}} {
						symbol, ok := byName[tc.alias+"."+field.name]
						if !ok || symbol.Kind != SymbolKindField || symbol.ParentQualifiedName != tc.alias ||
							symbol.NameRange.Start.Line != field.line || symbol.ParentID == "" {
							t.Errorf("missing/wrong typedef member %s: %+v present=%v", field.name, symbol, ok)
						}
						if leaked, found := byName[field.name]; found {
							t.Errorf("member leaked into global variable: %+v", leaked)
						}
					}
					if after, ok := byName["after_type"]; !ok || after.Kind != SymbolKindFunction ||
						after.NameRange.Start.Line != 5 {
						t.Errorf("following declaration missing: %+v", after)
					}
				})
			}
		}
	}
}
