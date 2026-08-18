package testarchitecture

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type evidenceMap struct {
	SchemaVersion int `json:"schemaVersion"`
	SourcePolicy  struct {
		IndependentFromImplementationRegistry bool `json:"independentFromImplementationRegistry"`
		ImplementationRegistryOracleForbidden bool `json:"implementationRegistryOracleForbidden"`
	} `json:"sourcePolicy"`
	RequiredCategories []string    `json:"requiredCategories"`
	TestGroups         []testGroup `json:"testGroups"`
	Gates              []gate      `json:"gates"`
	Invariants         []invariant `json:"invariants"`
}

type testGroup struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Directory      string   `json:"directory,omitempty"`
	Files          []string `json:"files,omitempty"`
	Classification string   `json:"classification"`
	FailureClasses []string `json:"failureClasses"`
	Platforms      []string `json:"platforms"`
	ExpenseTier    string   `json:"expenseTier"`
}

type gate struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	File            string   `json:"file,omitempty"`
	Anchor          string   `json:"anchor,omitempty"`
	Profile         string   `json:"profile,omitempty"`
	Purpose         string   `json:"purpose,omitempty"`
	Classification  string   `json:"classification"`
	FailureClasses  []string `json:"failureClasses"`
	Platforms       []string `json:"platforms"`
	ExpenseTier     string   `json:"expenseTier"`
	ReleaseBlocking bool     `json:"releaseBlocking"`
}

type invariant struct {
	ID              string        `json:"id"`
	Category        string        `json:"category"`
	Requirement     string        `json:"requirement"`
	CanonicalLayer  string        `json:"canonicalLayer"`
	Contracts       []contractRef `json:"contracts"`
	TestOwners      []string      `json:"testOwners"`
	GateOwners      []string      `json:"gateOwners"`
	FailureClasses  []string      `json:"failureClasses"`
	Platforms       []string      `json:"platforms"`
	ExpenseTier     string        `json:"expenseTier"`
	ReleaseBlocking bool          `json:"releaseBlocking"`
}

type contractRef struct {
	Path string `json:"path"`
	Role string `json:"role"`
}

