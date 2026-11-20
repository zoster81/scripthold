package handler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Independently selected Solidity maintenance tasks; original contracts are
// read as evidence only and never deployed, compiled or executed.
func TestSourceSymbolsOriginalSolidityAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original Solidity corpus unavailable")
	}
	for _, sc := range []extraAuditScenario{
		{
			project: "solidity-aave-v3", language: "solidity",
			// Locate total flash-loan premium admin control, bounded
			// validation, stored pool value, loan calculation and interface.
			search:        "updateFlashloanPremiumTotal",
			primary:       "contracts/protocol/pool/PoolConfigurator.sol",
			negativeFile:  "contracts/protocol/pool/DefaultReserveInterestRateStrategy.sol",
			negativeQuery: "updateFlashloanPremiumTotal",
			paths: []string{
				"contracts/protocol/pool/PoolConfigurator.sol",
				"contracts/protocol/pool/Pool.sol",
				"contracts/protocol/libraries/logic/FlashLoanLogic.sol",
				"contracts/interfaces/IPoolConfigurator.sol",
				"contracts/protocol/pool/DefaultReserveInterestRateStrategy.sol",
			},
			oracles: []extraAuditEvidence{
				{"contracts/protocol/pool/PoolConfigurator.sol", "contract PoolConfigurator is VersionedInitializable, IPoolConfigurator {"},
				{"contracts/protocol/pool/PoolConfigurator.sol", "function updateFlashloanPremiumTotal("},
				{"contracts/protocol/pool/PoolConfigurator.sol", "newFlashloanPremiumTotal <= PercentageMath.PERCENTAGE_FACTOR"},
				{"contracts/protocol/pool/PoolConfigurator.sol", "_pool.updateFlashloanPremiums(newFlashloanPremiumTotal, _pool.FLASHLOAN_PREMIUM_TO_PROTOCOL());"},
				{"contracts/protocol/pool/Pool.sol", "contract Pool is VersionedInitializable, PoolStorage, IPool {"},
				{"contracts/protocol/pool/Pool.sol", "_flashLoanPremiumTotal = flashLoanPremiumTotal;"},
				{"contracts/protocol/libraries/logic/FlashLoanLogic.sol", "function executeFlashLoan("},
				{"contracts/protocol/libraries/logic/FlashLoanLogic.sol", "vars.currentAmount.percentMul(vars.flashloanPremiumTotal)"},
				{"contracts/interfaces/IPoolConfigurator.sol", "function updateFlashloanPremiumTotal(uint128 newFlashloanPremiumTotal) external;"},
				{"contracts/protocol/pool/DefaultReserveInterestRateStrategy.sol", "contract DefaultReserveInterestRateStrategy"},
			},
			targets: []extraAuditSymbol{
				{"contracts/protocol/pool/PoolConfigurator.sol", "updateFlashloanPremiumTotal", 443},
				{"contracts/protocol/pool/PoolConfigurator.sol", "updateFlashloanPremiumToProtocol", 456},
				{"contracts/protocol/pool/Pool.sol", "updateFlashloanPremiums", 655},
				{"contracts/protocol/libraries/logic/FlashLoanLogic.sol", "executeFlashLoan", 70},
				{"contracts/interfaces/IPoolConfigurator.sol", "updateFlashloanPremiumTotal", 465},
			},
		},
		{
			project: "solidity-aircoin-official-aircash", language: "solidity",
			// Determine where to change cancel-order waiting time and
			// distinguish return-of-funds and listing remainders from
			// dispute processing.
			search: "cancelOrder", primary: "OrderStorage.sol",
			negativeFile: "AppealStorage.sol", negativeQuery: "cancelOrder",
			paths: []string{
				"OrderStorage.sol",
				"RestStorage.sol",
				"RecordStorage.sol",
				"RecordInterface.sol",
				"AppealStorage.sol",
			},
			oracles: []extraAuditEvidence{
				{"OrderStorage.sol", "uint256 cancelOrderTime = 30;"},
				{"OrderStorage.sol", "function setCancelOrderTime(uint256 _count) public onlyOwner {"},
				{"OrderStorage.sol", "function cancelOrder(uint256 _orderNo)"},
				{"OrderStorage.sol", "_order.orderDetail.tradeTime + cancelOrderTime * 1 minutes <"},
				{"OrderStorage.sol", "_recordStorage.addAvailableTotal("},
				{"OrderStorage.sol", "_restStorage.addRestRemainCount(_order.restNo, _order.coinCount);"},
				{"OrderStorage.sol", "_updateOrderStatus(_orderNo, 4);"},
				{"RestStorage.sol", "function addRestRemainCount(uint256 _restNo, uint256 _remainCount)"},
				{"RestStorage.sol", "_rest.restDetail.remainderCount = SafeMath.add("},
				{"RecordStorage.sol", "function addAvailableTotal("},
				{"RecordStorage.sol", "require(_amt > 0, \"Invalid transfer amount\");"},
				{"RecordInterface.sol", "function addAvailableTotal(address _addr, string memory _coinType, uint remainHoldCoin) external;"},
				{"AppealStorage.sol", "function applyAppeal(uint256 _o) external onlyBOS(_o) {"},
			},
			targets: []extraAuditSymbol{
				{"OrderStorage.sol", "setCancelOrderTime", 52},
				{"OrderStorage.sol", "cancelOrder", 392},
				{"OrderStorage.sol", "_updateOrderStatus", 467},
				{"RestStorage.sol", "addRestRemainCount", 515},
				{"RecordStorage.sol", "addAvailableTotal", 490},
				{"RecordInterface.sol", "addAvailableTotal", 14},
			},
		},
	} {
		checkAdditionalOriginalNavigation(t, corpus, sc)
	}
	// Independently read Aave's named contract/interface declarations.
	// A complete digest with orphaned functions is misleading.
	t.Run("aave-contract-owners", func(t *testing.T) {
		root := filepath.Join(corpus, "solidity-aave-v3")
		h := NewHandler([]string{root})
		for _, tc := range []struct {
			file, owner, member, kind string
		}{
			{"contracts/protocol/pool/PoolConfigurator.sol", "PoolConfigurator", "updateFlashloanPremiumTotal", "class"},
			{"contracts/protocol/pool/Pool.sol", "Pool", "updateFlashloanPremiums", "class"},
			{"contracts/interfaces/IPoolConfigurator.sol", "IPoolConfigurator", "updateFlashloanPremiumTotal", "interface"},
		} {
			path := filepath.Join(root, filepath.FromSlash(tc.file))
			requireAdditionalOriginalFile(t, path)
			failed, owners, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Query: tc.owner, Match: "exact",
				Language: "solidity", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || owners.Truncated || !owners.CoverageComplete {
				t.Fatalf("owner lookup %s: err=%v failed=%+v result=%+v", tc.owner, err, failed, owners)
			}
			ok := false
			for _, sym := range owners.Symbols {
				if sym.Name == tc.owner && string(sym.Kind) == tc.kind {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("missing original Solidity owner %s in %s: %+v", tc.owner, tc.file, owners.Symbols)
			}
			failed, members, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Query: tc.member, Match: "exact",
				Language: "solidity", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 16,
			})
			if err != nil || failed != nil || members.Truncated || !members.CoverageComplete {
				t.Fatalf("member lookup %s: err=%v failed=%+v", tc.member, err, failed)
			}
			owned := false
			for _, sym := range members.Symbols {
				if sym.Name == tc.member && sym.ParentQualifiedName == tc.owner && sym.QualifiedName == tc.owner+"."+tc.member {
					owned = true
				}
			}
			if !owned {
				t.Fatalf("orphan/misowned function %s.%s: %+v", tc.owner, tc.member, members.Symbols)
			}
		}
	})
}
