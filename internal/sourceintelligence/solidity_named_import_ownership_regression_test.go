package sourceintelligence

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Real Aave files use named imports before their top-level Solidity types.
// Complete coverage must never hide the owning contract/interface/library.
func TestSolidityNamedImportsPreserveTypeOwnership(t *testing.T) {
	cases := []struct {
		name, source string
		want         []string
		absent       []string
	}{
		{
			name: "named-import-contract",
			source: "pragma solidity ^0.8.10;\n" +
				"import {Base} from '../libraries/Base.sol';\n" +
				"contract Pool is Base {\n  function updateFlashloanPremiums() external {}\n}\n",
			want:   []string{"Pool", "Pool.updateFlashloanPremiums"},
			absent: []string{"updateFlashloanPremiums"},
		},
		{
			name: "named-import-interface",
			source: "pragma solidity ^0.8.10;\n" +
				"import {IPool} from '../interfaces/IPool.sol';\n" +
				"interface IPoolConfigurator {\n  function updateFlashloanPremiumTotal(uint128 value) external;\n}\n",
			want:   []string{"IPoolConfigurator", "IPoolConfigurator.updateFlashloanPremiumTotal"},
			absent: []string{"updateFlashloanPremiumTotal"},
		},
	}
	for _, tc := range cases {
		for _, eol := range []struct{ name, value string }{{"lf", "\n"}, {"crlf", "\r\n"}} {
			t.Run(tc.name+"/"+eol.name, func(t *testing.T) {
				result, err := (SolidityAnalyzer{}).Analyze(context.Background(),
					testSourceDocument("original.sol", strings.ReplaceAll(tc.source, "\n", eol.value)),
					testAnalyzeOptions(true, 256))
				if err != nil {
					t.Fatal(err)
				}
				assertSolidityOwners(t, result, tc.want, tc.absent)
			})
		}
	}
}

func TestSolidityOriginalAaveContractOwnership(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original corpus not configured")
	}
	for _, tc := range []struct {
		file string
		want []string
	}{
		{"contracts/protocol/pool/PoolConfigurator.sol", []string{"PoolConfigurator", "PoolConfigurator.updateFlashloanPremiumTotal"}},
		{"contracts/protocol/pool/Pool.sol", []string{"Pool", "Pool.updateFlashloanPremiums"}},
		{"contracts/interfaces/IPoolConfigurator.sol", []string{"IPoolConfigurator", "IPoolConfigurator.updateFlashloanPremiumTotal"}},
		{"contracts/protocol/pool/PoolStorage.sol", []string{"PoolStorage"}},
	} {
		t.Run(filepath.Base(tc.file), func(t *testing.T) {
			path := filepath.Join(corpus, "real-projects", "solidity-aave-v3", filepath.FromSlash(tc.file))
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read original %s: %v", tc.file, err)
			}
			result, err := (SolidityAnalyzer{}).Analyze(context.Background(),
				testSourceDocument(path, string(contents)), testAnalyzeOptions(true, 512))
			if err != nil {
				t.Fatal(err)
			}
			assertSolidityOwners(t, result, tc.want, nil)
		})
	}
}

func assertSolidityOwners(t *testing.T, result AnalyzerResult, want, absent []string) {
	t.Helper()
	got := symbolsByQualifiedName(result.Analysis.Symbols)
	for _, name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("missing owning Solidity symbol %q (coverage=%v, examples=%v)",
				name, result.Analysis.CoverageComplete, sortedSymbolQualifiedNames(result.Analysis.Symbols))
		}
	}
	for _, name := range absent {
		if _, ok := got[name]; ok {
			t.Errorf("orphaned Solidity member %q when owner is available", name)
		}
	}
}
