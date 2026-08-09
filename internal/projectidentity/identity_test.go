package projectidentity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	forkModule       = "github.com/zoster81/scripthold"
	forkRegistryName = "io.github.zoster81/scripthold"
	forkRepository   = "https://github.com/zoster81/scripthold"
)

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func TestGoModuleTargetsFork(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "module "+forkModule+"\n") {
		t.Fatalf("go.mod must declare module %q", forkModule)
	}
}

func TestUpstreamReferencesAreDocumentationOnly(t *testing.T) {
	root := repositoryRoot(t)
	upstreamOwner := "dimitar" + "-grigorov"
	allowedCounts := map[string]int{
		"README.md":    1,
		"CHANGELOG.md": 1,
		filepath.FromSlash("docs/PROJECT_DIRECTION.md"): 2,
		filepath.FromSlash("docs/PUBLISHING.md"):        1,
	}
	actualCounts := make(map[string]int)

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isIdentityTextFile(entry.Name()) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		count := strings.Count(string(data), upstreamOwner)
		if count == 0 {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		allowed, ok := allowedCounts[relative]
		if !ok {
			t.Errorf("operational file %s contains %d upstream repository reference(s)", relative, count)
			return nil
		}
		actualCounts[relative] = count
		if count != allowed {
			t.Errorf("documentation file %s contains %d upstream references, want %d", relative, count, allowed)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for path, expected := range allowedCounts {
		if actualCounts[path] != expected {
			t.Errorf("documentation file %s contains %d upstream references, want %d", path, actualCounts[path], expected)
		}
	}
}

func TestTrackedTextExcludesPrivateOperatorState(t *testing.T) {
	root := repositoryRoot(t)
	privateMarkers := []string{
		"D:" + `\OpenAI-Tunnel`,
		"D:" + `\\OpenAI-Tunnel`,
		"@" + "Ryzen9_0",
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isIdentityTextFile(entry.Name()) {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, marker := range privateMarkers {
			if strings.Contains(content, marker) {
				t.Errorf("tracked text file %s contains private operator marker %q", relative, marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestOperationalMetadataTargetsFork(t *testing.T) {
	root := repositoryRoot(t)
	assertFileContains(t, root, ".goreleaser.yml", "-X "+forkModule+"/filetoolsserver.Version={{.Version}}")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/publish-registry.yml"), "github.repository == 'zoster81/scripthold'")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/release.yml"), "uses: ./.github/workflows/publish-registry.yml")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/release.yml"), "node scripts/verify-release-version.js")
	assertFileContains(t, root, filepath.FromSlash("scripts/generate-server-json.js"), "const forkRepository = '"+forkRepository+"'")
}

func TestGoReleaserArchiveMetadataIsDeterministic(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".goreleaser.yml"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "builds_info:") {
		t.Error(".goreleaser.yml must define builds_info for archive binaries")
	}
	if got := strings.Count(content, "mtime: '{{ .CommitDate }}'"); got != 10 {
		t.Errorf(".goreleaser.yml deterministic mtime count = %d, want 10", got)
	}
	if got := strings.Count(content, "owner: root"); got != 10 {
		t.Errorf(".goreleaser.yml deterministic owner count = %d, want 10", got)
	}
	if got := strings.Count(content, "group: root"); got != 10 {
		t.Errorf(".goreleaser.yml deterministic group count = %d, want 10", got)
	}
	if got := strings.Count(content, "mode: 0644"); got != 9 {
		t.Errorf(".goreleaser.yml document mode count = %d, want 9", got)
	}
	if got := strings.Count(content, "mode: 0755"); got != 1 {
		t.Errorf(".goreleaser.yml binary mode count = %d, want 1", got)
	}
	assertFileContains(t, root, ".goreleaser.yml", "src: examples/start-openai-tunnel-stdio-plus-local-http.ps1")
	assertFileContains(t, root, ".goreleaser.yml", "src: examples/start-openai-tunnel-http-plus-local-stdio.ps1")
	assertFileContains(t, root, ".goreleaser.yml", "src: examples/start-local-stdio.ps1")
	assertFileContains(t, root, ".goreleaser.yml", "src: examples/start-local-http.ps1")
}

func TestPublicLauncherExamplesRemainFailClosed(t *testing.T) {
	root := repositoryRoot(t)
	tunnelExample := filepath.FromSlash("examples/start-openai-tunnel-stdio-plus-local-http.ps1")
	assertFileContains(t, root, tunnelExample, `$RuntimeApiKey = "REPLACE_WITH_RUNTIME_API_KEY"`)
	assertFileContains(t, root, tunnelExample, `$TunnelId = "tunnel_REPLACE_WITH_ID"`)
	assertFileContains(t, root, tunnelExample, `$TunnelId -notmatch '^tunnel_[0-9a-f]{32}$'`)
	assertFileContains(t, root, tunnelExample, `$AllowedDirectory = $allowedItem.FullName`)
	assertFileContains(t, root, tunnelExample, `$TokenFile = "C:\Path\To\scripthold.token"`)
	assertFileContains(t, root, tunnelExample, `$StdioBackupStore = "C:\Path\To\PrivateState\stdio"`)
	assertFileContains(t, root, tunnelExample, `$HttpBackupStore = "C:\Path\To\PrivateState\http"`)
	assertFileContains(t, root, tunnelExample, `"MCP_COMMAND"`)
	assertFileContains(t, root, tunnelExample, `"MCP_STDIO_LEGACY_HANDSHAKE"`)
	assertFileContains(t, root, tunnelExample, `$tunnelEnvironment = @{`)
	assertFileNotContains(t, root, tunnelExample, `$McpServerUrl`)
	assertFileContains(t, root, tunnelExample, `--transport=streamable-http`)
	assertFileContains(t, root, tunnelExample, `/api/status`)
	assertFileContains(t, root, tunnelExample, `probe_status`)
	assertFileContains(t, root, tunnelExample, `$EnableRunScript = $false`)
	assertFileContains(t, root, tunnelExample, `$EnableShell = $false`)
	assertFileContains(t, root, tunnelExample, `"MCP_HTTP_TOKEN_FILE"`)
	assertFileContains(t, root, tunnelExample, `"MCP_HTTP_ENABLE_EXECUTION"`)
	assertFileNotContains(t, root, tunnelExample, `Authorization: env:MCP_TUNNEL_AUTHORIZATION`)

	reverseExample := filepath.FromSlash("examples/start-openai-tunnel-http-plus-local-stdio.ps1")
	assertFileContains(t, root, reverseExample, `$McpServerUrl = "http://127.0.0.1:8765/mcp"`)
	assertFileContains(t, root, reverseExample, `Authorization: env:MCP_TUNNEL_AUTHORIZATION`)
	assertFileContains(t, root, reverseExample, `--transport=stdio`)
	assertFileContains(t, root, reverseExample, `"MCP_SERVER_URL"`)
	assertFileContains(t, root, reverseExample, `"MCP_COMMAND"`)
	assertFileNotContains(t, root, reverseExample, `"MCP_STDIO_LEGACY_HANDSHAKE" = "1"`)

	stdioExample := filepath.FromSlash("examples/start-local-stdio.ps1")
	assertFileContains(t, root, stdioExample, `--transport=stdio`)
	assertFileContains(t, root, stdioExample, `$EnableRunScript = $false`)
	assertFileContains(t, root, stdioExample, `$EnableShell = $false`)
	assertFileContains(t, root, stdioExample, `"CONTROL_PLANE_API_KEY"`)
	assertFileContains(t, root, stdioExample, `"MCP_HTTP_TOKEN"`)

	httpExample := filepath.FromSlash("examples/start-local-http.ps1")
	assertFileContains(t, root, httpExample, `$ListenAddress = "127.0.0.1:8765"`)
	assertFileContains(t, root, httpExample, `$AllowNonLoopback = $false`)
	assertFileContains(t, root, httpExample, `$EnableHttpExecution = $false`)
	assertFileContains(t, root, httpExample, `$EnableRunScript = $false`)
	assertFileContains(t, root, httpExample, `$EnableShell = $false`)
	assertFileContains(t, root, httpExample, `A non-loopback listener requires TLS or a trusted proxy CIDR.`)
	assertFileContains(t, root, httpExample, `"CONTROL_PLANE_API_KEY"`)
	assertFileContains(t, root, httpExample, `"CONTROL_PLANE_TUNNEL_ID"`)
}

func TestPublicLauncherExamplesExposeDurableTaskPolicy(t *testing.T) {
	root := repositoryRoot(t)
	examples := []string{
		filepath.FromSlash("examples/start-local-stdio.ps1"),
		filepath.FromSlash("examples/start-local-http.ps1"),
		filepath.FromSlash("examples/start-openai-tunnel-stdio-plus-local-http.ps1"),
		filepath.FromSlash("examples/start-openai-tunnel-http-plus-local-stdio.ps1"),
	}
	required := []string{
		"MCP_TASK_STORE_DIR",
		"MCP_TASK_MAX_CONCURRENCY",
		"MCP_TASK_MAX_QUEUED",
		"MCP_TASK_MAX_LOG_BYTES_PER_STREAM",
		"MCP_TASK_MAX_RUNTIME_SECONDS",
		"MCP_TASK_RETENTION_DAYS",
		"MCP_TASK_MAX_TERMINAL",
		"MCP_TASK_MAX_TOTAL_BYTES",
		"function Ensure-TaskSupervisor",
		`@("task-supervisor", "--",`,
		"supervisor.heartbeat",
		"worker.heartbeat",
	}
	for _, example := range examples {
		for _, value := range required {
			assertFileContains(t, root, example, value)
		}
		data, err := os.ReadFile(filepath.Join(root, example))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "function Start-TaskWorker") {
			t.Errorf("%s exposes the obsolete Start-TaskWorker name", example)
		}
	}
}

func TestContainerAndSmitheryMetadataMatchTheRuntimeContract(t *testing.T) {
	root := repositoryRoot(t)
	assertFileContains(t, root, "Dockerfile", "FROM golang:1.26.5-alpine3.24 AS builder")
	assertFileContains(t, root, "Dockerfile", "FROM alpine:3.24.1")
	assertFileContains(t, root, "Dockerfile", "USER 10001:10001")
	assertFileContains(t, root, "Dockerfile", `ENTRYPOINT ["/usr/local/bin/scripthold"]`)
	assertFileContains(t, root, "smithery.yaml", "command: '/usr/local/bin/scripthold'")
	assertFileContains(t, root, "smithery.yaml", "const args = ['--transport=stdio', config.allowedDirectory]")
}

func TestForkOwnedDownloaderPluginIsRemoved(t *testing.T) {
	root := repositoryRoot(t)
	for _, relativePath := range []string{
		filepath.FromSlash("plugin"),
		filepath.FromSlash(".claude-plugin"),
		filepath.FromSlash("scripts/bump-version.js"),
	} {
		_, err := os.Stat(filepath.Join(root, relativePath))
		if err == nil {
			t.Errorf("removed plugin path still exists: %s", relativePath)
			continue
		}
		if !os.IsNotExist(err) {
			t.Errorf("inspect removed plugin path %s: %v", relativePath, err)
		}
	}
}

func TestReleaseWorkflowsRunNativeAndContainerSmokes(t *testing.T) {
	root := repositoryRoot(t)
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/test.yml"), "TestExternal(StdioBinarySmoke|DurableTaskLifecycle|TaskSupervisorRecovery)")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/release.yml"), "TestExternal(StdioBinarySmoke|DurableTaskLifecycle|TaskSupervisorRecovery)")

	buildWorkflow := filepath.FromSlash(".github/workflows/build.yml")
	assertFileContains(t, root, buildWorkflow, "container-smoke:")
	assertFileContains(t, root, buildWorkflow, "MCP_EXTERNAL_SMOKE_EXECUTABLE=docker")
	assertFileContains(t, root, buildWorkflow, "--transport=streamable-http /data")
	assertFileContains(t, root, buildWorkflow, `token="$(sudo cat "${workdir}/secrets/token")"`)

	data, err := os.ReadFile(filepath.Join(root, buildWorkflow))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	chmodIndex := strings.Index(content, `chmod 0600 "${workdir}/secrets/token" "${workdir}/secrets/key.pem"`)
	chownIndex := strings.Index(content, `sudo chown -R 10001:10001 "${workdir}/data" "${workdir}/secrets"`)
	if chmodIndex < 0 || chownIndex < 0 {
		t.Fatal("container workflow must set secret modes and mapped ownership explicitly")
	}
	if chmodIndex > chownIndex {
		t.Error("container workflow must set secret modes before transferring ownership to UID 10001")
	}
}

func TestValidationToolVersionsArePinned(t *testing.T) {
	root := repositoryRoot(t)
	assertFileContains(t, root, filepath.FromSlash("scripts/validate-workflows.sh"), "ACTIONLINT_VERSION=1.7.12")
	assertFileContains(t, root, filepath.FromSlash("scripts/validate-workflows.sh"), "SHELLCHECK_VERSION=0.11.0")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/test.yml"), "actions/checkout@v7.0.1")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/test.yml"), "actions/setup-go@v7.0.0")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/build.yml"), "actions/upload-artifact@v7.0.1")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/test.yml"), "staticcheck@v0.7.0")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/test.yml"), "govulncheck@v1.6.0")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/release.yml"), "goreleaser/goreleaser-action@v7.2.3")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/release.yml"), "version: 'v2.17.1'")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/publish-registry.yml"), "MCP_PUBLISHER_VERSION: 1.8.1")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/publish-registry.yml"), "MCP_PUBLISHER_LINUX_AMD64_SHA256: a06c9096dcb9727c13555b6be26c7effa707b01f06a4c561ba7a3635443cf2cc")
	assertFileContains(t, root, filepath.FromSlash(".github/workflows/publish-registry.yml"), "./mcp-publisher validate")
}

