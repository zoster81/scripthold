package sourceintelligence

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
)

type rustEnumVariantOracle struct {
	name string
	line int
	kind SymbolKind
}

func TestRustEnumVariantsTwoOriginalProjects(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		want, neighbors   []rustEnumVariantOracle
	}{
		{
			name: "tokio", env: "SCRIPTHOLD_REAL_RUST_ENUM_TOKIO",
			marker: "pub enum Either<L, R> {",
			want: []rustEnumVariantOracle{
				{"Either.Left", 66, SymbolKindConstant},
				{"Either.Right", 67, SymbolKindConstant},
			},
			neighbors: []rustEnumVariantOracle{
				{"Either", 65, SymbolKindEnum},
				{"delegate_call", 73, SymbolKindEntity},
			},
		},
		{
			name: "serde-json", env: "SCRIPTHOLD_REAL_RUST_ENUM_SERDE",
			marker: "pub enum ErrorKind {",
			want:   []rustEnumVariantOracle{{"ErrorKind.Other", 9, SymbolKindConstant}},
			neighbors: []rustEnumVariantOracle{
				{"ErrorKind", 8, SymbolKindEnum},
				{"Error", 14, SymbolKindStruct},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original Rust source not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 20000 {
				t.Fatalf("invalid original %v %v", fi, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(b)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("original enum marker drift")
			}
			checkRustEnumVariantSource(t, source, tc.want, tc.neighbors)
		})
	}
	const portable = "pub enum Choice {\n Empty,\n Pair(u32, (u8, u8)),\n Named { value: u32 },\n Number = 3,\n}\nmod nested {\n pub enum State { Ready, Waiting(u8) }\n pub fn next() {}\n}\nfn after() {}\n"
	for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(nl.name, func(t *testing.T) {
			source := strings.ReplaceAll(portable, "\n", nl.value)
			checkRustEnumVariantSource(t, source,
				[]rustEnumVariantOracle{
					{"Choice.Empty", 2, SymbolKindConstant},
					{"Choice.Pair", 3, SymbolKindConstant},
					{"Choice.Named", 4, SymbolKindConstant},
					{"Choice.Number", 5, SymbolKindConstant},
					{"nested.State.Ready", 8, SymbolKindConstant},
					{"nested.State.Waiting", 8, SymbolKindConstant},
				},
				[]rustEnumVariantOracle{
					{"Choice", 1, SymbolKindEnum},
					{"nested", 7, SymbolKindModule},
					{"nested.State", 8, SymbolKindEnum},
					{"nested.next", 9, SymbolKindFunction},
					{"after", 11, SymbolKindFunction},
				})
		})
	}
}

func TestRustEnumVariantsUnsupportedFailClosed(t *testing.T) {
	tooMany := "enum Many {\n"
	for i := 0; i < 130; i++ {
		tooMany += " V" + strconv.Itoa(i) + ",\n"
	}
	tooMany += "}\nfn after() {}\n"
	cases := []struct{ name, source string }{
		{"duplicate", "enum Bad { A, A }\nfn after() {}\n"},
		{"empty-entry", "enum Bad { A,, B }\nfn after() {}\n"},
		{"quoted-name", "enum Bad { \"A\" }\nfn after() {}\n"},
		{"macro-invocation", "enum Bad { generated!(), Other }\nfn after() {}\n"},
		{"unsupported-suffix", "enum Bad { A(u8) extra }\nfn after() {}\n"},
		{"too-many", tooMany},
		{"too-many-bytes", "enum Bad { A," + strings.Repeat(" ", 17000) + "B }\nfn after() {}\n"},
	}
	for _, tc := range cases {
		for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+nl.name, func(t *testing.T) {
				doc := sourceDocumentForScanner(strings.ReplaceAll(tc.source, "\n", nl.value))
				doc.Path = "invalid.rs"
				res, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				after := false
				for _, s := range res.Analysis.Symbols {
					if s.QualifiedName == "after" && s.Kind == SymbolKindFunction {
						after = true
					}
					if s.NativeKind == "enum-variant" {
						t.Errorf("unsupported enum invented a complete variant %+v", s)
					}
				}
				if !after {
					t.Error("unsupported enum consumed a neighboring authored function")
				}
				found := false
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "rust-unsupported-enum-variants" && d.Range != nil {
						found = true
					}
				}
				if !found || res.Analysis.CoverageComplete {
					t.Errorf("unsupported enum must be source-ranged incomplete: diagnostics=%+v", res.Analysis.Diagnostics)
				}
			})
		}
	}
}

func checkRustEnumVariantSource(t *testing.T, source string, want, neighbors []rustEnumVariantOracle) {
	t.Helper()
	doc := sourceDocumentForScanner(source)
	doc.Path = "original.rs"
	res, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range append(append([]rustEnumVariantOracle{}, want...), neighbors...) {
		var found bool
		for _, s := range res.Analysis.Symbols {
			if s.QualifiedName == w.name && s.Kind == w.kind && s.NameRange.Start.Line == w.line {
				if w.kind == SymbolKindConstant && (s.NativeKind != "enum-variant" || s.ParentQualifiedName == "" || s.DeclarationRange.End.Line < w.line) {
					t.Errorf("owned Rust enum variant has incorrect range, owner or native kind %+v", s)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing independently inventoried Rust symbol %v (coverage=%v diagnostics=%+v)", w, res.Analysis.CoverageComplete, res.Analysis.Diagnostics)
		}
	}
}
