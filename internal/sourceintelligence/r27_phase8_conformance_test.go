package sourceintelligence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/zoster81/scripthold/internal/operation"
)

func TestR27Phase8ConformanceAcrossEncodingsAndDeterminism(t *testing.T) {
	tests := []struct {
		name, language, extension, encoding, text string
		bom                                       bool
		want                                      []string
	}{
		{name: "perl-windows1252-crlf", language: "perl", extension: ".pl", encoding: "windows-1252", text: "# café\r\npackage Demo;\r\nsub run { 1 }\r\n", want: []string{"Demo", "Demo.run"}},
		{name: "lua-utf16le", language: "lua", extension: ".lua", encoding: "utf-16le", bom: true, text: "-- résumé\nfunction run() end\n", want: []string{"run"}},
		{name: "luau-utf16be", language: "luau", extension: ".luau", encoding: "utf-16be", bom: true, text: "export type User = { name: string }\nlocal function run(): User return { name = \"x\" } end\n", want: []string{"User", "run"}},
		{name: "elixir-utf32le", language: "elixir", extension: ".ex", encoding: "utf-32le", bom: true, text: "# café\ndefmodule Demo do\n  def run(), do: :ok\nend\n", want: []string{"Demo", "Demo.run"}},
		{name: "erlang-windows1252", language: "erlang", extension: ".erl", encoding: "windows-1252", text: "% café\n-module(demo).\nrun() -> ok.\n", want: []string{"demo", "demo.run"}},
		{name: "gleam-utf16le", language: "gleam", extension: ".gleam", encoding: "utf-16le", bom: true, text: "// résumé\npub type User { User(name: String) }\npub fn run() { Nil }\n", want: []string{"User", "run"}},
		{name: "groovy-windows1252-crlf", language: "groovy", extension: ".groovy", encoding: "windows-1252", text: "// café\r\npackage demo\r\nclass Worker { def run() {} }\r\n", want: []string{"demo", "demo.Worker", "demo.Worker.run"}},
		{name: "shell-utf16be", language: "shell", extension: ".shx", encoding: "utf-16be", bom: true, text: "# résumé\nbuild() { :; }\n", want: []string{"build"}},
		{name: "bash-utf32le", language: "bash", extension: ".bash", encoding: "utf-32le", bom: true, text: "# café\nfunction deploy() { :; }\n", want: []string{"deploy"}},
		{name: "tcl-windows1252", language: "tcl", extension: ".tcl", encoding: "windows-1252", text: "# café\nnamespace eval demo { proc run {} {} }\n", want: []string{"demo", "demo.run"}},
		{name: "autohotkey-utf16le", language: "autohotkey", extension: ".ahk", encoding: "utf-16le", bom: true, text: "; résumé\nclass Worker { Run() { return 1 } }\n", want: []string{"Worker", "Worker.Run"}},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fixture"+tc.extension)
			if err := os.WriteFile(path, encodeSourceFixture(t, tc.encoding, tc.text, tc.bom), 0o600); err != nil {
				t.Fatal(err)
			}
			document, err := OpenSourceDocument(context.Background(), path, OpenDocumentOptions{RequestedEncoding: tc.encoding, MaxFileBytes: 4 * 1024 * 1024, MaxDecodedCharacters: 1_000_000})
			if err != nil {
				t.Fatal(err)
			}
			descriptor, _ := registry.Resolve(tc.language)
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatalf("missing analyzer %s", tc.language)
			}
			first, err := analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			second, err := analyzer.Analyze(context.Background(), document, phase3AnalyzeOptions(true, 256))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("%s output is nondeterministic", tc.language)
			}
			if !first.Analysis.CoverageComplete || first.Analysis.Truncated {
				t.Fatalf("%s conformance partial: %+v", tc.language, first.Analysis)
			}
			names := sortedSymbolQualifiedNames(first.Analysis.Symbols)
			for _, want := range tc.want {
				if !containsSortedString(names, want) {
					t.Fatalf("%s missing %s; symbols=%v", tc.language, want, names)
				}
			}
		})
	}
}

