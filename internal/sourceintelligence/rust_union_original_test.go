package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type rustUnionOracle struct {
	name string
	line int
	kind SymbolKind
}

func TestRustUnionTwoIndependentOriginalProjects(t *testing.T) {
	for _, tc := range []struct {
		name, env, anchor string
		want, adjacent    []rustUnionOracle
	}{
		{
			name: "embassy", env: "SCRIPTHOLD_REAL_RUST_UNION_EMBASSY", anchor: "pub union KbOptionsParams {",
			want: []rustUnionOracle{
				{"KbOptionsParams", 73, SymbolKindType},
				{"KbOptionsParams.authenticate", 74, SymbolKindField},
				{"KbOptionsParams.load_sb", 75, SymbolKindField},
			},
			adjacent: []rustUnionOracle{{"KbOptions", 80, SymbolKindStruct}, {"KbAuthenticate", 60, SymbolKindStruct}},
		},
		{
			name: "windows-rs", env: "SCRIPTHOLD_REAL_RUST_UNION_WINDOWS", anchor: "pub union IMAGE_COR20_HEADER_0 {",
			want: []rustUnionOracle{
				{"IMAGE_COR20_HEADER_0", 26, SymbolKindType},
				{"IMAGE_COR20_HEADER_0.EntryPointToken", 27, SymbolKindField},
				{"IMAGE_COR20_HEADER_0.EntryPointRVA", 28, SymbolKindField},
				{"IMAGE_SECTION_HEADER_0", 192, SymbolKindType},
				{"IMAGE_SECTION_HEADER_0.PhysicalAddress", 193, SymbolKindField},
				{"IMAGE_SECTION_HEADER_0.VirtualSize", 194, SymbolKindField},
			},
			adjacent: []rustUnionOracle{{"IMAGE_COR20_HEADER", 5, SymbolKindStruct}, {"IMAGE_SECTION_HEADER", 173, SymbolKindStruct}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("complete independent original Rust union file not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 1<<16 {
				t.Fatalf("invalid independent original: %v %v", fi, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			text := string(b)
			if !strings.Contains(text, tc.anchor) {
				t.Fatal("full original provenance anchor drift")
			}
			checkRustUnionSource(t, text, tc.want, tc.adjacent)
		})
	}
	const source = "#[repr(C)]\npub union Slots<T> {\n pub left: T,\n pub right: u64,\n}\nmod outer {\n pub union Packet { pub code: u32, pub bits: f32 }\n pub struct After;\n}\nfn after() {}\n"
	for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(nl.name, func(t *testing.T) {
			text := strings.ReplaceAll(source, "\n", nl.value)
			checkRustUnionSource(t, text, []rustUnionOracle{
				{"Slots", 2, SymbolKindType}, {"Slots.left", 3, SymbolKindField}, {"Slots.right", 4, SymbolKindField},
				{"outer.Packet", 7, SymbolKindType}, {"outer.Packet.code", 7, SymbolKindField}, {"outer.Packet.bits", 7, SymbolKindField},
			}, []rustUnionOracle{{"outer", 6, SymbolKindModule}, {"outer.After", 8, SymbolKindStruct}, {"after", 10, SymbolKindFunction}})
		})
	}
}

func TestRustUnionMalformedAndOpaque(t *testing.T) {
	cases := []struct{ name, source string }{
		{"missing-name", "union { pub ghost: u32 }\nfn after() {}\n"},
		{"numeric-name", "union 12 { pub ghost: u32 }\nfn after() {}\n"},
		{"missing-body", "union Wrong;\nfn after() {}\n"},
		{"interrupted-header", "union Broken fn after() {}\n"},
		{"oversized-body", "union Huge {" + strings.Repeat(" pub ghost: u32,", 1500) + " }\nfn after() {}\n"},
	}
	for _, tc := range cases {
		for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+nl.name, func(t *testing.T) {
				src := strings.ReplaceAll(tc.source, "\n", nl.value)
				doc := sourceDocumentForScanner(src)
				doc.Path = "invalid.rs"
				res, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
				if err != nil {
					t.Fatal(err)
				}
				var after bool
				for _, s := range res.Analysis.Symbols {
					if s.Kind == SymbolKindFunction && s.Name == "after" {
						after = true
					}
					if s.Kind == SymbolKindType && s.NativeKind == "union" {
						t.Errorf("malformed/oversized union invented as complete declaration: %+v", s)
					}
					if s.Kind == SymbolKindField && s.Name == "ghost" {
						t.Errorf("malformed union invented a field: %+v", s)
					}
				}
				if !after {
					t.Error("malformed union swallowed neighboring function")
				}
				hasDiagnostic := false
				for _, d := range res.Analysis.Diagnostics {
					if d.Code == "rust-unsupported-union" && d.Range != nil {
						hasDiagnostic = true
					}
				}
				if !hasDiagnostic || res.Analysis.CoverageComplete {
					t.Errorf("malformed union must mark incomplete with source-ranged diagnostic: %+v", res.Analysis.Diagnostics)
				}
			})
		}
	}
	const generated = "macro_rules! hidden { () => { union Invented { pub ghost: u32 } }; }\nhidden!();\npub struct Actual;\n"
	doc := sourceDocumentForScanner(generated)
	doc.Path = "macro_opaque.rs"
	res, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	actual := false
	for _, s := range res.Analysis.Symbols {
		if s.Name == "Actual" && s.Kind == SymbolKindStruct {
			actual = true
		}
		if s.Name == "Invented" || s.Name == "ghost" {
			t.Errorf("unexpanded macro invented source declaration: %+v", s)
		}
	}
	if !actual {
		t.Error("macro hiding union swallowed adjacent actual declaration")
	}
}

func checkRustUnionSource(t *testing.T, src string, wants, adjacent []rustUnionOracle) {
	t.Helper()
	doc := sourceDocumentForScanner(src)
	doc.Path = "original.rs"
	res, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range append(append([]rustUnionOracle{}, wants...), adjacent...) {
		var s NormalizedSymbol
		ok := false
		for _, candidate := range res.Analysis.Symbols {
			if candidate.QualifiedName == w.name && candidate.Kind == w.kind && candidate.NameRange.Start.Line == w.line {
				s, ok = candidate, true
				break
			}
		}
		if !ok {
			t.Errorf("independently inventoried Rust symbol %v absent among %v", w, sortedSymbolQualifiedNames(res.Analysis.Symbols))
			continue
		}
		if w.kind == SymbolKindType && (s.NativeKind != "union" || s.BodyRange == nil || s.SignatureRange == nil) {
			t.Errorf("Rust union lacks structural body/header: %+v", s)
		}
		if w.kind == SymbolKindField && s.ParentQualifiedName == "" {
			t.Errorf("Rust union field lacks source ownership: %+v", s)
		}
	}
}
