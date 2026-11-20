package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zoster81/scripthold/internal/config"
)

// Complete source-derived header inventory; the connected server may run older
// C macro parsing and must not be conflated with development handler results.
func TestSourceSymbolsOriginalCJSONUtilsHeader(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	path := filepath.Join(root, filepath.FromSlash("real-projects/c-davegamble-cjson/cJSON_Utils.h"))
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "c", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || len(out.Files) != 1 {
		t.Fatalf("C original header outline: failed=%+v err=%v output=%+v", failed, err, out)
	}
	want := map[string]int{
		"cJSONUtils_GetPointer":                      34,
		"cJSONUtils_GetPointerCaseSensitive":         35,
		"cJSONUtils_GeneratePatches":                 39,
		"cJSONUtils_GeneratePatchesCaseSensitive":    40,
		"cJSONUtils_AddPatchToArray":                 42,
		"cJSONUtils_ApplyPatches":                    44,
		"cJSONUtils_ApplyPatchesCaseSensitive":       45,
		"cJSONUtils_MergePatch":                      70,
		"cJSONUtils_MergePatchCaseSensitive":         71,
		"cJSONUtils_GenerateMergePatch":              74,
		"cJSONUtils_GenerateMergePatchCaseSensitive": 75,
		"cJSONUtils_FindPointerFromObjectTo":         78,
		"cJSONUtils_SortObject":                      81,
		"cJSONUtils_SortObjectCaseSensitive":         82,
	}
	seen := make(map[string]int, len(out.Symbols))
	for _, sym := range out.Symbols {
		seen[sym.QualifiedName]++
		if line, ok := want[sym.QualifiedName]; !ok || string(sym.Kind) != "function" ||
			sym.NameRange.Start.Line != line || sym.ParentQualifiedName != "" {
			t.Errorf("unexpected original C declaration: %+v", sym)
		}
	}
	for name := range want {
		if seen[name] != 1 {
			t.Errorf("%s declaration count %d, want 1", name, seen[name])
		}
	}
	if len(out.Symbols) != len(want) {
		t.Errorf("found %d declarations, want %d", len(out.Symbols), len(want))
	}
}

// Real source is read only; selected declarations were independently annotated.
func TestSourceSymbolsGroovyPackerOriginalSelectedFile(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	path := filepath.Join(root, filepath.FromSlash("real-projects/groovy-mcxiaoke-gradle-packer-plugin/plugin/src/main/groovy/com/mcxiaoke/packer/AndroidPackerExtension.groovy"))
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "groovy", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || out.Truncated || !out.CoverageComplete {
		t.Fatalf("original Groovy outline failed: %v %+v", err, failed)
	}
	want := map[string]int{
		"com.mcxiaoke.packer":                                               1,
		"com.mcxiaoke.packer.AndroidPackerExtension":                        6,
		"com.mcxiaoke.packer.AndroidPackerExtension.archiveOutput":          11,
		"com.mcxiaoke.packer.AndroidPackerExtension.archiveNameFormat":      28,
		"com.mcxiaoke.packer.AndroidPackerExtension.manifestMatcher":        33,
		"com.mcxiaoke.packer.AndroidPackerExtension.buildNumberAuto":        40,
		"com.mcxiaoke.packer.AndroidPackerExtension.buildNumberTypeMatcher": 44,
		"com.mcxiaoke.packer.AndroidPackerExtension.AndroidPackerExtension": 47,
	}
	seen := map[string]int{}
	for _, s := range out.Symbols {
		seen[s.QualifiedName]++
		if line, ok := want[s.QualifiedName]; !ok || s.NameRange.Start.Line != line {
			t.Errorf("unexpected declaration: %s %s line %d", s.QualifiedName, s.Kind, s.NameRange.Start.Line)
		}
	}
	for name := range want {
		if seen[name] != 1 {
			t.Errorf("missing/duplicated: %s (%d)", name, seen[name])
		}
	}
	const pkg = "com.mcxiaoke.packer"
	const owner = pkg + ".AndroidPackerExtension"
	var ctorID string
	for _, s := range out.Symbols {
		kind, parent := "field", owner
		switch s.QualifiedName {
		case pkg:
			kind, parent = "package", ""
		case owner:
			kind, parent = "class", pkg
		case owner + ".AndroidPackerExtension":
			kind = "constructor"
			ctorID = s.ID
		}
		if string(s.Kind) != kind || s.ParentQualifiedName != parent {
			t.Errorf("wrong kind/ownership for %s: kind=%s parent=%s", s.QualifiedName, s.Kind, s.ParentQualifiedName)
		}
	}
	if len(out.Symbols) != len(want) || len(out.Files) != 1 || ctorID == "" {
		t.Fatalf("original file inventory mismatch: count=%d expected=%d", len(out.Symbols), len(want))
	}
	for _, view := range []string{"tree", "compact"} {
		failed, outline, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: view, Paths: []string{path},
			Language: "groovy", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
		})
		if err != nil || failed != nil || outline.SymbolCount != len(want) ||
			!outline.CoverageComplete || len(outline.OutlineFiles) != 1 ||
			outline.OutlineFiles[0].SourceFingerprint != out.Files[0].SourceFingerprint {
			t.Fatalf("original %s outline inconsistent: err=%v failed=%+v", view, err, failed)
		}
	}
	failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Presentation: "compact", Paths: []string{path},
		Language: "groovy", Encoding: "utf-8", MaxFiles: 1,
	})
	if err != nil || failed != nil || !digest.CoverageComplete ||
		digest.SymbolCount != len(want) || len(digest.OverviewFiles) != 1 {
		t.Fatalf("original compact digest mismatch: err=%v failed=%+v", err, failed)
	}
	failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Language: "groovy", Encoding: "utf-8",
		Query: "buildNumberTypeMatcher", Match: "exact", MaxSymbols: 8,
	})
	if err != nil || failed != nil || len(found.Symbols) != 1 ||
		found.Symbols[0].QualifiedName != owner+".buildNumberTypeMatcher" {
		t.Fatalf("original find mismatch: err=%v failed=%+v symbols=%+v", err, failed, found.Symbols)
	}
	failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: ctorID,
		SourceFingerprint: out.Files[0].SourceFingerprint,
		Language:          "groovy", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil || failed != nil || shown.Show == nil ||
		!strings.Contains(shown.Show.Text, "archiveOutput = new File(") {
		t.Fatalf("original constructor show mismatch: err=%v failed=%+v shown=%+v", err, failed, shown.Show)
	}
	failed, result, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
		Operation: "search", Mode: "structural", Paths: []string{path},
		Language: "groovy", Encoding: "utf-8", Query: "manifestMatcher",
		Match: "exact", MaxResults: 8,
	})
	if err != nil || failed != nil || result.Search == nil ||
		len(result.Search.Matches) != 1 ||
		result.Search.Matches[0].QualifiedName != owner+".manifestMatcher" {
		t.Fatalf("original structural search mismatch: err=%v failed=%+v search=%+v", err, failed, result.Search)
	}
}