func TestR27Phase8ProductionOpaqueAndMultilineBoundaries(t *testing.T) {
	t.Run("perl-quoted-heredoc", func(t *testing.T) {
		text := "my $data = <<'EOF';\nsub HeredocFake {}\nEOF\nsub Real {}\n"
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		if containsSortedString(names, "HeredocFake") || !containsSortedString(names, "Real") {
			t.Fatalf("Perl heredoc leaked declarations: %v", names)
		}
	})

	t.Run("perl-bareword-heredoc", func(t *testing.T) {
		text := "my $data = <<EOF;\nsub HeredocFake {}\nEOF\nsub Real {}\n"
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		if containsSortedString(names, "HeredocFake") || !containsSortedString(names, "Real") {
			t.Fatalf("Perl bareword heredoc leaked declarations: %v", names)
		}
	})

	t.Run("perl-quote-like-operators", func(t *testing.T) {
		text := "my $q = q{sub QFake { [ } apostrophe's };\n" +
			"my $qq = qq(sub QQFake { ] });\n" +
			"my @words = qw{alpha beta [ gamma};\n" +
			"my $compiled = qr{foo\\e[K[bar]};\n" +
			"if ($value =~ m{foo\\e[K[bar]}) { return 1; }\n" +
			"$value =~ s{foo[bar]}{replacement \\e[K};\n" +
			"$value =~ tr{abc[]}{xyz{} };\n" +
			"$value =~ y/a[b]/x{y}/;\n" +
			"sub Real {}\n"
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Perl quote-like operators reported partial: %+v", result.Analysis)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, fake := range []string{"QFake", "QQFake"} {
			if containsSortedString(names, fake) {
				t.Fatalf("Perl quote-like operator leaked %s: %v", fake, names)
			}
		}
		if !containsSortedString(names, "Real") {
			t.Fatalf("Perl real function missing after quote-like operators: %v", names)
		}
	})

	t.Run("perl-bare-slash-regex", func(t *testing.T) {
		text := "sub Real {\n" +
			"  my $header = shift;\n" +
			"  return 1 if $header =~ /\\b(foo|bar)-?(\\d[\\d.]*)?\\b/;\n" +
			"  my @lines = grep { /./ && !/^\\s*#/ } qw(alpha beta);\n" +
			"  return scalar @lines;\n" +
			"}\n"
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Perl slash regex reported partial: %+v", result.Analysis)
		}
		if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "Real") {
			t.Fatalf("Perl real function missing after slash regex: %v", names)
		}
	})

	t.Run("perl-division-not-slash-regex", func(t *testing.T) {
		text := "sub Real {\n" +
			"  my $ratio = $a / ($b + $c);\n" +
			"  my $scaled = $ratio / 2;\n" +
			"  return $scaled;\n" +
			"}\n"
		masked, complete := maskPerlNonCode(text)
		if !complete || masked != text {
			t.Fatalf("Perl division was altered by non-code masking: complete=%v masked=%q", complete, masked)
		}
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Perl division reported partial: %+v", result.Analysis)
		}
	})

	t.Run("perl-quote-like-letter-hash-keys", func(t *testing.T) {
		text := "sub Real {\n" +
			"  my %opt;\n" +
			"  $opt{m} = 1;\n" +
			"  $opt->{q} = 2;\n" +
			"  $opt{ s } = 3;\n" +
			"  $opt->{ y } = 4;\n" +
			"  return $opt{m};\n" +
			"}\n"
		masked, complete := maskPerlNonCode(text)
		if !complete || masked != text {
			t.Fatalf("Perl hash keys were altered by quote-like masking: complete=%v masked=%q", complete, masked)
		}
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Perl quote-like-letter hash keys reported partial: %+v", result.Analysis)
		}
	})

	t.Run("perl-closing-delimiter-quote-like-operator", func(t *testing.T) {
		text := "sub Real {\n" +
			"  return 1 if m}foo};\n" +
			"}\n"
		masked, complete := maskPerlNonCode(text)
		if !complete || masked == text {
			t.Fatalf("Perl closing-delimiter quote-like operator was not masked: complete=%v masked=%q", complete, masked)
		}
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Perl closing-delimiter quote-like operator reported partial: %+v", result.Analysis)
		}
	})

	t.Run("perl-leading-equals-continuation-is-not-pod", func(t *testing.T) {
		text := "sub Real {\n" +
			"  my $filter\n" +
			"      = $enabled ? sub { 1 }\n" +
			"      : sub { 0 };\n" +
			"  return $filter;\n" +
			"}\n"
		masked, complete := maskPerlNonCode(text)
		if !complete || masked != text {
			t.Fatalf("Perl leading-equals continuation was treated as POD: complete=%v masked=%q", complete, masked)
		}
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Perl leading-equals continuation reported partial: %+v", result.Analysis)
		}
	})

	t.Run("perl-slash-regex-comment-marker", func(t *testing.T) {
		text := "sub Real {\n" +
			"  if ( $header =~ /^#!/ ) {\n" +
			"    return ($1) if $header =~ /\\b(foo|bar)\\b/;\n" +
			"  }\n" +
			"  my @lines = grep { /./ && !/^\\s*#/ } @input;\n" +
			"  my $ratio = $a * $b * $c;\n" +
			"  return scalar @lines;\n" +
			"}\n"
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid Perl slash regex containing comment markers reported partial: %+v", result.Analysis)
		}
		if names := sortedSymbolQualifiedNames(result.Analysis.Symbols); !containsSortedString(names, "Real") {
			t.Fatalf("Perl real function missing after slash regex comment markers: %v", names)
		}
	})

	t.Run("perl-namespaced-print-parenthesized-heredoc", func(t *testing.T) {
		text := "sub Real {\n" +
			"  App::Ack::print( <<'END_OF_HELP' );\n" +
			"sub HeredocFake {\n" +
			"line(s) [text]\n" +
			"END_OF_HELP\n" +
			"}\n" +
			"sub After {}\n"
		result, err := (PerlAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
			t.Fatalf("valid parenthesized Perl print heredoc reported partial: %+v", result.Analysis)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		if containsSortedString(names, "HeredocFake") || !containsSortedString(names, "Real") || !containsSortedString(names, "After") {
			t.Fatalf("Perl parenthesized print heredoc symbols=%v", names)
		}
	})

	t.Run("groovy-slashy-and-dollar-slashy", func(t *testing.T) {
		text := "def a = /class SlashyFake { def nope() {} }/\ndef b = $/class DollarFake { def nope2() {} }/$\ndef real() {}\n"
		result, err := (GroovyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, fake := range []string{"SlashyFake", "SlashyFake.nope", "DollarFake", "DollarFake.nope2"} {
			if containsSortedString(names, fake) {
				t.Fatalf("Groovy slashy string leaked %s: %v", fake, names)
			}
		}
		if !containsSortedString(names, "real") {
			t.Fatalf("Groovy real function missing: %v", names)
		}
	})

	t.Run("autohotkey-backtick-escaped-quote", func(t *testing.T) {
		text := "value := \"text`\" FakeString() { return 0 }\"\nReal() { return 1 }\n"
		result, err := (AutoHotkeyAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		if containsSortedString(names, "FakeString") || !containsSortedString(names, "Real") {
			t.Fatalf("AutoHotkey escaped string boundary symbols=%v", names)
		}
	})

	t.Run("elixir-multiline-function-scope", func(t *testing.T) {
		text := "defmodule Demo.Worker do\n  def run(\n    value\n  ) do\n    if value do\n      value\n    end\n  end\n  def next(), do: :ok\nend\n"
		result, err := (ElixirAnalyzer{}).Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 64))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Analysis.CoverageComplete {
			t.Fatalf("Elixir multiline function lowered coverage: %+v", result.Analysis.Diagnostics)
		}
		names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
		for _, want := range []string{"Demo.Worker", "Demo.Worker.run", "Demo.Worker.next"} {
			if !containsSortedString(names, want) {
				t.Fatalf("Elixir multiline scope missing %s: %v", want, names)
			}
		}
	})
}