func TestBackupStoreFuzzTargetsRunInCI(t *testing.T) {
	root := repositoryRoot(t)
	workflow := filepath.FromSlash(".github/workflows/test.yml")
	for _, target := range []string{
		"FuzzDecodeDescriptor",
		"FuzzDecodeManifest",
		"FuzzDecodeIndex",
		"FuzzDecodeListCursor",
		"FuzzValidateGCPlan",
		"FuzzParseBackupDiagnosticCommand",
	} {
		assertFileContains(t, root, workflow, target)
	}
}

func assertFileContains(t *testing.T, root, relativePath, expected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), expected) {
		t.Errorf("%s must contain %q", relativePath, expected)
	}
}

func assertFileNotContains(t *testing.T, root, relativePath, unexpected string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, relativePath))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), unexpected) {
		t.Errorf("%s must not contain %q", relativePath, unexpected)
	}
}
func TestRegistryTemplateTargetsFork(t *testing.T) {
	root := repositoryRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "server.template.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
		Repository  struct {
			URL string `json:"url"`
		} `json:"repository"`
		Packages []struct {
			Identifier string `json:"identifier"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Name != forkRegistryName {
		t.Errorf("registry name = %q, want %q", manifest.Name, forkRegistryName)
	}
	if len(manifest.Description) == 0 || len(manifest.Description) > 100 {
		t.Errorf("registry description length = %d, want 1..100", len(manifest.Description))
	}
	if manifest.Homepage != forkRepository || manifest.Repository.URL != forkRepository {
		t.Errorf("registry repository metadata must target %s", forkRepository)
	}
	for _, pkg := range manifest.Packages {
		if !strings.HasPrefix(pkg.Identifier, forkRepository+"/releases/download/") {
			t.Errorf("package identifier %q does not target fork releases", pkg.Identifier)
		}
	}
}

func isIdentityTextFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".go", ".mod", ".sum", ".json", ".yml", ".yaml", ".js", ".mjs", ".cjs", ".ps1", ".sh", ".bat", ".cmd", ".md", ".txt", ".toml", ".xml", ".ini", ".conf":
		return true
	}
	return name == "Makefile" || name == "Dockerfile"
}
