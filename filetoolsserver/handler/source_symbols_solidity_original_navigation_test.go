package handler

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two independently selected ERC20 maintenance tasks, one per original project,
// are first checked against original source and adjacent behavioral tests.
func TestSourceSymbolsOriginalSolidityERC20TaskNavigation(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Solidity corpus is not configured")
	}
	type evidence struct{ file, text string }
	type target struct {
		file, name, excerpt string
		line                int
	}
	cases := []struct {
		project, query, unrelated, negativeName string
		files                                   []string
		evidence                                []evidence
		targets                                 []target
	}{
		{
			project:      "solidity-openzeppelin",
			query:        "_spendAllowance",
			unrelated:    "contracts/token/ERC721/ERC721.sol",
			negativeName: "_spendAllowance",
			files: []string{
				"contracts/token/ERC20/ERC20.sol",
				"contracts/token/ERC20/extensions/ERC20Burnable.sol",
				"contracts/token/ERC721/ERC721.sol",
			},
			evidence: []evidence{
				{"contracts/token/ERC20/ERC20.sol", "_spendAllowance(from, spender, value);"},
				{"contracts/token/ERC20/ERC20.sol", "if (currentAllowance < type(uint256).max) {"},
				{"contracts/token/ERC20/ERC20.sol", "_approve(owner, spender, currentAllowance - value, false);"},
				{"contracts/token/ERC20/extensions/ERC20Burnable.sol", "_spendAllowance(account, _msgSender(), value);"},
				{"test/token/ERC20/ERC20.test.js", "shouldBehaveLikeERC20(initialSupply, { forcedApproval });"},
				{"test/token/ERC20/ERC20.behavior.js", "when the spender has unlimited allowance"},
				{"test/token/ERC20/ERC20.behavior.js", "expect(await this.token.allowance(this.holder, this.recipient)).to.equal(ethers.MaxUint256);"},
				{"contracts/token/ERC721/ERC721.sol", "function transferFrom(address from, address to, uint256 tokenId)"},
			},
			targets: []target{
				{"contracts/token/ERC20/ERC20.sol", "transferFrom", "_spendAllowance(from, spender, value);", 142},
				{"contracts/token/ERC20/ERC20.sol", "_spendAllowance", "if (currentAllowance < type(uint256).max) {", 294},
				{"contracts/token/ERC20/extensions/ERC20Burnable.sol", "burnFrom", "_spendAllowance(account, _msgSender(), value);", 34},
				{"contracts/token/ERC721/ERC721.sol", "transferFrom", "ERC721InvalidReceiver", 115},
			},
		},
		{
			project:      "solidity-solmate",
			query:        "testInfiniteApproveTransferFrom",
			unrelated:    "src/tokens/ERC721.sol",
			negativeName: "testInfiniteApproveTransferFrom",
			files: []string{
				"src/tokens/ERC20.sol",
				"src/test/ERC20.t.sol",
				"src/tokens/ERC721.sol",
			},
			evidence: []evidence{
				{"src/tokens/ERC20.sol", "if (allowed != type(uint256).max) allowance[from][msg.sender] = allowed - amount;"},
				{"src/test/ERC20.t.sol", "function testTransferFrom() public {"},
				{"src/test/ERC20.t.sol", "function testInfiniteApproveTransferFrom() public {"},
				{"src/test/ERC20.t.sol", "assertEq(token.allowance(from, address(this)), type(uint256).max);"},
				{"src/tokens/ERC721.sol", "function safeTransferFrom("},
			},
			targets: []target{
				{"src/tokens/ERC20.sol", "transferFrom", "if (allowed != type(uint256).max)", 90},
				{"src/test/ERC20.t.sol", "testTransferFrom", "assertEq(token.allowance(from, address(this)), 0);", 56},
				{"src/test/ERC20.t.sol", "testInfiniteApproveTransferFrom", "assertEq(token.allowance(from, address(this)), type(uint256).max);", 73},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.project, func(t *testing.T) {
			root := filepath.Join(corpus, tc.project)
			paths := make([]string, 0, len(tc.files))
			for _, rel := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(rel))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original Solidity source missing or non-regular %s: %v", rel, err)
				}
				paths = append(paths, path)
			}
			for _, oracle := range tc.evidence {
				path := filepath.Join(root, filepath.FromSlash(oracle.file))
				info, err := os.Lstat(path)
				if err != nil || !info.Mode().IsRegular() {
					t.Fatalf("original oracle unavailable %s: %v", oracle.file, err)
				}
				body, err := os.ReadFile(path)
				if err != nil || !strings.Contains(string(body), oracle.text) {
					t.Fatalf("original source evidence %s %q changed: %v", oracle.file, oracle.text, err)
				}
			}
			h := NewHandler([]string{root})
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Paths: paths, Language: "solidity",
				Encoding: "utf-8", Presentation: "compact", MaxFiles: len(paths),
			})
			if err != nil || failed != nil || digest.Truncated || digest.FilesParsed != len(paths) {
				t.Fatalf("bounded digest error=%v failed=%+v result=%+v", err, failed, digest)
			}
			data, err := json.Marshal(digest)
			if err != nil || len(data) > 8192 {
				t.Fatalf("initial map too large: bytes=%d err=%v", len(data), err)
			}
			t.Logf("originalProject=1 files=%d symbols=%d compactBytes=%d coverageComplete=%v",
				digest.FilesParsed, digest.SymbolCount, len(data), digest.CoverageComplete)
			for _, want := range tc.targets {
				path := filepath.Join(root, filepath.FromSlash(want.file))
				failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "find", Paths: []string{path}, Query: want.name,
					Match: "exact", Language: "solidity", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 20,
				})
				if err != nil || failed != nil || found.Truncated || len(found.Files) != 1 {
					t.Fatalf("find %s: error=%v failed=%+v found=%+v", want.name, err, failed, found)
				}
				var symbolID string
				for _, sym := range found.Symbols {
					if sym.Name == want.name && string(sym.Kind) == "method" &&
						sym.NativeKind == "function" && sym.NameRange.Start.Line == want.line {
						symbolID = sym.ID
						break
					}
				}
				if symbolID == "" {
					t.Fatalf("missing exact original method %s:%d %s, results=%+v",
						want.file, want.line, want.name, found.Symbols)
				}
				failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "show", Path: path, SymbolID: symbolID,
					SourceFingerprint: found.Files[0].SourceFingerprint,
					Language:          "solidity", Encoding: "utf-8", MaxBytes: 8192,
				})
				if err != nil || failed != nil || shown.Show == nil ||
					!strings.Contains(shown.Show.Text, want.excerpt) {
					t.Fatalf("verified method excerpt %s: error=%v failed=%+v show=%+v", want.name, err, failed, shown.Show)
				}
			}
			failed, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Paths: paths, Query: tc.query,
				Language: "solidity", Encoding: "utf-8", Match: "exact",
				Mode: "structural", MaxResults: 20,
			})
			if err != nil || failed != nil || result.Search == nil || len(result.Search.Matches) == 0 {
				t.Fatalf("structural method search %s: error=%v failed=%+v result=%+v", tc.query, err, failed, result.Search)
			}
			failed, negative, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{filepath.Join(root, filepath.FromSlash(tc.unrelated))},
				Query: tc.negativeName, Match: "exact",
				Language: "solidity", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || negative.Truncated || len(negative.Symbols) != 0 {
				t.Fatalf("unrelated ERC721 invented an ERC20 target %s: error=%v failed=%+v symbols=%+v",
					tc.negativeName, err, failed, negative.Symbols)
			}
		})
	}
}