// Check the entire selected original Solidity file against a source-derived inventory.
func TestSourceSymbolsSolidityAccessOriginalSelectedFile(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	path := filepath.Join(root, filepath.FromSlash("real-projects/solidity-fravoll-solidity-patterns/AccessRestriction/AccessRestriction.sol"))
	h := NewHandler([]string{root})
	failed, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path},
		Language: "solidity", Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 64,
	})
	if err != nil || failed != nil || !out.CoverageComplete || out.Truncated || len(out.Files) != 1 {
		t.Fatalf("original Solidity outline failed: err=%v failed=%+v", err, failed)
	}
	want := map[string]struct {
		kind string
		line int
	}{
		"AccessRestriction":                 {"class", 6},
		"AccessRestriction.owner":           {"field", 8},
		"AccessRestriction.lastOwnerChange": {"field", 9},
		"AccessRestriction.onlyBy":          {"method", 11},
		"AccessRestriction.onlyAfter":       {"method", 16},
		"AccessRestriction.costs":           {"method", 21},
		"AccessRestriction.changeOwner":     {"method", 29},
		"AccessRestriction.buyContract":     {"method", 33},
	}
	seen := map[string]int{}
	for _, s := range out.Symbols {
		seen[s.QualifiedName]++
		w, ok := want[s.QualifiedName]
		parent := "AccessRestriction"
		if s.QualifiedName == "AccessRestriction" {
			parent = ""
		}
		if !ok || string(s.Kind) != w.kind || s.NameRange.Start.Line != w.line || s.ParentQualifiedName != parent {
			t.Errorf("unexpected original Solidity declaration %+v", s)
		}
	}
	for name := range want {
		if seen[name] != 1 {
			t.Errorf("missing/duplicate %s: %d", name, seen[name])
		}
	}
}

