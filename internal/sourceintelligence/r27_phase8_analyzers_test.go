package sourceintelligence

import (
	"context"
	"testing"
)

func TestR27Phase8DynamicBEAMAndScriptingAnalyzersExposeDistinctNativeStructure(t *testing.T) {
	tests := []struct {
		language string
		analyzer SourceAnalyzer
		text     string
		want     map[string]SymbolKind
		deps     []string
	}{
		{
			language: "perl", analyzer: PerlAnalyzer{},
			text: "package Demo::Worker;\nuse JSON::PP;\nrequire \"helpers.pm\";\nsub run { return 1; }\n",
			want: map[string]SymbolKind{"Demo::Worker": SymbolKindModule, "Demo::Worker.run": SymbolKindFunction},
			deps: []string{"JSON::PP", "helpers.pm"},
		},
		{
			language: "lua", analyzer: LuaAnalyzer{},
			text: "local util = require(\"util.core\")\nfunction run(x) return x end\nlocal function helper() return true end\n",
			want: map[string]SymbolKind{"run": SymbolKindFunction, "helper": SymbolKindFunction},
			deps: []string{"util.core"},
		},
		{
			language: "luau", analyzer: LuauAnalyzer{},
			text: "local util = require(\"util\")\nexport type User = { name: string }\nlocal function run(user: User): User return user end\n",
			want: map[string]SymbolKind{"User": SymbolKindType, "run": SymbolKindFunction},
			deps: []string{"util"},
		},
		{
			language: "elixir", analyzer: ElixirAnalyzer{},
			text: "defmodule Demo.Worker do\n  alias Demo.Helper\n  import Enum\n  require Logger\n  def run(x) do\n    x\n  end\n  defp helper(x), do: x\nend\n",
			want: map[string]SymbolKind{"Demo.Worker": SymbolKindModule, "Demo.Worker.run": SymbolKindFunction, "Demo.Worker.helper": SymbolKindFunction},
			deps: []string{"Demo.Helper", "Enum", "Logger"},
		},
		{
			language: "erlang", analyzer: ErlangAnalyzer{},
			text: "-module(worker).\n-include(\"worker.hrl\").\n-import(lists, [map/2]).\n-record(state, {value}).\nrun(X) -> X.\nrun(X) when X > 0 -> X.\nhelper() -> ok.\n",
			want: map[string]SymbolKind{"worker": SymbolKindModule, "worker.state": SymbolKindStruct, "worker.run": SymbolKindFunction, "worker.helper": SymbolKindFunction},
			deps: []string{"worker.hrl", "lists"},
		},
		{
			language: "gleam", analyzer: GleamAnalyzer{},
			text: "import gleam/list\npub type User { User(name: String) }\npub fn run(x: Int) -> Int { x }\nconst answer = 42\n",
			want: map[string]SymbolKind{"User": SymbolKindType, "run": SymbolKindFunction, "answer": SymbolKindConstant},
			deps: []string{"gleam/list"},
		},
		{
			language: "groovy", analyzer: GroovyAnalyzer{},
			text: "package demo\nimport java.time.Instant\ntrait Worker { def run() {} }\nclass Service implements Worker { String name; def run() {} }\ndef top() {}\n",
			want: map[string]SymbolKind{"demo": SymbolKindPackage, "demo.Worker": SymbolKindTrait, "demo.Worker.run": SymbolKindMethod, "demo.Service": SymbolKindClass, "demo.Service.name": SymbolKindField, "demo.Service.run": SymbolKindMethod, "demo.top": SymbolKindFunction},
			deps: []string{"java.time.Instant"},
		},
		{
			language: "shell", analyzer: ShellAnalyzer{},
			text: ". \"./lib.sh\"\nbuild() { echo build; }\n",
			want: map[string]SymbolKind{"build": SymbolKindFunction},
			deps: []string{"./lib.sh"},
		},
		{
			language: "bash", analyzer: BashAnalyzer{},
			text: "source \"./lib.bash\"\nfunction deploy() { echo deploy; }\ntest_fn() { :; }\n",
			want: map[string]SymbolKind{"deploy": SymbolKindFunction, "test_fn": SymbolKindFunction},
			deps: []string{"./lib.bash"},
		},
		{
			language: "tcl", analyzer: TclAnalyzer{},
			text: "package require Tcl 8.6\nsource \"lib.tcl\"\nnamespace eval demo {\n  proc run {x} { return $x }\n}\nproc top {} {}\n",
			want: map[string]SymbolKind{"demo": SymbolKindNamespace, "demo.run": SymbolKindFunction, "top": SymbolKindFunction},
			deps: []string{"Tcl", "lib.tcl"},
		},
		{
			language: "autohotkey", analyzer: AutoHotkeyAnalyzer{},
			text: "#Include \"lib.ahk\"\nclass Worker extends Base {\n  Run(x) { return x }\n}\nTop(x) { return x }\n",
			want: map[string]SymbolKind{"Worker": SymbolKindClass, "Worker.Run": SymbolKindMethod, "Top": SymbolKindFunction},
			deps: []string{"lib.ahk"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.language, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Analysis.CoverageComplete || result.Analysis.Truncated {
				t.Fatalf("%s analysis partial: %+v", tc.language, result.Analysis)
			}
			if tc.analyzer.Language() != tc.language {
				t.Fatalf("language=%q want %q", tc.analyzer.Language(), tc.language)
			}
			byName := symbolsByQualifiedName(result.Analysis.Symbols)
			for name, kind := range tc.want {
				if symbol, ok := byName[name]; !ok || symbol.Kind != kind {
					t.Fatalf("%s missing %s kind=%s; symbol=%+v exists=%v all=%v", tc.language, name, kind, symbol, ok, sortedSymbolQualifiedNames(result.Analysis.Symbols))
				}
			}
			if got := dependencyValues(result.Dependencies); !sameStringSet(got, tc.deps) {
				t.Fatalf("%s dependencies=%v want=%v", tc.language, got, tc.deps)
			}
		})
	}
}

