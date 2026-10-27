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
			seen := symbolsByQualifiedName(result.Analysis.Symbols)
			for _, expected := range tc.present {
				symbol, ok := seen[expected.name]
				if !ok || symbol.Kind != expected.kind || symbol.DeclarationRange.Start.Line != expected.line {
					t.Errorf("expected %q kind=%q line=%d, found=%v got=%q line=%d", expected.name, expected.kind,
						expected.line, ok, symbol.Kind, symbol.DeclarationRange.Start.Line)
				}
			}
			for _, name := range tc.forbidden {
				if symbol, ok := seen[name]; ok {
					t.Errorf("expression, comment, or local reference emitted as declaration %q: %+v", name, symbol)
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