func TestR27Phase8MalformedCancellationAndLimits(t *testing.T) {
	malformed := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
	}{
		{"perl", PerlAnalyzer{}, "sub good {}\n\"unterminated"},
		{"lua", LuaAnalyzer{}, "function good() end\n\"unterminated"},
		{"luau", LuauAnalyzer{}, "local function good() end\n\"unterminated"},
		{"elixir", ElixirAnalyzer{}, "def good(), do: :ok\n\"\"\"unterminated"},
		{"erlang", ErlangAnalyzer{}, "good() -> ok.\n\"unterminated"},
		{"gleam", GleamAnalyzer{}, "pub fn good() { Nil }\n\"unterminated"},
		{"groovy", GroovyAnalyzer{}, "def good() {}\n/* unterminated"},
		{"shell", ShellAnalyzer{}, "good() { :; }\ncat <<'EOF'\nunterminated\n"},
		{"bash", BashAnalyzer{}, "good() { :; }\n\"unterminated"},
		{"tcl", TclAnalyzer{}, "proc good {} {}\n\"unterminated"},
		{"autohotkey", AutoHotkeyAnalyzer{}, "Good() { return 1 }\n\"unterminated"},
	}
	for _, tc := range malformed {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(false, 64))
			if err != nil {
				t.Fatal(err)
			}
			if result.Analysis.CoverageComplete || len(result.Analysis.Diagnostics) == 0 {
				t.Fatalf("%s malformed source did not lower coverage: %+v", tc.name, result.Analysis)
			}
		})
	}

	for _, analyzer := range phase8Analyzers() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := analyzer.Analyze(ctx, sourceDocumentForScanner("function good() {}\n"), phase3AnalyzeOptions(false, 16))
		if operation.KindOf(err) != operation.KindCancelled {
			t.Fatalf("%s cancellation err=%v kind=%v", analyzer.Language(), err, operation.KindOf(err))
		}
	}

	for _, tc := range []struct {
		language string
		analyzer SourceAnalyzer
		text     string
	}{
		{"perl", PerlAnalyzer{}, generatedPhase8Perl(1200)},
		{"lua", LuaAnalyzer{}, generatedPhase8Lua(1200)},
		{"luau", LuauAnalyzer{}, generatedPhase8Lua(1200)},
		{"elixir", ElixirAnalyzer{}, generatedPhase8Elixir(1200)},
		{"erlang", ErlangAnalyzer{}, generatedPhase8Erlang(1200)},
		{"gleam", GleamAnalyzer{}, generatedPhase8Gleam(1200)},
		{"groovy", GroovyAnalyzer{}, generatedPhase8Groovy(1200)},
		{"shell", ShellAnalyzer{}, generatedPhase8Shell(1200)},
		{"bash", BashAnalyzer{}, generatedPhase8Shell(1200)},
		{"tcl", TclAnalyzer{}, generatedPhase8Tcl(1200)},
		{"autohotkey", AutoHotkeyAnalyzer{}, generatedPhase8AHK(1200)},
	} {
		t.Run("limit-"+tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Analysis.Symbols) != 128 || !result.Analysis.Truncated || result.Analysis.CoverageComplete {
				t.Fatalf("%s bounded result: symbols=%d truncated=%v complete=%v diagnostics=%+v", tc.language, len(result.Analysis.Symbols), result.Analysis.Truncated, result.Analysis.CoverageComplete, result.Analysis.Diagnostics)
			}
		})
	}
}