func TestR27Phase8OpaqueDataAndQuotingDoNotLeakDeclarations(t *testing.T) {
	tests := []struct {
		name     string
		analyzer SourceAnalyzer
		text     string
		fake     []string
		real     string
	}{
		{
			name: "perl-pod-data-and-string", analyzer: PerlAnalyzer{},
			text: "=pod\nsub PodFake {}\n=cut\nmy $s = \"sub StringFake {}\";\nsub Real {}\n__DATA__\nsub DataFake {}\n",
			fake: []string{"PodFake", "StringFake", "DataFake"}, real: "Real",
		},
		{
			name: "lua-long-brackets", analyzer: LuaAnalyzer{},
			text: "--[[ function CommentFake() end ]]\nlocal s = [[function StringFake() end]]\nfunction Real() end\n",
			fake: []string{"CommentFake", "StringFake"}, real: "Real",
		},
		{
			name: "elixir-heredoc", analyzer: ElixirAnalyzer{},
			text: "text = \"\"\"\ndef StringFake do\nend\n\"\"\"\ndef real do\n  :ok\nend\n",
			fake: []string{"StringFake"}, real: "real",
		},
		{
			name: "groovy-triple-string", analyzer: GroovyAnalyzer{},
			text: "def s = \"\"\"class StringFake { def nope() {} }\"\"\"\ndef real() {}\n",
			fake: []string{"StringFake", "nope"}, real: "real",
		},
		{
			name: "shell-heredoc-and-quotes", analyzer: ShellAnalyzer{},
			text: "cat <<'EOF'\nfake_data() { :; }\nEOF\nprintf '%s\\n' \"quoted_fake() { :; }\"\nreal() { :; }\n",
			fake: []string{"fake_data", "quoted_fake"}, real: "real",
		},
		{
			name: "bash-heredoc-and-backticks", analyzer: BashAnalyzer{},
			text: "cat <<-EOF\n\tfake_data() { :; }\n\tEOF\necho `printf 'fake_cmd() { :; }'`\nreal() { :; }\n",
			fake: []string{"fake_data", "fake_cmd"}, real: "real",
		},
		{
			name: "tcl-braced-data", analyzer: TclAnalyzer{},
			text: "set data { proc BracedFake {} {} }\n# proc CommentFake {} {}\nproc Real {} {}\n",
			fake: []string{"BracedFake", "CommentFake"}, real: "Real",
		},
		{
			name: "autohotkey-string-comment", analyzer: AutoHotkeyAnalyzer{},
			text: "; FakeComment() {}\ns := \"FakeString() {}\"\nReal() { return 1 }\n",
			fake: []string{"FakeComment", "FakeString"}, real: "Real",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.analyzer.Analyze(context.Background(), sourceDocumentForScanner(tc.text), phase3AnalyzeOptions(false, 128))
			if err != nil {
				t.Fatal(err)
			}
			names := sortedSymbolQualifiedNames(result.Analysis.Symbols)
			for _, fake := range tc.fake {
				if containsSortedString(names, fake) {
					t.Fatalf("%s leaked declaration %s: %v", tc.name, fake, names)
				}
			}
			if !containsSortedString(names, tc.real) {
				t.Fatalf("%s missing real declaration %s: %v", tc.name, tc.real, names)
			}
		})
	}
}

func TestR27Phase8RegistryProviderIdentityAndCapabilityCeilings(t *testing.T) {
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]AnalyzerID{
		"perl":       AnalyzerPerl,
		"lua":        AnalyzerLua,
		"luau":       AnalyzerLuau,
		"elixir":     AnalyzerElixir,
		"erlang":     AnalyzerErlang,
		"gleam":      AnalyzerGleam,
		"groovy":     AnalyzerGroovy,
		"shell":      AnalyzerShell,
		"bash":       AnalyzerBash,
		"tcl":        AnalyzerTcl,
		"autohotkey": AnalyzerAutoHotkey,
	}
	seen := make(map[AnalyzerID]string, len(want))
	for language, wantID := range want {
		descriptor, ok := registry.Lookup(language)
		if !ok {
			t.Fatalf("missing Phase 8 registry row %s", language)
		}
		if descriptor.Analyzer != wantID || !descriptor.Capabilities.SourceAnalysis || !descriptor.Capabilities.Declarations || !descriptor.Capabilities.Ranges || !descriptor.Capabilities.Dependencies {
			t.Fatalf("Phase 8 registry row %s = %+v", language, descriptor)
		}
		if previous, duplicate := seen[descriptor.Analyzer]; duplicate {
			t.Fatalf("Phase 8 analyzer identity %q shared by %s and %s", descriptor.Analyzer, previous, language)
		}
		seen[descriptor.Analyzer] = language
		if descriptor.Capabilities.ScopeResolvedReferences || descriptor.Capabilities.ProjectResolvedReferences || descriptor.Capabilities.ProjectResolvedDefinitions || descriptor.Capabilities.Implementations || descriptor.Capabilities.Overrides || descriptor.Capabilities.SemanticRelations || descriptor.Capabilities.IncrementalIndex {
			t.Fatalf("Phase 8 row %s overclaims project/semantic/index capability: %+v", language, descriptor.Capabilities)
		}
		analyzer, available := AnalyzerFor(descriptor)
		if !available || analyzer.ID() != wantID || analyzer.Language() != language {
			t.Fatalf("Phase 8 analyzer dispatch %s = %+v available=%v", language, analyzer, available)
		}
	}
}
