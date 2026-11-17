package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

type rustMacroWant struct {
	name string
	line int
}

func TestRustMacroRulesTwoOriginalProjects(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker string
		want              []rustMacroWant
		neighbors         []rustMacroWant
		forbidden         []string
	}{
		{
			name: "tokio-stream", env: "SCRIPTHOLD_REAL_RUST_MACRO_TOKIO",
			marker:    "macro_rules! assert_ready_none {",
			want:      []rustMacroWant{{"assert_ready_none", 9}},
			neighbors: []rustMacroWant{{"pin_box", 18}},
			forbidden: []string{"DidPoll", "poll_next"},
		},
		{
			name: "rustc-marker", env: "SCRIPTHOLD_REAL_RUST_MACRO_RUSTC",
			marker: "macro_rules! impls_dyn_send_neg {",
			want: []rustMacroWant{
				{"impls_dyn_send_neg", 21}, {"already_send", 54},
				{"impl_dyn_send", 73}, {"impls_dyn_sync_neg", 101},
				{"already_sync", 135}, {"impl_dyn_sync", 156},
			},
			neighbors: []rustMacroWant{{"assert_dyn_sync", 187}, {"IntoDynSyncSend", 196}},
			forbidden: []string{"$t1", "$generics1"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("two complete independent original Rust macro sources not configured")
			}
			fi, err := os.Lstat(path)
			if err != nil || !fi.Mode().IsRegular() || fi.Size() < 100 || fi.Size() > 15000 {
				t.Fatalf("invalid original %v %v", fi, err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source := string(b)
			if !strings.Contains(source, tc.marker) {
				t.Fatal("authored original source marker drift")
			}
			lines := strings.Split(source, "\n")
			checkRustMacroDeclarations(t, source, lines, tc.want, tc.neighbors, tc.forbidden)
		})
	}
	const portable = "#[macro_export]\nmacro_rules! demo {\n () => { struct Generated; fn hidden() {} };\n}\nmod inner {\n macro_rules! local { () => { struct Fiction; } }\n pub fn after() {}\n}\nfn final_fn() {}\n"
	for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
		t.Run(nl.name, func(t *testing.T) {
			s := strings.ReplaceAll(portable, "\n", nl.value)
			checkRustMacroDeclarations(t, s, strings.Split(s, nl.value),
				[]rustMacroWant{{"demo", 2}, {"inner.local", 6}},
				[]rustMacroWant{{"inner", 5}, {"inner.after", 7}, {"final_fn", 9}},
				[]string{"Generated", "hidden", "Fiction"})
		})
	}
}

func TestRustMacroRulesInvalidSyntaxFailClosed(t *testing.T) {
	cases := []struct {
		name, src string
	}{
		{"no-name", "macro_rules! { () => { struct Fiction; fn ghost() {} } }\nfn after() {}\n"},
		{"numeric-name", "macro_rules! 123 { () => { struct Fiction; fn ghost() {} } }\nfn after() {}\n"},
		{"missing-bang", "macro_rules nope { () => { struct Fiction; fn ghost() {} } }\nfn after() {}\n"},
	}
	for _, tc := range cases {
		for _, nl := range []struct{ name, value string }{{"LF", "\n"}, {"CRLF", "\r\n"}} {
			t.Run(tc.name+"/"+nl.name, func(t *testing.T) {
				source := strings.ReplaceAll(tc.src, "\n", nl.value)
				doc := sourceDocumentForScanner(source)
				doc.Path = "malformed.rs"
				result, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 64))
				if err != nil {
					t.Fatal(err)
				}
				symbols := symbolsByQualifiedName(result.Analysis.Symbols)
				if neighbor, ok := symbols["after"]; !ok || neighbor.Kind != SymbolKindFunction || neighbor.NameRange.Start.Line != 2 {
					t.Errorf("Rust malformed macro consumed adjacent genuine function: %v %+v", ok, neighbor)
				}
				for _, bad := range []string{"Fiction", "ghost", "nope", "123"} {
					if symbol, ok := symbols[bad]; ok {
						t.Errorf("malformed macro invented a declaration: %+v", symbol)
					}
				}
				found := false
				for _, d := range result.Analysis.Diagnostics {
					if d.Code == "rust-unsupported-macro-rules" && d.Range != nil {
						found = true
					}
				}
				if !found || result.Analysis.CoverageComplete {
					t.Errorf("unsupported Rust macro should be source-ranged incomplete: coverage=%v diagnostics=%+v", result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
				}
			})
		}
	}
}

func TestRustMacroRulesInvocationRemainsOpaque(t *testing.T) {
	source := "macro_rules! make { () => { struct Fiction; fn ghost() {} }; }\nmake!();\nfn after() {}\n"
	doc := sourceDocumentForScanner(source)
	doc.Path = "invocation.rs"
	res, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 128))
	if err != nil {
		t.Fatal(err)
	}
	nodes := symbolsByQualifiedName(res.Analysis.Symbols)
	if nodes["make"].NativeKind != "macro_rules" || nodes["after"].Kind != SymbolKindFunction {
		t.Fatalf("authored macro/function must remain navigable: %v", sortedSymbolQualifiedNames(res.Analysis.Symbols))
	}
	for _, name := range []string{"Fiction", "ghost"} {
		if _, ok := nodes[name]; ok {
			t.Errorf("unexpanded Rust macro manufactured source symbol %s", name)
		}
	}
	if !res.Analysis.CoverageComplete {
		t.Errorf("valid bounded Rust macro/invocation unexpectedly partial: %+v", res.Analysis.Diagnostics)
	}
}

func checkRustMacroDeclarations(t *testing.T, source string, lines []string, wants, neighbors []rustMacroWant, forbidden []string) {
	t.Helper()
	doc := sourceDocumentForScanner(source)
	doc.Path = "original.rs"
	result, err := (RustAnalyzer{}).Analyze(context.Background(), doc, testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	symbols := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, w := range wants {
		short := w.name[strings.LastIndex(w.name, ".")+1:]
		if w.line < 1 || w.line > len(lines) || !strings.Contains(lines[w.line-1], "macro_rules! "+short) {
			t.Fatalf("independently read source line mismatch: %+v", w)
		}
		sym, ok := symbols[w.name]
		if !ok || sym.Kind != SymbolKindEntity || sym.NativeKind != "macro_rules" || sym.NameRange.Start.Line != w.line {
			t.Errorf("original Rust macro declaration missing/misclassified want=%+v found=%v got=%+v", w, ok, sym)
		}
		if ok && (sym.BodyRange == nil || sym.SignatureRange == nil || sym.DeclarationRange.End.Line < w.line) {
			t.Errorf("original Rust macro %s lacks bounded declaration/header/body source ranges: %+v", w.name, sym)
		}
	}
	for _, w := range neighbors {
		if sym, ok := symbols[w.name]; !ok || sym.NameRange.Start.Line != w.line {
			t.Errorf("adjacent authored declaration lost: %+v got=%+v exists=%v", w, sym, ok)
		}
	}
	for _, n := range forbidden {
		if sym, ok := symbols[n]; ok {
			t.Errorf("opaque macro/invocation leaked false declaration: %+v", sym)
		}
	}
}