// Each selected original file has an independently reviewed, complete inventory.
// The corpus remains a private opt-in test input rather than a repository fixture.
func TestSourceSymbolsAdditionalOriginalBraceInventories(t *testing.T) {
	root := os.Getenv("SCRIPTHOLD_SOURCE_CORPUS_ROOT")
	if root == "" {
		t.Skip("private original corpus is not configured")
	}
	type declaration struct {
		qualified, kind string
		line            int
	}
	cases := []struct {
		name, relative, language, pkg, owner, query, snippet string
		want                                                 []declaration
	}{
		{
			name:     "groovy-method-signature",
			relative: "real-projects/groovy-jenkinsci-jenkinspipelineunit/src/main/groovy/com/lesfurets/jenkins/unit/MethodSignature.groovy",
			language: "groovy", pkg: "com.lesfurets.jenkins.unit",
			owner: "com.lesfurets.jenkins.unit.MethodSignature",
			query: "argsToString", snippet: "return args.collect",
			want: []declaration{
				{"com.lesfurets.jenkins.unit", "package", 1},
				{"com.lesfurets.jenkins.unit.MethodSignature", "class", 8},
				{"com.lesfurets.jenkins.unit.MethodSignature.name", "field", 9},
				{"com.lesfurets.jenkins.unit.MethodSignature.args", "field", 10},
				{"com.lesfurets.jenkins.unit.MethodSignature.method", "method", 12},
				{"com.lesfurets.jenkins.unit.MethodSignature.MethodSignature", "constructor", 16},
				{"com.lesfurets.jenkins.unit.MethodSignature.argsToString", "method", 21},
				{"com.lesfurets.jenkins.unit.MethodSignature.equals", "method", 32},
				{"com.lesfurets.jenkins.unit.MethodSignature.hashCode", "method", 54},
				{"com.lesfurets.jenkins.unit.MethodSignature.toString", "method", 63},
			},
		},
		{
			name:     "groovy-directory-manager",
			relative: "real-projects/groovy-kezong-fat-aar-android/source/src/main/groovy/com/kezong/fataar/DirectoryManager.groovy",
			language: "groovy", pkg: "com.kezong.fataar",
			owner: "com.kezong.fataar.DirectoryManager",
			query: "getRJavaDirectory", snippet: "return sProject.file",
			want: []declaration{
				{"com.kezong.fataar", "package", 1},
				{"com.kezong.fataar.DirectoryManager", "class", 10},
				{"com.kezong.fataar.DirectoryManager.RE_BUNDLE_FOLDER", "field", 12},
				{"com.kezong.fataar.DirectoryManager.INTERMEDIATES_TEMP_FOLDER", "field", 14},
				{"com.kezong.fataar.DirectoryManager.sProject", "field", 16},
				{"com.kezong.fataar.DirectoryManager.attach", "method", 18},
				{"com.kezong.fataar.DirectoryManager.getReBundleDirectory", "method", 22},
				{"com.kezong.fataar.DirectoryManager.getRJavaDirectory", "method", 26},
				{"com.kezong.fataar.DirectoryManager.getRClassDirectory", "method", 30},
				{"com.kezong.fataar.DirectoryManager.getRJarDirectory", "method", 34},
				{"com.kezong.fataar.DirectoryManager.getMergeClassDirectory", "method", 38},
				{"com.kezong.fataar.DirectoryManager.getKotlinMetaDirectory", "method", 42},
			},
		},
		{
			name:     "solidity-atoken-harness",
			relative: "real-projects/solidity-aave-v3/certora/harness/ATokenHarness.sol",
			language: "solidity", owner: "ATokenHarness",
			query: "scaledTotalSupply", snippet: "uint256 val = super.scaledTotalSupply();",
			want: []declaration{
				{"ATokenHarness", "class", 13},
				{"ATokenHarness.ATokenHarness", "constructor", 17},
				{"ATokenHarness.scaledTotalSupply", "method", 19},
				{"ATokenHarness.additionalData", "method", 24},
				{"ATokenHarness.scaledBalanceOfToBalanceOf", "method", 28},
			},
		},
		{
			name:     "groovy-aspectjx-config",
			relative: "real-projects/groovy-hujiangtechnology-gradle_plugin_android_aspectjx/aspectjx/src/main/groovy/com/hujiang/gradle/plugin/android/aspectjx/AJXConfig.groovy",
			language: "groovy", pkg: "com.hujiang.gradle.plugin.android.aspectjx", owner: "com.hujiang.gradle.plugin.android.aspectjx.AJXConfig",
			query: "getVariants", snippet: "return hasAppPlugin ?",
			want: []declaration{
				{"com.hujiang.gradle.plugin.android.aspectjx", "package", 16},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig", "class", 32},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig.project", "field", 34},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig.hasAppPlugin", "field", 35},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig.hasLibPlugin", "field", 36},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig.plugin", "field", 37},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig.AJXConfig", "constructor", 39},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig.getVariants", "method", 55},
				{"com.hujiang.gradle.plugin.android.aspectjx.AJXConfig.getBootClasspath", "method", 63},
			},
		},
		{
			name:     "groovy-palantir-docker",
			relative: "groovy-palantir-gradle-docker/src/main/groovy/com/palantir/gradle/docker/DockerComposeExtension.groovy",
			language: "groovy", pkg: "com.palantir.gradle.docker", owner: "com.palantir.gradle.docker.DockerComposeExtension",
			query: "templateToken", snippet: "this.templateTokens.put(key, value)",
			want: []declaration{
				{"com.palantir.gradle.docker", "package", 16},
				{"com.palantir.gradle.docker.DockerComposeExtension", "class", 21},
				{"com.palantir.gradle.docker.DockerComposeExtension.project", "field", 22},
				{"com.palantir.gradle.docker.DockerComposeExtension.template", "field", 24},
				{"com.palantir.gradle.docker.DockerComposeExtension.dockerComposeFile", "field", 25},
				{"com.palantir.gradle.docker.DockerComposeExtension.templateTokens", "field", 26},
				{"com.palantir.gradle.docker.DockerComposeExtension.DockerComposeExtension", "constructor", 28},
				{"com.palantir.gradle.docker.DockerComposeExtension.setTemplate", "method", 34},
				{"com.palantir.gradle.docker.DockerComposeExtension.setDockerComposeFile", "method", 38},
				{"com.palantir.gradle.docker.DockerComposeExtension.setTemplateTokens", "method", 42},
				{"com.palantir.gradle.docker.DockerComposeExtension.templateToken", "method", 46},
				{"com.palantir.gradle.docker.DockerComposeExtension.getTemplateTokens", "method", 50},
				{"com.palantir.gradle.docker.DockerComposeExtension.getTemplate", "method", 54},
				{"com.palantir.gradle.docker.DockerComposeExtension.getDockerComposeFile", "method", 58},
			},
		},
		{
			name:     "groovy-google-protobuf",
			relative: "real-projects/groovy-google-protobuf-gradle-plugin/src/main/groovy/com/google/protobuf/gradle/ExecutableLocator.groovy",
			language: "groovy", pkg: "com.google.protobuf.gradle", owner: "com.google.protobuf.gradle.ExecutableLocator",
			query: "resolve", snippet: "this.artifactFiles = artifactFiles",
			want: []declaration{
				{"com.google.protobuf.gradle", "package", 29},
				{"com.google.protobuf.gradle.ExecutableLocator", "class", 43},
				{"com.google.protobuf.gradle.ExecutableLocator.name", "field", 45},
				{"com.google.protobuf.gradle.ExecutableLocator.artifact", "field", 47},
				{"com.google.protobuf.gradle.ExecutableLocator.path", "field", 48},
				{"com.google.protobuf.gradle.ExecutableLocator.artifactFiles", "field", 50},
				{"com.google.protobuf.gradle.ExecutableLocator.simplifiedArtifactName", "field", 51},
				{"com.google.protobuf.gradle.ExecutableLocator.ExecutableLocator", "constructor", 53},
				{"com.google.protobuf.gradle.ExecutableLocator.getName", "method", 58},
				{"com.google.protobuf.gradle.ExecutableLocator.setArtifact", "method", 66},
				{"com.google.protobuf.gradle.ExecutableLocator.setPath", "method", 74},
				{"com.google.protobuf.gradle.ExecutableLocator.getArtifact", "method", 79},
				{"com.google.protobuf.gradle.ExecutableLocator.getPath", "method", 83},
				{"com.google.protobuf.gradle.ExecutableLocator.getArtifactFiles", "method", 88},
				{"com.google.protobuf.gradle.ExecutableLocator.getSimplifiedArtifactName", "method", 95},
				{"com.google.protobuf.gradle.ExecutableLocator.resolve", "method", 102},
			},
		},
		{
			name:     "groovy-autoregister",
			relative: "groovy-luckybilly-autoregister/autoregister/src/main/groovy/com/billy/android/register/AutoRegisterConfig.groovy",
			language: "groovy", pkg: "com.billy.android.register", owner: "com.billy.android.register.AutoRegisterConfig",
			query: "reset", snippet: "info.reset()",
			want: []declaration{
				{"com.billy.android.register", "package", 1},
				{"com.billy.android.register.AutoRegisterConfig", "class", 10},
				{"com.billy.android.register.AutoRegisterConfig.registerInfo", "field", 12},
				{"com.billy.android.register.AutoRegisterConfig.list", "field", 14},
				{"com.billy.android.register.AutoRegisterConfig.project", "field", 16},
				{"com.billy.android.register.AutoRegisterConfig.cacheEnabled", "field", 17},
				{"com.billy.android.register.AutoRegisterConfig.AutoRegisterConfig", "constructor", 19},
				{"com.billy.android.register.AutoRegisterConfig.convertConfig", "method", 21},
				{"com.billy.android.register.AutoRegisterConfig.checkRegisterInfo", "method", 57},
				{"com.billy.android.register.AutoRegisterConfig.deleteFile", "method", 83},
				{"com.billy.android.register.AutoRegisterConfig.reset", "method", 90},
				{"com.billy.android.register.AutoRegisterConfig.toString", "method", 97},
			},
		},
		{
			name:     "solidity-openzeppelin-ownable",
			relative: "real-projects/solidity-openzeppelin/contracts/access/Ownable.sol",
			language: "solidity", pkg: "", owner: "Ownable",
			query: "transferOwnership", snippet: "_transferOwnership(newOwner)",
			want: []declaration{
				{"Ownable", "class", 20},
				{"Ownable._owner", "field", 21},
				{"Ownable.OwnableUnauthorizedAccount", "type", 26},
				{"Ownable.OwnableInvalidOwner", "type", 31},
				{"Ownable.OwnershipTransferred", "event", 33},
				{"Ownable.Ownable", "constructor", 38},
				{"Ownable.onlyOwner", "method", 48},
				{"Ownable.owner", "method", 56},
				{"Ownable._checkOwner", "method", 63},
				{"Ownable.renounceOwnership", "method", 76},
				{"Ownable.transferOwnership", "method", 84},
				{"Ownable._transferOwnership", "method", 95},
			},
		},
		{
			name:     "solidity-compound-jump",
			relative: "real-projects/solidity-compound/contracts/BaseJumpRateModelV2.sol",
			language: "solidity", pkg: "", owner: "BaseJumpRateModelV2",
			query: "utilizationRate", snippet: "return borrows * BASE",
			want: []declaration{
				{"BaseJumpRateModelV2", "class", 11},
				{"BaseJumpRateModelV2.NewInterestParams", "event", 12},
				{"BaseJumpRateModelV2.BASE", "field", 14},
				{"BaseJumpRateModelV2.owner", "field", 19},
				{"BaseJumpRateModelV2.blocksPerYear", "field", 24},
				{"BaseJumpRateModelV2.multiplierPerBlock", "field", 29},
				{"BaseJumpRateModelV2.baseRatePerBlock", "field", 34},
				{"BaseJumpRateModelV2.jumpMultiplierPerBlock", "field", 39},
				{"BaseJumpRateModelV2.kink", "field", 44},
				{"BaseJumpRateModelV2.BaseJumpRateModelV2", "constructor", 54},
				{"BaseJumpRateModelV2.updateJumpRateModel", "method", 67},
				{"BaseJumpRateModelV2.utilizationRate", "method", 80},
				{"BaseJumpRateModelV2.getBorrowRateInternal", "method", 96},
				{"BaseJumpRateModelV2.getSupplyRate", "method", 116},
				{"BaseJumpRateModelV2.updateJumpRateModelInternal", "method", 130},
			},
		},
		{
			name:     "groovy-gradle-node",
			relative: "groovy-srs-gradle-node-plugin/src/main/groovy/com/moowork/gradle/node/NodePlugin.groovy",
			language: "groovy", pkg: "com.moowork.gradle.node",
			owner: "com.moowork.gradle.node.NodePlugin",
			query: "configureSetupTask", snippet: "this.setupTask.setEnabled",
			want: []declaration{
				{"com.moowork.gradle.node", "package", 1},
				{"com.moowork.gradle.node.NodePlugin", "class", 15},
				{"com.moowork.gradle.node.NodePlugin.project", "field", 18},
				{"com.moowork.gradle.node.NodePlugin.config", "field", 20},
				{"com.moowork.gradle.node.NodePlugin.setupTask", "field", 22},
				{"com.moowork.gradle.node.NodePlugin.npmSetupTask", "field", 24},
				{"com.moowork.gradle.node.NodePlugin.yarnSetupTask", "field", 26},
				{"com.moowork.gradle.node.NodePlugin.apply", "method", 29},
				{"com.moowork.gradle.node.NodePlugin.addGlobalTypes", "method", 47},
				{"com.moowork.gradle.node.NodePlugin.addTasks", "method", 54},
				{"com.moowork.gradle.node.NodePlugin.addGlobalTaskType", "method", 63},
				{"com.moowork.gradle.node.NodePlugin.addNpmRule", "method", 68},
				{"com.moowork.gradle.node.NodePlugin.addYarnRule", "method", 97},
				{"com.moowork.gradle.node.NodePlugin.configureSetupTask", "method", 117},
				{"com.moowork.gradle.node.NodePlugin.configureNpmSetupTask", "method", 122},
				{"com.moowork.gradle.node.NodePlugin.configureYarnSetupTask", "method", 127},
			},
		},
		{
			name:     "solidity-wotsplus-library",
			relative: "real-projects/solidity-quipnetwork-hashsigs-solidity/contracts/WOTSPlus.sol",
			language: "solidity", owner: "WOTSPlus",
			query: "verifyWithRandomizationElements", snippet: "return computedHash == quipAddress.publicKeyHash",
			want: []declaration{
				{"WOTSPlus", "module", 22},
				{"WOTSPlus.SignatureSize", "field", 26},
				{"WOTSPlus.PublicKeySize", "field", 28},
				{"WOTSPlus.Hash", "method", 31},
				{"WOTSPlus.HashLen", "field", 38},
				{"WOTSPlus.MessageLen", "field", 49},
				{"WOTSPlus.ChainLen", "field", 61},
				{"WOTSPlus.LgChainLen", "field", 63},
				{"WOTSPlus.NumMessageChunks", "field", 70},
				{"WOTSPlus.NumChecksumChunks", "field", 78},
				{"WOTSPlus.NumSignatureChunks", "field", 80},
				{"WOTSPlus.WinternitzAddress", "struct", 82},
				{"WOTSPlus.WinternitzAddress.publicSeed", "field", 83},
				{"WOTSPlus.WinternitzAddress.publicKeyHash", "field", 84},
				{"WOTSPlus.WinternitzElements", "struct", 87},
				{"WOTSPlus.WinternitzElements.elements", "field", 88},
				{"WOTSPlus.WinternitzMessage", "struct", 91},
				{"WOTSPlus.WinternitzMessage.messageHash", "field", 92},
				{"WOTSPlus.verify", "method", 102},
				{"WOTSPlus.verifyWithRandomizationElements", "method", 163},
				{"WOTSPlus.sign", "method", 217},
				{"WOTSPlus.generateKeyPair", "method", 244},
				{"WOTSPlus.generateRandomizationElements", "method", 282},
				{"WOTSPlus.chain", "method", 294},
				{"WOTSPlus.xor", "method", 311},
				{"WOTSPlus.setSlice32", "method", 316},
				{"WOTSPlus.prf", "method", 326},
				{"WOTSPlus.ComputeMessageHashChainIndexes", "method", 337},
				{"WOTSPlus.checksum", "method", 345},
				{"WOTSPlus.toBaseW", "method", 372},
			},
		},
		{
			name:     "solidity-appeal-storage",
			relative: "real-projects/solidity-aircoin-official-aircash/AppealStorage.sol",
			language: "solidity", owner: "AppealStorage",
			query: "searchAppeal", snippet: "return appeals[_o];",
			want: []declaration{
				{"AppealStorage", "class", 10},
				{"AppealStorage._oSt", "field", 11},
				{"AppealStorage._rSt", "field", 12},
				{"AppealStorage._uSt", "field", 13},
				{"AppealStorage.recAddr", "field", 14},
				{"AppealStorage.Appeal", "struct", 16},
				{"AppealStorage.Appeal.user", "field", 17},
				{"AppealStorage.Appeal.appealNo", "field", 18},
				{"AppealStorage.Appeal.orderNo", "field", 19},
				{"AppealStorage.Appeal.witness", "field", 20},
				{"AppealStorage.Appeal.buyer", "field", 21},
				{"AppealStorage.Appeal.seller", "field", 22},
				{"AppealStorage.Appeal.mortgage", "field", 23},
				{"AppealStorage.Appeal.status", "field", 24},
				{"AppealStorage.Appeal.appealTime", "field", 25},
				{"AppealStorage.Appeal.witTakeTime", "field", 26},
				{"AppealStorage.Appeal.obTakeTime", "field", 27},
				{"AppealStorage.Appeal.detail", "field", 28},
				{"AppealStorage.AppealDetail", "struct", 31},
				{"AppealStorage.AppealDetail.finalAppealAddr", "field", 32},
				{"AppealStorage.AppealDetail.updateTime", "field", 33},
				{"AppealStorage.AppealDetail.witnessReason", "field", 34},
				{"AppealStorage.AppealDetail.witnessAppealStatus", "field", 35},
				{"AppealStorage.AppealDetail.observerReason", "field", 36},
				{"AppealStorage.AppealDetail.witnessHandleTime", "field", 37},
				{"AppealStorage.AppealDetail.observerHandleTime", "field", 38},
				{"AppealStorage.AppealDetail.observerAddr", "field", 39},
				{"AppealStorage.AppealDetail.witnessHandleReward", "field", 40},
				{"AppealStorage.AppealDetail.observerHandleReward", "field", 41},
				{"AppealStorage.AppealDetail.witnessHandleCredit", "field", 42},
				{"AppealStorage.AppealDetail.observerHandleCredit", "field", 43},
				{"AppealStorage.AppealDetail.witReward", "field", 44},
				{"AppealStorage.AppealDetail.witSub", "field", 45},
				{"AppealStorage.AppealDetail.witCreditR", "field", 46},
				{"AppealStorage.AppealDetail.witCreditS", "field", 47},
				{"AppealStorage.appeals", "field", 50},
				{"AppealStorage.appealIndex", "field", 51},
				{"AppealStorage.appealList", "field", 53},
				{"AppealStorage.AppealStorage", "constructor", 55},
				{"AppealStorage.onlyWit", "method", 66},
				{"AppealStorage.onlyOb", "method", 73},
				{"AppealStorage.onlyBOS", "method", 80},
				{"AppealStorage._insert", "method", 90},
				{"AppealStorage.chanT", "method", 139},
				{"AppealStorage.applyAppeal", "method", 188},
				{"AppealStorage.takeWit", "method", 196},
				{"AppealStorage.takeOb", "method", 214},
				{"AppealStorage.changeHandler", "method", 232},
				{"AppealStorage.witOrOb", "method", 255},
				{"AppealStorage.applyFinal", "method", 263},
				{"AppealStorage.witnessOpt", "method", 291},
				{"AppealStorage.observerOpt", "method", 338},
				{"AppealStorage.searchAppeal", "method", 446},
				{"AppealStorage.searchAppealList", "method", 454},
			},
		},
		{
			name:     "solidity-mighty-arbitrage",
			relative: "real-projects/solidity-mighty-alien-ai-trader-bot/contract.sol",
			language: "solidity", owner: "Arbitrage",
			query: "executeArbitrage", snippet: "emit ArbitrageExecuted",
			want: []declaration{
				{"ISwapRouter", "interface", 4},
				{"ISwapRouter.ExactInputParams", "struct", 5},
				{"ISwapRouter.ExactInputParams.path", "field", 6},
				{"ISwapRouter.ExactInputParams.recipient", "field", 7},
				{"ISwapRouter.ExactInputParams.deadline", "field", 8},
				{"ISwapRouter.ExactInputParams.amountIn", "field", 9},
				{"ISwapRouter.ExactInputParams.amountOutMinimum", "field", 10},
				{"ISwapRouter.exactInput", "method", 12},
				{"ISwapRouterV2", "interface", 15},
				{"ISwapRouterV2.swapExactTokensForTokens", "method", 16},
				{"ISwapRouterV2.swapExactETHForTokens", "method", 24},
				{"Arbitrage", "class", 65},
				{"Arbitrage.ROUTER", "field", 69},
				{"Arbitrage.WETH", "field", 70},
				{"Arbitrage.NATIVE", "field", 71},
				{"Arbitrage.owner", "field", 76},
				{"Arbitrage.allowed", "field", 77},
				{"Arbitrage.allowedRouters", "field", 78},
				{"Arbitrage.paused", "field", 79},
				{"Arbitrage._reentrancyLock", "field", 80},
				{"Arbitrage.defaultTokenOut", "field", 82},
				{"Arbitrage.defaultFee", "field", 83},
				{"Arbitrage.minQuickSwapAmount", "field", 85},
				{"Arbitrage.maxQuickSwapAmount", "field", 86},
				{"Arbitrage.Swapped", "event", 91},
				{"Arbitrage.ArbitrageExecuted", "event", 92},
				{"Arbitrage.TokenAllowedSet", "event", 93},
				{"Arbitrage.RouterAllowedSet", "event", 94},
				{"Arbitrage.PausedSet", "event", 95},
				{"Arbitrage.Withdraw", "event", 96},
				{"Arbitrage.EmergencyWithdrawAll", "event", 97},
				{"Arbitrage.Deposit", "event", 98},
				{"Arbitrage.DefaultTokenOutSet", "event", 99},
				{"Arbitrage.DefaultFeeSet", "event", 100},
				{"Arbitrage.MinQuickSwapAmountSet", "event", 101},
				{"Arbitrage.MaxQuickSwapAmountSet", "event", 102},
				{"Arbitrage.onlyOwner", "method", 141},
				{"Arbitrage.whenNotPaused", "method", 146},
				{"Arbitrage.nonReentrant", "method", 151},
				{"Arbitrage.receive", "method", 169},
				{"Arbitrage.Arbitrage", "constructor", 161},
				{"Arbitrage.setTokenAllowed", "method", 174},
				{"Arbitrage.setRouterAllowed", "method", 179},
				{"Arbitrage.setPaused", "method", 184},
				{"Arbitrage.withdraw", "method", 189},
				{"Arbitrage.setDefaultTokenOut", "method", 201},
				{"Arbitrage.setDefaultFee", "method", 208},
				{"Arbitrage.setMinQuickSwapAmount", "method", 213},
				{"Arbitrage.setMaxQuickSwapAmount", "method", 218},
				{"Arbitrage._safeTransfer", "method", 226},
				{"Arbitrage._safeTransferFrom", "method", 234},
				{"Arbitrage._safeApprove", "method", 242},
				{"Arbitrage._getAllowance", "method", 249},
				{"Arbitrage._balanceOf", "method", 257},
				{"Arbitrage._approveIfNeeded", "method", 266},
				{"Arbitrage._firstToken", "method", 276},
				{"Arbitrage._lastToken", "method", 281},
				{"Arbitrage._unwrapAndSend", "method", 286},
				{"Arbitrage.swap", "method", 296},
				{"Arbitrage._executeSwap", "method", 323},
				{"Arbitrage.quickSwap", "method", 358},
				{"Arbitrage.quickSwapFromBalance", "method", 377},
				{"Arbitrage.SwapLeg", "struct", 400},
				{"Arbitrage.SwapLeg.router", "field", 401},
				{"Arbitrage.SwapLeg.path", "field", 402},
				{"Arbitrage.SwapLeg.amountOutMinimum", "field", 403},
				{"Arbitrage.SwapLeg.useV2", "field", 404},
				{"Arbitrage.SwapLeg.v2Path", "field", 405},
				{"Arbitrage.executeArbitrage", "method", 408},
				{"Arbitrage._swapLeg", "method", 444},
				{"Arbitrage.getBalance", "method", 501},
				{"Arbitrage.revokeApproval", "method", 505},
			},
		},
		{
			name:     "solidity-lens-hub",
			relative: "real-projects/solidity-lens-protocol-core/contracts/LensHub.sol",
			language: "solidity", owner: "LensHub",
			query: "createProfile", snippet: "return profileId;",
			want: []declaration{
				{"LensHub", "class", 49},
				{"LensHub.onlyProfileOwnerOrDelegatedExecutor", "method", 59},
				{"LensHub.whenPublishingEnabled", "method", 64},
				{"LensHub.LensHub", "constructor", 71},
				{"LensHub.createProfile", "method", 84},
				{"LensHub.setProfileMetadataURI", "method", 101},
				{"LensHub.setProfileMetadataURIWithSig", "method", 109},
				{"LensHub.setFollowModule", "method", 119},
				{"LensHub.setFollowModuleWithSig", "method", 128},
				{"LensHub.changeDelegatedExecutorsConfig", "method", 139},
				{"LensHub.changeDelegatedExecutorsConfig", "method", 155},
				{"LensHub.changeDelegatedExecutorsConfigWithSig", "method", 164},
				{"LensHub.post", "method", 194},
				{"LensHub.postWithSig", "method", 207},
				{"LensHub.comment", "method", 222},
				{"LensHub.commentWithSig", "method", 235},
				{"LensHub.mirror", "method", 250},
				{"LensHub.mirrorWithSig", "method", 263},
				{"LensHub.quote", "method", 278},
				{"LensHub.quoteWithSig", "method", 291},
				{"LensHub.follow", "method", 310},
				{"LensHub.followWithSig", "method", 333},
				{"LensHub.unfollow", "method", 358},
				{"LensHub.unfollowWithSig", "method", 370},
				{"LensHub.setBlockStatus", "method", 385},
				{"LensHub.setBlockStatusWithSig", "method", 394},
				{"LensHub.collectLegacy", "method", 405},
				{"LensHub.collectLegacyWithSig", "method", 424},
				{"LensHub.act", "method", 445},
				{"LensHub.actWithSig", "method", 463},
				{"LensHub.incrementNonce", "method", 483},
				{"LensHub.isFollowing", "method", 492},
				{"LensHub.isDelegatedExecutorApproved", "method", 498},
				{"LensHub.isDelegatedExecutorApproved", "method", 507},
				{"LensHub.getDelegatedExecutorsConfigNumber", "method", 515},
				{"LensHub.getDelegatedExecutorsPrevConfigNumber", "method", 520},
				{"LensHub.getDelegatedExecutorsMaxConfigNumberSet", "method", 525},
				{"LensHub.isBlocked", "method", 530},
				{"LensHub.getContentURI", "method", 535},
				{"LensHub.getProfile", "method", 541},
				{"LensHub.getPublication", "method", 546},
				{"LensHub.isActionModuleEnabledInPublication", "method", 554},
				{"LensHub.getPublicationType", "method", 563},
				{"LensHub.getFollowModule", "method", 570},
				{"LensHub.isFollowing", "method", 580},
			},
		},
	}
	h := NewHandler([]string{root})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, filepath.FromSlash(tc.relative))
			failed, result, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "outline", Presentation: "detailed", Paths: []string{path},
				Language: tc.language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
			})
			if err != nil || failed != nil || !result.CoverageComplete || result.Truncated ||
				len(result.Files) != 1 || result.Files[0].SourceFingerprint == "" {
				t.Fatalf("incomplete original file inventory: err=%v failed=%+v result=%+v", err, failed, result)
			}
			type declarationKey struct {
				qualified string
				line      int
			}
			want := make(map[declarationKey]declaration, len(tc.want))
			for _, item := range tc.want {
				want[declarationKey{item.qualified, item.line}] = item
			}
			seen := make(map[declarationKey]int, len(result.Symbols))
			for _, got := range result.Symbols {
				key := declarationKey{got.QualifiedName, got.NameRange.Start.Line}
				seen[key]++
				expected, ok := want[key]
				parent := ""
				if got.QualifiedName != tc.pkg && got.QualifiedName != tc.owner {
					if last := strings.LastIndex(got.QualifiedName, "."); last >= 0 {
						parent = got.QualifiedName[:last]
					}
				} else if got.QualifiedName == tc.owner {
					parent = tc.pkg
				}
				if !ok || string(got.Kind) != expected.kind ||
					got.NameRange.Start.Line != expected.line || got.ParentQualifiedName != parent {
					t.Errorf("wrong/extra source declaration: %s kind=%s line=%d owner=%s expected=%+v present=%v", got.QualifiedName, got.Kind, got.NameRange.Start.Line, got.ParentQualifiedName, expected, ok)
				}
			}
			for _, expected := range tc.want {
				key := declarationKey{expected.qualified, expected.line}
				if seen[key] != 1 {
					t.Errorf("missing/duplicate %s line %d: %d", expected.qualified, expected.line, seen[key])
				}
			}
			if len(result.Symbols) != len(tc.want) {
				t.Errorf("symbol count %d expected %d", len(result.Symbols), len(tc.want))
			}
			for _, presentation := range []string{"tree", "compact"} {
				failed, view, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
					Operation: "outline", Presentation: presentation, Paths: []string{path},
					Language: tc.language, Encoding: "utf-8", MaxFiles: 1, MaxSymbols: 128,
				})
				if err != nil || failed != nil || !view.CoverageComplete ||
					view.SymbolCount != len(tc.want) || len(view.OutlineFiles) != 1 ||
					view.OutlineFiles[0].SourceFingerprint != result.Files[0].SourceFingerprint {
					t.Errorf("%s outline mismatch: err=%v failed=%+v", presentation, err, failed)
				}
			}
			failed, digest, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "digest", Presentation: "compact", Paths: []string{path},
				Language: tc.language, Encoding: "utf-8", MaxFiles: 1,
			})
			if err != nil || failed != nil || !digest.CoverageComplete ||
				digest.SymbolCount != len(tc.want) || len(digest.OverviewFiles) != 1 {
				t.Errorf("digest mismatch: err=%v failed=%+v", err, failed)
			}
			failed, found, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "find", Paths: []string{path}, Language: tc.language,
				Encoding: "utf-8", Query: tc.query, Match: "exact", MaxSymbols: 8,
			})
			qualified := tc.owner + "." + tc.query
			if err != nil || failed != nil || len(found.Symbols) != 1 ||
				found.Symbols[0].QualifiedName != qualified {
				t.Fatalf("original find mismatch: err=%v failed=%+v found=%+v", err, failed, found.Symbols)
			}
			failed, shown, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
				Operation: "show", Path: path, Language: tc.language, Encoding: "utf-8",
				SymbolID: found.Symbols[0].ID, SourceFingerprint: result.Files[0].SourceFingerprint,
				MaxBytes: 4096,
			})
			if err != nil || failed != nil || shown.Show == nil ||
				!strings.Contains(shown.Show.Text, tc.snippet) {
				t.Errorf("original show mismatch: err=%v failed=%+v", err, failed)
			}
			failed, queried, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "search", Mode: "structural", Paths: []string{path},
				Language: tc.language, Encoding: "utf-8", Query: tc.query,
				Match: "exact", MaxResults: 8,
			})
			if err != nil || failed != nil || queried.Search == nil ||
				len(queried.Search.Matches) != 1 ||
				queried.Search.Matches[0].QualifiedName != qualified {
				t.Errorf("original structural search mismatch: err=%v failed=%+v query=%+v", err, failed, queried.Search)
			}
			failed, contextResult, err := h.SourceQuery(context.Background(), nil, SourceQueryInput{
				Operation: "context", Paths: []string{path}, Language: tc.language,
				Encoding: "utf-8", BodyPolicy: "prefer", BudgetBytes: 4096,
				MaxItems: 4, MaxDepth: 1,
				Targets: []SourceSelectorInput{{
					Kind: "symbol", Path: path, SymbolID: found.Symbols[0].ID,
					SourceFingerprint: result.Files[0].SourceFingerprint,
				}},
			})
			if err != nil || failed != nil || contextResult.Context == nil ||
				len(contextResult.Context.Items) == 0 ||
				string(contextResult.Context.Items[0].Representation) != "body" ||
				!strings.Contains(contextResult.Context.Items[0].Text, tc.snippet) {
				t.Errorf("original context body mismatch: err=%v failed=%+v", err, failed)
			}
		})
	}
}

