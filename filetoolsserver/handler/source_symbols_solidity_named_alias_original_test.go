package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The local MCP digest must retain authored Solidity import source paths even
// when the source renames a selected imported symbol.
func TestSolidityOriginalNamedAliasImportDigest(t *testing.T) {
	for _, tc := range []struct {
		name, env, marker, module string
		line                      int
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
				t.Fatalf("original source unavailable or unbounded: %v %v", info, err)
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(contents), tc.marker) {
				t.Fatal("original Solidity import evidence drift")
			}
			h := NewHandler([]string{filepath.Dir(path)})
			failure, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: []string{path}, Language: "solidity", MaxFiles: 1, MaxSymbols: 512,
			})
			if err != nil || failure != nil || out.Truncated || len(out.Digests) != 1 ||
				out.Digests[0].SourceFingerprint == "" {
				t.Fatalf("original Solidity MCP digest unavailable: %v %+v %+v", err, failure, out)
			}
			found := false
			for _, dep := range out.Digests[0].Dependencies {
				if dep.Kind != "import" || strings.Contains(dep.Value, " as ") ||
					strings.Contains(dep.Value, "console2") {
					t.Fatalf("invented Solidity import module: %+v", dep)
				}
				if dep.Value == tc.module && dep.Range.Start.Line == tc.line &&
					dep.Range.End.Line == tc.line && dep.Alias == "" {
					found = true
				}
			}
			if !found {
				t.Fatalf("authored Solidity source-unit import missing: %+v", out.Digests[0].Dependencies)
			}
		})
	}
}
