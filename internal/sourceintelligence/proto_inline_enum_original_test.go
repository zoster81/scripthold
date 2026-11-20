package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type protoInlineEnumWant struct {
	qualified, parent, member string
	line                      int
}

// Source oracles are complete independently authored upstream files or
// verbatim protobuf literals extracted from a complete upstream host file.
func TestProtoInlineEnumOriginalSources(t *testing.T) {
	const fixture = "syntax = \"proto2\";\n" +
		"enum Foo { FOO_UNKNOWN = 1;}\n" +
		"enum Bar {BAR_UNKNOWN=1;}\n" +
		"enum Thud { VALUE = 0; }\n" +
		"enum Multi { ZERO = 0; ONE = 1; }\n" +
		"// authored comment must not define members\n"
	want := []protoInlineEnumWant{
		{"Foo.FOO_UNKNOWN", "Foo", "FOO_UNKNOWN", 2},
		{"Bar.BAR_UNKNOWN", "Bar", "BAR_UNKNOWN", 3},
		{"Thud.VALUE", "Thud", "VALUE", 4},
		{"Multi.ZERO", "Multi", "ZERO", 5},
		{"Multi.ONE", "Multi", "ONE", 5},
	}
	for _, tc := range []struct{ name, source string }{
		{"LF", fixture},
		{"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run("portable/"+tc.name, func(t *testing.T) {
			assertProtoInlineEnumValues(t, tc.source, want)
		})
	}
	t.Run("unsupported-single-line-fail-closed", func(t *testing.T) {
		for _, body := range []string{
			"ZERO = 0; ONE = ;",
			"ZERO=0, ONE=1;",
			"ZERO=0; ONE=1 [(deprecated)=true];",
			"ZERO=+1;",
			"ZERO=0x10;",
			"ZERO=0; reserved 1 to 2;",
			strings.Repeat("LONG=1; ", 800),
			strings.Repeat("X=1; ", 129),
		} {
			source := "enum Unsupported { " + body + " }\nenum Stable { STABLE=0; NEXT=1; }\n"
			res, err := (ProtoAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source),
				testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			byName := symbolsByQualifiedName(res.Analysis.Symbols)
			for _, name := range []string{"Stable.STABLE", "Stable.NEXT"} {
				if _, ok := byName[name]; !ok {
					t.Errorf("valid neighboring proto enum member missing: %s", name)
				}
			}
			for name := range byName {
				if strings.HasPrefix(name, "Unsupported.") {
					t.Errorf("unsupported proto inline enum fabricated %q from %q", name, body)
				}
			}
			found := false
			for _, diagnostic := range res.Analysis.Diagnostics {
				if diagnostic.Code == "proto-unsupported-inline-enum" && diagnostic.Range != nil {
					found = true
				}
			}
			if res.Analysis.CoverageComplete || !found {
				t.Errorf("unsupported proto inline enum missing source-ranged partial warning: %+v", res.Analysis)
			}
		}
	})
	t.Run("negative-values-and-comments", func(t *testing.T) {
		const source = "enum Signed { NEG = -1; /* DO_NOT_INDEX = 3; */ POS=2; }\n"
		assertProtoInlineEnumValues(t, source, []protoInlineEnumWant{
			{"Signed.NEG", "Signed", "NEG", 1},
			{"Signed.POS", "Signed", "POS", 1},
		})
	})
	for _, tc := range []struct {
		name, env, marker string
		wants             []protoInlineEnumWant
	}{
		{
			name:   "buf-protocompile",
			env:    "SCRIPTHOLD_REAL_PROTO_INLINE_BUF",
			marker: "enum Foo { FOO_UNKNOWN = 1;}",
			wants: []protoInlineEnumWant{
				{"Foo.FOO_UNKNOWN", "Foo", "FOO_UNKNOWN", 8},
				{"Bar.BAR_UNKNOWN", "Bar", "BAR_UNKNOWN", 9},
			},
		},
		{
			name:   "buf-protocompile-multi",
			env:    "SCRIPTHOLD_REAL_PROTO_INLINE_BUF_MULTI",
			marker: "enum TestEnum{ ZERO = 0; ONE = 1; }",
			wants: []protoInlineEnumWant{
				{"TestEnum.ZERO", "TestEnum", "ZERO", 1},
				{"TestEnum.ONE", "TestEnum", "ONE", 1},
			},
		},
		{
			name:   "google-protobuf",
			env:    "SCRIPTHOLD_REAL_PROTO_INLINE_GOOGLE",
			marker: "enum Thud { VALUE = 0; }",
			wants: []protoInlineEnumWant{
				{"foo.Thud.VALUE", "foo.Thud", "VALUE", 4},
			},
		},
	} {
		t.Run("original/"+tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Protocol Buffers source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 30 || info.Size() > 1500 {
				t.Fatalf("authored original proto missing, non-regular or oversized: %v %v", info, err)
			}
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(body)
			if !strings.Contains(source, tc.marker) {
				t.Fatalf("authored protobuf source marker drift")
			}
			assertProtoInlineEnumValues(t, source, tc.wants)
		})
	}
}

func assertProtoInlineEnumValues(t *testing.T, source string, wants []protoInlineEnumWant) {
	t.Helper()
	res, err := (ProtoAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(source),
		testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	byName := symbolsByQualifiedName(res.Analysis.Symbols)
	lines := strings.Split(source, "\n")
	for _, w := range wants {
		if w.line < 1 || w.line > len(lines) {
			t.Fatalf("invalid original source line for %+v", w)
		}
		column := strings.Index(lines[w.line-1], w.member) + 1
		if column == 0 {
			t.Fatalf("missing original member token %+v", w)
		}
		value, ok := byName[w.qualified]
		if !ok || value.Kind != SymbolKindConstant ||
			value.NativeKind != "enum-value" ||
			value.ParentQualifiedName != w.parent ||
			value.NameRange.Start.Line != w.line ||
			value.NameRange.Start.Column != column ||
			value.DeclarationRange.Start.Line != w.line {
			t.Errorf("protobuf original inline member %q at %d:%d missing/misclassified: %+v",
				w.qualified, w.line, column, value)
		}
	}
	for _, bogus := range []string{"Foo.LICENSE", "Bar.enum", "Thud.syntax"} {
		if _, ok := byName[bogus]; ok {
			t.Errorf("fabricated protobuf source member %q", bogus)
		}
	}
}
