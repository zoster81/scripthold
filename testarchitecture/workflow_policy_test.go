package testarchitecture

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var (
	actionUseLinePattern   = regexp.MustCompile(`(?m)^[ \t]*uses:[ \t]*([^#\s]+)`)
	semverActionRefPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	shaActionRefPattern    = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
)

func TestGitHubActionsUseVersionPinnedRefs(t *testing.T) {
	root := repositoryRoot(t)
	workflows := filepath.Join(root, ".github", "workflows")
	var violations []string

	err := filepath.WalkDir(workflows, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yml" && ext != ".yaml" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range actionUseLinePattern.FindAllSubmatch(data, -1) {
			action := strings.Trim(string(match[1]), `"'`)
			if actionRefIsVersionPinned(action) {
				continue
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			violations = append(violations, filepath.ToSlash(relative)+":"+action)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan GitHub workflow action refs: %v", err)
	}
	if len(violations) != 0 {
		sort.Strings(violations)
		t.Fatalf("GitHub Actions must use local workflows, full semantic-version refs, or commit SHAs: %v", violations)
	}
}

func TestTestSuiteUsesFailClosedEvidenceTiers(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatalf("read Test Suite workflow: %v", err)
	}
	workflow := string(data)

	for _, required := range []string{
		"name: Test Suite",
		"name: Release candidate",
		"test-ci-policy.js",
		"node scripts/test-ci-policy.js verify",
		"node scripts/check-markdown-links.js",
		"node scripts/run-fuzz.js --profile smoke",
		"run-targeted --failure-class race --platform linux --race",
		"run-targeted --failure-class platform --platform",
		"gitleaks/gitleaks-action@v3.0.0",
		"GITLEAKS_VERSION: '8.30.0'",
		"goreleaser/goreleaser-action@v7.2.3",
		"args: check",
		"if: ${{ always() }}",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("Test Suite workflow is missing %q", required)
		}
	}
	if count := strings.Count(workflow, "go vet ./..."); count != 1 {
		t.Errorf("go vet invocation count = %d, want 1", count)
	}
	if count := strings.Count(workflow, "node scripts/run-fuzz.js --profile smoke"); count != 1 {
		t.Errorf("canonical fuzz smoke invocation count = %d, want 1", count)
	}
	if strings.Contains(workflow, "name: Test (${{ matrix.os }})") {
		t.Error("legacy all-purpose OS matrix must be replaced by evidence-specific jobs")
	}
}

func TestActionRefPinPolicyRejectsFloatingRefs(t *testing.T) {
	for action, want := range map[string]bool{
		"./.github/workflows/reusable.yml":                      true,
		"actions/checkout@v7.0.1":                               true,
		"owner/action@0123456789abcdef0123456789abcdef01234567": true,
		"actions/checkout@v7":                                   false,
		"actions/checkout@main":                                 false,
		"actions/checkout@latest":                               false,
		"actions/checkout":                                      false,
	} {
		if got := actionRefIsVersionPinned(action); got != want {
			t.Errorf("actionRefIsVersionPinned(%q) = %v, want %v", action, got, want)
		}
	}
}

func actionRefIsVersionPinned(action string) bool {
	if strings.HasPrefix(action, "./") {
		return true
	}
	at := strings.LastIndex(action, "@")
	if at <= 0 || at == len(action)-1 {
		return false
	}
	ref := action[at+1:]
	return semverActionRefPattern.MatchString(ref) || shaActionRefPattern.MatchString(ref)
}
