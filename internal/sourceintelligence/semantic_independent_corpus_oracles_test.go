package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// These optional independent checks use paths supplied by the operator.
// Corpus data and workstation locations are intentionally not checked in.
func TestSemanticIndependentRealCorpusOracles(t *testing.T) {
	type want struct {
		name string
		kind SymbolKind
		line int
	}
	cases := []struct {
		name, env, language string
		present             []want
		forbidden           []string
	}{
		{"c-curl-altsvc", "SCRIPTHOLD_REAL_C_A", "c", []want{
			{"main", SymbolKindFunction, 32},
		}, []string{"curl_easy_init", "curl_easy_setopt", "curl_global_cleanup"}},
		{"groovy-benchmark-ackermann", "SCRIPTHOLD_REAL_GROOVY_A", "groovy", []want{
			{"A", SymbolKindFunction, 8},
		}, []string{"println", "args", "Ghost"}},
		{"groovy-exercism-hello", "SCRIPTHOLD_REAL_GROOVY_B", "groovy", []want{
			{"HelloWorld", SymbolKindClass, 1}, {"HelloWorld.hello", SymbolKindMethod, 3},
		}, []string{"HelloWorld.World", "String"}},
		{"gleam-echo-original", "SCRIPTHOLD_REAL_GLEAM_A", "gleam", []want{
			{"main", SymbolKindFunction, 4},
		}, []string{"echo", "Gleam", "ghost"}},
		{"common-lisp-clog-demo", "SCRIPTHOLD_REAL_COMMONLISP_A", "common-lisp", []want{
			{"clog-demo-1", SymbolKindPackage, 1},
			{"clog-demo-1.display-width", SymbolKindConstant, 11},
			{"clog-demo-1.new-food", SymbolKindFunction, 20},
		}, []string{"create-div", "defpackage", "clog-demo-1.ghost"}},
		{"vbscript-reactos-wscript", "SCRIPTHOLD_REAL_VBSCRIPT_A", "vbscript", []want{
			{"winetest", SymbolKindVariable, 21}, {"ok", SymbolKindFunction, 24},
		}, []string{"CreateObject", "winetest.ok", "reportSuccess"}},
		{"fortran-stdlib-meshgrid", "SCRIPTHOLD_REAL_FORTRAN_A", "fortran", []want{
			{"example_meshgrid", SymbolKindModule, 1},
			{"example_meshgrid.print_2d_array", SymbolKindFunction, 29},
		}, []string{"meshgrid", "linspace", "example_meshgrid.print"}},
		{"emacs-mal-core", "SCRIPTHOLD_REAL_EMACS_A", "emacs-lisp", []want{
			{"mal-boolean", SymbolKindFunction, 4}, {"mal-=", SymbolKindFunction, 6},
			{"mal-map-=", SymbolKindFunction, 29}, {"mal-conj", SymbolKindFunction, 44},
			{"elisp-to-mal", SymbolKindFunction, 55},
		}, []string{"seq", "mal-true", "mal/types", "mal.Ghost"}},
		{"flow-project-ambient-declaration", "SCRIPTHOLD_REAL_FLOW_A", "flow", []want{
			{"ccc", SymbolKindConstant, 3},
		}, []string{"Array", "string", "haste_pkg"}},
		{"zig-stdlib-wasm-object", "SCRIPTHOLD_REAL_ZIG_A", "zig", []want{
			{"Object", SymbolKindConstant, 1},
			{"RelativeSlice", SymbolKindStruct, 53}, {"RelativeSlice.off", SymbolKindField, 54},
			{"RelativeSlice.len", SymbolKindField, 55},
			{"SegmentInfo", SymbolKindStruct, 58},
		}, []string{"Wasm.Ghost", "std.Ghost", "pub", "SegmentInfo.Ghost"}},
		{"ocaml-stdlib-option", "SCRIPTHOLD_REAL_OCAML_A", "ocaml", []want{
			{"none", SymbolKindVariable, 18}, {"some", SymbolKindFunction, 19},
			{"Syntax", SymbolKindModule, 64},
		}, []string{"Option.Ghost", "None"}},
		{"ocaml-exercism-connect", "SCRIPTHOLD_REAL_OCAML_B", "ocaml", []want{
			{"eq_cell", SymbolKindFunction, 6}, {"IntTuple", SymbolKindModule, 11},
			{"to_matrix", SymbolKindFunction, 28},
		}, []string{"Phantom", "IntTuple.Ghost"}},
		{"erlang-recon-docsite", "SCRIPTHOLD_REAL_ERLANG_A", "erlang", []want{
			{"docsite", SymbolKindModule, 9}, {"docsite.main", SymbolKindFunction, 15},
			{"docsite.content", SymbolKindFunction, 39}, {"docsite.write", SymbolKindFunction, 109},
		}, []string{"docsite.rebar3", "docsite.filename"}},
		{"erlang-yaws-negotiation", "SCRIPTHOLD_REAL_ERLANG_B", "erlang", []want{
			{"yaws_content_negotiation", SymbolKindModule, 1},
			{"yaws_content_negotiation.compute_rsva", SymbolKindFunction, 9},
			{"yaws_content_negotiation.get_other_header", SymbolKindFunction, 30},
		}, []string{"yaws_content_negotiation.ghost", "yaws_content_negotiation.Headers"}},
		{"octave-polynomial-double", "SCRIPTHOLD_REAL_OCTAVE_A", "octave", []want{
			{"double", SymbolKindFunction, 1},
		}, []string{"poly", "p"}},
		{"qbasic-qb64pe-acalc", "SCRIPTHOLD_REAL_QBASIC_A", "qbasic", []want{
			{"DrawCursor", SymbolKindFunction, 2}, {"DrawCursor", SymbolKindFunction, 52},
			{"DrawInner", SymbolKindFunction, 69}, {"DrawOuter", SymbolKindFunction, 124},
			{"OldX", SymbolKindVariable, 7}, {"OldY", SymbolKindVariable, 7},
		}, []string{"SIN", "LOG", "OldAngle.Ghost"}},
		{"systemverilog-yosys-example", "SCRIPTHOLD_REAL_SYSTEMVERILOG_A", "systemverilog", []want{
			{"top", SymbolKindModule, 1},
		}, []string{"assume", "assert", "top.initial"}},
		{"systemverilog-pygments-flipflop", "SCRIPTHOLD_REAL_SYSTEMVERILOG_B", "systemverilog", []want{
			{"toplevel", SymbolKindModule, 1}, {"toplevel.flop1", SymbolKindSignal, 5},
			{"toplevel.flop2", SymbolKindSignal, 6},
		}, []string{"toplevel.reset", "posedge"}},
		{"nim-stdlib-atomic", "SCRIPTHOLD_REAL_NIM_A", "nim", []want{
			{"AtomicWithGeneric", SymbolKindClass, 4}, {"AtomicWithGeneric.value", SymbolKindField, 5},
			{"initAtomicWithGeneric", SymbolKindFunction, 7},
		}, []string{"atomics", "result"}},
		{"nim-exercism-hello", "SCRIPTHOLD_REAL_NIM_B", "nim", []want{
			{"hello", SymbolKindFunction, 1},
		}, []string{"World", "string"}},
		{"julia-stdlib-buffer", "SCRIPTHOLD_REAL_JULIA_A", "julia", []want{
			{"Buffer", SymbolKindStruct, 4}, {"read_to_buffer", SymbolKindFunction, 29},
		}, []string{"UInt8", "Buffer.Ghost"}},
		{"julia-exercism-tree", "SCRIPTHOLD_REAL_JULIA_B", "julia", []want{
			{"BinarySearchTree", SymbolKindStruct, 1}, {"traverse", SymbolKindFunction, 44},
		}, []string{"nothing", "vector"}},
		{"css-sphinx-design-overrides", "SCRIPTHOLD_REAL_CSS_A", "css", []want{
			{".sd-card-header", SymbolKindSelector, 2},
		}, []string{"font-size", "Audiowide"}},
		{"css-esp-ebus-common", "SCRIPTHOLD_REAL_CSS_B", "css", []want{
			{"body.centered", SymbolKindSelector, 20}, {".container", SymbolKindSelector, 24},
			{".row", SymbolKindSelector, 28},
		}, []string{"font-family", "padding", "#fafbfc"}},
		{"terraform-core-check", "SCRIPTHOLD_REAL_TERRAFORM_A", "terraform", []want{
			{"test_instance.foo", SymbolKindResource, 1}, {"check.test_instance_exists", SymbolKindSection, 5},
		}, []string{"bar", "check.test_instance_exists.foo"}},
		{"terraform-aws-vpc", "SCRIPTHOLD_REAL_TERRAFORM_B", "terraform", []want{
			{"terraform", SymbolKindSection, 1}, {"terraform.required_providers", SymbolKindSection, 4},
			{"terraform.provider_meta.aws", SymbolKindSection, 11},
		}, []string{"hashicorp/aws", "terraform.source"}},
		{"xaml-abp-app", "SCRIPTHOLD_REAL_XAML_A", "xaml", []want{
			{"MyCompanyName.MyProjectName.App", SymbolKindClass, 5},
		}, []string{"MyCompanyName.MyProjectName.App.Resources", "Resources/Styles/Colors.xaml"}},
		{"xaml-desktop-toasts", "SCRIPTHOLD_REAL_XAML_B", "xaml", []want{
			{"DesktopToastsSample.App", SymbolKindClass, 1},
		}, []string{"DesktopToastsSample.MainWindow", "StartupUri"}},
		{"ansible-windows-ping", "SCRIPTHOLD_REAL_ANSIBLE_A", "ansible-yaml", []want{
			{"Ping", SymbolKindSection, 3}, {"Ping.ping", SymbolKindOperation, 7},
		}, []string{"Ping.win_ping", "all"}},
		{"svelte-custom-method", "SCRIPTHOLD_REAL_SVELTE_A", "svelte", []want{
			{"updateFoo", SymbolKindFunction, 4}, {"foo", SymbolKindVariable, 8},
		}, []string{"value", "custom-element"}},
		{"scala3-compiler-main", "SCRIPTHOLD_REAL_SCALA_A", "scala", []want{
			{"dotty.tools.dotc.Main", SymbolKindModule, 5},
		}, []string{"dotty.tools.dotc.Driver", "dotty.tools.dotc.class"}},
		{"verilog-yosys-key-expansion", "SCRIPTHOLD_REAL_VERILOG_A", "verilog", []want{
			{"aes_key_expand_128", SymbolKindModule, 4}, {"aes_key_expand_128.w", SymbolKindSignal, 9},
		}, []string{"aes_key_expand_128.assign", "aes_key_expand_128.kld_ref"}},
		{"elixir-atom-standard-library", "SCRIPTHOLD_REAL_ELIXIR_A", "elixir", []want{
			{"Atom", SymbolKindModule, 5}, {"Atom.to_string", SymbolKindFunction, 62},
			{"Atom.to_charlist", SymbolKindFunction, 78}, {"Atom.to_char_list", SymbolKindFunction, 85},
		}, []string{"Atom.apple", "Atom.ghost"}},
		{"graphql-gqlgen", "SCRIPTHOLD_REAL_GRAPHQL_A", "graphql", []want{
			{"Role", SymbolKindEnum, 8}, {"MyQuery", SymbolKindType, 18}, {"MyQuery.todo", SymbolKindField, 19},
		}, []string{"forceResolver"}},
		{"graphql-hasura", "SCRIPTHOLD_REAL_GRAPHQL_B", "graphql", []want{
			{"Mutation", SymbolKindType, 1}, {"Mutation.actionName", SymbolKindField, 2}, {"SampleInput", SymbolKindType, 7},
		}, []string{"arg1"}},
		{"proto-addressbook", "SCRIPTHOLD_REAL_PROTO_A", "proto", []want{
			{"tutorial.Person", SymbolKindType, 6}, {"tutorial.Person.name", SymbolKindField, 7},
		}, []string{"AddressBookProtos"}},
		{"proto-descriptor", "SCRIPTHOLD_REAL_PROTO_B", "proto", []want{
			{"test.MyMessage", SymbolKindType, 33}, {"test.MyMessage.foo", SymbolKindField, 36},
		}, []string{"optimize_for"}},
		{"sass-timeline", "SCRIPTHOLD_REAL_SASS_A", "sass", []want{
			{"primary", SymbolKindVariable, 1}, {"body", SymbolKindSelector, 13},
		}, []string{"background-color"}},
		{"sass-rem", "SCRIPTHOLD_REAL_SASS_B", "sass", []want{
			{"browser-context", SymbolKindVariable, 2}, {"rem", SymbolKindFunction, 3},
		}, []string{"unitless"}},
		{"al-sales", "SCRIPTHOLD_REAL_AL_A", "al", []want{
			{"SalesDataMgmt", SymbolKindModule, 15}, {"SalesDataMgmt.GetSalesJSON", SymbolKindMethod, 19},
		}, []string{"CustLedgEntry"}},
		{"al-csv", "SCRIPTHOLD_REAL_AL_B", "al", []want{
			{"BIT CSV TXT Process Lines", SymbolKindModule, 1}, {"BIT CSV TXT Process Lines.OnRun", SymbolKindMethod, 12},
		}, []string{"DataExch"}},
		{"vue-header", "SCRIPTHOLD_REAL_VUE_A", "vue", []want{
			{"props", SymbolKindConstant, 13}, {"emit", SymbolKindConstant, 20},
		}, []string{"ReplStore"}},
		{"vue-nuxt", "SCRIPTHOLD_REAL_VUE_B", "vue", []want{
			{"props", SymbolKindConstant, 9}, {".grid", SymbolKindSelector, 59},
		}, []string{"useHead"}},
		{"r-sample", "SCRIPTHOLD_REAL_R_A", "r", []want{
			{"Square", SymbolKindFunction, 18}, {"countdown", SymbolKindFunction, 28},
		}, []string{"print"}},
		{"r-analysis", "SCRIPTHOLD_REAL_R_B", "r", []want{
			{"gtable_select", SymbolKindFunction, 9}, {"gtable_stack", SymbolKindFunction, 17},
		}, []string{"library"}},
		{"pascal-lsp", "SCRIPTHOLD_REAL_PASCAL_A", "pascal", []want{
			{"LSP.Base", SymbolKindModule, 21}, {"LSP.Base.TLSPContextCustomJSONRPCHandler", SymbolKindClass, 49},
		}, []string{"LSP.Base.TGhost"}},
		{"pascal-brook", "SCRIPTHOLD_REAL_PASCAL_B", "pascal", []want{
			{"BrookFCLCGIBroker", SymbolKindModule, 16}, {"BrookFCLCGIBroker.TBrookApplication", SymbolKindClass, 32},
		}, []string{"BrookFCLCGIBroker.TGhost"}},
		{"powershell-class", "SCRIPTHOLD_REAL_POWERSHELL_A", "powershell", []want{
			{"Foo", SymbolKindClass, 13}, {"Foo.Method", SymbolKindMethod, 16}, {"GetBar", SymbolKindFunction, 22},
		}, []string{"Foo.LocalVar1"}},
		{"powershell-history", "SCRIPTHOLD_REAL_POWERSHELL_B", "powershell", []want{
			{"Save-HistoryAll", SymbolKindFunction, 1}, {"Save-HistoryIncremental", SymbolKindFunction, 9}, {"Search-History", SymbolKindFunction, 44},
		}, []string{"Save-History"}},
		{"perl-package", "SCRIPTHOLD_REAL_PERL_A", "perl", []want{
			{"Test::One", SymbolKindModule, 8}, {"Test::One::Two", SymbolKindModule, 10}, {"Test", SymbolKindModule, 12},
		}, []string{"Test.PodGhost"}},
		{"perl-ack", "SCRIPTHOLD_REAL_PERL_B", "perl", []want{
			{"App::Ack", SymbolKindModule, 1},
		}, []string{"App::Ack.Ghost"}},
		{"mql5-oscillator", "SCRIPTHOLD_REAL_MQL5_A", "mql5", []want{
			{"ENUM_AOS_BI", SymbolKindEnum, 17}, {"AosPeriod", SymbolKindVariable, 24},
		}, []string{"Indicator"}},
		{"mql5-argus", "SCRIPTHOLD_REAL_MQL5_B", "mql5", []want{
			{"ADX_Period", SymbolKindVariable, 23}, {"OnInit", SymbolKindFunction, 48},
		}, []string{"ADX Regime"}},
		{"vb6-modcode", "SCRIPTHOLD_REAL_VB6_A", "vb6", []want{
			{"modCode", SymbolKindModule, 1}, {"modCode.progname", SymbolKindConstant, 6}, {"modCode.currentState", SymbolKindEnum, 10},
		}, []string{"modCode.VB_Name"}},
		{"vb6-module", "SCRIPTHOLD_REAL_VB6_B", "vb6", []want{
			{"Module1", SymbolKindModule, 1}, {"Module1.SC_CLOSE", SymbolKindConstant, 4},
		}, []string{"Module1.VB_Name"}},
		{"ada-expression", "SCRIPTHOLD_REAL_ADA_A", "ada", []want{
			{"Test", SymbolKindFunction, 3}, {"Test.Tagged_Procedure", SymbolKindFunction, 7}, {"Test.Tagged_Procedure.Boolean_As_String", SymbolKindFunction, 8},
		}, []string{"Test.Tagged_Procedure.dummy0", "Test.Tagged_Procedure.dummy1"}},
		{"ada-kde", "SCRIPTHOLD_REAL_ADA_B", "ada", []want{
			{"LAL_DDA", SymbolKindFunction, 5}, {"LAL_DDA.App_Setup", SymbolKindFunction, 57},
		}, []string{"LAL_DDA.Fake"}},
		{"clojure-for", "SCRIPTHOLD_REAL_CLOJURE_A", "clojure", []want{
			{"prime?", SymbolKindFunction, 1},
		}, []string{"for"}},
		{"clojure-highlight", "SCRIPTHOLD_REAL_CLOJURE_B", "clojure", []want{
			{"highlighting", SymbolKindNamespace, 2}, {"highlighting.something-else", SymbolKindFunction, 6}, {"highlighting.bar", SymbolKindFunction, 21},
		}, []string{"highlighting.Ghost"}},
		{"clojure-ctags-metadata", "SCRIPTHOLD_REAL_CLOJURE_META", "clojure", []want{
			{"my-fn", SymbolKindFunction, 1}, {"my-private-fn", SymbolKindFunction, 3},
			{"my-public-fn", SymbolKindFunction, 5}, {"my-hinted-fn", SymbolKindFunction, 7},
			{"my-private-hinted-fn", SymbolKindFunction, 9}, {"so-many-hints", SymbolKindFunction, 11},
		}, []string{"private", "foo", "MyReturnType"}},
		{"tcl-ctags-regex", "SCRIPTHOLD_REAL_TCL_A", "tcl", []want{
			{"proc1", SymbolKindFunction, 2}, {"proc2", SymbolKindFunction, 8},
			{"proc3", SymbolKindFunction, 10}, {"proc4", SymbolKindFunction, 15},
		}, []string{"regexp"}},
		{"tcl-sample-even-odd", "SCRIPTHOLD_REAL_TCL_B", "tcl", []want{
			{"usage", SymbolKindFunction, 1},
		}, []string{"lindex", "regexp", "expr"}},
		{"ada-ctags-block-scope", "SCRIPTHOLD_REAL_ADA_BLOCK", "ada", []want{
			{"Input", SymbolKindPackage, 1}, {"Input.F", SymbolKindFunction, 2}, {"Input.G", SymbolKindFunction, 23},
		}, []string{"Input.F.G", "Input.F.P", "Input.G.D"}},
		{"ada-ctags-altivec", "SCRIPTHOLD_REAL_ADA_ALTIVEC", "ada", []want{
			{"Altivec", SymbolKindFunction, 14},
		}, []string{"Altivec.To_Vector", "Altivec.View_A"}},
		{"lua-ctags-nested-table", "SCRIPTHOLD_REAL_LUA_A", "lua", []want{
			{"a.b.c", SymbolKindFunction, 3}, {"ax.by.cz", SymbolKindFunction, 10},
		}, []string{"a.b", "ax.by", "print"}},
		{"lua-kde-highlight", "SCRIPTHOLD_REAL_LUA_B", "lua", []want{
			{"obj:foo", SymbolKindFunction, 172}, {"obj.bar", SymbolKindFunction, 176},
		}, []string{"string.gfind", "string.gmatch", "xyz"}},
		{"ruby-ctags-method-body", "SCRIPTHOLD_REAL_RUBY_A", "ruby", []want{
			{"A", SymbolKindClass, 1}, {"A.a", SymbolKindMethod, 2}, {"A.b", SymbolKindMethod, 5},
		}, []string{"super", "A.super", "do"}},
		{"ruby-kde-class", "SCRIPTHOLD_REAL_RUBY_B", "ruby", []want{
			{"Periods", SymbolKindClass, 150}, {"Periods.each", SymbolKindMethod, 151},
			{"ClientManager", SymbolKindClass, 205}, {"ClientManager.ClientManager", SymbolKindConstructor, 208},
		}, []string{"dumpDebugInfo", "Periods.Classical", "ClientManager.DBClient"}},
		{"php-ctags-classes", "SCRIPTHOLD_REAL_PHP_A", "php", []want{
			{"Foo", SymbolKindClass, 16}, {"Foo.Foo", SymbolKindConstructor, 17},
			{"Foo.method1", SymbolKindMethod, 21}, {"Bar", SymbolKindClass, 30},
			{"Bar.Bar", SymbolKindConstructor, 31}, {"Bar.method1", SymbolKindMethod, 35},
		}, []string{"Bar.method2", "functions", "classes"}},
		{"php-kde-mixed", "SCRIPTHOLD_REAL_PHP_B", "php", []want{
			{"test", SymbolKindFunction, 9}, {"gen_three_nulls", SymbolKindFunction, 141},
			{"User", SymbolKindClass, 168}, {"User.User", SymbolKindConstructor, 173},
			{"MyClass", SymbolKindClass, 180},
		}, []string{"DateTime", "DateTime.get", "some_js_var", "JSFake"}},
		{"rust-ctags-circle", "SCRIPTHOLD_REAL_RUST_A", "rust", []want{
			{"main", SymbolKindFunction, 4}, {"Circle.x", SymbolKindField, 8},
			{"Circle.radius", SymbolKindField, 10}, {"Circle.area", SymbolKindMethod, 14},
		}, []string{"println", "Circle.println", "Ghost"}},
		{"rust-sample-even-odd", "SCRIPTHOLD_REAL_RUST_B", "rust", []want{
			{"usage", SymbolKindFunction, 8}, {"parse_int", SymbolKindFunction, 13},
			{"main", SymbolKindFunction, 17},
		}, []string{"println", "FromStr", "parse"}},
		{"toml-kde-highlighting", "SCRIPTHOLD_REAL_TOML_A", "toml", []want{
			{"baseURL", SymbolKindKey, 2}, {"Author", SymbolKindSection, 5},
			{"Author.name", SymbolKindKey, 6}, {"outputs.home", SymbolKindKey, 10},
			{"params", SymbolKindSection, 12}, {"params.social", SymbolKindSection, 28},
			{"params.social.name", SymbolKindKey, 29},
			{"params.social.'key2'", SymbolKindKey, 36},
			{"params.social.'quoted \"value\"'", SymbolKindKey, 37},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies", SymbolKindSection, 40},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit.name", SymbolKindKey, 47},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit.color", SymbolKindKey, 48},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit.flavor", SymbolKindKey, 49},
			{"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.site.\"google.com\"", SymbolKindKey, 50},
		}, []string{"params . social", "params . social.name", "phantom",
			"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.fruit. color",
			"target.'cfg(all(windows, target_env=\"msvc\"))'.build-dependencies.site.google.com"}},
		{"yaml-kde-highlighting", "SCRIPTHOLD_REAL_YAML_A", "yaml", []want{
			{"key", SymbolKindKey, 5}, {"boolean", SymbolKindKey, 9},
			{"null_value", SymbolKindKey, 10}, {"single quotes", SymbolKindKey, 11},
			{"double quotes", SymbolKindKey, 12}, {"decimal", SymbolKindKey, 14},
			{"foo", SymbolKindKey, 42},
		}, []string{"comment", "string", "O'Brien", "syntax", "phantom"}},
		{"yaml-ctags-openapi", "SCRIPTHOLD_REAL_YAML_B", "yaml", []want{
			{"openapi", SymbolKindKey, 1}, {"info", SymbolKindKey, 2},
			{"info.title", SymbolKindKey, 3}, {"info.version", SymbolKindKey, 5},
			{"paths", SymbolKindKey, 11}, {"paths./sample/path", SymbolKindKey, 12},
			{"paths./sample/path.get", SymbolKindKey, 13},
			{"paths./sample/path.get.summary", SymbolKindKey, 14},
			{"components.schemas.NullableField", SymbolKindKey, 26},
		}, []string{"servers.url", "servers.description", "tags.name", "fake"}},
		{"swift-kde-arguments", "SCRIPTHOLD_REAL_SWIFT_A", "swift", []want{
			{"Options.maxArgumentNameLength", SymbolKindProperty, 35},
			{"Options.addArguments", SymbolKindMethod, 45},
			{"parseArguments", SymbolKindFunction, 58},
			{"preprocessArguments", SymbolKindFunction, 98},
			{"Foo", SymbolKindInterface, 689},
			{"Foo.foo", SymbolKindMethod, 690},
			{"FooImpl", SymbolKindClass, 695},
		}, []string{"UnicodeScalarView", "FormatError", "FooImpl.foo", "popFirst"}},
		{"swift-sample-even-odd", "SCRIPTHOLD_REAL_SWIFT_B", "swift", []want{
			{"is_even", SymbolKindVariable, 13},
		}, []string{"print", "Even", "Odd", "System.exit"}},
		{"nix-kde-bindings", "SCRIPTHOLD_REAL_NIX_A", "nix", []want{
			{"bool", SymbolKindVariable, 4}, {"var1", SymbolKindVariable, 5},
		}, []string{"false", "Multi-line", "HOME", "builtins"}},
	}
	registry, err := DefaultLanguageRegistry()
	if err != nil {
		t.Fatal(err)
	}
	ran := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent source corpus not configured")
			}
			ran++
			info, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() {
				t.Fatal("corpus case must be a regular source file, not a link")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			descriptor, ok := registry.Resolve(tc.language)
			if !ok || descriptor.ID != tc.language {
				t.Fatal("no canonical analyzer")
			}
			analyzer, ok := AnalyzerFor(descriptor)
			if !ok {
				t.Fatal("provider not active")
			}
			result, err := analyzer.Analyze(context.Background(), testSourceDocument(filepath.Base(path), string(data)), testAnalyzeOptions(true, 2048))
			if err != nil {
				t.Fatal(err)
			}
			assertAnalyzerResultCoordinates(t, result)
			seen := make(map[string][]NormalizedSymbol, len(result.Analysis.Symbols))
			for _, symbol := range result.Analysis.Symbols {
				seen[symbol.QualifiedName] = append(seen[symbol.QualifiedName], symbol)
			}
			for _, expected := range tc.present {
				matched := false
				for _, symbol := range seen[expected.name] {
					if symbol.Kind == expected.kind && symbol.DeclarationRange.Start.Line == expected.line {
						matched = true
						break
					}
				}
				if !matched {
					t.Errorf("expected declaration %q kind=%q line=%d, found=%+v", expected.name, expected.kind,
						expected.line, seen[expected.name])
				}
			}
			for _, name := range tc.forbidden {
				if symbols := seen[name]; len(symbols) > 0 {
					t.Errorf("expression, comment, or local reference emitted as declaration %q: %+v", name, symbols)
				}
			}
			if result.Analysis.Truncated {
				t.Errorf("oracle cannot prove completeness with truncated source output")
			}
		})
	}
	if ran == 0 {
		t.Skip("no independent corpus paths configured; portable semantic matrix remains active")
	}
	t.Logf("independent real source files checked: %d/%d; these cases do not certify all native providers", ran, len(cases))
}

// The KDE shell syntax-highlighting corpus intentionally contains malformed
// constructs. An analyzer may omit declarations, but must not claim completeness.
func TestSemanticIndependentMalformedBashCorpusCoverage(t *testing.T) {
	path := os.Getenv("SCRIPTHOLD_REAL_BASH_A")
	if path == "" {
		t.Skip("independent malformed shell corpus not configured")
	}
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatal("corpus must be a regular source file, not a link")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (BashAnalyzer{}).Analyze(context.Background(), testSourceDocument(filepath.Base(path), string(data)), testAnalyzeOptions(true, 2048))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.CoverageComplete || result.Analysis.Truncated {
		t.Fatalf("malformed corpus must report incomplete, untruncated analysis: %+v", result.Analysis)
	}
}
