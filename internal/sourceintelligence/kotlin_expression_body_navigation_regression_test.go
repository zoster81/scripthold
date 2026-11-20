package sourceintelligence

import (
	"context"
	"strings"
	"testing"
)

// Preserve the authored source of line-wrapped Kotlin expression bodies,
// without swallowing the next method or treating a lambda as the method body.
func TestKotlinLineWrappedExpressionFunctionBody(t *testing.T) {
	const source = "class Tree {\n" +
		"  protected open fun isLoggable(tag: String?, priority: Int): Boolean =\n" +
		"    isLoggable(priority)\n" +
		"  protected open fun formatMessage(message: String, args: Array<out Any?>): String =\n" +
		"    message.format(*args)\n" +
		"  fun getter(kmProperty: KmProperty): Method? =\n" +
		"    kmProperty.getterSignature?.let { signature -> findMethod(clazz, signature) }\n" +
		"  fun next(): String = \"ok\"\n" +
		"}\n"
	cases := []struct {
		name       string
		start, end int
		body       string
	}{
		{"isLoggable", 2, 3, "    isLoggable(priority)"},
		{"formatMessage", 4, 5, "    message.format(*args)"},
		{"getter", 6, 7, "    kmProperty.getterSignature?.let { signature -> findMethod(clazz, signature) }"},
		{"next", 8, 8, ""},
	}
	for _, variant := range []struct{ name, eol string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(variant.name, func(t *testing.T) {
			result, err := (KotlinAnalyzer{}).Analyze(context.Background(),
				sourceDocumentForScanner(strings.ReplaceAll(source, "\n", variant.eol)),
				testAnalyzeOptions(true, 128))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.Truncated || !result.Analysis.CoverageComplete {
				t.Fatalf("supported Kotlin declarations should remain complete: %+v", result.Analysis)
			}
			seen := map[string]bool{}
			for _, symbol := range result.Analysis.Symbols {
				for _, tc := range cases {
					if symbol.Name != tc.name || symbol.Kind != SymbolKindMethod {
						continue
					}
					seen[tc.name] = true
					if symbol.DeclarationRange.Start.Line != tc.start || symbol.DeclarationRange.End.Line != tc.end {
						t.Errorf("%s declaration range %v want %d-%d", tc.name, symbol.DeclarationRange, tc.start, tc.end)
					}
					if (tc.body != "" && strings.Contains(symbol.Signature, tc.body)) || strings.Contains(symbol.Signature, "    ") {
						t.Errorf("%s signature contains body: %q", tc.name, symbol.Signature)
					}
				}
			}
			for _, tc := range cases {
				if !seen[tc.name] {
					t.Errorf("missing Kotlin method %s", tc.name)
				}
			}
		})
	}
}
