# Scripthold — Secure MCP Server for Local Workspaces

<!-- mcp-name: io.github.zoster81/scripthold -->

[![Go Report Card](https://goreportcard.com/badge/github.com/zoster81/scripthold)](https://goreportcard.com/report/github.com/zoster81/scripthold)
[![Release](https://img.shields.io/github/v/release/zoster81/scripthold)](https://github.com/zoster81/scripthold/releases/latest)
[![License: GPL-3.0](https://img.shields.io/github/license/zoster81/scripthold)](LICENSE)
[![MCP Registry](https://img.shields.io/badge/MCP_Registry-Scripthold-blue)](https://registry.modelcontextprotocol.io/?search=io.github.zoster81%2Fscripthold)

**Code from the web. Work locally. Recover safely.**

Scripthold is a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that gives web, desktop, and CLI agents controlled access to explicitly authorized local workspaces. It reads and writes legacy text safely, exposes deterministic repository-oriented workflows, supports authenticated Streamable HTTP as well as stdio, and can optionally run durable asynchronous local tasks.

AI clients see `Настройки` — not `????` or `Íàñòðîéêè`.

Scripthold detects encodings from bytes and decoded-text evidence rather than filenames, presents text to the MCP client as UTF-8, and preserves or deliberately converts encoding, BOM, and line endings through bounded-memory and durable filesystem operations.

- **36 tools and 3 guided prompts** over one authoritative catalog in the current next-major source tree; the public `2.2.0` release exposes 30 tools.
- **168 registered encodings**, including UTF-32 LE/BE and broad portable legacy coverage; automatic detection remains intentionally more conservative than explicit codec support.
- **Secure filesystem boundaries** with resolved-root containment, deterministic traversal, Windows reparse/junction handling, staged mutation, conflict detection, and no-replace creation.
- **Verified change workflows** with deterministic fingerprints, one-shot edit approval, strict patch packages, persistent backup integration, and typed verification.
- **Offline backup recovery** with deterministic persisted review plans, immutable source evidence, fully verified reconstruction into a separate staged destination, mandatory full audit, no-replace promotion, and path-free provenance.
- **Durable asynchronous execution** with idempotent admission, an owner-only task store, bounded queue/logs, independent supervisor/worker/executor lifecycle, recovery, logical locks, and cancellation.
- **Fail-closed Streamable HTTP** with bearer authentication, loopback defaults, exact Host/Origin checks, bounded resources, no CORS, and explicit TLS/proxy requirements for non-loopback exposure.

**Scripthold was built with Scripthold.**

> **Lineage:** Scripthold originated from the [original `mcp-file-tools` project](https://github.com/dimitar-grigorov/mcp-file-tools), created by **Dimitar Grigorov**, and retains its GPL-3.0 lineage and permanent attribution. See [Project Direction](docs/PROJECT_DIRECTION.md).

## Current release

**Scripthold `2.2.0`** is the current public release. It exposes 30 tools, 3 guided prompts, and 168 registered encodings. The GitHub Release publishes six raw binaries, six platform archives, and `checksums.txt`; GitHub-only workflows add the six MCPB bundles, `mcpb-checksums.txt`, and the MCP Registry publication for `io.github.zoster81/scripthold`.

R22 completed the global encoding expansion and full UTF-32 pipeline, **R23 completed on 2026-08-12**, **R24 completed on 2026-08-13**, **R25 completed on 2026-08-13**, and **R26 completed on 2026-08-14**. R24 established the 34-tool Unreleased next-major filesystem surface; R25 adds the read-only `source_symbols` source-navigation tool as the 35th catalog entry. `source_symbols` provides bounded `outline`, `digest`, `find`, and fingerprint-bound `show` over the initial Go, C#, VB.NET, Python, and Classic ASP canaries without external parser/compiler/LSP runtime dependencies. R26 adds explicit offline `backup-store recover-plan` / `recover-apply` reconstruction into a separate fully audited destination while preserving the damaged source store as evidence; its exact pushed implementation commit passed native Windows/Ubuntu/macOS CI and the aggregate `Release candidate` gate. R27 was activated on 2026-08-14; Phase 1 added `source_query` as the 36th Unreleased catalog entry and froze one compact read-only `search` / `relations` / `context` contract while preserving `source_symbols`. Phases 2 through 18 are now complete. The profile-driven native scanner/recognizer foundation now backs 101 active production providers: the five R25 canaries; the C/C++/Java/Kotlin, JavaScript/TypeScript/Rust, PHP/Ruby/Swift/Pascal/Delphi, Basic/.NET/composite, MQL4/MQL5 and specialty C-like waves; distinct Perl, Lua, Luau, Elixir, Erlang, Gleam, Groovy, POSIX shell, Bash, Tcl, and AutoHotkey providers; distinct Fortran, COBOL, Ada, MATLAB, Octave, Julia, R, Haskell, OCaml, Common Lisp, Clojure, and Emacs Lisp providers; the Phase 10 data/infra/hardware/document wave covering SQL/PLSQL, GraphQL, Terraform/HCL, Nix, Protocol Buffers, VHDL, Verilog/SystemVerilog, Assembly, HTML/XML, CSS/SCSS/Sass/Less, JSON/YAML/TOML/Markdown, OpenAPI, and Ansible-oriented YAML; the Phase 11 composite/template wave covering Vue, Svelte, Astro, PHP/HTML, JSP, Jinja, Twig, Blade, and EJS while completing host HTML/client JavaScript/CSS delegation for ASP.NET Web Forms, Razor, and Blazor; and the Phase 16 Scala and Flow providers that complete every approved R27 target-catalog row while leaving Dockerfile and Make as auxiliary inactive registry metadata. Classic ASP was also extended from VBScript-only embedded analysis to truthful VBScript/JScript delegation. Their mechanically projected capability rows are in [docs/LANGUAGE_CAPABILITIES.md](docs/LANGUAGE_CAPABILITIES.md). Shared-extension Basic dialects and `.mqh` remain fail-closed when evidence cannot distinguish their dialects; generic `.m` remains ambiguous among Objective-C, MATLAB, and Octave without independent content/project evidence; .NET composite formats preserve host coordinates across structurally declared server regions, host HTML, and supported client JavaScript/CSS regions; dynamic, scientific/legacy, and Phase 10 non-general-purpose providers expose only the structural facts justified by their native recognizers without claiming unsupported runtime/compiler/type semantics or fictitious programming-language semantics. Phase 12 added bounded project resolution; Phase 13 made `source_query` structural search plus supported dependency/reference/type/trace/impact/cycle relations operational with explicit evidence, ambiguity and graph bounds. Phase 14 added deterministic fingerprint-verified task-context assembly with target/enclosing/dependency/type priorities, exact decoded UTF-8 budgeting, body-to-signature degradation, signature-only deeper relations, and fail-closed stale-source handling. Phase 15 adds a bounded process-local incremental index with content/analyzer/configuration fingerprint invalidation, immutable coherent generations, conservative whole-model relation recomputation after changes, active generation/fingerprint stale binding, generation-bound coverage, and no retained complete source bodies or persistent storage. Phase 16 closes the approved provider catalog with distinct Scala and Flow providers; Phase 17 verifies repository-scale bounds, concurrent generation consistency, security boundaries, benchmarks, and a heterogeneous MCP workflow; Phase 18 completes registry-first documentation, fuzzing, normal/race/static/vulnerability, catalog, and documentation gates. Textual/lexical query modes remain delegated to existing decoded text search and analyzer-unproven callers/callees/overrides remain unsupported. **R27 completed on 2026-08-16; Phases 0 through 18 are complete.** `2.2.0` remains the current public release until an explicitly authorized later release is published. Current/future milestone state lives in [docs/ROADMAP.md](docs/ROADMAP.md), the completed R23 contract in [docs/MCP_MUTATION_SURFACE.md](docs/MCP_MUTATION_SURFACE.md), the completed R24 contract in [docs/SAFE_FILESYSTEM_OPERATIONS.md](docs/SAFE_FILESYSTEM_OPERATIONS.md), the completed R25 contract in [docs/SOURCE_INTELLIGENCE.md](docs/SOURCE_INTELLIGENCE.md), the completed R26 contract in [docs/BACKUP_RECOVERY.md](docs/BACKUP_RECOVERY.md), the completed R27 contract in [docs/MULTILANGUAGE_CODE_INTELLIGENCE.md](docs/MULTILANGUAGE_CODE_INTELLIGENCE.md), completed milestone history in [docs/ROADMAP_HISTORY.md](docs/ROADMAP_HISTORY.md), and release changes in [CHANGELOG.md](CHANGELOG.md). Publication does not imply any operator-specific deployment state.

## Transport and authorization model

| Transport | Typical use | Security boundary | Roots behavior |
|---|---|---|---|
| stdio | Local MCP clients and secure tunnel bridges | Client configuration plus operating-system process boundary | Startup directories are authoritative; dynamic client roots are accepted only when startup roots are empty |
| Streamable HTTP | Persistent localhost services, containers, trusted proxies, explicitly secured remote services | Bearer token on every MCP request; loopback by default; TLS or trusted proxy boundary for non-loopback | Startup directories are immutable and shared by all requests; HTTP clients cannot mutate roots |

Both transports use the same `BuildServer` path and expose the same tools, prompts, limits, encoding behavior, error model, and execution policy.

Allowed directories are a **process-wide authorization boundary**. Sessions separate protocol lifecycle and cancellation; they are not per-agent filesystem ACLs. If two agents require technical isolation, run separate Scripthold processes with narrower roots and, for concurrent Git writes, separate checkouts or worktrees.

MCP `2026-07-28` is supported through the stable Go SDK. Native HTTP serves stateless modern requests beside retained stateful legacy sessions under the same outer authentication, Host/Origin, resource, logging, and execution controls. See [docs/MCP_2026_07_28_ADOPTION.md](docs/MCP_2026_07_28_ADOPTION.md) and [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md).

## Tool catalog

### File and directory operations

- [`read_text_file`](TOOLS.md#read_text_file) — stream decoded text with bounded output and optional line numbers.
- [`read_multiple_files`](TOOLS.md#read_multiple_files) — deterministic bounded batch reads with per-file status.
- [`write_whole_file`](TOOLS.md#write_whole_file) — replace complete file contents through the shared encoder.
- [`edit_file`](TOOLS.md#edit_file) — read-only exact edit preview with approval fingerprints and a one-shot capability.
- [`edit_file_apply`](TOOLS.md#edit_file_apply) — apply only the exact prepared edit identified by `previewId`.
- [`patch_package`](TOOLS.md#patch_package) — read-only inspect/dry-run/verify for declared multi-file edits.
- [`patch_package_apply`](TOOLS.md#patch_package_apply) — apply only a prepared patch-package capability.
- [`list_directory`](TOOLS.md#list_directory) — list directory entries with filtering and deterministic sorting.
- [`tree`](TOOLS.md#tree) — compact `.gitignore`-aware deterministic tree output.
- [`get_file_info`](TOOLS.md#get_file_info) — read file or directory metadata.
- [`filesystem_package`](TOOLS.md#filesystem_package) — read-only bounded preparation for coordinated no-replace create/copy/move/delete filesystem changes.
- [`filesystem_package_apply`](TOOLS.md#filesystem_package_apply) — apply one prepared filesystem package by one-shot `previewId`.
- [`search_files`](TOOLS.md#search_files) — bounded `.gitignore`-aware glob search.
- [`source_symbols`](TOOLS.md#source_symbols) — bounded read-only source `outline`, `digest`, `find`, and fingerprint-bound `show` navigation.
- [`source_query`](TOOLS.md#source_query) — bounded R27 read-only structural search, supported project relations, and fingerprint-verified task-context assembly.
- [`fingerprint_paths`](TOOLS.md#fingerprint_paths) — deterministic SHA-256 state fingerprints.
- [`verify_state`](TOOLS.md#verify_state) — bounded typed JSON/text/Git-diff/fingerprint checks.
- [`backup_store`](TOOLS.md#backup_store) — read-only status/history/compare/audit plus restore/GC preparation for the optional persistent store.
- [`backup_restore_apply`](TOOLS.md#backup_restore_apply) — apply one prepared original-target restore.
- [`backup_gc_apply`](TOOLS.md#backup_gc_apply) — apply one prepared generation-bound backup GC plan.
- [`grep_text_files`](TOOLS.md#grep_text_files) — paged regex search with deterministic partial-coverage reporting.
### Encoding and service tools

- [`detect_encoding`](TOOLS.md#detect_encoding) — conservative encoding detection with confidence or explicit ambiguity.
- [`convert_encoding`](TOOLS.md#convert_encoding) — read-only exact single/batch conversion preview.
- [`convert_encoding_apply`](TOOLS.md#convert_encoding_apply) — apply a prepared exact conversion by `previewId`.
- [`detect_line_endings`](TOOLS.md#detect_line_endings) — bounded LF/CRLF/mixed analysis.
- [`change_line_endings`](TOOLS.md#change_line_endings) — line-ending conversion while preserving encoding/BOM semantics.
- [`manage_bom`](TOOLS.md#manage_bom) — detect BOM state or prepare an exact add/strip change.
- [`manage_bom_apply`](TOOLS.md#manage_bom_apply) — apply one prepared BOM mutation by `previewId`.
- [`list_encodings`](TOOLS.md#list_encodings) — authoritative runtime encoding inventory.
- [`list_allowed_directories`](TOOLS.md#list_allowed_directories) — report process-authorized roots.
- [`check_for_updates`](TOOLS.md#check_for_updates) — notification-only fork release check.
### Durable task execution

- [`task_run`](TOOLS.md#task_run) — durably enqueue idempotent shell or script work.
- [`task_list`](TOOLS.md#task_list) — page/filter persistent task metadata.
- [`task_get`](TOOLS.md#task_get) — inspect current/terminal task state and bounded lifecycle history.
- [`task_logs`](TOOLS.md#task_logs) — read bounded stdout/stderr with absolute cursors.
- [`task_cancel`](TOOLS.md#task_cancel) — cancel queued work or terminate a running process tree.

The detailed schemas, outputs, limits, and examples are authoritative in [TOOLS.md](TOOLS.md). `internal/toolcatalog/catalog.json` is the source of truth for runtime tool metadata.

### Encoding support

`list_encodings` is authoritative for canonical names, aliases, and capability metadata. Scripthold `2.2.0` exposes 168 canonical read/write encodings across Unicode, IBM/DOS/EBCDIC, ISO-8859, Windows, classic Mac/KOI8/other single-byte families, and East Asian/stateful multibyte families.

The production runtime remains pure Go. Additional mappings and state machines derived from pinned GNU libiconv evidence are checked in and require no libiconv/GCC dependency during ordinary build or execution. UTF-32 LE/BE are full text encodings with strict scalar validation; generic byte-order-unspecified `utf-32` remains intentionally rejected. See [docs/GLOBAL_ENCODING_COVERAGE.md](docs/GLOBAL_ENCODING_COVERAGE.md) for the completed R22 contract.

## Installation

Choose **stdio** when the MCP client should own the child process or a secure bridge expects a local command. Choose **Streamable HTTP** for a persistent authenticated service. Both expose the same public behavior.

### Use a published release

Scripthold-named releases use raw binary names of the form `scripthold_<os>_<arch>` (with `.exe` on Windows) and matching platform archives. Historical `2.0.0` predates the rename and retains its original asset names.

For reproducible installations, use a specific semantic release rather than `@main` or an assumed historical asset name. Verify the published asset against `checksums.txt` before installation.

### Build from source

```bash
git clone https://github.com/zoster81/scripthold.git
cd scripthold
go test ./...
go build -o scripthold ./cmd/scripthold
```

The module path is `github.com/zoster81/scripthold`.

### Local stdio clients

Pass every startup-authorized directory as an argument:

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

A roots-capable stdio client may provide dynamic roots only when the process starts without directory arguments. `MCP_STDIO_LEGACY_HANDSHAKE=1` exists only for legacy bridges that probe discovery and repeat an equivalent legacy initialization on one persistent child; leave it disabled for normal modern clients.

### Native Streamable HTTP

HTTP requires exactly one bearer-token source. A minimal loopback PowerShell start is:

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

The MCP endpoint is `http://127.0.0.1:8765/mcp`; `/healthz` and `/readyz` expose minimal liveness/readiness status. The token must be sent as `Authorization: Bearer <token>` on every MCP request. Do not put tokens in command-line arguments, URLs, cookies, or query parameters.

Non-loopback listeners require explicit opt-in plus TLS or an explicitly trusted proxy boundary. Browser CORS is not enabled. See [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md) before exposing HTTP beyond loopback.

### OpenAI Secure MCP Tunnel

The repository includes sanitized PowerShell examples for tunnel and local topologies:

| Example | Topology |
|---|---|
| [`start-local-stdio.ps1`](examples/start-local-stdio.ps1) | One foreground local stdio server. |
| [`start-local-http.ps1`](examples/start-local-http.ps1) | One authenticated HTTP server; loopback by default. |
| [`start-openai-tunnel-stdio-plus-local-http.ps1`](examples/start-openai-tunnel-stdio-plus-local-http.ps1) | Tunnel to a dedicated stdio child plus an independent local HTTP process. |
| [`start-openai-tunnel-http-plus-local-stdio.ps1`](examples/start-openai-tunnel-http-plus-local-stdio.ps1) | Tunnel to authenticated HTTP plus an independent local stdio child. |

Copy an example outside the Git checkout before replacing placeholders. Never commit Runtime API keys, Tunnel IDs, bearer tokens, or private state paths. The tunnel setup uses OpenAI's official [`tunnel-client`](https://github.com/openai/tunnel-client); consult the official client documentation for current OpenAI control-plane requirements.

The example launchers keep `task_run` execution disabled by default. Script and shell execution remain separate authorizations, and HTTP additionally requires `MCP_HTTP_ENABLE_EXECUTION=1`.

### Container image

The repository Dockerfile builds a statically linked binary and runs as unprivileged UID/GID `10001`. The image is transport-neutral.

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

The mounted directory must be accessible to UID/GID `10001`. HTTP containers should mount token/TLS files read-only, publish only the intended port, and preserve the security contract in [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md).

## Security model

- File tools access only explicitly authorized roots after canonical path resolution.
- Recursive operations do not follow escaping symlinks, junctions, or other reparse points.
- Mutations stage and revalidate before commit; initially missing destinations use no-replace creation. Single-file mutators classify the bounded actual target state after failures that may occur beyond the commit boundary instead of reporting preview-predicted changes as fact.
- Failed MCP tool calls preserve stable error metadata/text; tools with structured output also expose `errorCode` and human-readable `message` there, retaining any existing partial-state evidence.
- The optional backup store must be a separate non-overlapping owner-only authority and is inaccessible to ordinary file tools.
- `task_run` is disabled by default. Script tasks validate/fingerprint the script and execute an owner-only matching snapshot; shell tasks validate the logical shell name before durable admission, confine only the working directory, and otherwise run with the executor identity's operating-system permissions.
- HTTP adds authentication, Host/Origin, proxy/TLS, resource, logging, and execution boundaries; it is not a replacement for operating-system isolation.

Detailed contracts: [HTTP security](docs/HTTP_SECURITY.md), [verified changes](docs/VERIFIED_CHANGE_WORKFLOWS.md), [persistent backups](docs/PERSISTENT_BACKUP_LIFECYCLE.md), [offline backup diagnostics](docs/OFFLINE_BACKUP_DIAGNOSTICS.md), [R23 mutation surface](docs/MCP_MUTATION_SURFACE.md), [R24 safe filesystem operations](docs/SAFE_FILESYSTEM_OPERATIONS.md), [R25 source intelligence](docs/SOURCE_INTELLIGENCE.md), [R26 backup recovery](docs/BACKUP_RECOVERY.md), and [durable tasks](docs/DURABLE_TASKS.md).

## Configuration

The most important process-wide variables are summarized below. Subsystem documents contain the precise security and lifecycle semantics.

| Variable | Purpose | Default |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` or `streamable-http`; CLI `--transport` takes precedence. | `stdio` |
| `MCP_DEFAULT_ENCODING` | Encoding for newly created files when no encoding is supplied. | `utf-8` |
| `MCP_MAX_FILE_BYTES` | Full-document source-size limit. | `67108864` |
| `MCP_MAX_DECODED_CHARACTERS` | Maximum decoded characters returned by `read_text_file`. | `16777216` |
| `MCP_MAX_LINE_BYTES` | Maximum decoded UTF-8 bytes in one line. | `16777216` |
| `MCP_MAX_BATCH_FILES` | Maximum items in bounded batch/path-list operations. | `256` |
| `MCP_MAX_MATCHES` | Server maximum for grep matches. | `10000` |
| `MCP_MAX_OUTPUT_BYTES` | Aggregate structured/text output budget. | `67108864` |
| `MCP_SOURCE_MAX_FILES` | R25 source files considered per request before stricter global ceilings. | `256` |
| `MCP_SOURCE_MAX_AGGREGATE_BYTES` | Aggregate raw source bytes selected by one source-intelligence request. | `67108864` |
| `MCP_SOURCE_MAX_FILE_BYTES` | Per-file source-intelligence byte ceiling. | `8388608` |
| `MCP_SOURCE_MAX_SYMBOLS` | Retained source-symbol ceiling per request/analyzer budget. | `10000` |
| `MCP_SOURCE_MAX_CONCURRENCY` | Bounded source-analysis worker count. | `4` |
| `MCP_SOURCE_MAX_REQUEST_SECONDS` | Source-intelligence request deadline. | `30` |
| `MCP_SOURCE_MAX_OUTPUT_BYTES` | Source-intelligence structured output budget before the global output ceiling. | `16777216` |
| `MCP_SOURCE_MAX_RESULTS` | R27 retained search/relation result ceiling. | `10000` |
| `MCP_SOURCE_MAX_GRAPH_NODES` | R27 graph-node ceiling. | `5000` |
| `MCP_SOURCE_MAX_GRAPH_EDGES` | R27 graph-edge ceiling. | `20000` |
| `MCP_SOURCE_MAX_GRAPH_DEPTH` | R27 graph traversal depth ceiling. | `8` |
| `MCP_SOURCE_MAX_CONTEXT_BYTES` | R27 task-context byte budget. | `1048576` |
| `MCP_SOURCE_MAX_CONTEXT_ITEMS` | R27 retained context-item ceiling. | `256` |
| `MCP_SOURCE_MAX_INDEX_PROJECTS` | R27 retained process-local index-scope ceiling. | `4` |
| `MCP_SOURCE_MAX_INDEX_GENERATIONS` | R27 retained generations per index scope. | `2` |
| `MCP_MAX_FILESYSTEM_PACKAGE_OPERATIONS` | Maximum operations in one `filesystem-package-v1` manifest. | `256` |
| `MCP_MAX_FILESYSTEM_PACKAGE_BYTES` | Maximum prepared filesystem-package manifest size. | `16777216` |
| `MCP_MAX_FILESYSTEM_RECURSIVE_ENTRIES` | Maximum entries in one exact recursive copy/delete scope. | `100000` |
| `MCP_MAX_FILESYSTEM_RECURSIVE_DEPTH` | Maximum exact recursive copy/delete depth. | `128` |
| `MCP_MAX_FILESYSTEM_AGGREGATE_BYTES` | Maximum aggregate source bytes in one filesystem package. | `1073741824` |
| `MCP_MAX_FILESYSTEM_STAGING_BYTES` | Maximum aggregate bytes staged before filesystem-package commit. | `1073741824` |
| `MCP_MAX_FILESYSTEM_PACKAGE_PREVIEWS` | Maximum retained filesystem-package preview capabilities. | `16` |
| `MCP_MAX_FILESYSTEM_PACKAGE_PREVIEW_BYTES` | Maximum aggregate retained preview state. | `134217728` |
| `MCP_FILESYSTEM_PACKAGE_PREVIEW_TTL_SECONDS` | Filesystem-package preview lifetime. | `900` |
| `MCP_MEMORY_THRESHOLD` | Deprecated fallback for file/output byte limits. | unset |
| `MCP_HTTP_ADDR` | HTTP listen address. | `127.0.0.1:8765` |
| `MCP_HTTP_PATH` | MCP endpoint path. | `/mcp` |
| `MCP_HTTP_TOKEN_FILE` / `MCP_HTTP_TOKEN` | Mutually exclusive HTTP bearer-token sources. | unset |
| `MCP_HTTP_ALLOWED_HOSTS` | Additional exact Host values. | listener-derived |
| `MCP_HTTP_ALLOWED_ORIGINS` | Exact accepted Origin values; no CORS headers are emitted. | empty |
| `MCP_HTTP_ALLOW_NON_LOOPBACK` | Required opt-in for non-loopback binding. | disabled |
| `MCP_HTTP_TLS_CERT_FILE` / `MCP_HTTP_TLS_KEY_FILE` | Direct HTTPS certificate/key pair. | unset |
| `MCP_HTTP_TRUSTED_PROXY_CIDRS` | Immediate trusted proxy networks. | empty |
| `MCP_HTTP_MAX_BODY_BYTES` | Per-POST body limit. | `16777216` |
| `MCP_HTTP_MAX_INFLIGHT_BODY_BYTES` | Aggregate concurrent POST-body reservation. | `67108864` |
| `MCP_HTTP_MAX_CONCURRENT_REQUESTS` | Concurrent non-SSE HTTP handlers. | `64` |
| `MCP_HTTP_SESSION_TIMEOUT` | Legacy stateful session idle timeout. | `15m` |
| `MCP_HTTP_ENABLE_EXECUTION` | Additional HTTP-only execution gate. | disabled |
| `MCP_BACKUP_STORE_DIR` | Enables the dedicated persistent backup store. | unset |
| `MCP_BACKUP_DEFAULT_POLICY` | Default persistent pre-state policy for approval-bound edit/package/BOM/encoding mutations: `disabled` or `required`. | `disabled` |
| `MCP_TASK_STORE_DIR` | Enables the owner-only durable task registry. | unset |
| `MCP_ENABLE_RUN_SCRIPT` | Authorizes `task_run kind=script`. | disabled |
| `MCP_ENABLE_SHELL` | Authorizes unrestricted `task_run kind=shell`. | disabled |
| `MCP_ENABLE_EXECUTION` | Authorizes both task kinds. | disabled |

Backup limits, task-store limits, edit/package preview limits, and the full HTTP configuration contract are documented in [docs/PERSISTENT_BACKUP_LIFECYCLE.md](docs/PERSISTENT_BACKUP_LIFECYCLE.md), [docs/DURABLE_TASKS.md](docs/DURABLE_TASKS.md), [TOOLS.md](TOOLS.md), and [docs/HTTP_SECURITY.md](docs/HTTP_SECURITY.md).

## Typical uses

- Read and safely modify legacy source/configuration files without changing their encoding accidentally.
- Search mixed-encoding repositories with explicit partial-coverage evidence.
- Navigate declarations in Go, C#, VB.NET, Python, and Classic ASP without reading every complete source file.
- Preview and approve edits or multi-file patch packages against deterministic fingerprints.
- Keep approval-bound persistent backups and restore a selected original target safely.
- Recover trustworthy records from a damaged backup store offline into a separate audited destination without modifying the source evidence.
- Run long builds/tests through durable tasks without tying process lifetime to one MCP request.
- Serve the same workspace tools through local stdio, authenticated HTTP, containers, or a secure tunnel bridge.

Example:

```text
User: Read config.ini and change the title to "Настройки".
Assistant: read_text_file (cp1251) -> edit_file preview preserving cp1251 -> explicit approval -> edit_file_apply(previewId)
```

## Development and contribution

Prerequisite Go version is declared by `go.mod`.

```bash
go test ./...
go build -o scripthold ./cmd/scripthold
```

Contributor workflow is in [CONTRIBUTING.md](CONTRIBUTING.md). Coding agents should read the root [AGENTS.md](AGENTS.md) and the nearest scoped guide. Reusable verification is in [docs/DEVELOPMENT_CHECKLIST.md](docs/DEVELOPMENT_CHECKLIST.md), current planning in [docs/ROADMAP.md](docs/ROADMAP.md), and publication in [docs/PUBLISHING.md](docs/PUBLISHING.md).

The intentional 1.8-to-2.0 breaking changes remain documented in [docs/MIGRATION_2.0.md](docs/MIGRATION_2.0.md). The Unreleased next-major R23/R24 MCP surface migration is documented separately in [docs/MIGRATION_3.0.md](docs/MIGRATION_3.0.md).

## License

GPL-3.0 — see [LICENSE](LICENSE).