func TestSourceSymbolsOutlineFindShowAndStaleFingerprint(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	content := "package sample\n\ntype Box struct { Value int }\nfunc Work(value int) int { return value }\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	_, outline, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8", IncludeSignatures: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outline.CoordinateSystem != sourceCoordinateSystem || outline.FilesConsidered != 1 || outline.FilesParsed != 1 || !outline.CoverageComplete {
		t.Fatalf("outline summary = %+v", outline)
	}
	if len(outline.Files) != 1 || outline.Files[0].Detection.State != "exact" || outline.Files[0].SourceFingerprint == "" {
		t.Fatalf("outline file evidence = %+v", outline.Files)
	}
	var workID string
	for _, symbol := range outline.Symbols {
		if symbol.Name == "Work" {
			workID = symbol.ID
			if symbol.Signature == "" {
				t.Fatal("requested outline signature is empty")
			}
		}
	}
	if workID == "" {
		t.Fatalf("Work missing from outline: %+v", outline.Symbols)
	}

	_, found, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: "Work", Match: "exact", Language: "go", Encoding: "utf-8", MaxSymbols: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found.Symbols) != 1 || found.Symbols[0].ID != workID {
		t.Fatalf("find result = %+v", found.Symbols)
	}

	_, shown, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: workID, SourceFingerprint: outline.Files[0].SourceFingerprint,
		Language: "go", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if shown.Show == nil || !strings.Contains(shown.Show.Text, "func Work(value int) int") || shown.Show.SymbolID != workID {
		t.Fatalf("show output = %+v", shown.Show)
	}

	if err := os.WriteFile(path, []byte(content+"\nvar Changed = true\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale, _, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{
		Operation: "show", Path: path, SymbolID: workID, SourceFingerprint: outline.Files[0].SourceFingerprint,
		Language: "go", Encoding: "utf-8", MaxBytes: 4096,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale == nil || !stale.IsError || stale.Meta[ErrorCodeMetaKey] != ErrCodeConflict {
		t.Fatalf("stale show result = %+v", stale)
	}
}

func TestSourceSymbolsDigestCompactOverview(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "sample.go")
	code := "package sample\ntype Router struct{}\nfunc NewRouter() *Router { return &Router{} }\nfunc (r *Router) ServeHTTP() {}\n"
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	call := func(presentation string) SourceSymbolsOutput {
		t.Helper()
		result, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "digest", Paths: []string{path}, Language: "go", Presentation: presentation,
		})
		if err != nil || result != nil {
			t.Fatalf("digest %q: result=%+v err=%v", presentation, result, err)
		}
		return out
	}
	legacy, detailed := call(""), call("detailed")
	if !reflect.DeepEqual(legacy, detailed) || len(legacy.Digests) != 1 || len(legacy.Files) != 1 {
		t.Fatal("digest without presentation must preserve previous output")
	}
	compact := call("compact")
	if compact.Presentation != "compact" || len(compact.Files) != 0 || len(compact.Digests) != 0 ||
		len(compact.OverviewFiles) != 1 || compact.SymbolCount != legacy.SymbolCount ||
		compact.CoverageComplete != legacy.CoverageComplete || compact.FilesParsed != legacy.FilesParsed {
		t.Fatalf("compact digest lost coverage or changed shape: %+v", compact)
	}
	file := compact.OverviewFiles[0]
	if file.Path != path || file.SourceFingerprint != legacy.Files[0].SourceFingerprint ||
		file.Encoding != "utf-8" || len(file.DeclarationCounts) == 0 {
		t.Fatalf("compact digest lost file evidence: %+v", file)
	}
	found := false
	for _, entry := range file.EntryPoints {
		if entry.Name == "NewRouter" && entry.Kind == "function" && entry.Line > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing useful entry point: %+v", file.EntryPoints)
	}
	result, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "digest", Paths: []string{path}, Presentation: "tree",
	})
	if err != nil || result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid digest presentation accepted: result=%+v err=%v", result, err)
	}
}

