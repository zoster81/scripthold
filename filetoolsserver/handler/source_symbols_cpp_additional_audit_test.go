package handler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Original C++ cases are selected independently from different corpus waves.
// External project binaries and tests are never executed.
func TestSourceSymbolsOriginalCppAdditionalAudit(t *testing.T) {
	corpus := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if corpus == "" {
		t.Skip("original C++ corpus unavailable")
	}
	for _, tc := range []struct {
		wave                                string
		sc                                  extraAuditScenario
		ownerFile, ownerName, ownerFragment string
	}{
		{
			wave: "replacement-corpus-wave6",
			sc: extraAuditScenario{
				project: "project-cpp-an-tao-trantor", language: "cpp",
				// Find intranet IPv4/IPv6 classification and defaults,
				// unit assertions and distinguish unrelated logging.
				search:        "isIntranetIp",
				primary:       "trantor/net/InetAddress.cc",
				negativeFile:  "trantor/utils/Logger.cc",
				negativeQuery: "isIntranetIp",
				paths: []string{
					"trantor/net/InetAddress.cc",
					"trantor/net/InetAddress.h",
					"trantor/unittests/InetAddressUnittest.cc",
					"trantor/utils/Logger.cc",
				},
				oracles: []extraAuditEvidence{
					{"trantor/net/InetAddress.cc", "InetAddress::InetAddress(uint16_t port, bool loopbackOnly, bool ipv6)"},
					{"trantor/net/InetAddress.cc", "bool InetAddress::isIntranetIp() const"},
					{"trantor/net/InetAddress.cc", "bool InetAddress::isLoopbackIp() const"},
					{"trantor/net/InetAddress.cc", "ip_addr >= 0xC0A80000 && ip_addr <= 0xC0A8FFFF"},
					{"trantor/net/InetAddress.h", "bool isIntranetIp() const;"},
					{"trantor/net/InetAddress.h", "bool isUnspecified() const"},
					{"trantor/unittests/InetAddressUnittest.cc", "TEST(InetAddress, innerIpTest)"},
					{"trantor/unittests/InetAddressUnittest.cc", "InetAddress(\"192.168.0.1\", 0).isIntranetIp()"},
					{"trantor/unittests/InetAddressUnittest.cc", "InetAddress(\"example.com\", 0).isUnspecified()"},
					{"trantor/unittests/InetAddressUnittest.cc", "TEST(InetAddress, toIpPortNetEndianTest)"},
				},
				targets: []extraAuditSymbol{
					{"trantor/net/InetAddress.cc", "isIntranetIp", 133},
					{"trantor/net/InetAddress.cc", "isLoopbackIp", 175},
					{"trantor/net/InetAddress.h", "isIntranetIp", 154},
				},
			},
			ownerFile: "trantor/net/InetAddress.cc", ownerName: "isIntranetIp", ownerFragment: "InetAddress",
		},
		{
			wave: "replacement-corpus-wave4",
			sc: extraAuditScenario{
				project: "project-cpp-batucangd-sorcerershowdown", language: "cpp",
				// Discover battle-end rules when no player survives,
				// spectator behavior and other turn coordination.
				search:        "GameEndCheck",
				primary:       "SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp",
				negativeFile:  "SorcererShowdownSource/code/source/GameManagement/PlayerManager.cpp",
				negativeQuery: "GameEndCheck",
				paths: []string{
					"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp",
					"SorcererShowdownSource/code/header/GameManagement/BattleManager.h",
					"SorcererShowdownSource/code/source/GameManagement/PlayerManager.cpp",
					"SorcererShowdownSource/code/header/Characters/Character.h",
				},
				oracles: []extraAuditEvidence{
					{"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", "bool BattleManager::GameEndCheck(bool spectator_mode) {"},
					{"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", "if (sc->GetCharacterHealth() > 0) {"},
					{"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", "if (sc->IsThePlayer()) player_found = true;"},
					{"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", "return (!spectator_mode && !player_found) || (alive_sorcerers <= 1);"},
					{"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", "void BattleManager::SetupChoice(Character*& chosen, bool& choosing, bool& multi)"},
					{"SorcererShowdownSource/code/header/GameManagement/BattleManager.h", "class BattleManager {"},
					{"SorcererShowdownSource/code/header/GameManagement/BattleManager.h", "bool GameEndCheck(bool spectator_mode);"},
					{"SorcererShowdownSource/code/source/GameManagement/PlayerManager.cpp", "void PlayerManager::OnPlayerTurn(Character* player, Battlefield& bf) {"},
					{"SorcererShowdownSource/code/header/Characters/Character.h", "bool IsThePlayer() const;"},
				},
				targets: []extraAuditSymbol{
					{"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", "GameEndCheck", 14},
					{"SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", "SetupChoice", 110},
					{"SorcererShowdownSource/code/header/GameManagement/BattleManager.h", "BattleManager", 8},
					{"SorcererShowdownSource/code/source/GameManagement/PlayerManager.cpp", "OnPlayerTurn", 53},
				},
			},
			ownerFile: "SorcererShowdownSource/code/source/GameManagement/BattleManager.cpp", ownerName: "GameEndCheck", ownerFragment: "BattleManager",
		},
	} {
		root := filepath.Join(corpus, "qualification", tc.wave)
		checkAdditionalOriginalNavigation(t, root, tc.sc)
		t.Run(tc.sc.project+"/owner", func(t *testing.T) {
			projectRoot := filepath.Join(root, tc.sc.project)
			p := filepath.Join(projectRoot, filepath.FromSlash(tc.ownerFile))
			requireAdditionalOriginalFile(t, p)
			h := NewHandler([]string{projectRoot})
			failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{p}, Query: tc.ownerName,
				Match: "exact", Language: "cpp", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 15,
			})
			if err != nil || failed != nil || len(out.Symbols) != 1 {
				t.Fatalf("C++ owner lookup %s err=%v failed=%+v symbols=%+v", tc.ownerName, err, failed, out.Symbols)
			}
			sym := out.Symbols[0]
			if !strings.Contains(sym.QualifiedName, tc.ownerFragment) &&
				!strings.Contains(sym.ParentQualifiedName, tc.ownerFragment) {
				t.Errorf("original out-of-class member %s lost owner %q: qualified=%q parent=%q",
					tc.ownerName, tc.ownerFragment, sym.QualifiedName, sym.ParentQualifiedName)
			}
		})
	}
}