func TestR27Phase8DetectionIsConservativeAndKeepsShellProvidersDistinct(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, text, want string }{
		{"script.pl", "use strict;\npackage Demo;\nsub run {}\n", "perl"},
		{"main.lua", "local function run() end\n", "lua"},
		{"main.luau", "--!strict\nexport type User = { name: string }\n", "luau"},
		{"worker.ex", "defmodule Demo do\nend\n", "elixir"},
		{"worker.erl", "-module(worker).\nrun() -> ok.\n", "erlang"},
		{"main.gleam", "pub fn run() { Nil }\n", "gleam"},
		{"build.groovy", "def run() {}\n", "groovy"},
		{"build.sh", "#!/bin/sh\nbuild() { :; }\n", "bash"},
		{"main.tcl", "proc run {} {}\n", "tcl"},
		{"main.ahk", "#Requires AutoHotkey v2.0\nRunIt() { return 1 }\n", "autohotkey"},
	} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: tc.path, Text: tc.text})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionProbable || result.Language != tc.want {
			t.Fatalf("%s detection=%+v want probable %s", tc.path, result, tc.want)
		}
	}

	explicitShell, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "script", Text: "build() { :; }\n", ExplicitLanguage: "shell"})
	if err != nil {
		t.Fatal(err)
	}
	if explicitShell.State != DetectionExact || explicitShell.Language != "shell" {
		t.Fatalf("explicit POSIX shell detection=%+v", explicitShell)
	}

	for _, tc := range []struct{ text, want string }{
		{"defmodule Demo.Worker do\nend\n", "elixir"},
		{"-module(worker).\nrun() -> ok.\n", "erlang"},
		{"--!strict\nexport type User = { name: string }\n", "luau"},
		{"#Requires AutoHotkey v2.0\nRunIt() { return 1 }\n", "autohotkey"},
	} {
		result, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "source", Text: tc.text})
		if err != nil {
			t.Fatal(err)
		}
		if result.State != DetectionProbable || result.Language != tc.want || !hasDetectionEvidence(result, EvidenceContentMarker) {
			t.Fatalf("content-only %s detection=%+v", tc.want, result)
		}
	}

	plain, err := DetectLanguage(context.Background(), registry, DetectionInput{Path: "source", Text: "function run() { return 1 }\n"})
	if err != nil {
		t.Fatal(err)
	}
	if plain.Language == "shell" || plain.Language == "bash" {
		t.Fatalf("generic function syntax overdetected as shell: %+v", plain)
	}
}