func TestEvidenceMapIsIndependentCompleteAndCurrent(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testarchitecture", "evidence-map.json"))
	if err != nil {
		t.Fatalf("read evidence map: %v", err)
	}
	var got evidenceMap
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode evidence map: %v", err)
	}
	if got.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", got.SchemaVersion)
	}
	if !got.SourcePolicy.IndependentFromImplementationRegistry || !got.SourcePolicy.ImplementationRegistryOracleForbidden {
		t.Fatal("evidence map must remain independent from the implementation registry under test")
	}

	requiredCategories := []string{
		"identity-catalog-mcp", "path-security", "encoding-text", "mutation-backup",
		"durable-tasks", "http-protocol-redaction", "source-intelligence", "release-publication",
	}
	assertExactSet(t, "requiredCategories", got.RequiredCategories, requiredCategories)
	classifications := set("canonical-owner", "supporting-end-to-end", "overlap", "stale-partial-overlap", "external-only", "candidate-removal")
	failureClasses := set("deterministic", "race", "fuzz", "platform", "integration", "external-acceptance")
	platforms := set("all", "linux", "windows", "darwin", "github", "local")
	expenseTiers := set("fast", "medium", "slow", "very-slow")

	groupByID := make(map[string]testGroup, len(got.TestGroups))
	goDirectories := make(map[string]string)
	for _, group := range got.TestGroups {
		validateID(t, "test group", group.ID)
		if _, exists := groupByID[group.ID]; exists {
			t.Fatalf("duplicate test group id %q", group.ID)
		}
		groupByID[group.ID] = group
		validateEnum(t, "test group classification", group.Classification, classifications)
		validateList(t, "test group failureClasses", group.FailureClasses, failureClasses)
		validateList(t, "test group platforms", group.Platforms, platforms)
		validateEnum(t, "test group expenseTier", group.ExpenseTier, expenseTiers)
		switch group.Kind {
		case "go-package":
			if group.Directory == "" || len(group.Files) != 0 {
				t.Fatalf("go-package group %q must set directory only", group.ID)
			}
			if prior, exists := goDirectories[group.Directory]; exists {
				t.Fatalf("Go test directory %q owned by both %q and %q", group.Directory, prior, group.ID)
			}
			goDirectories[group.Directory] = group.ID
			assertPath(t, root, group.Directory, true)
		case "node", "integration-harness":
			if group.Directory != "" || len(group.Files) == 0 {
				t.Fatalf("%s group %q must set files only", group.Kind, group.ID)
			}
			for _, file := range group.Files {
				assertPath(t, root, file, false)
			}
		default:
			t.Fatalf("test group %q has unknown kind %q", group.ID, group.Kind)
		}
	}
	assertAllGoTestDirectoriesClassified(t, root, goDirectories)

	gateByID := make(map[string]gate, len(got.Gates))
	for _, item := range got.Gates {
		validateID(t, "gate", item.ID)
		if _, exists := gateByID[item.ID]; exists {
			t.Fatalf("duplicate gate id %q", item.ID)
		}
		gateByID[item.ID] = item
		validateEnum(t, "gate classification", item.Classification, classifications)
		validateList(t, "gate failureClasses", item.FailureClasses, failureClasses)
		validateList(t, "gate platforms", item.Platforms, platforms)
		validateEnum(t, "gate expenseTier", item.ExpenseTier, expenseTiers)
		switch item.Kind {
		case "github-workflow":
			if item.File == "" {
				t.Fatalf("GitHub gate %q must set its workflow file", item.ID)
			}
			assertPath(t, root, item.File, false)
		case "local-private":
			if item.File != "" || item.Anchor != "" {
				t.Fatalf("local-private gate %q must not expose workstation paths or anchors", item.ID)
			}
			if item.Profile == "" || strings.TrimSpace(item.Purpose) == "" {
				t.Fatalf("local-private gate %q must declare a profile and complementary purpose", item.ID)
			}
			if item.Profile != "Focused" && item.Profile != "PrePush" && item.Profile != "Release" {
				t.Fatalf("local-private gate %q uses unknown profile %q", item.ID, item.Profile)
			}
		default:
			t.Fatalf("gate %q has unknown kind %q", item.ID, item.Kind)
		}
	}

	requiredCategorySet := set(got.RequiredCategories...)
	categorySeen := make(map[string]bool)
	invariantIDs := make(map[string]bool)
	for _, item := range got.Invariants {
		validateID(t, "invariant", item.ID)
		if invariantIDs[item.ID] {
			t.Fatalf("duplicate invariant id %q", item.ID)
		}
		invariantIDs[item.ID] = true
		if !requiredCategorySet[item.Category] {
			t.Fatalf("invariant %q uses undeclared category %q", item.ID, item.Category)
		}
		categorySeen[item.Category] = true
		if strings.TrimSpace(item.Requirement) == "" || strings.TrimSpace(item.CanonicalLayer) == "" {
			t.Fatalf("invariant %q must define requirement and canonicalLayer", item.ID)
		}
		if len(item.Contracts) == 0 {
			t.Fatalf("invariant %q has no independent contract reference", item.ID)
		}
		for _, contract := range item.Contracts {
			if contract.Path == "internal/toolcatalog/catalog.json" {
				t.Fatalf("invariant %q uses implementation registry as contract oracle", item.ID)
			}
			if strings.TrimSpace(contract.Role) == "" {
				t.Fatalf("invariant %q contract %q has empty role", item.ID, contract.Path)
			}
			assertPath(t, root, contract.Path, false)
		}
		if len(item.TestOwners) == 0 {
			t.Fatalf("invariant %q has no test owner", item.ID)
		}
		for _, owner := range item.TestOwners {
			if _, ok := groupByID[owner]; !ok {
				t.Fatalf("invariant %q references unknown test owner %q", item.ID, owner)
			}
		}
		if len(item.GateOwners) == 0 {
			t.Fatalf("invariant %q has no gate owner", item.ID)
		}
		for _, owner := range item.GateOwners {
			if _, ok := gateByID[owner]; !ok {
				t.Fatalf("invariant %q references unknown gate owner %q", item.ID, owner)
			}
		}
		validateList(t, "invariant failureClasses", item.FailureClasses, failureClasses)
		validateList(t, "invariant platforms", item.Platforms, platforms)
		validateEnum(t, "invariant expenseTier", item.ExpenseTier, expenseTiers)
	}
	for _, category := range got.RequiredCategories {
		if !categorySeen[category] {
			t.Fatalf("required category %q has no invariant", category)
		}
	}
}

func TestLocalValidationProfilesAreExplicitAndComplementary(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "testarchitecture", "evidence-map.json"))
	if err != nil {
		t.Fatalf("read evidence map: %v", err)
	}
	var got evidenceMap
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode evidence map: %v", err)
	}

	local := make(map[string]gate)
	for _, item := range got.Gates {
		if item.Kind == "local-private" {
			local[item.ID] = item
		}
	}
	want := map[string]string{
		"local.focused": "Focused",
		"local.prepush": "PrePush",
		"local.release": "Release",
	}
	if len(local) != len(want) {
		t.Fatalf("local-private gate count = %d, want %d: %v", len(local), len(want), local)
	}
	for id, profile := range want {
		item, ok := local[id]
		if !ok {
			t.Fatalf("missing local-private gate %q", id)
		}
		if item.Profile != profile {
			t.Fatalf("local-private gate %q profile = %q, want %q", id, item.Profile, profile)
		}
		if strings.TrimSpace(item.Purpose) == "" {
			t.Fatalf("local-private gate %q must explain its complementary role", id)
		}
	}
}

