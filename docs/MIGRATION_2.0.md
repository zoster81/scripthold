# Migration from 1.8 to 2.0

This is a **historical migration snapshot** for the intentional public API and deployment changes from Scripthold 1.8 to `v2.0.0`. It describes the 2.0 boundary, not the current 2.2 surface. For current behavior use the root [README.md](../README.md), [TOOLS.md](../TOOLS.md), and subsystem documentation.

## Breaking-change table

| 1.8 behavior | 2.0 behavior | Migration |
|---|---|---|
| `directory_tree` was exposed as a deprecated JSON-tree tool. | `directory_tree` is removed. The server exposes 23 tools. | Use `tree`. Rename `excludePatterns` to `exclude`; consume its compact `tree`, `fileCount`, `dirCount`, and `truncated` fields instead of parsing JSON stored inside a string. |
| `detect_encoding` returned `has_bom`; `manage_bom` returned `hasBom`. | Both tools return `hasBOM`. | Update JSON consumers to read `hasBOM`. Other BOM fields remain camelCase. |
| Single-tool failures exposed only human-readable text. Batch read failures used a smaller code vocabulary. | Every MCP tool error includes `_meta.errorCode`; batch items use the same vocabulary. | Branch on the stable code and retain the text for diagnostics. |
| Ambiguous non-empty content could fall back silently to UTF-8 in text tools. | `detect_encoding` reports `ambiguous: true`; other text tools fail with `ENCODING_AMBIGUOUS` until `encoding` is supplied explicitly. | Call `detect_encoding`, inspect `ambiguous`, and pass an explicit registered encoding when needed. |
| Empty files were detected as UTF-8 without explaining that the result was conventional. | `detect_encoding` returns `encoding: "utf-8"`, `confidence: 0`, and `assumed: true`. Text tools treat empty input as UTF-8. | No override is required unless the caller intends to create content in another encoding. |
| `MCP_MEMORY_THRESHOLD` controlled several unrelated limits. | Separate hard limits control file input, decoded characters, lines, batches, matches, output, and future HTTP sessions. | Set the specific variables below. `MCP_MEMORY_THRESHOLD` remains a deprecated fallback for file and output byte limits during migration. |
| UTF-32 BOMs could be detected or added/removed but UTF-32 was not a registered text encoding. | UTF-32 remains BOM-management only. | Use `manage_bom` for UTF-32 signatures. Convert UTF-32 content externally before calling read, edit, grep, or conversion tools. |
| The fork shipped an optional Claude Code marketplace plugin that downloaded and cached release binaries. | The fork-owned downloader plugin and marketplace metadata are removed. | Configure the released binary directly as a normal stdio MCP server, or use native Streamable HTTP. |
| The container used an unpinned Alpine runtime, ran as root, and exposed `./server` from `/app`. | The image uses pinned bases, runs as UID/GID `10001`, installs the binary at `/usr/local/bin/scripthold`, and uses `/data` as its working directory. | Mount allowed roots explicitly under `/data`, grant UID/GID `10001` the required permissions, and update orchestration commands and health checks. |

## Stable error codes

Single-tool errors expose the code at `_meta.errorCode`. `read_multiple_files.results[].errorCode` uses the same values:

- `INVALID_INPUT`
- `INVALID_PATH`
- `ACCESS_DENIED`
- `SYMLINK_ESCAPE`
- `NOT_FOUND`
- `PERMISSION`
- `ENCODING`
- `ENCODING_AMBIGUOUS`
- `CONFLICT`
- `CANCELLED`
- `LIMIT`
- `IO_ERROR`
- `INTERNAL_ERROR`
- `OPERATION_FAILED`

`OPERATION_FAILED` is the 2.0 fallback for errors that do not have a more specific domain category. Successful results do not include an error code. Later 2.x releases added additional tool-specific behavior and error refinements; consult current [TOOLS.md](../TOOLS.md) rather than extending this historical list.

## Configurable limits

All values must be positive decimal integers.

| Variable | Default | Scope |
|---|---:|---|
| `MCP_MAX_FILE_BYTES` | 67,108,864 | Full-document operations such as `edit_file`. |
| `MCP_MAX_DECODED_CHARACTERS` | 16,777,216 | Maximum returned decoded characters for `read_text_file`; a smaller request `maxCharacters` is allowed. |
| `MCP_MAX_LINE_BYTES` | 16,777,216 | Maximum bytes in one decoded UTF-8 line. |
| `MCP_MAX_BATCH_FILES` | 256 | Maximum paths accepted by `read_multiple_files`. |
| `MCP_MAX_MATCHES` | 10,000 | Server maximum for `grep_text_files.maxMatches`. The request default remains 1,000. |
| `MCP_MAX_OUTPUT_BYTES` | 67,108,864 | Aggregate read output, retained grep state, and inconsistent-line output. |
| `MCP_MAX_SESSIONS` | 128 | Maximum live native Streamable HTTP sessions. |
| `MCP_MEMORY_THRESHOLD` | — | Deprecated fallback for `MCP_MAX_FILE_BYTES` and `MCP_MAX_OUTPUT_BYTES`. Specific variables take precedence. |

## Transport selection and directory policy

R11 separates process configuration, CLI parsing, shared server construction, and transport startup. Stdio remains the default, while native stateful Streamable HTTP can be selected explicitly:

```text
--transport=stdio
--transport=streamable-http
MCP_TRANSPORT=stdio
MCP_TRANSPORT=streamable-http
```

