# Scripthold — Secure MCP Server for Local Workspaces

<!-- mcp-name: io.github.zoster81/scripthold -->

[![Test Suite](https://github.com/zoster81/scripthold/actions/workflows/test.yml/badge.svg?branch=main&event=push)](https://github.com/zoster81/scripthold/actions/workflows/test.yml?query=branch%3Amain)
[![CodeQL](https://github.com/zoster81/scripthold/actions/workflows/codeql.yml/badge.svg?branch=main&event=push)](https://github.com/zoster81/scripthold/actions/workflows/codeql.yml?query=branch%3Amain)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/zoster81/scripthold/badge)](https://scorecard.dev/viewer/?uri=github.com/zoster81/scripthold)
[![GitHub Release](https://img.shields.io/github/v/release/zoster81/scripthold)](https://github.com/zoster81/scripthold/releases/latest)
[![Release downloads](https://img.shields.io/github/downloads/zoster81/scripthold/total?label=Release%20downloads)](https://github.com/zoster81/scripthold/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/zoster81/scripthold?logo=go)](go.mod)
[![License: GPL-3.0](https://img.shields.io/github/license/zoster81/scripthold)](LICENSE)
[![MCP Registry](https://img.shields.io/badge/MCP_Registry-Scripthold-blue)](https://registry.modelcontextprotocol.io/?search=io.github.zoster81%2Fscripthold)
[![Glama](https://glama.ai/mcp/servers/zoster81/scripthold/badges/score.svg)](https://glama.ai/mcp/servers/zoster81/scripthold)

**Code from the web. Work locally. Recover safely.**

Scripthold is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that gives web, desktop, and CLI agents controlled access to explicitly authorized local workspaces. It safely handles legacy text encodings, deterministic file changes, source navigation, backups, and optional durable local execution over stdio or authenticated Streamable HTTP.

AI clients see `Настройки` — not `????` or `Íàñòðîéêè`.

## What Scripthold provides

The current public release is **Scripthold 3.2.0** with 38 tools and 3 guided prompts.

- **168 registered text encodings** with content-based detection, UTF-32 LE/BE support, and conservative ambiguity handling.
- **101 active Source Intelligence providers** for bounded declaration navigation, structural search, selected project relations, and verified context assembly.
- **Secure workspace boundaries** with canonical-root containment, Windows reparse/junction handling, deterministic traversal, and missing-path validation.
- **Approval-bound mutations** using preview/apply capabilities, exact fingerprints, conflict checks, staged writes, and truthful partial-state reporting.
- **Optional persistent backups** with history, compare, audit, restore, explicit garbage collection, pinning, and exact-ID deletion.
- **Durable asynchronous tasks** with idempotency, persistent state, bounded logs, recovery, locks, and cancellation.
- **Reliable long-running read-only calls** through optional deferred-operation storage and bounded retained-result retrieval.
- **Authenticated Streamable HTTP** with loopback defaults, bearer authentication, Host/Origin validation, resource limits, and explicit TLS/proxy requirements for non-loopback exposure.

Scripthold originated from the [original `mcp-file-tools` project](https://github.com/dimitar-grigorov/mcp-file-tools), created by **Dimitar Grigorov**, and retains its GPL-3.0 lineage. See [Project Direction](docs/PROJECT_DIRECTION.md).

## Quick start

### Use a published release

Download the asset for your platform from the [latest release](https://github.com/zoster81/scripthold/releases/latest) and verify it against `checksums.txt`.

### Build from source

```bash
git clone https://github.com/zoster81/scripthold.git
cd scripthold
go test ./...
go build -o scripthold ./cmd/scripthold
```

The module path is `github.com/zoster81/scripthold`.

### Local stdio client

Pass each authorized directory as a startup argument:

```json
{
  "mcpServers": {
    "scripthold": {
      "type": "stdio",
      "command": "C:\\Tools\\scripthold_windows_amd64.exe",
      "args": ["D:\\Projects"]
    }
  }
}
```

Startup roots are authoritative. A roots-capable stdio client may supply dynamic roots only when the process starts without directory arguments.

### Streamable HTTP

HTTP requires exactly one bearer-token source. A minimal loopback PowerShell launch is:

```powershell
$tokenPath = Join-Path $env:TEMP "scripthold.token"
$bytes = New-Object byte[] 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
[System.IO.File]::WriteAllText($tokenPath, [Convert]::ToBase64String($bytes), [System.Text.UTF8Encoding]::new($false))

$env:MCP_HTTP_TOKEN_FILE = $tokenPath
$env:MCP_HTTP_ADDR = "127.0.0.1:8765"
.\scripthold_windows_amd64.exe --transport=streamable-http D:\Projects
```

The MCP endpoint is `http://127.0.0.1:8765/mcp`; `/healthz` and `/readyz` expose liveness/readiness. Send the token as `Authorization: Bearer <token>` on every MCP request.

Do not expose HTTP beyond loopback without reading [HTTP Security](docs/HTTP_SECURITY.md). Non-loopback use requires explicit opt-in and TLS or a trusted proxy boundary.

### OpenAI Secure MCP Tunnel

Sanitized PowerShell examples are available under [`examples/`](examples/):

- [`start-local-stdio.ps1`](examples/start-local-stdio.ps1)
- [`start-local-http.ps1`](examples/start-local-http.ps1)
- [`start-openai-tunnel-stdio-plus-local-http.ps1`](examples/start-openai-tunnel-stdio-plus-local-http.ps1)
- [`start-openai-tunnel-http-plus-local-stdio.ps1`](examples/start-openai-tunnel-http-plus-local-stdio.ps1)

Copy an example outside the checkout before replacing placeholders. Never commit Runtime API keys, Tunnel IDs, bearer tokens, or private state paths.

### Container

```bash
docker build --build-arg VERSION=dev -t scripthold:dev .

docker run --rm -i \
  --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --mount type=bind,source=/absolute/project,target=/data \
  scripthold:dev --transport=stdio /data
```

The image runs as unprivileged UID/GID `10001`.

## Tool groups

[`TOOLS.md`](TOOLS.md) is the detailed public reference for schemas, parameters, examples, limits, and error behavior. The catalog currently includes:

| Area | Main tools |
|---|---|
| Text/files | [`read_text_file`](TOOLS.md#read_text_file), [`read_multiple_files`](TOOLS.md#read_multiple_files), [`write_whole_file`](TOOLS.md#write_whole_file), [`edit_file`](TOOLS.md#edit_file), [`edit_file_apply`](TOOLS.md#edit_file_apply), [`grep_text_files`](TOOLS.md#grep_text_files) |
| Multi-file changes | [`patch_package`](TOOLS.md#patch_package), [`patch_package_apply`](TOOLS.md#patch_package_apply), [`filesystem_package`](TOOLS.md#filesystem_package), [`filesystem_package_apply`](TOOLS.md#filesystem_package_apply) |
| Directory/state | [`list_directory`](TOOLS.md#list_directory), [`tree`](TOOLS.md#tree), [`search_files`](TOOLS.md#search_files), [`get_file_info`](TOOLS.md#get_file_info), [`fingerprint_paths`](TOOLS.md#fingerprint_paths), [`verify_state`](TOOLS.md#verify_state) |
| Encoding | [`detect_encoding`](TOOLS.md#detect_encoding), [`convert_encoding`](TOOLS.md#convert_encoding), [`convert_encoding_apply`](TOOLS.md#convert_encoding_apply), [`detect_line_endings`](TOOLS.md#detect_line_endings), [`change_line_endings`](TOOLS.md#change_line_endings), [`manage_bom`](TOOLS.md#manage_bom), [`manage_bom_apply`](TOOLS.md#manage_bom_apply), [`list_encodings`](TOOLS.md#list_encodings) |
| Backups | [`backup_store`](TOOLS.md#backup_store), [`backup_restore_apply`](TOOLS.md#backup_restore_apply), [`backup_gc_apply`](TOOLS.md#backup_gc_apply), [`backup_delete`](TOOLS.md#backup_delete) |
| Source Intelligence | [`source_symbols`](TOOLS.md#source_symbols), [`source_query`](TOOLS.md#source_query) |
| Durable work | [`task_run`](TOOLS.md#task_run), [`task_list`](TOOLS.md#task_list), [`task_get`](TOOLS.md#task_get), [`task_logs`](TOOLS.md#task_logs), [`task_cancel`](TOOLS.md#task_cancel), [`deferred_operation`](TOOLS.md#deferred_operation) |
| Service | [`list_allowed_directories`](TOOLS.md#list_allowed_directories), [`check_for_updates`](TOOLS.md#check_for_updates) |

## Source Intelligence

Source Intelligence is read-only. It does not execute project code and does not require external parser/compiler/LSP processes.

`source_symbols` provides bounded `outline`, `digest`, `find`, and fingerprint-bound `show`. `source_query` provides structural search, supported project relations, dependency graphs, and fingerprint-verified context. Unsupported or ambiguous relationships fail closed rather than being guessed.

The generated provider/capability matrix is in [Language Capabilities](docs/LANGUAGE_CAPABILITIES.md).

## Safety model

- File access is limited to explicitly authorized roots after canonical path validation.
- Symlink, junction, reparse-point, alias, and missing-path escapes fail closed.
- Encoding detection uses bytes and decoded-content evidence, never filenames or extensions.
- Mutations stage and revalidate before commit; initially missing destinations use no-replace semantics.
- Preview/apply workflows bind the approved operation to a one-shot capability so mutation parameters cannot be changed at apply time.
- Multi-file mutations do not claim transactional rollback; failures report committed/unchanged/unknown state where applicable.
- The optional backup store is a separate protected authority outside public roots.
- `task_run` execution is disabled by default. Shell and script execution require explicit authorization; HTTP requires an additional HTTP execution opt-in.
- HTTP is authenticated and loopback-only by default.

Current architectural boundaries are summarized in [Architecture](docs/ARCHITECTURE.md).

## Key configuration

Most installations need only a small subset of environment variables:

| Variable | Purpose | Default |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` or `streamable-http` | `stdio` |
| `MCP_DEFAULT_ENCODING` | Encoding for new text files | `utf-8` |
| `MCP_HTTP_ADDR` | HTTP listen address | `127.0.0.1:8765` |
| `MCP_HTTP_TOKEN_FILE` / `MCP_HTTP_TOKEN` | Mutually exclusive bearer-token sources | unset |
| `MCP_HTTP_ALLOW_NON_LOOPBACK` | Permit non-loopback binding when other security requirements are met | disabled |
| `MCP_BACKUP_STORE_DIR` | Enable the persistent backup store | unset |
| `MCP_BACKUP_DEFAULT_POLICY` | Default persistent backup policy for eligible approval-bound mutations | `disabled` |
| `MCP_TASK_STORE_DIR` | Enable durable task persistence | unset |
| `MCP_DEFERRED_STORE_DIR` | Enable durable ownership for eligible long-running read-only calls | unset |
| `MCP_ENABLE_RUN_SCRIPT` | Allow `task_run kind=script` | disabled |
| `MCP_ENABLE_SHELL` | Allow `task_run kind=shell` | disabled |
| `MCP_ENABLE_EXECUTION` | Allow both task kinds | disabled |
| `MCP_HTTP_ENABLE_EXECUTION` | Additional HTTP execution gate | disabled |

Detailed limits and subsystem-specific options are documented where they are used: [TOOLS.md](TOOLS.md), [Durable Tasks](docs/DURABLE_TASKS.md), and [HTTP Security](docs/HTTP_SECURITY.md).

## Typical uses

- Safely edit legacy source/configuration files without corrupting encoding or line endings.
- Search mixed-encoding repositories with explicit partial-coverage evidence.
- Navigate heterogeneous source trees without loading entire projects into the model.
- Preview and approve exact single- or multi-file changes.
- Keep persistent pre-state backups and restore selected versions.
- Run long builds/tests without tying process lifetime to one MCP request.
- Serve the same workspace toolset through local stdio, authenticated HTTP, containers, or a secure tunnel bridge.

## Documentation

- [Tool Reference](TOOLS.md) — public MCP tools, schemas, examples, and limits.
- [Architecture](docs/ARCHITECTURE.md) — current product and security boundaries.
- [HTTP Security](docs/HTTP_SECURITY.md) — HTTP deployment and threat model.
- [Durable Tasks](docs/DURABLE_TASKS.md) — persistent task execution.
- [Language Capabilities](docs/LANGUAGE_CAPABILITIES.md) — generated Source Intelligence matrix.
- [Roadmap](docs/ROADMAP.md) — current and future work only.
- [Migration 2.0](docs/MIGRATION_2.0.md) / [Migration 3.0](docs/MIGRATION_3.0.md) — intentional compatibility changes.
- [Contributing](CONTRIBUTING.md) / [Publishing](docs/PUBLISHING.md) / [Security](SECURITY.md) — maintainer and contributor guidance.
- [Changelog](CHANGELOG.md) — concise release and unreleased user-visible changes.

## Development

The required Go version is declared by `go.mod`.

```bash
go mod verify
go test ./...
golangci-lint run ./...
go build -o scripthold ./cmd/scripthold
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before proposing changes.

## License

GPL-3.0 — see [LICENSE](LICENSE).