func generatedPhase8Perl(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "sub f%04d { 1 }\n", i)
	}
	return b.String()
}

func generatedPhase8Lua(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "function f%04d() end\n", i)
	}
	return b.String()
}

func generatedPhase8Elixir(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "def f%04d(), do: :ok\n", i)
	}
	return b.String()
}

func generatedPhase8Erlang(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "f%04d() -> ok.\n", i)
	}
	return b.String()
}

func generatedPhase8Gleam(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "pub fn f%04d() { Nil }\n", i)
	}
	return b.String()
}

func generatedPhase8Groovy(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "def f%04d() {}\n", i)
	}
	return b.String()
}

func generatedPhase8Shell(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "f%04d() { :; }\n", i)
	}
	return b.String()
}

func generatedPhase8Tcl(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "proc f%04d {} {}\n", i)
	}
	return b.String()
}

func generatedPhase8AHK(count int) string {
	var b strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&b, "F%04d() { return 1 }\n", i)
	}
	return b.String()
}

func phase8Analyzers() []SourceAnalyzer {
	return []SourceAnalyzer{
		PerlAnalyzer{}, LuaAnalyzer{}, LuauAnalyzer{}, ElixirAnalyzer{}, ErlangAnalyzer{}, GleamAnalyzer{}, GroovyAnalyzer{}, ShellAnalyzer{}, BashAnalyzer{}, TclAnalyzer{}, AutoHotkeyAnalyzer{},
	}
}

func FuzzR27Phase8AnalyzersNoPanic(f *testing.F) {
	seeds := []struct {
		text     string
		selector uint8
	}{
		{"package Demo;\nsub run {}\n", 0},
		{"function run() end\n", 1},
		{"export type User = { name: string }\n", 2},
		{"defmodule Demo do\n  def run(), do: :ok\nend\n", 3},
		{"-module(demo).\nrun() -> ok.\n", 4},
		{"pub fn run() { Nil }\n", 5},
		{"class Worker { def run() {} }\n", 6},
		{"run() { :; }\n", 7},
		{"function run() { :; }\n", 8},
		{"proc run {} {}\n", 9},
		{"class Worker { Run() { return 1 } }\n", 10},
	}
	for _, seed := range seeds {
		f.Add(seed.text, seed.selector)
	}
	f.Fuzz(func(t *testing.T, text string, selector uint8) {
		analyzers := phase8Analyzers()
		analyzer := analyzers[int(selector)%len(analyzers)]
		result, err := analyzer.Analyze(context.Background(), sourceDocumentForScanner(text), phase3AnalyzeOptions(false, 128))
		if err != nil {
			if kind := operation.KindOf(err); kind != operation.KindInvalidInput && kind != operation.KindLimit && kind != operation.KindUnsupported {
				t.Fatalf("unexpected %s fuzz error: %v kind=%v", analyzer.Language(), err, kind)
			}
			return
		}
		if len(result.Analysis.Symbols) > 128 {
			t.Fatalf("%s fuzz result exceeded symbol bound: %d", analyzer.Language(), len(result.Analysis.Symbols))
		}
		for _, symbol := range result.Analysis.Symbols {
			if symbol.Name == "" || symbol.QualifiedName == "" || symbol.DeclarationRange.Start.Line <= 0 || symbol.DeclarationRange.Start.Column <= 0 {
				t.Fatalf("%s fuzz emitted invalid symbol: %+v", analyzer.Language(), symbol)
			}
		}
	})
}