func TestSourceSymbolsOutlinePresentations(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	path := filepath.Join(root, "sample.go")
	code := "package sample\ntype Box struct { Value int }\nfunc (b *Box) Get() int { return b.Value }\n"
	if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	call := func(view string, kinds []string) SourceSymbolsOutput {
		t.Helper()
		result, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Paths: []string{path}, Language: "go",
			Presentation: view, Kinds: kinds, IncludeSignatures: true,
		})
		if result != nil || err != nil {
			t.Fatalf("outline %q: %+v %v", view, result, err)
		}
		return out
	}
	defaultTree, explicitTree := call("", nil), call("tree", nil)
	if !reflect.DeepEqual(defaultTree, explicitTree) || len(defaultTree.OutlineFiles) != 1 || len(defaultTree.Symbols) != 0 {
		t.Fatal("omitted presentation must return the tree view")
	}
	legacy := call("detailed", nil)
	if len(legacy.Symbols) == 0 || len(legacy.Files) != 1 || len(legacy.OutlineFiles) != 0 {
		t.Fatal("explicit detailed must retain the legacy output contract")
	}
	for _, view := range []string{"compact", "tree"} {
		out := call(view, nil)
		if out.SymbolCount != legacy.SymbolCount || out.CoverageComplete != legacy.CoverageComplete ||
			len(out.Symbols) != 0 || len(out.Files) != 0 || len(out.OutlineFiles) != 1 {
			t.Fatalf("%s changed counts or output shape: %+v", view, out)
		}
		file := out.OutlineFiles[0]
		if file.Path != path || file.SourceFingerprint != legacy.Files[0].SourceFingerprint ||
			file.Encoding != "utf-8" {
			t.Fatalf("%s missing file identity: %+v", view, file)
		}
		seen := make(map[string]bool)
		var walk func([]*SourceOutlineNode)
		walk = func(nodes []*SourceOutlineNode) {
			for _, n := range nodes {
				if n.SymbolID == "" || seen[n.SymbolID] || n.Line <= 0 {
					t.Fatalf("%s duplicate or invalid node: %+v", view, n)
				}
				seen[n.SymbolID] = true
				if n.Name == "Get" && n.Signature == "" {
					t.Fatalf("%s method signature missing", view)
				}
				walk(n.Children)
			}
		}
		walk(file.Symbols)
		if len(seen) != legacy.SymbolCount {
			t.Fatalf("%s retained %d/%d symbols", view, len(seen), legacy.SymbolCount)
		}
	}
	filtered := call("tree", []string{"method"})
	if len(filtered.OutlineFiles) != 1 || len(filtered.OutlineFiles[0].Symbols) != 1 ||
		filtered.OutlineFiles[0].Symbols[0].ParentID == "" {
		t.Fatalf("lost filtered ancestor: %+v", filtered.OutlineFiles)
	}
	result, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "outline", Paths: []string{path}, Presentation: "unsupported",
	})
	if result == nil || !result.IsError || err != nil || result.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("invalid view: %+v %v", result, err)
	}
}
func TestSourceSymbolsOutlineKindsLimitAfterAnalysis(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	source := "package sample\ntype Box struct { First int; Second int; Third int }\nfunc (b *Box) One() {}\nfunc (b *Box) Two() {}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	call := func(limit int) SourceSymbolsOutput {
		t.Helper()
		result, out, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
			Operation: "outline", Presentation: "tree", Paths: []string{path}, Language: "go",
			Kinds: []string{"method"}, MaxFiles: 1, MaxSymbols: limit,
		})
		if err != nil || result != nil {
			t.Fatalf("outline filtered limit %d: result=%+v err=%v", limit, result, err)
		}
		return out
	}
	both := call(2)
	if both.SymbolCount != 2 || !both.CoverageComplete || both.Truncated ||
		len(both.OutlineFiles) != 1 || !both.OutlineFiles[0].CoverageComplete ||
		len(both.OutlineFiles[0].Symbols) != 2 {
		t.Fatalf("matching symbols must not be lost behind unmatched declarations: %+v", both)
	}
	if both.OutlineFiles[0].Symbols[0].Name != "One" ||
		both.OutlineFiles[0].Symbols[1].Name != "Two" {
		t.Fatalf("unexpected filtered ordering: %+v", both.OutlineFiles[0].Symbols)
	}
	first := call(1)
	if first.SymbolCount != 1 || !first.Truncated || first.CoverageComplete ||
		len(first.OutlineFiles) != 1 || !first.OutlineFiles[0].CoverageComplete {
		t.Fatalf("matching output truncation must not be confused with analyzer coverage: %+v", first)
	}
}