func TestGoTestArchitectureUsesCurrentBehaviorNames(t *testing.T) {
	root := repositoryRoot(t)
	pathMarker := regexp.MustCompile(`(?i)(?:^|[/_])(?:r\d+|phase_?\d+)(?:[_/.]|$)`)
	phaseMarker := regexp.MustCompile(`(?i)phase\d+`)
	releaseMarker := regexp.MustCompile(`(?i)^r\d+`)
	fileSet := token.NewFileSet()
	var violations []string
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
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		relative = filepath.ToSlash(relative)
		if pathMarker.MatchString(relative) {
			violations = append(violations, relative)
		}
		parsed, parseErr := parser.ParseFile(fileSet, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if !ok {
				return true
			}
			if phaseMarker.MatchString(identifier.Name) || releaseMarker.MatchString(identifier.Name) {
				position := fileSet.Position(identifier.Pos())
				violations = append(violations, relative+":"+identifier.Name+":"+strconv.Itoa(position.Line))
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan Go test architecture: %v", err)
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("historical milestone/phase naming remains in permanent Go test architecture: %v", violations)
	}
}

func TestProductionGoHasNoExplicitTestOnlyHookIdentifiers(t *testing.T) {
	root := repositoryRoot(t)
	marker := regexp.MustCompile(`(?i)[A-Za-z0-9_]*(?:testhooks?|testonly)[A-Za-z0-9_]*`)
	forbiddenLegacySeams := []string{
		"commitHook",
		"fingerprintPathsWithHook",
		"patchPackageAfterPrepare",
		"patchPackageAfterStage",
		"reconcileCycle",
	}
	var violations []string
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
		if !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if match := marker.Find(content); match != nil {
			violations = append(violations, filepath.ToSlash(relative)+":"+string(match))
		}
		for _, forbidden := range forbiddenLegacySeams {
			if strings.Contains(string(content), forbidden) {
				violations = append(violations, filepath.ToSlash(relative)+":"+forbidden)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan production Go files: %v", err)
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("explicit test-only hooks found in production Go: %v", violations)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(current))
}

func assertAllGoTestDirectoriesClassified(t *testing.T, root string, classified map[string]string) {
	t.Helper()
	actual := make(map[string]bool)
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
		if strings.HasSuffix(entry.Name(), "_test.go") {
			rel, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			actual[filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk Go tests: %v", err)
	}
	var missing, stale []string
	for dir := range actual {
		if _, ok := classified[dir]; !ok {
			missing = append(missing, dir)
		}
	}
	for dir := range classified {
		if !actual[dir] {
			stale = append(stale, dir)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) != 0 || len(stale) != 0 {
		t.Fatalf("Go test group classification drift: missing=%v stale=%v", missing, stale)
	}
}

func assertPath(t *testing.T, root, relative string, wantDir bool) string {
	t.Helper()
	if filepath.IsAbs(relative) || relative == "" {
		t.Fatalf("path must be repository-relative: %q", relative)
	}
	clean := filepath.Clean(filepath.FromSlash(relative))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		t.Fatalf("path escapes repository: %q", relative)
	}
	path := filepath.Join(root, clean)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("referenced path %q: %v", relative, err)
	}
	if info.IsDir() != wantDir {
		t.Fatalf("referenced path %q directory=%v, want %v", relative, info.IsDir(), wantDir)
	}
	return path
}

func validateID(t *testing.T, kind, id string) {
	t.Helper()
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, " \t\r\n") {
		t.Fatalf("%s id must be non-empty and whitespace-free: %q", kind, id)
	}
}
func validateEnum(t *testing.T, field, value string, allowed map[string]bool) {
	t.Helper()
	if !allowed[value] {
		t.Fatalf("%s has invalid value %q", field, value)
	}
}
func validateList(t *testing.T, field string, values []string, allowed map[string]bool) {
	t.Helper()
	if len(values) == 0 {
		t.Fatalf("%s must not be empty", field)
	}
	for _, value := range values {
		validateEnum(t, field, value, allowed)
	}
}
func assertExactSet(t *testing.T, field string, got, want []string) {
	t.Helper()
	gs, ws := set(got...), set(want...)
	if len(gs) != len(got) || len(gs) != len(ws) {
		t.Fatalf("%s = %v, want %v", field, got, want)
	}
	for value := range ws {
		if !gs[value] {
			t.Fatalf("%s missing %q", field, value)
		}
	}
}
func set(values ...string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}
