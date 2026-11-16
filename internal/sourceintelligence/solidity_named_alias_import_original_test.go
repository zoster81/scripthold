package sourceintelligence

import (
	"context"
	"os"
	"strings"
	"testing"
)

// Solidity named-import aliases rename symbols in the importing scope; they
// must never be presented as changes to the authored module path.
func TestSolidityNamedAliasImportsOriginalSourceEvidence(t *testing.T) {
	const fixture = "pragma solidity ^0.8.20;\n" +
		"import {Thing as LocalThing} from \"./Thing.sol\";\n" +
		"import {One as LocalOne, Two} from \"../Two.sol\";\n" +
		"import \"./plain.sol\";\n" +
		"// import {Fake as F} from \"fake-comment.sol\";\n" +
		"contract Holder { string constant decoy = \"import fake-string.sol;\"; }\n"
	for _, tc := range []struct{ name, text string }{
		{"LF", fixture}, {"CRLF", strings.ReplaceAll(fixture, "\n", "\r\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := solidityAliasImportAnalysis(t, tc.text)
			if len(result.Dependencies) != 3 {
				t.Fatalf("unexpected Solidity named-import dependencies: %+v", result.Dependencies)
			}
			for _, want := range []struct {
				path string
				line int
			}{
				{"./Thing.sol", 2}, {"../Two.sol", 3}, {"./plain.sol", 4},
			} {
				assertSolidityImportedPath(t, result.Dependencies, want.path, want.line)
			}
		})
	}
	for _, tc := range []struct {
		name, env, marker, path string
		line                    int
	}{
		{"forge-std", "SCRIPTHOLD_REAL_SOL_ALIAS_FORGE",
			"import {console as console2} from \"./console.sol\";", "./console.sol", 4},
		{"uniswap-v4", "SCRIPTHOLD_REAL_SOL_ALIAS_UNISWAP",
			"import {LiquidityMathTest as LiquidityMath} from \"src/test/LiquidityMathTest.sol\";",
			"src/test/LiquidityMathTest.sol", 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := os.Getenv(tc.env)
			if path == "" {
				t.Skip("independent original Solidity source not configured")
			}
			info, err := os.Lstat(path)
			if err != nil || !info.Mode().IsRegular() || info.Size() < 50 || info.Size() > 12000 {
				t.Fatalf("original Solidity source unavailable or unbounded: %v %v", info, err)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(contents), tc.marker) {
				t.Fatal("Solidity original source oracle drift")
			}
			result := solidityAliasImportAnalysis(t, string(contents))
			assertSolidityImportedPath(t, result.Dependencies, tc.path, tc.line)
			for _, dep := range result.Dependencies {
				if strings.Contains(dep.Value, " as ") || strings.Contains(dep.Value, "console2") ||
					strings.Contains(dep.Value, "LiquidityMath}") {
					t.Fatalf("fabricated module name from imported symbol alias: %+v", dep)
				}
			}
		})
	}
}

func solidityAliasImportAnalysis(t *testing.T, text string) AnalyzerResult {
	t.Helper()
	result, err := (SolidityAnalyzer{}).Analyze(context.Background(),
		testSourceDocument("named-alias.sol", text), testAnalyzeOptions(true, 512))
	if err != nil {
		t.Fatal(err)
	}
	if result.Analysis.Truncated {
		t.Fatal("Solidity source analysis unexpectedly truncated")
	}
	return result
}

func assertSolidityImportedPath(t *testing.T, deps []StructuralDependency, path string, line int) {
	t.Helper()
	for _, dep := range deps {
		if dep.Value != path {
			continue
		}
		if dep.Kind != StructuralDependencyImport || dep.Alias != "" ||
			dep.Range.Start.Line != line || dep.Range.End.Line != line {
			t.Fatalf("imported Solidity module %q at line %d has wrong evidence: %+v", path, line, dep)
		}
		return
	}
	t.Fatalf("authored imported Solidity module %q at line %d missing: %+v", path, line, deps)
}