func TestSourceSymbolsDirectoryOrderingFiltersDigestAndUnsupportedCoverage(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"b.cs":  "namespace Demo { public class B { public void Run() {} } }\n",
		"a.go":  "package alpha\nfunc Run() {}\n",
		"z.txt": "ordinary prose that is not source code\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	ctx := context.Background()
	_, first, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, IncludeSignatures: false})
	if err != nil {
		t.Fatal(err)
	}
	_, second, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, IncludeSignatures: false})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("directory outline is nondeterministic")
	}
	if len(first.Files) != 3 || filepath.Base(first.Files[0].Path) != "a.go" || filepath.Base(first.Files[1].Path) != "b.cs" || filepath.Base(first.Files[2].Path) != "z.txt" {
		t.Fatalf("file ordering = %+v", first.Files)
	}
	if first.CoverageComplete || first.FilesSkipped != 1 || first.Files[2].ErrorCode != ErrCodeUnsupported {
		t.Fatalf("unsupported coverage = %+v", first)
	}

	_, filtered, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, Includes: []string{"*.go"}, Kinds: []string{"function"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Files) != 1 || filepath.Base(filtered.Files[0].Path) != "a.go" || len(filtered.Symbols) != 1 || filtered.Symbols[0].Name != "Run" {
		t.Fatalf("filtered outline = %+v", filtered)
	}

	_, digest, err := h.SourceSymbols(ctx, nil, SourceSymbolsInput{Operation: "digest", Paths: []string{root}, Includes: []string{"*.cs"}, Language: "csharp", Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if len(digest.Digests) != 1 || digest.Digests[0].SourceBytes == 0 || len(digest.Digests[0].DeclarationCounts) == 0 || digest.Symbols != nil {
		t.Fatalf("digest = %+v", digest)
	}
}

func TestSourceSymbolsConcurrencyDoesNotChangeDeterministicOutput(t *testing.T) {
	root := t.TempDir()
	for index := 0; index < 24; index++ {
		path := filepath.Join(root, fmt.Sprintf("file-%02d.go", index))
		content := fmt.Sprintf("package p%02d\nfunc F%02d() {}\n", index, index)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := config.Load()
	serialConfig := *base
	serialConfig.Source.MaxConcurrency = 1
	parallelConfig := *base
	parallelConfig.Source.MaxConcurrency = 4
	input := SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{root}, Encoding: "utf-8"}
	_, serial, err := NewHandler([]string{root}, WithConfig(&serialConfig)).SourceSymbols(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	_, parallel, err := NewHandler([]string{root}, WithConfig(&parallelConfig)).SourceSymbols(context.Background(), nil, input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(serial, parallel) {
		t.Fatalf("source output changed with concurrency:\nserial=%+v\nparallel=%+v", serial, parallel)
	}
}

func TestSourceSymbolsFindResultLimitDoesNotTruncateAnalysis(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "late.go")
	var source strings.Builder
	source.WriteString("package sample\n")
	for index := 0; index < 40; index++ {
		fmt.Fprintf(&source, "func F%02d() {}\n", index)
	}
	if err := os.WriteFile(path, []byte(source.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	toolErr, found, err := NewHandler([]string{root}).SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: "F39", Match: "exact", Language: "go", Encoding: "utf-8", MaxSymbols: 1,
	})
	if err != nil || toolErr != nil {
		t.Fatalf("find late symbol err=%v toolErr=%+v", err, toolErr)
	}
	if len(found.Symbols) != 1 || found.Symbols[0].Name != "F39" || !found.CoverageComplete || found.Truncated {
		t.Fatalf("late find result = %+v", found)
	}
}

func TestSourceSymbolsFindRelevantFileDetails(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("file-%02d.go", i)
		symbol := fmt.Sprintf("Other%02d", i)
		if i == 4 {
			symbol = "Wanted"
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte("package sample\nfunc "+symbol+"() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "extra.txt"), []byte("unrecognized"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})
	input := SourceSymbolsInput{Operation: "find", Paths: []string{root}, Query: "Wanted", Match: "exact", MaxSymbols: 2}
	toolErr, full, err := h.SourceSymbols(context.Background(), nil, input)
	if err != nil || toolErr != nil || len(full.Files) != 9 {
		t.Fatalf("full find: %+v %+v %v", toolErr, full, err)
	}
	input.FileDetails = "relevant"
	toolErr, compact, err := h.SourceSymbols(context.Background(), nil, input)
	if err != nil || toolErr != nil {
		t.Fatalf("compact find: %+v %v", toolErr, err)
	}
	if len(compact.Files) != 2 || filepath.Base(compact.Files[0].Path) != "extra.txt" ||
		filepath.Base(compact.Files[1].Path) != "file-04.go" {
		t.Fatalf("compact file evidence: %+v", compact.Files)
	}
	if !reflect.DeepEqual(compact.Symbols, full.Symbols) || compact.FilesConsidered != full.FilesConsidered ||
		compact.FilesParsed != full.FilesParsed || compact.FilesSkipped != full.FilesSkipped ||
		compact.CoverageComplete != full.CoverageComplete || compact.Truncated != full.Truncated {
		t.Fatalf("compact changed result or coverage: %+v vs %+v", compact, full)
	}
	input.Query = "Missing"
	_, missing, err := h.SourceSymbols(context.Background(), nil, input)
	if err != nil || len(missing.Files) != 1 || missing.Files[0].Status == "parsed" {
		t.Fatalf("missing must retain unsupported file evidence: %+v %v", missing, err)
	}
	input.FileDetails = "unknown"
	toolErr, _, err = h.SourceSymbols(context.Background(), nil, input)
	if err != nil || toolErr == nil || !toolErr.IsError || toolErr.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("unknown file detail: %+v %v", toolErr, err)
	}
}

func TestSourceSymbolsFindQueryLimitCountsUnicodeScalars(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc Work() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := NewHandler([]string{root})

	accepted, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: strings.Repeat("é", 512), Language: "go", Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted != nil && accepted.IsError {
		t.Fatalf("512-scalar query was rejected: %+v", accepted)
	}

	rejected, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{
		Operation: "find", Paths: []string{path}, Query: strings.Repeat("é", 513), Language: "go", Encoding: "utf-8",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rejected == nil || !rejected.IsError || rejected.Meta[ErrorCodeMetaKey] != ErrCodeInvalidInput {
		t.Fatalf("513-scalar query result = %+v, want INVALID_INPUT", rejected)
	}
}

func TestSourceSymbolsValidationOrderAndMessages(t *testing.T) {
	h := NewHandler([]string{t.TempDir()})
	tooManyKinds := make([]string, 33)
	tests := []struct {
		name    string
		input   SourceSymbolsInput
		code    string
		message string
	}{
		{
			name:    "show dispatch precedes batch validation",
			input:   SourceSymbolsInput{Operation: " SHOW "},
			code:    ErrCodeInvalidInput,
			message: "show requires path, symbolId, sourceFingerprint, language, and encoding",
		},
		{
			name:    "operation precedes paths",
			input:   SourceSymbolsInput{Operation: "unknown"},
			code:    ErrCodeInvalidInput,
			message: "operation must be outline, digest, find, or show",
		},
		{
			name:    "paths required",
			input:   SourceSymbolsInput{Operation: "outline"},
			code:    ErrCodeInvalidInput,
			message: "paths must contain at least one path",
		},
		{
			name:    "max files precedes max symbols",
			input:   SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{"unused"}, MaxFiles: -1, MaxSymbols: -1},
			code:    ErrCodeInvalidInput,
			message: "maxFiles must be positive",
		},
		{
			name:    "max symbols precedes find query",
			input:   SourceSymbolsInput{Operation: "find", Paths: []string{"unused"}, MaxSymbols: -1},
			code:    ErrCodeInvalidInput,
			message: "maxSymbols must be positive",
		},
		{
			name:    "find query validation",
			input:   SourceSymbolsInput{Operation: "find", Paths: []string{"unused"}},
			code:    ErrCodeInvalidInput,
			message: "query must be a non-empty string up to 512 Unicode scalar values",
		},
		{
			name:    "find match validation",
			input:   SourceSymbolsInput{Operation: "find", Paths: []string{"unused"}, Query: "name", Match: "suffix"},
			code:    ErrCodeInvalidInput,
			message: "match must be exact, prefix, or qualified",
		},
		{
			name:    "kinds limit",
			input:   SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{"unused"}, Kinds: tooManyKinds},
			code:    ErrCodeLimit,
			message: "kinds exceeds the 32-item limit",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, _, err := h.SourceSymbols(context.Background(), nil, test.input)
			if err != nil {
				t.Fatal(err)
			}
			if result == nil || !result.IsError || result.Meta[ErrorCodeMetaKey] != test.code {
				t.Fatalf("result = %+v, want code %s", result, test.code)
			}
			if len(result.Content) != 1 {
				t.Fatalf("content = %+v, want one text item", result.Content)
			}
			content, ok := result.Content[0].(*mcp.TextContent)
			if !ok || content.Text != test.message {
				t.Fatalf("message = %#v, want %q", content, test.message)
			}
		})
	}
}

func TestSourceSymbolsLimitsCancellationAndDeniedRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.go")
	if err := os.WriteFile(path, []byte("package sample\nfunc A() {}\nfunc B() {}\nfunc C() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Load()
	cfg.Source.MaxSymbols = 2
	cfg.Source.MaxAggregateBytes = 16
	h := NewHandler([]string{root}, WithConfig(cfg))

	limitResult, _, err := h.SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if limitResult == nil || !limitResult.IsError || limitResult.Meta[ErrorCodeMetaKey] != ErrCodeLimit {
		t.Fatalf("aggregate limit result = %+v", limitResult)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cancelResult, _, err := NewHandler([]string{root}).SourceSymbols(cancelled, nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{path}, Language: "go", Encoding: "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	if cancelResult == nil || !cancelResult.IsError || cancelResult.Meta[ErrorCodeMetaKey] != ErrCodeCancelled {
		t.Fatalf("cancel result = %+v", cancelResult)
	}

	outside := filepath.Join(t.TempDir(), "outside.go")
	if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	denied, _, err := NewHandler([]string{root}).SourceSymbols(context.Background(), nil, SourceSymbolsInput{Operation: "outline", Presentation: "detailed", Paths: []string{outside}, Language: "go"})
	if err != nil {
		t.Fatal(err)
	}
	if denied == nil || !denied.IsError || denied.Meta[ErrorCodeMetaKey] != ErrCodeAccessDenied {
		t.Fatalf("denied root result = %+v", denied)
	}
}

func TestSourceFileSelectionKeepsLexicographicPrefixWithoutRetainingAllPaths(t *testing.T) {
	root := canonicalHandlerTestDir(t)
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"z.go", "a.go", "middle.go", "nested/b.go", "nested/a.go", "note.txt"} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte("package demo\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h := NewHandler([]string{root})
	paths := []string{filepath.Join(root, "z.go"), filepath.Join(root, "nested"), root, filepath.Join(root, "a.go")}
	want := []string{
		filepath.Join(root, "a.go"), filepath.Join(root, "middle.go"),
		filepath.Join(root, "nested", "a.go"), filepath.Join(root, "nested", "b.go"), filepath.Join(root, "z.go"),
	}
	for limit := 1; limit <= len(want)+1; limit++ {
		selected, truncated, err := h.collectBoundedSourceFiles(context.Background(), paths, []string{"*.go"}, nil, false, limit)
		if err != nil {
			t.Fatalf("limit %d: %v", limit, err)
		}
		expectedCount := len(want)
		if limit < expectedCount {
			expectedCount = limit
		}
		if !reflect.DeepEqual(selected, want[:expectedCount]) || truncated != (len(want) > limit) {
			t.Fatalf("limit %d: selected=%v truncated=%t; want=%v truncated=%t", limit, selected, truncated, want[:expectedCount], len(want) > limit)
		}
	}
}