The CLI option takes precedence over the environment default. Streamable HTTP fails startup unless exactly one bearer-token source is configured through `MCP_HTTP_TOKEN` or the preferred `MCP_HTTP_TOKEN_FILE`. It binds to `127.0.0.1:8765` by default and uses `/mcp`; non-loopback exposure requires the additional controls in [`HTTP_SECURITY.md`](HTTP_SECURITY.md).

At the 2.0 boundary, allowed directories are process-wide policy. Every connection or HTTP session attached to one server process shares the same configured roots, then-current 23-tool catalog, limits, execution flags, and error behavior. Sessions separate protocol lifecycle, cancellation, and concurrent requests; they do not create per-agent filesystem ACLs. Prompt instructions may narrow an agent's intended write scope, but technical isolation requires separate processes and, for concurrent Git changes, separate checkouts or worktrees.

Startup directories remain authoritative and immutable for the process. A roots-capable stdio client may provide dynamic MCP roots only when the process starts without directory arguments. HTTP disables client roots and cannot mutate the process-wide set.

At the 2.0 boundary, native HTTP reauthenticates every MCP request, validates exact Host and Origin values, emits no CORS allow headers, bounds per-request and aggregate body memory, limits sessions and concurrent handlers, and disables event replay. The then-public synchronous `run_script` and `shell` tools require `MCP_HTTP_ENABLE_EXECUTION=1` in addition to their tool-specific or combined authorization flag. Later releases replaced those public tools with durable `task_run`; see [DURABLE_TASKS.md](DURABLE_TASKS.md). After HTTP startup configuration is validated, token source variables are removed so optional child processes cannot inherit them.

### HTTP configuration migration

A minimal loopback deployment requires exactly one token source:

```text
MCP_TRANSPORT=streamable-http
MCP_HTTP_TOKEN_FILE=/absolute/path/to/token
MCP_HTTP_ADDR=127.0.0.1:8765
```

The endpoint is `/mcp` unless `MCP_HTTP_PATH` is set. Browser-origin requests remain denied unless their exact origin is listed. Do not migrate credentials into command-line arguments, URLs, cookies, or query strings.

| Variable | Migration note |
|---|---|
| `MCP_HTTP_ALLOWED_HOSTS` | Add exact public or proxy Host values; wildcards are rejected. |
| `MCP_HTTP_ALLOWED_ORIGINS` | Add exact browser origins only when browser access is intentionally supported. This does not enable CORS headers. |
| `MCP_HTTP_ALLOW_NON_LOOPBACK` | Required before binding a non-loopback address. |
| `MCP_HTTP_TLS_CERT_FILE`, `MCP_HTTP_TLS_KEY_FILE` | Configure both for direct non-loopback TLS. |
| `MCP_HTTP_TRUSTED_PROXY_CIDRS` | Configure only the immediate proxy networks that may supply `X-Forwarded-For`. |
| `MCP_HTTP_MAX_BODY_BYTES` | Per-request POST body limit; default 16 MiB. |
| `MCP_HTTP_MAX_INFLIGHT_BODY_BYTES` | Aggregate concurrent POST body reservation; default 64 MiB. |
| `MCP_HTTP_MAX_CONCURRENT_REQUESTS` | Concurrent non-SSE handler limit; default 64. |
| `MCP_HTTP_SESSION_TIMEOUT` | Stateful session idle timeout; default 15 minutes. |
| `MCP_HTTP_ENABLE_EXECUTION` | Additional HTTP-only authorization required for execution tools. |

## Container migration

The 2.0 Dockerfile is transport-neutral. Select the transport explicitly and pass allowed roots as process arguments. For stdio, a hardened baseline is:

```bash
docker run --rm -i \
  --read-only \
  --cap-drop=ALL \
  --security-opt=no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --mount type=bind,source=/absolute/project,target=/data \
  scripthold:2.0.0 --transport=stdio /data
```

The mounted directory must be accessible to UID/GID `10001`. HTTP deployments should mount token and TLS files read-only, publish the configured port, and probe `/healthz` for liveness and `/readyz` for readiness. The image declares `SIGTERM`; supervisors should allow the documented graceful-shutdown interval before forcing termination.

## Release and installation migration

For the 2.0 transition, release tags became tied to dated `CHANGELOG.md` headings instead of duplicated plugin/marketplace versions. Distribution and Registry procedures have evolved since then; [PUBLISHING.md](PUBLISHING.md) is authoritative for current releases.

Existing Claude Code users should replace the removed marketplace plugin with an ordinary stdio server entry that invokes the downloaded 2.0 binary and supplies allowed directories directly or through supported client roots.

## Migration checklist

1. Replace `directory_tree` with `tree` and update renamed output fields.
2. Update error handling to consume `_meta.errorCode` and the unified batch codes.
3. Set explicit encodings for ambiguous non-empty legacy files.
4. Replace broad `MCP_MEMORY_THRESHOLD` settings with the specific `MCP_MAX_*` limits.
5. Choose `stdio` or `streamable-http` explicitly for deployment automation.
6. Review process-wide roots and split processes where technical isolation is required.
7. Configure bearer authentication, Host/Origin, TLS/proxy, body, concurrency, and session settings before enabling HTTP.
8. Require both authorization layers before exposing execution tools over HTTP.
9. Update container paths, ownership, mounts, health probes, and stop behavior.
10. Replace the removed Claude Code downloader plugin with direct binary configuration.
11. Run the complete 2.0 23-tool catalog and representative read/write/error smoke tests before cutover.

## Schema review

Apart from the changes listed above, existing 1.8 input and output field names remain unchanged in R10. Optional fields continue to be omitted when they do not apply. The 2.0 schema tests reject snake_case output tags and verify that runtime registration matches the authoritative tool catalog.
