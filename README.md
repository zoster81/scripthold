# Scripthold — Secure MCP Server for Local Workspaces

<!-- mcp-name: io.github.zoster81/scripthold -->

[![Go Report Card](https://goreportcard.com/badge/github.com/zoster81/scripthold)](https://goreportcard.com/report/github.com/zoster81/scripthold)
[![Release](https://img.shields.io/github/v/release/zoster81/scripthold)](https://github.com/zoster81/scripthold/releases/latest)
[![License: GPL-3.0](https://img.shields.io/github/license/zoster81/scripthold)](LICENSE)
[![MCP Registry](https://img.shields.io/badge/MCP_Registry-Scripthold-blue)](https://registry.modelcontextprotocol.io/?search=io.github.zoster81%2Fscripthold)

**Code from the web. Work locally. Recover safely.**

**Scripthold is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server.** It gives ChatGPT and other MCP-compatible web, desktop, and CLI clients controlled access to explicitly authorized local workspaces.

The server exposes secure, encoding-aware tools to read, edit, convert, execute, test, back up, and restore local work. Clients can connect through **stdio**, authenticated **Streamable HTTP**, or a secure tunnel that bridges to either transport.

**Scripthold was built with Scripthold.** The project has been developed and verified through the same web-to-local workflow it provides to its users.

> **Lineage:** Scripthold originated from the [original project](docs/PROJECT_DIRECTION.md#relationship-to-upstream), created by **Dimitar Grigorov**. The project retains its GPL-3.0 lineage and permanent attribution.

AI clients see `Настройки` — not `????` or `Íàñòðîéêè`.

Scripthold detects text encodings from bytes rather than filenames, presents UTF-8 to the client, and preserves or deliberately converts encoding, BOM, and line endings through bounded-memory and durable filesystem operations.

- **30 tools and 3 guided prompts over both transports** — one catalog, one process-wide root policy, one error model, and equivalent behavior through stdio and Streamable HTTP.
- **Agent-oriented repository workflows** — optional read line numbers, paged/multi-mode grep, `.gitignore` traversal, bounded sorting, batch conversion previews, approval-bound one-shot edits, strict patches, and ambiguity-safe fuzzy matching.
- **24 registered encodings** — Cyrillic, Windows-125x, ISO-8859, KOI8, UTF-16 LE/BE, GBK/GB18030, and other legacy text formats.
- **Fail-closed HTTP service** — bearer authentication on every MCP request, loopback defaults, exact Host/Origin checks, bounded sessions and request resources, no CORS, and explicit TLS/proxy requirements for non-loopback exposure.
- **Secure filesystem and mutation model** — resolved-root containment, deterministic traversal, bounded streaming, staged writes, practical concurrent-change detection, operation-specific transactional `.bak` handling, and no-replace creation.
- **Durable asynchronous execution** — `task_run` admits shell/script work into an owner-only persistent queue; independent workers, bounded logs, recovery, parallelism, logical locks, and cancellation keep long builds alive across connector restarts.

**Suitable for:** persistent local or containerized MCP services, desktop and CLI clients, secure tunnel bridges such as the OpenAI Secure MCP Tunnel, and legacy codebases whose text encoding cannot be inferred reliably from a filename or extension.

## Project Direction

Scripthold began as a deployment-oriented fork for ChatGPT Web, but version 2.0 established an independently versioned downstream project rather than a thin synchronization branch. It owns its Go module, MCP Registry identity, release pipeline, public API decisions, transport architecture, container contract, and security documentation.

| Transport | Typical deployment | Security boundary | Roots behavior |
|---|---|---|---|
| stdio | Local MCP clients and secure tunnel bridges | Client configuration and operating-system process boundary | Startup directories are authoritative; dynamic roots are a compatibility fallback only when startup roots are empty |
| stateful Streamable HTTP | Persistent localhost services, containers, trusted proxies, and explicitly secured remote services | Bearer token on every MCP request; loopback by default; TLS or a trusted proxy boundary for non-loopback listeners | Startup directories are immutable and shared by every session; HTTP client roots are disabled |

Both transports use the same `BuildServer` path and expose the same 30 tools, 3 prompts, encoding behavior, limits, typed errors, and execution policy. The HTTP trust model is defined in [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md); the fork's independent scope and relationship to upstream are defined in [docs/PROJECT_DIRECTION.md](docs/PROJECT_DIRECTION.md).

The OpenAI Secure MCP Tunnel remains a supported stdio deployment option, not the identity or only use case of the project. The fork does not require Claude Code, Codex, ChatGPT, or another specific MCP host. Version 2.0 removes the fork-owned Claude Code downloader plugin to avoid maintaining a second network installer and cache trust boundary; any compatible client can invoke the released binary directly or connect to its HTTP endpoint.

### Process-wide directory and session model

Allowed directories are a **process-wide authorization boundary**. Every MCP connection or future HTTP session attached to one server process sees the same configured directory set and the same 30 tools, limits, execution flags, and error behavior. A session represents an independent protocol connection with its own requests, cancellation, and lifecycle; it is not a per-agent filesystem role or sandbox.

This deliberately supports deployments where several agents work on different projects under one allowed drive or workspace, read shared documentation or libraries, and follow prompt-level rules about where each agent may write. The server does not enforce those per-agent read/write conventions. When technical isolation is required, run separate server processes with narrower allowed directories and, for concurrent Git work, separate checkouts or worktrees.

Directories supplied when the process starts remain authoritative and cannot be changed by a session. For stdio compatibility only, a roots-capable client may provide dynamic MCP roots when the process starts with no directory arguments. Streamable HTTP disables client roots and every HTTP session shares the same process-wide configured directories.

The fork-specific architecture includes authoritative process roots, Windows drive-root handling, optional local execution tools, a shared encoding/BOM-aware streaming text core, deterministic secure traversal, durable atomic mutations, transport-independent typed operation errors, bounded ordered concurrency and aggregate output budgets, shared process preparation, a transport-independent server builder, a fail-closed native Streamable HTTP transport, and an authoritative tool-metadata catalog. The upstream project remains the source of the original encoding-aware file-tool implementation.

## Release Status

Version `2.1.1` is the current public Scripthold release, with 30 tools and 3 guided prompts over stdio and Streamable HTTP. Its GitHub Release includes six raw binaries, six platform archives, six OS/architecture-specific MCPB bundles, and their checksum manifests; `io.github.zoster81/scripthold` version `2.1.1` is active in the MCP Registry. Version `2.0.0` remains the historical 23-tool rollback baseline and keeps its pre-rebrand assets and Registry identity.

The `2.1.x` line consolidates completed R15–R21 work with durable asynchronous tasks, persistent backup/restore/GC, offline backup diagnostics, and MCP `2026-07-28` support through official Go SDK `v1.7.0`. HTTP serves stateless `2026-07-28` requests beside retained stateful legacy sessions behind the same security boundary. Release `2.1.1` is published, deployed, and rollback-verified; see [docs/ROADMAP.md](docs/ROADMAP.md) for current milestone status.

Encoding detection remains content-based and extension-independent. The semantic-tag workflow requires a dated changelog entry before generating release assets and Registry metadata.

## What It Does

Provides 30 tools for file operations, encoding conversion, state verification, update checks, and durable local execution, plus 3 guided prompts:
- [`read_text_file`](TOOLS.md#read_text_file) - Stream decoded text with bounded line/output memory and optional absolute line numbers
- [`read_multiple_files`](TOOLS.md#read_multiple_files) - Read files in deterministic order under one aggregate decoded-output budget
- [`write_whole_file`](TOOLS.md#write_whole_file) - Replace the complete file contents through the shared encoder with explicit `auto`/`always`/`never`/`preserve` BOM policy
- [`edit_file`](TOOLS.md#edit_file) - Direct edits or bounded one-shot preview/apply, including optional required pre-state backup, exact/flexible/fuzzy operations, and strict unified patches
- [`patch_package`](TOOLS.md#patch_package) - Inspect, preview, apply, and verify bounded declared multi-file edits, with optional all-target required backups and explicit partial-state evidence
- [`copy_file`](TOOLS.md#copy_file) - Copy a file to a new location
- [`delete_file`](TOOLS.md#delete_file) - Delete a file
- [`list_directory`](TOOLS.md#list_directory) - Browse directories with filtering and deterministic name/mtime/size sorting
- [`tree`](TOOLS.md#tree) - Compact deterministic `.gitignore`-aware tree through the shared secure walker
- [`search_files`](TOOLS.md#search_files) - `.gitignore`-aware glob search with bounded globally correct sorting
- [`fingerprint_paths`](TOOLS.md#fingerprint_paths) - Stream deterministic SHA-256 state fingerprints with optional bounded entry details
- [`verify_state`](TOOLS.md#verify_state) - Run bounded typed JSON, text-format, fixed Git diff, and fingerprint checks without arbitrary shell execution
- [`backup_store`](TOOLS.md#backup_store) - Review, restore, and explicitly garbage-collect the optional persistent store through bounded one-shot workflows
- [`grep_text_files`](TOOLS.md#grep_text_files) - Paged regex search with pattern/filter arrays and content/path/count modes
- [`detect_encoding`](TOOLS.md#detect_encoding) - Auto-detect file encoding with confidence score
- [`convert_encoding`](TOOLS.md#convert_encoding) - Single/batch conversion, dry-run previews, unsupported-rune locations, and durable writes
- [`detect_line_endings`](TOOLS.md#detect_line_endings) - Stream CRLF/LF/mixed detection with bounded inconsistent-line output
- [`change_line_endings`](TOOLS.md#change_line_endings) - Stream LF/CRLF conversion while preserving encoding, BOM, and unrelated bytes
- [`manage_bom`](TOOLS.md#manage_bom) - Inspect a bounded prefix or stream BOM add/strip through durable staging
- [`list_encodings`](TOOLS.md#list_encodings) - Show all supported encodings
- [`get_file_info`](TOOLS.md#get_file_info) - Get file/directory metadata
- [`create_directory`](TOOLS.md#create_directory) - Create directories recursively (mkdir -p)
- [`move_file`](TOOLS.md#move_file) - Move or rename files and directories
- [`list_allowed_directories`](TOOLS.md#list_allowed_directories) - Show accessible directories
- [`task_run`](TOOLS.md#task_run) - Durably enqueue an idempotent shell or script task without holding the MCP request open
- [`task_list`](TOOLS.md#task_list) - Page and filter the persistent task registry
- [`task_get`](TOOLS.md#task_get) - Read status, result, worker liveness, and bounded lifecycle history
- [`task_logs`](TOOLS.md#task_logs) - Read bounded stdout/stderr incrementally with absolute cursors
- [`task_cancel`](TOOLS.md#task_cancel) - Cancel queued work or terminate a running task process tree
- [`check_for_updates`](TOOLS.md#check_for_updates) - Check the latest release of this fork with a cached GitHub request

**Supported encodings (24 total):**
- **Unicode:** UTF-8, UTF-16 LE, UTF-16 BE
- **Cyrillic:** Windows-1251, KOI8-R, KOI8-U, CP866, ISO-8859-5
- **Western European:** Windows-1252, ISO-8859-1, ISO-8859-15
- **Central European:** Windows-1250, ISO-8859-2
- **Greek:** Windows-1253, ISO-8859-7
- **Turkish:** Windows-1254, ISO-8859-9
- **Chinese:** GBK, GB18030
- **Other:** Hebrew (Windows-1255), Arabic (Windows-1256), Baltic (Windows-1257), Vietnamese (Windows-1258), Thai (Windows-874)

`manage_bom` additionally recognizes UTF-32 LE/BE BOM signatures, but UTF-32 is not one of the 24 registered read/write encodings.

See [TOOLS.md](TOOLS.md) for detailed parameters and examples.

The whole-document replacement tool is deliberately named `write_whole_file` rather than the shorter historical `write_file`. The operation replaces the complete target contents with the supplied `content`; text omitted from the request is discarded. Making that destructive scope explicit in the tool name reduces the risk of an agent mistaking it for an incremental edit or append operation. Use `edit_file` when only part of an existing document should change.

**Security:** File operations and `task_run` script paths are restricted to allowed directories. Recursive filesystem tools resolve every visited entry through a shared secure walker and skip symlinks, Windows junctions, and other reparse points that resolve outside those directories. Mutation handlers revalidate paths before commit and use optimistic snapshots plus atomic or no-replace platform operations. Before a script task starts, its script and working directory are revalidated and SHA-256-matching bytes are copied to an owner-only private snapshot used for execution. A shell task revalidates only its working directory; the command itself remains unrestricted and runs with the operating-system permissions of the executor identity. The private task store is owner-only, non-overlapping, and inaccessible to ordinary file tools.

## Architecture

The current design is organized around a few stable boundaries:

- content-based encoding detection and bounded streaming across 24 registered encodings;
- resolved-root filesystem confinement, secure recursive traversal, and durable staged mutations;
- deterministic fingerprints, one-shot edit/package approval workflows, and structured verification;
- an optional non-overlapping persistent backup store with approval-bound capture, original-target restore, explicit GC, and offline diagnosis;
- an optional owner-only durable task store with idempotent admission, persistent queueing, detached executors, bounded cursor logs, recovery, retention, and a restart supervisor;
- one process-wide tool/root policy shared by stdio and Streamable HTTP;
- authenticated Streamable HTTP with stateless MCP `2026-07-28` requests beside retained stateful legacy sessions under one security pipeline;
- disabled-by-default execution tools with a second HTTP execution gate;
- one authoritative tool catalog feeding runtime registration, documentation checks, GoReleaser packaging, and MCP Registry projection.

See [docs/PROJECT_DIRECTION.md](docs/PROJECT_DIRECTION.md) for scope, [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md) for the HTTP trust model, and [CHANGELOG.md](CHANGELOG.md) for detailed change history.

## Installation

Choose **stdio** when the MCP client should own the child-process lifecycle or when a secure bridge expects a local command. Choose **Streamable HTTP** when the server should run as a persistent authenticated service, including localhost, containers, or a TLS/trusted-proxy deployment. The recipes below are deployment options, not a priority order; both transports expose the same tools and policy.

### Stdio through the OpenAI Secure MCP Tunnel

The default Windows tunnel example configures `tunnel-client` with `MCP_COMMAND`, so the tunnel owns one Scripthold stdio child. A second Scripthold process exposes authenticated Streamable HTTP only on loopback for independent local clients. The tunnel branch never connects to that HTTP endpoint, and the HTTP branch receives no OpenAI control-plane credentials.

Requirements:

- Windows PowerShell 5.1 or later;
- the official OpenAI [`tunnel-client`](https://github.com/openai/tunnel-client) executable;
- a Windows build of this fork;
- an OpenAI Runtime API key with the tunnel permissions required by your OpenAI configuration;
- a valid Tunnel ID;
- one explicit local directory to expose to both isolated Scripthold processes;
- one private bearer-token file containing at least 32 characters for the local HTTP process;
- separate backup-store directories when persistent backups are enabled for both processes.

This project uses OpenAI's official Secure MCP Tunnel client, not a third-party tunnel implementation. See the [official OpenAI tunnel-client repository](https://github.com/openai/tunnel-client) and the [OpenAI Secure MCP Tunnel guide](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels) for tunnel installation, permissions, control-plane setup, and current product requirements.

The official client is the customer-run agent that connects a private or localhost MCP server to OpenAI-hosted products while keeping the MCP server off the public internet.

#### Build the fork locally

```powershell
git clone https://github.com/zoster81/scripthold.git
Set-Location .\scripthold
go test ./...
go build -o scripthold_windows_amd64.exe ./cmd/scripthold
```

The Go module is `github.com/zoster81/scripthold`, and all internal imports resolve through the fork namespace. Build from source for development commits; use only fork-owned release tags with matching assets for packaged installations.

`go install github.com/zoster81/scripthold/cmd/scripthold@main` installs the current development source. For reproducible installations, prefer an explicit Scripthold semantic tag once published rather than relying on `@latest` across the historical repository rename.

#### Download a fork release

Release `2.0.0` predates the Scripthold asset rename and therefore keeps the historical `mcp-file-tools_*` filenames. Version `2.1.0` and later use `scripthold_<os>_<arch>` names as defined by `.goreleaser.yml`. For development commits that are not semantic releases, build from source as shown above instead of relying on `releases/latest` asset names.

#### OpenAI Tunnel quick start

Four fail-closed PowerShell examples cover the supported local and tunnel topologies:

| Example | Topology |
|---|---|
| [`start-local-stdio.ps1`](examples/start-local-stdio.ps1) | One foreground local stdio server. |
| [`start-local-http.ps1`](examples/start-local-http.ps1) | One authenticated HTTP server; loopback by default. |
| [`start-openai-tunnel-stdio-plus-local-http.ps1`](examples/start-openai-tunnel-stdio-plus-local-http.ps1) | **Default:** tunnel to a dedicated stdio child, plus an independent local HTTP process. |
| [`start-openai-tunnel-http-plus-local-stdio.ps1`](examples/start-openai-tunnel-http-plus-local-stdio.ps1) | Reverse example: tunnel to authenticated HTTP, plus an independent foreground local stdio child. |

Use the default example when OpenAI should reach Scripthold through stdio while local clients use HTTP. The reverse example is deliberately separate so choosing it cannot silently change the default topology.

Place these files in the same private working directory:

```text
tunnel-client.exe
scripthold_windows_amd64.exe
start-openai-tunnel-stdio-plus-local-http.ps1
```

Copy the example outside the Git checkout before entering credentials:

```powershell
$runDirectory = "$env:LOCALAPPDATA\OpenAI-Mcp-Tunnel"
New-Item -ItemType Directory -Force $runDirectory | Out-Null
Copy-Item .\examples\start-openai-tunnel-stdio-plus-local-http.ps1 $runDirectory
Copy-Item .\scripthold_windows_amd64.exe $runDirectory
# Copy tunnel-client.exe from your OpenAI tunnel installation into the same directory.
notepad "$runDirectory\start-openai-tunnel-stdio-plus-local-http.ps1"
```

Replace only the placeholders:

```powershell
$RuntimeApiKey = "REPLACE_WITH_RUNTIME_API_KEY"
$TunnelId = "tunnel_REPLACE_WITH_ID"
$AllowedDirectory = "C:\Path\To\AllowedProject"
$TokenFile = "C:\Path\To\scripthold.token"
$StdioBackupStore = "C:\Path\To\PrivateState\stdio"
$HttpBackupStore = "C:\Path\To\PrivateState\http"
$TaskStore = "C:\Path\To\PrivateState\tasks"
```

The tunnel identifier must be `tunnel_` followed by exactly 32 lowercase hexadecimal characters. Never commit the edited script, bearer token, or private state. The default example keeps both `task_run` kinds disabled, uses different backup stores for the two frontend processes, and shares one separate task store plus supervisor. When execution is enabled, stdio uses the selected kind gate and HTTP additionally requires its transport gate.

To enable script execution for supported files located inside an allowed directory, change:

```powershell
$EnableRunScript = $true
```

To enable unrestricted shell commands, change:

```powershell
$EnableShell = $true
```

`task_run` with `kind=script` validates and fingerprints the script plus working directory; `kind=shell` validates only its working directory and can access anything permitted to the Windows identity running the executor. Enable these capabilities only for a trusted connector and after reviewing [TOOLS.md](TOOLS.md#durable-task-execution) and [docs/DURABLE_TASKS.md](docs/DURABLE_TASKS.md).

Run the test from Windows PowerShell with the complete one-line command:

```powershell
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "$env:LOCALAPPDATA\OpenAI-Mcp-Tunnel\start-openai-tunnel-stdio-plus-local-http.ps1"
```

From Command Prompt, use:

```bat
powershell.exe -NoLogo -NoProfile -ExecutionPolicy Bypass -File "%LOCALAPPDATA%\OpenAI-Mcp-Tunnel\start-openai-tunnel-stdio-plus-local-http.ps1"
```

The default script starts authenticated loopback HTTP as one process, runs `tunnel-client doctor --explain` with `MCP_COMMAND`, then starts the tunnel and verifies both its dedicated stdio child and the enabled `main` channel with `probe_status=ok`. Ambient HTTP URL/header variables are cleared from the tunnel branch; OpenAI control-plane variables are cleared from the HTTP branch. The local operator UI remains at `http://127.0.0.1:8080/ui`.

### Other stdio MCP clients

The same binary can be used directly by clients that launch local stdio MCP servers. Supply every allowed directory as a command-line argument.

```json
{
  "mcpServers": {
    "scripthold": {
      "type": "stdio",
      "command": "C:\\Tools\\scripthold_windows_amd64.exe",
      "args": ["D:\\Projects", "C:\\Users\\YOUR_NAME\\Documents"]
    }
  }
}
```

The transport can be selected explicitly with `--transport=stdio` or `MCP_TRANSPORT=stdio`. A roots-capable stdio client may provide workspace directories dynamically only when the process starts without directory arguments. Once directories are configured at startup, they remain the authoritative process-wide set. Stdio bridges that probe `server/discover` and may initialize the same persistent child twice may set `MCP_STDIO_LEGACY_HANDSHAKE=1`; this override rejects discovery before SDK state changes and treats only an equivalent repeated legacy `initialize` as idempotent. A repeat with different parameters is rejected. Leave it unset for normal modern stdio discovery; Streamable HTTP ignores it.

### Native Streamable HTTP

The native HTTP transport is bearer-authenticated and bound to loopback by default. Current source serves MCP `2026-07-28` stateless requests beside retained stateful legacy sessions on the same endpoint; every request shares the process-wide startup roots, and HTTP clients cannot add or change them. The tracked HTTP launcher is a standalone reference even when a private deployment launcher starts both transports.

Create a private token file and start the endpoint from PowerShell:

```powershell
$tokenPath = Join-Path $env:TEMP "scripthold.token"
$tokenBytes = New-Object byte[] 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try { $rng.GetBytes($tokenBytes) } finally { $rng.Dispose() }
[System.IO.File]::WriteAllText(
    $tokenPath,
    [Convert]::ToBase64String($tokenBytes),
    [System.Text.UTF8Encoding]::new($false)
)

$env:MCP_HTTP_TOKEN_FILE = $tokenPath
$env:MCP_HTTP_ADDR = "127.0.0.1:8765"
.\scripthold_windows_amd64.exe --transport=streamable-http D:\Projects
```

The MCP endpoint is `http://127.0.0.1:8765/mcp`. Clients must send the token as `Authorization: Bearer <token>` on every MCP `POST`, `GET`, and `DELETE` request. `/healthz` and `/readyz` expose only minimal liveness/readiness status. A complete sanitized Windows launcher with loopback defaults, optional TLS/proxy settings, environment restoration, and both execution gates disabled is available at [`examples/start-local-http.ps1`](examples/start-local-http.ps1).

`MCP_HTTP_TOKEN` and `MCP_HTTP_TOKEN_FILE` are cleared from the server process environment immediately after startup configuration is validated, preventing optional execution tools from inheriting the credential. The token itself remains fixed for the process lifetime; rotation requires a controlled restart.

Do not put tokens in command-line arguments, URLs, cookies, or query parameters. Browser CORS is disabled. Non-loopback listeners require explicit opt-in plus TLS or an explicitly trusted proxy boundary. See [`docs/HTTP_SECURITY.md`](docs/HTTP_SECURITY.md) for the complete deployment and threat model.

### Container image

The repository Dockerfile uses the Go version declared by `go.mod`, a version-pinned Alpine runtime, a statically linked binary, and an unprivileged runtime identity (`10001:10001`). The container working directory is `/data`; cache and temporary files use `/tmp/scripthold`. The image remains transport-neutral, so its entry point is the server binary and callers select stdio or Streamable HTTP explicitly.

Build a development image with an explicit embedded version:

```bash
docker build --build-arg VERSION=dev -t scripthold:dev .
```

A hardened stdio invocation mounts exactly one allowed root and keeps the rest of the container filesystem read-only:

```bash
docker run --rm -i \
  --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --mount type=bind,source=/absolute/project,target=/data \
  scripthold:dev --transport=stdio /data
```

The mounted directory must be accessible to UID/GID `10001`. For native HTTP, mount the workspace at `/data`, mount the bearer token and TLS files read-only under `/run/secrets`, publish port `8765`, and supply the fail-closed non-loopback/TLS settings documented above. A direct-TLS deployment can use an orchestration health check equivalent to:

```text
wget --no-check-certificate --spider -q https://127.0.0.1:8765/healthz
```

The Dockerfile intentionally does not bake in a health check because stdio has no HTTP endpoint. HTTP orchestrators should use `/healthz` for liveness and `/readyz` for readiness; stdio supervisors should monitor the process lifecycle. `SIGTERM` is the declared container stop signal and reaches the server's graceful-shutdown path.

### Updating the fork

The update checker is notification-only and never replaces binaries. For a manual installation, stop the client or service, install the verified asset for the desired release, restart it, and run the relevant transport smoke checks. Do not assume a historical asset filename: `2.0.0` uses the pre-rebrand name, while Scripthold-named releases use `scripthold_<os>_<arch>`.

Set `MCP_NO_UPDATE_CHECK=1` before starting the server to disable release checks.

### Project lineage and independence

This project originated from the [original upstream repository](https://github.com/dimitar-grigorov/mcp-file-tools) and retains its GPL-3.0 lineage and attribution. The fork now owns its module path, release pipeline, update source, MCP Registry namespace, public API decisions, transport architecture, and security model. It is maintained as an independent downstream project rather than a branch expected to remain merge-compatible with later upstream releases.

Upstream continues to evolve separately and is explicitly credited as the source for the R15 agent-workflow feature set and relevant implementation approaches. The code in this fork is reworked for its own architecture rather than mechanically synchronized, and useful functionality, implementation techniques, tests, or security improvements may flow in either direction through normal discussion or GPL-3.0-compatible contributions. Several fork capabilities remain intentionally outside upstream's narrower product direction. See [docs/PROJECT_DIRECTION.md](docs/PROJECT_DIRECTION.md#reciprocal-feature-exchange) for the reciprocal-exchange, maintenance, and contribution boundaries.

## How to Use

Once the connector is active, ask ChatGPT Web or the connected MCP client:
- "List all .pas files in the allowed project directory"
- "Read config.ini and detect its encoding"
- "Show all supported encodings"
- "Read MainForm.dfm using CP1251 encoding"
- "Detect this extensionless file's encoding and line endings"
- "Convert data.legacy from mixed endings to CRLF without changing its encoding or BOM"
- "Convert multilingual.data from UTF-8 to UTF-16 LE with `bom: auto` and create a backup"
- "List the largest matching files while respecting `.gitignore`"
- "Show only file paths containing either of these two regex patterns"
- "Run the UTF-8 migration prompt for this project and preview every unsupported character before writing"

**Security:** File tools access only explicitly allowed directories:
- **OpenAI Tunnel quick start:** the directory embedded in the tunnel's `MCP_COMMAND` is authoritative for its stdio child; the separately started HTTP process receives its own startup directory argument;
- **roots-capable stdio clients:** client-provided roots are accepted only when the process starts without configured directories;
- **multiple sessions:** every connection to one process shares the same allowed directories; prompt instructions may narrow an agent's intended write scope but are not server-enforced ACLs;
- **durable execution:** `task_run` script tasks validate and fingerprint their script and working directory, while shell tasks validate only their working directory and are otherwise unrestricted; the owner-only task store is outside public roots;
- **optional backup store:** `MCP_BACKUP_STORE_DIR` must be a separate canonical non-overlapping path and is denied to ordinary file tools; metadata actions never expose object bytes or internal paths, restore is restricted to a verified manifest's original authorized target, and GC is explicit, generation-bound, pin-aware, manifest-first, reference-counted, and never background-triggered.

## Configuration

The server can be configured via environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| `MCP_TRANSPORT` | Process transport selection: `stdio` or `streamable-http`. The CLI `--transport` option takes precedence. | `stdio` |
| `MCP_STDIO_LEGACY_HANDSHAKE` | Stdio-only compatibility for bridges that probe `server/discover` and repeat an equivalent legacy `initialize` on one persistent child. Discovery is rejected before SDK state changes; different repeated initialization is rejected. HTTP ignores this setting. | disabled |
| `MCP_HTTP_ADDR` | Native HTTP listen address. Only `localhost` or an IP literal is accepted; non-loopback requires explicit opt-in. | `127.0.0.1:8765` |
| `MCP_HTTP_PATH` | Clean absolute MCP endpoint path, distinct from `/healthz` and `/readyz`. | `/mcp` |
| `MCP_HTTP_TOKEN` | Bearer token supplied through the environment. Exactly one token source is required for HTTP. | unset |
| `MCP_HTTP_TOKEN_FILE` | Preferred bearer-token source; must reference a regular readable file. Mutually exclusive with `MCP_HTTP_TOKEN`. | unset |
| `MCP_HTTP_ALLOWED_HOSTS` | Additional comma-separated exact `Host` values. Wildcards and suffix matching are rejected. | listener-derived |
| `MCP_HTTP_ALLOWED_ORIGINS` | Comma-separated exact browser origins. Empty rejects every request carrying `Origin`; no CORS allow headers are emitted. | empty |
| `MCP_HTTP_ALLOW_NON_LOOPBACK` | Explicit opt-in required for a non-loopback listener. | disabled |
| `MCP_HTTP_TLS_CERT_FILE` | TLS certificate for direct HTTPS. Must be configured with `MCP_HTTP_TLS_KEY_FILE`. | unset |
| `MCP_HTTP_TLS_KEY_FILE` | TLS private key for direct HTTPS. Must be configured with `MCP_HTTP_TLS_CERT_FILE`. | unset |
| `MCP_HTTP_TRUSTED_PROXY_CIDRS` | Comma-separated proxy networks permitted to supply a bounded `X-Forwarded-For` chain. | empty |
| `MCP_HTTP_MAX_BODY_BYTES` | Maximum body size of one HTTP POST. | `16777216` |
| `MCP_HTTP_MAX_INFLIGHT_BODY_BYTES` | Aggregate reservation budget for concurrent HTTP POST bodies. | `67108864` |
| `MCP_HTTP_MAX_CONCURRENT_REQUESTS` | Maximum simultaneous non-SSE HTTP handlers. SSE streams remain bounded by `MCP_MAX_SESSIONS`. | `64` |
| `MCP_HTTP_SESSION_TIMEOUT` | Idle lifetime of a stateful HTTP session. | `15m` |
| `MCP_HTTP_ENABLE_EXECUTION` | Additional HTTP-only gate required before either `task_run` execution kind can use its existing authorization flag. | disabled |
| `MCP_DEFAULT_ENCODING` | Default encoding for newly created files when `write_whole_file` is called without `encoding`. Existing files keep a confidently detected encoding. Legacy encodings such as `cp1251` remain available as explicit overrides. | `utf-8` |
| `MCP_MAX_FILE_BYTES` | Hard source-size limit for full-document operations such as `edit_file`. | `67108864` |
| `MCP_MAX_DECODED_CHARACTERS` | Maximum decoded characters returned by `read_text_file`. | `16777216` |
| `MCP_MAX_LINE_BYTES` | Maximum bytes in one decoded UTF-8 line. | `16777216` |
| `MCP_MAX_BATCH_FILES` | Maximum items accepted by bounded batch operations, including `read_multiple_files`, `fingerprint_paths`, patch packages, and `verify_state` checks/path lists. | `256` |
| `MCP_MAX_MATCHES` | Server maximum for `grep_text_files.maxMatches`. | `10000` |
| `MCP_MAX_FINGERPRINT_ENTRIES` | Maximum files plus directories inspected by one `fingerprint_paths` request. | `100000` |
| `MCP_MAX_FINGERPRINT_ENTRY_DETAILS` | Maximum optional per-entry fingerprint records returned by one request. | `1000` |
| `MCP_MAX_EDIT_PREVIEWS` | Maximum live one-shot `edit_file` previews retained by one server process. | `128` |
| `MCP_MAX_EDIT_PREVIEW_BYTES` | Aggregate dynamic bytes retained by live edit previews; independent of normal result output. | `67108864` |
| `MCP_EDIT_PREVIEW_TTL_SECONDS` | Lifetime of one edit preview before lazy expiration and handle cleanup. | `900` |
| `MCP_MAX_PATCH_PACKAGE_BYTES` | Maximum encoded semantic size of one `patch-package-v1` manifest. | `16777216` |
| `MCP_MAX_PATCH_PACKAGE_PREPARED_BYTES` | Aggregate retained prepared bytes, diffs, paths, and metadata during one package dry run. | `67108864` |
| `MCP_MAX_PATCH_PACKAGE_PREVIEWS` | Maximum live one-shot package previews retained by one server process. | `16` |
| `MCP_MAX_PATCH_PACKAGE_PREVIEW_BYTES` | Aggregate bytes retained by live package previews. | `134217728` |
| `MCP_PATCH_PACKAGE_PREVIEW_TTL_SECONDS` | Lifetime of one package preview before lazy expiration and identity cleanup. | `900` |
| `MCP_MAX_OUTPUT_BYTES` | Aggregate read output, retained grep state, fingerprint details, edit/package/restore/GC responses, verification diagnostics, and inconsistent-line output budget. | `67108864` |
| `MCP_MAX_SESSIONS` | Maximum live native Streamable HTTP sessions. | `128` |
| `MCP_BACKUP_STORE_DIR` | Optional dedicated internal store. Must be absolute, canonical, non-overlapping with public roots, owner-only, and exclusively lockable. Enables required edit/package capture, bounded review/audit, original-target restore, and explicit generation-bound GC. | unset |
| `MCP_BACKUP_MAX_TOTAL_BYTES` | Maximum unique object bytes admitted by the internal capture primitive, including durable orphans and live conservative reservations. Hard maximum: 1 TiB. | `1073741824` |
| `MCP_BACKUP_MAX_OBJECT_BYTES` | Maximum bytes in one internally captured object. Hard maximum: 1 GiB. | `67108864` |
| `MCP_BACKUP_MAX_MANIFESTS` | Maximum live internal manifests and bounded recovery/audit scale. Hard maximum: 1,000,000. | `10000` |
| `MCP_BACKUP_MAX_VERSIONS_PER_TARGET` | Maximum internally captured unpinned manifest versions for one target; pinned captures use the separate global pinned quota. Hard maximum: 10,000. | `32` |
| `MCP_BACKUP_MAX_PINNED` | Maximum manifests created with immutable pinned state by internal capture. Hard maximum: 100,000. | `256` |
| `MCP_BACKUP_RETENTION_DAYS` | Age threshold used only by explicit `gcDryRun`; never an automatic deletion timer. Hard maximum: 3,650 days. | `30` |
| `MCP_BACKUP_PLAN_TTL_SECONDS` | Restore and GC one-shot capability lifetime. Hard maximum: 86,400 seconds. | `900` |
| `MCP_MEMORY_THRESHOLD` | Deprecated fallback for `MCP_MAX_FILE_BYTES` and `MCP_MAX_OUTPUT_BYTES`; specific variables take precedence. | unset |
| `MCP_ENABLE_RUN_SCRIPT` | Enables `task_run` with `kind=script`. Accepted true values: `1`, `true`, `yes`, `on`, `enabled`. | disabled |
| `MCP_ENABLE_SHELL` | Enables unrestricted `task_run` with `kind=shell`. Accepted true values: `1`, `true`, `yes`, `on`, `enabled`. | disabled |
| `MCP_ENABLE_EXECUTION` | Enables both `task_run` execution kinds; use only in a trusted environment. | disabled |
| `MCP_TASK_STORE_DIR` | Enables the owner-only durable task registry; the store must be link-free, outside public roots, and separate from `MCP_BACKUP_STORE_DIR`. Allowed directories may change between restarts without recreating the task store. | unset |
| `MCP_TASK_MAX_CONCURRENCY` | Maximum simultaneous starting/running tasks. | `2` |
| `MCP_TASK_MAX_QUEUED` | Maximum queued tasks. | `64` |
| `MCP_TASK_MAX_LOG_BYTES_PER_STREAM` | Retained fixed head plus rolling tail for each stdout/stderr stream. | `8388608` |
| `MCP_TASK_MAX_RUNTIME_SECONDS` | Operator runtime ceiling; `0` means unlimited. | `0` |
| `MCP_TASK_RETENTION_DAYS` | Ordinary retention age for terminal tasks. | `7` |
| `MCP_TASK_MAX_TERMINAL` | Maximum retained terminal tasks. | `1000` |
| `MCP_TASK_MAX_TOTAL_BYTES` | Total task-registry retention target. | `536870912` |

To override, set environment variables in the tunnel launcher or another stdio client configuration:
```json
{
  "mcpServers": {
    "scripthold": {
      "command": "C:\\Tools\\scripthold_windows_amd64.exe",
      "args": ["D:\\Projects"],
      "env": {
        "MCP_DEFAULT_ENCODING": "utf-8"
      }
    }
  }
}
```

## Use Cases

### Legacy Codebases

Many legacy projects use non-UTF-8 encodings that AI assistants can't handle natively:

- **Delphi/Pascal** (Windows-1251): Source files with Cyrillic UI text
- **Extensionless or custom-format text** (UTF-16, Windows code pages, ISO-8859, or UTF-8): detect from content and use an explicit encoding when evidence is ambiguous
- **Visual Basic 6** (Windows-1252): Forms and config files with Western European characters
- **Legacy PHP/HTML** (CP1251, ISO-8859-1): Web apps with localized content
- **Old config files** (Various): INI, properties, registry files with legacy encodings

**How it works:**
```
User: Read config.ini and change the title to "Настройки"
Assistant: [read_text_file with cp1251] → [modify UTF-8] → [write_whole_file with cp1251]
```

The original encoding can be preserved while the public `bom` policy controls BOM output explicitly. The default `auto` policy writes UTF-8 and legacy encodings without BOM and UTF-16 LE/BE with their canonical BOM; use `preserve` when BOM presence must match an existing file.

## Contributing

Contributor workflow is documented in [`CONTRIBUTING.md`](CONTRIBUTING.md). The intentional 1.8-to-2.0 API changes are listed in [`docs/MIGRATION_2.0.md`](docs/MIGRATION_2.0.md). Coding agents should read the root [`AGENTS.md`](AGENTS.md) and the nearest scoped `AGENTS.md` before editing a subtree. Public planning and verification gates remain in [`docs/ROADMAP.md`](docs/ROADMAP.md) and [`docs/DEVELOPMENT_CHECKLIST.md`](docs/DEVELOPMENT_CHECKLIST.md).

## Development

**Prerequisites:** Go 1.26+

```bash
# Run tests
go test ./...

# Build
go build -o scripthold ./cmd/scripthold
```

### Debugging with MCP Inspector

[MCP Inspector](https://github.com/modelcontextprotocol/inspector) provides a web UI for testing MCP servers.

**Prerequisites:** Node.js v18+

```bash
# Run with allowed directory (required)
npx @modelcontextprotocol/inspector go run ./cmd/scripthold -- /path/to/allowed/dir

# Or with built binary
npx @modelcontextprotocol/inspector ./scripthold.exe C:\Projects
```

Opens a browser where you can view tools, call them with custom arguments, and inspect responses.

### Manual Debugging

Run the server with an allowed directory and send JSON-RPC commands via stdin:

```bash
# Specify transport and allowed directory
go run ./cmd/scripthold --transport=stdio /path/to/project
```

Example commands (paste into terminal):

```json
{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_directory","arguments":{"path":"/path/to/project","pattern":"*.go"}}}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"read_text_file","arguments":{"path":"/path/to/project/main.pas","encoding":"cp1251"}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"detect_encoding","arguments":{"path":"/path/to/project/file.txt"}}}
```

## License

GPL-3.0 - see [LICENSE](LICENSE)
