package testarchitecture

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

type fuzzManifest struct {
	SchemaVersion int `json:"schemaVersion"`
	Profiles      map[string]struct {
		FuzzTime string `json:"fuzzTime"`
	} `json:"profiles"`
	Targets []struct {
		ID             string   `json:"id"`
		Package        string   `json:"package"`
		Target         string   `json:"target"`
		RiskCategories []string `json:"riskCategories"`
		Profiles       []string `json:"profiles"`
	} `json:"targets"`
}

var (
	fuzzTargetNamePattern = regexp.MustCompile(`^Fuzz[A-Za-z0-9_]+$`)
	fuzzTimePattern       = regexp.MustCompile(`^[1-9][0-9]*(?:x|ms|s|m)$`)
)

func TestFuzzManifestCoversRepositoryAndRequiredSmokeRisks(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testarchitecture", "fuzz-manifest.json"))
	if err != nil {
		t.Fatalf("read fuzz manifest: %v", err)
	}
	var manifest fuzzManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode fuzz manifest: %v", err)
	}
	if manifest.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", manifest.SchemaVersion)
	}
	if len(manifest.Profiles) != 2 {
		t.Fatalf("profiles = %v, want exactly smoke and qualification", manifest.Profiles)
	}
	for _, profile := range []string{"smoke", "qualification"} {
		config, ok := manifest.Profiles[profile]
		if !ok {
			t.Fatalf("missing fuzz profile %q", profile)
		}
		if !fuzzTimePattern.MatchString(config.FuzzTime) {
			t.Fatalf("profile %q fuzzTime = %q, want bounded Go fuzztime", profile, config.FuzzTime)
		}
	}

	allowedRisks := set(
		"path-security",
		"http-host-origin-proxy",
		"http-protocol-jsonrpc",
		"backup-format",
		"backup-recovery",
		"backup-gc",
		"encoding-detection",
		"encoding-stream",
		"command-task-parsing",
		"filesystem-ignore",
		"handler-patch-fuzzy",
		"source-intelligence",
	)
	requiredSmokeRisks := []string{
		"path-security",
		"http-host-origin-proxy",
		"http-protocol-jsonrpc",
		"backup-format",
		"backup-recovery",
		"backup-gc",
		"encoding-detection",
		"encoding-stream",
		"command-task-parsing",
		"filesystem-ignore",
		"handler-patch-fuzzy",
		"source-intelligence",
	}

	manifestTargets := make(map[string]string, len(manifest.Targets))
	smokeRisks := make(map[string]bool)
	smokeTargets := 0
	ids := make(map[string]bool)
	for _, target := range manifest.Targets {
		validateID(t, "fuzz target", target.ID)
		if ids[target.ID] {
			t.Fatalf("duplicate fuzz target id %q", target.ID)
		}
		ids[target.ID] = true
		if target.Package != "." && (!strings.HasPrefix(target.Package, "./") || strings.Contains(target.Package, "..")) {
			t.Fatalf("fuzz target %q has unsafe package %q", target.ID, target.Package)
		}
		if !fuzzTargetNamePattern.MatchString(target.Target) {
			t.Fatalf("fuzz target %q has invalid Go fuzz entrypoint %q", target.ID, target.Target)
		}
		if len(target.RiskCategories) == 0 {
			t.Fatalf("fuzz target %q has no risk category", target.ID)
		}
		for _, risk := range target.RiskCategories {
			if !allowedRisks[risk] {
				t.Fatalf("fuzz target %q uses unknown risk category %q", target.ID, risk)
			}
		}
		profiles := set(target.Profiles...)
		if len(profiles) != len(target.Profiles) {
			t.Fatalf("fuzz target %q has duplicate profiles %v", target.ID, target.Profiles)
		}
		if !profiles["qualification"] {
			t.Fatalf("fuzz target %q must participate in qualification", target.ID)
		}
		for profile := range profiles {
			if profile != "smoke" && profile != "qualification" {
				t.Fatalf("fuzz target %q uses unknown profile %q", target.ID, profile)
			}
		}
		if profiles["smoke"] {
			smokeTargets++
			for _, risk := range target.RiskCategories {
				smokeRisks[risk] = true
			}
		}
		key := target.Package + "\x00" + target.Target
		if prior, exists := manifestTargets[key]; exists {
			t.Fatalf("fuzz entrypoint %s/%s owned by both %q and %q", target.Package, target.Target, prior, target.ID)
		}
		manifestTargets[key] = target.ID
	}
	if len(manifest.Targets) == 0 || smokeTargets == 0 || smokeTargets >= len(manifest.Targets) {
		t.Fatalf("fuzz tiering must use a non-empty strict smoke subset: smoke=%d total=%d", smokeTargets, len(manifest.Targets))
	}
	for _, risk := range requiredSmokeRisks {
		if !smokeRisks[risk] {
			t.Fatalf("smoke fuzz profile does not cover required risk %q", risk)
		}
	}

	actualTargets := repositoryFuzzEntrypoints(t, root)
	var missing, stale []string
	for key := range actualTargets {
		if _, ok := manifestTargets[key]; !ok {
			missing = append(missing, printableFuzzKey(key))
		}
	}
	for key := range manifestTargets {
		if _, ok := actualTargets[key]; !ok {
			stale = append(stale, printableFuzzKey(key))
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) != 0 || len(stale) != 0 {
		t.Fatalf("fuzz manifest drift: missing=%v stale=%v", missing, stale)
	}
}

func TestGitHubFuzzJobUsesSharedRunnerInsteadOfHardCodedTargets(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "node scripts/run-fuzz.js --profile smoke") {
		t.Fatal("GitHub fuzz job must invoke the shared fuzz runner")
	}
	if strings.Contains(content, "-fuzz '^Fuzz") {
		t.Fatal("GitHub fuzz job must not hard-code individual fuzz entrypoints")
	}
}

func repositoryFuzzEntrypoints(t *testing.T, root string) map[string]bool {
	t.Helper()
	result := make(map[string]bool)
	files := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		relativeDir, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		packagePath := "."
		if relativeDir != "." {
			packagePath = "./" + filepath.ToSlash(relativeDir)
		}
		for _, declaration := range parsed.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv != nil || !fuzzTargetNamePattern.MatchString(function.Name.Name) {
				continue
			}
			result[packagePath+"\x00"+function.Name.Name] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan fuzz entrypoints: %v", err)
	}
	return result
}

func printableFuzzKey(key string) string {
	return strings.Replace(key, "\x00", "/", 1)
}
