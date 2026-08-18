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
	allowedPaths := map[string]bool{
		"README.md":    true,
		"CHANGELOG.md": true,
		filepath.FromSlash("docs/PROJECT_DIRECTION.md"): true,
		filepath.FromSlash("docs/PUBLISHING.md"):        true,
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
		count := strings.Count(string(data), upstreamOwner)
		if count == 0 {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if !allowedPaths[relative] {
			t.Errorf("operational file %s contains %d upstream repository reference(s)", relative, count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
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
	assertDockerBaseImagesAreVersionPinned(t, root)
	assertFileContains(t, root, "Dockerfile", "USER 10001:10001")
	assertFileContains(t, root, "Dockerfile", `ENTRYPOINT ["/usr/local/bin/scripthold"]`)
	assertFileContains(t, root, "smithery.yaml", "command: '/usr/local/bin/scripthold'")
	assertFileContains(t, root, "smithery.yaml", "const args = ['--transport=stdio', config.allowedDirectory]")
}

func assertDockerBaseImagesAreVersionPinned(t *testing.T, root string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	fromCount := 0
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if !strings.HasPrefix(line, "FROM ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Errorf("malformed Dockerfile FROM instruction: %q", line)
			continue
		}
		fromCount++
		image := fields[1]
		if strings.Contains(image, "@sha256:") {
			continue
		}
		lastSlash := strings.LastIndex(image, "/")
		lastColon := strings.LastIndex(image, ":")
		if lastColon <= lastSlash || lastColon == len(image)-1 {
			t.Errorf("Dockerfile base image %q must use an immutable digest or explicit version tag", image)
			continue
		}
		tag := image[lastColon+1:]
		if strings.EqualFold(tag, "latest") || tag[0] < '0' || tag[0] > '9' || !strings.Contains(tag, ".") {
			t.Errorf("Dockerfile base image %q must use an immutable digest or explicit version tag", image)
		}
	}
	if fromCount == 0 {
		t.Error("Dockerfile must declare at least one base image")
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
