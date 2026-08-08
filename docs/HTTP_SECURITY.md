# Streamable HTTP Security Design

This document is the approved R12 security design for native MCP Streamable HTTP in `scripthold`. R13 must implement these requirements without weakening the process-wide filesystem policy established in R11.

## Scope

R12 defines the trust model, secure defaults, configuration contract, request pipeline, session policy, resource limits, logging rules, negative tests, and release blockers for Streamable HTTP.

R12 approved this design before implementation. R13 implements it with the pinned MCP Go SDK while preserving stdio.

R20 extends this design through [MCP_2026_07_28_ADOPTION.md](MCP_2026_07_28_ADOPTION.md). R20 is complete in source: the same endpoint and outer security pipeline route supported legacy versions to the verified stateful handler and exact `2026-07-28` to a stateless SDK handler. Host, Origin, authentication, proxy, rate, body, concurrency, timeout, logging, execution, readiness, and shutdown controls remain common; only legacy traffic enters session admission.

## Security objectives

The HTTP transport must:

- prevent unauthenticated access to configured filesystem roots and optional execution tools;
- preserve the same authoritative 27-tool source catalog, allowed-directory checks, encoding behavior, limits, and error mapping as stdio;
- prevent browser-origin attacks, DNS rebinding, token leakage, session hijacking, and untrusted proxy spoofing;
- bound request bodies, headers, concurrent requests, sessions, idle lifetime, and shutdown time;
- keep credentials, session identifiers, tool arguments, file contents, and sensitive paths out of HTTP logs;
- remain safe by default on a developer workstation without requiring a reverse proxy.

## Non-goals

The first native HTTP implementation will not provide:

- per-session or per-agent filesystem ACLs;
- a browser application or permissive CORS endpoint;
- anonymous HTTP access;
- an OAuth authorization server;
- cookie authentication;
- tokens in URLs or query parameters;
- persistent sessions across process restarts;
- event replay or durable SSE resumption;
- network sandboxing for `run_script` or `shell`;
- tenant isolation inside one server process.

## Assets

The security design protects:

- every file and directory reachable through the configured process roots;
- the operating-system identity and permissions of the server process;
- execution capability when `run_script` or `shell` is enabled;
- bearer credentials and TLS private keys;
- session identifiers, one-shot edit and patch-package preview capabilities, and in-flight MCP messages;
- server availability, memory, goroutines, file descriptors, and process slots;
- logs, which must not become a secondary store of secrets or file contents.

## Actors

- **Operator:** configures roots, listener, credentials, TLS, and optional execution.
- **Trusted MCP client:** presents a valid bearer token and is intentionally granted the complete process-wide tool policy.
- **Authenticated but faulty client:** may send malformed, oversized, concurrent, or expensive requests.
- **Browser attacker:** attempts cross-origin requests or DNS rebinding from a page controlled by an attacker.
- **Network attacker:** observes or alters traffic where TLS is absent outside loopback.
- **Untrusted proxy or peer:** spoofs forwarding headers, Host, Origin, or source identity.
- **Compromised trusted client:** has the same filesystem authority as every other holder of the configured token.

## Trust boundaries

```text
MCP client
    -> network / reverse-proxy boundary
    -> Host and Origin validation
    -> bearer authentication
    -> rate, concurrency, body, and session admission
    -> MCP Streamable HTTP handler
    -> shared MCP server and 27 tools
    -> allowed-root and execution policy
    -> local filesystem and operating system
```

Authentication establishes permission to use the server. An MCP session does not.

## Process-wide filesystem policy

Allowed directories are immutable process-wide policy:

- every HTTP session sees the same startup roots;
- HTTP clients cannot add, replace, or remove roots;
- MCP client roots are disabled for HTTP;
- all sessions share the same tool catalog, limits, execution flags, and error behavior;
- prompt instructions may narrow an agent's intended write area, but the server does not enforce those conventions;
- technical isolation requires separate server processes and, for concurrent Git writes, separate checkouts or worktrees.

## Supported deployment profiles

### Loopback direct — default

- Listen on `127.0.0.1` by default.
- Plain HTTP is permitted only on loopback.
- Bearer authentication is still mandatory.
- The SDK's localhost Host protection remains enabled.
- Requests with an `Origin` header are rejected unless the exact origin is explicitly allowed.

### Reverse proxy

- The application bearer token remains mandatory; a proxy does not bypass application authentication.
- TLS may terminate at the proxy.
- The backend should bind to loopback whenever possible.
- Forwarded headers are ignored unless the immediate peer belongs to an explicitly configured trusted-proxy CIDR.
- Even for trusted proxies, only the minimum required forwarding fields are accepted.
- Public Host values and browser origins must be listed explicitly.

### Direct non-loopback TLS

- Non-loopback binding requires an explicit opt-in.
- A certificate and matching private key are mandatory unless every accepted peer is constrained to an explicitly trusted reverse-proxy network.
- Bearer authentication, Host validation, and Origin validation remain mandatory.
- Plain unauthenticated LAN or Internet exposure is unsupported.

## Configuration contract

R13 must use the following contract. Unknown, contradictory, malformed, or insecure combinations fail startup rather than falling back silently.

| Setting | Default | Requirement |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `streamable-http` selects native HTTP. |
| `MCP_HTTP_ADDR` | `127.0.0.1:8765` | Exact listen address using `localhost` or an IP literal. Non-loopback addresses require explicit opt-in. |
| `MCP_HTTP_PATH` | `/mcp` | Absolute clean path; query parameters do not select transport or credentials. |
| `MCP_HTTP_TOKEN` | unset | Bearer token supplied through the environment. Mutually exclusive with `MCP_HTTP_TOKEN_FILE`. |
| `MCP_HTTP_TOKEN_FILE` | unset | Preferred token source. Read once at startup. Mutually exclusive with `MCP_HTTP_TOKEN`. |
| `MCP_HTTP_ALLOWED_HOSTS` | listener-derived | Comma-separated exact Host values. Wildcards are forbidden. |
| `MCP_HTTP_ALLOWED_ORIGINS` | empty | Comma-separated exact origins. Empty means every request carrying `Origin` is rejected. |
| `MCP_HTTP_ALLOW_NON_LOOPBACK` | disabled | Required for any non-loopback listener. |
| `MCP_HTTP_TLS_CERT_FILE` | unset | Required with `MCP_HTTP_TLS_KEY_FILE` for direct non-loopback TLS. |
| `MCP_HTTP_TLS_KEY_FILE` | unset | Required with `MCP_HTTP_TLS_CERT_FILE`; never logged. |
| `MCP_HTTP_TRUSTED_PROXY_CIDRS` | empty | Exact proxy networks allowed to supply forwarding headers. |
| `MCP_HTTP_MAX_BODY_BYTES` | `16777216` | Maximum body of one HTTP POST before SDK decoding. |
| `MCP_HTTP_MAX_INFLIGHT_BODY_BYTES` | `67108864` | Aggregate reservation budget for concurrent POST bodies. Must be at least the per-request limit. |
| `MCP_HTTP_MAX_CONCURRENT_REQUESTS` | `64` | Process-wide semaphore for non-SSE HTTP handlers. |
| `MCP_HTTP_SESSION_TIMEOUT` | `15m` | Idle lifetime for a stateful MCP session. |
| `MCP_HTTP_ENABLE_EXECUTION` | disabled | Additional transport-specific gate for execution tools. |
| `MCP_MAX_SESSIONS` | `128` | Existing process-wide maximum for live HTTP sessions. |

Fixed initial defaults:

- maximum request headers: 64 KiB;
- read-header timeout: 5 seconds;
- HTTP idle timeout: 2 minutes;
- normal POST request deadline: 11 minutes, preserving the existing 10-minute execution maximum plus cleanup margin;
- graceful-shutdown deadline: 15 seconds;
- per-peer rate: 20 requests per second with a burst of 40.

These defaults may become configurable later only when operational evidence justifies more settings.

## Startup validation

Startup must fail when:

- neither token source is configured;
- both token sources are configured;
- the token is empty, shorter than 32 bytes, contains NUL, or contains embedded line breaks;
- the token file is not a regular readable file;
- the HTTP path is empty, relative, contains a query, does not clean to itself, or conflicts with `/healthz` or `/readyz`;
- a non-loopback listener is requested without `MCP_HTTP_ALLOW_NON_LOOPBACK=1`;
- only one of the TLS certificate or key is supplied;
- direct non-loopback service has neither TLS nor an explicit trusted-proxy boundary;
- an allowed Host or Origin contains a wildcard, user information, path where forbidden, or invalid syntax;
- any byte, duration, concurrency, rate, or session limit is zero, negative, overflowing, or outside its documented hard range.

## Authentication

- Every MCP `POST`, `GET`, and `DELETE` request requires `Authorization: Bearer <token>`.
- Health and readiness endpoints are the only unauthenticated routes; they still pass Host, Origin, and rate checks.
- Credentials in query parameters, cookies, alternate headers, or MCP message bodies are never accepted.
- Missing or invalid credentials return `401 Unauthorized` with a minimal `WWW-Authenticate: Bearer` challenge.
- Token comparison uses SHA-256 digests and `crypto/subtle.ConstantTimeCompare`.
- Token values and complete Authorization headers are never logged.
- The token is loaded once at startup and remains immutable for the process lifetime.
- After the token digest and principal are derived, `MCP_HTTP_TOKEN` and `MCP_HTTP_TOKEN_FILE` are removed from the process environment so optional child processes cannot inherit them.
- A static token maps to one stable internal principal identifier. The raw token is not used as the principal ID.
- The authenticated principal is placed in the request context so the MCP SDK can bind a session to that identity.
- Authentication is revalidated on every HTTP request; possession of a session identifier never replaces authentication.

The first implementation is a private-service bearer-token profile, not a complete OAuth authorization-server implementation. It must remain compatible with deployment behind an OAuth-capable reverse proxy, while still requiring the application bearer token unless a later reviewed milestone changes that rule.

## Host and DNS-rebinding protection

- The SDK localhost protection must remain enabled.
- An application allowlist validates `Host` before authentication.
- Listener-derived localhost forms are accepted by default, including the configured port.
- Additional public or proxy Host values require exact entries in `MCP_HTTP_ALLOWED_HOSTS`.
- Wildcards, suffix matching, and trusting arbitrary `X-Forwarded-Host` values are forbidden.
- Forwarding headers are ignored unless the direct peer is in `MCP_HTTP_TRUSTED_PROXY_CIDRS`.
- Requests with a disallowed Host return `403 Forbidden` without revealing the allowlist.

## Origin, browser, and CORS policy

- Every request carrying `Origin` is validated, including safe methods such as `GET`.
- Origins are matched exactly by scheme, host, and effective port.
- The default allowlist is empty, so browser-origin requests fail closed.
- Missing `Origin` is accepted for non-browser MCP clients, subject to all other controls.
- `null`, wildcard, malformed, opaque, user-info-bearing, or path-bearing origins are rejected.
- A rejected origin returns `403 Forbidden` before bearer-token processing.
- No `Access-Control-Allow-Origin`, credentialed CORS, or wildcard CORS header is emitted.
- `OPTIONS` is not a transport method and returns `405 Method Not Allowed` unless a later reviewed browser profile is added.
- Go's `http.CrossOriginProtection` may be used as defense in depth, but it does not replace the explicit all-method MCP Origin check.

## TLS and proxy handling

- Plain HTTP is acceptable only on loopback or across an explicitly trusted private proxy hop.
- Direct non-loopback traffic requires TLS.
- The server does not generate self-signed certificates automatically.
- TLS files are read at startup and their paths, keys, and certificate contents are not logged.
- Only a bounded `X-Forwarded-For` chain is consumed, and only from trusted proxy peers. `Forwarded`, `X-Forwarded-Host`, and `X-Forwarded-Proto` are ignored.
- When plaintext non-loopback service is permitted only behind a proxy, direct peers outside `MCP_HTTP_TRUSTED_PROXY_CIDRS` are rejected before routing.
- A trusted proxy never bypasses application bearer authentication.
- Client IP used for rate limiting comes from the socket peer unless a trusted proxy supplied a syntactically valid, bounded forwarding chain.
- Forwarding chains are parsed from right to left, stop at the first untrusted address, and reject malformed or excessively long input.

## Request admission pipeline

The HTTP middleware order is security-significant:

1. reject new work during shutdown;
2. identify the socket peer and optional trusted proxy;
3. validate Host;
4. validate Origin on every method;
5. apply per-peer rate limiting;
6. serve minimal health/readiness routes when applicable;
7. authenticate the bearer token for the MCP endpoint;
8. validate transport method and protocol-version header shape;
9. enforce global non-SSE concurrent-request admission plus per-request and aggregate body limits;
10. route exact `2026-07-28` to the stateless SDK handler or supported legacy traffic to the stateful SDK handler;
11. admit or locate an MCP session only on the legacy stateful path;
12. delegate to the selected pinned SDK Streamable HTTP handler;
13. emit a redacted access-log record.

Failures are rejected at the earliest safe stage and must not initialize an MCP session.

## HTTP methods and protocol behavior

- Supported legacy protocol versions retain stateful `POST`, `GET`, and `DELETE` according to Streamable HTTP.
- Exact `2026-07-28` uses stateless `POST` only; `GET` and `DELETE` return `405 Method Not Allowed` with `Allow: POST`.
- `POST` requires an accepted MCP content type and a bounded body on both generations.
- Legacy `GET` remains the authenticated SSE stream of an existing session, and legacy `DELETE` terminates the authenticated session identified by `Mcp-Session-Id`.
- Requests after legacy initialization must carry one of the exact supported legacy `Mcp-Protocol-Version` values.
- Any `Mcp-Session-Id` header on exact `2026-07-28` is rejected rather than ignored.
- `Last-Event-ID` remains rejected for both generations because no event store is configured.
- A missing required legacy session identifier returns `400`; an unknown, expired, or terminated legacy session returns `404`; a session bound to another principal returns `403`.
- Malformed, repeated, empty, comma-joined, or whitespace-variant protocol-version values fail before SDK dispatch. Any other exact unsupported singleton bypasses legacy session admission and reaches only the stateless SDK error path so the SDK can return `UnsupportedProtocolVersionError` with HTTP `400`; it is never treated as an accepted protocol generation.
- `Mcp-Method` and `Mcp-Name` are untrusted metadata; SDK header/body consistency validation remains authoritative after routing.
- Malformed JSON, invalid batches, content-type errors, and protocol validation failures produce deterministic client errors without stack traces.

## Session policy

R13 uses stateful sessions with these rules. They remain authoritative for legacy protocol requests after R20. Exact `2026-07-28` requests now bypass session admission entirely and do not emulate a hidden protocol session.

- session identifiers use the pinned SDK default `crypto/rand.Text` generator and are not replaced by predictable application IDs;
- a session identifier is routing state, not authentication;
- every request is authenticated independently;
- each session is bound to the authenticated principal recorded during initialization;
- session identifiers are never accepted from URLs and are never logged in full;
- idle sessions expire after 15 minutes by default;
- explicit `DELETE`, idle expiry, failed initialization, and process shutdown release session resources;
- `MCP_MAX_SESSIONS` is enforced before creating a new session;
- reaching the session limit returns `429 Too Many Requests` without evicting an active session;
- the admission layer independently tracks lifecycle because the SDK session map is internal;
- session admission reserves capacity before initialization and releases it on failed initialization, explicit DELETE, observed 404, idle expiry, and shutdown;
- tracker entries have one idempotent release path so duplicate cleanup signals cannot underflow the count;
- the application tracker uses the SDK idle timeout plus a one-second cleanup grace, pauses expiry for active POST requests, and permits SSE-only sessions to expire unless the client sends keepalive traffic;
- sessions are not persisted across restart.

All clients sharing one static token share one authenticated principal. A trusted client that also obtains another client's session identifier could therefore address that session. This is an accepted single-trust-domain risk; stronger tenant isolation requires distinct server processes or a later OAuth identity model.

## Event streams and resumption

- The first R13 implementation keeps `EventStore` unset.
- Normal stateful SSE is supported, but interrupted streams are not durably replayed.
- `Last-Event-ID` resumption requiring an event store is rejected deterministically.
- This avoids storing MCP messages, tool results, paths, or file content outside the active process.
- Durable replay may be considered only in a separate reviewed design with storage limits, encryption, retention, and cleanup semantics.

## Resource limits and denial-of-service behavior

- A bounded read-closer limits the body before SDK decoding without prebuffering a second full copy; overflow is translated deterministically to `413 Request Entity Too Large`.
- The initial per-request body limit is 16 MiB. Larger HTTP writes require an explicit operator increase or use of stdio.
- An aggregate 64 MiB in-flight body budget reserves known content lengths and the full per-request limit for chunked bodies; saturation fails fast with `429`.
- Header bytes are bounded at the `http.Server` layer.
- A process-wide semaphore caps active non-SSE HTTP handlers at 64. Long-lived SSE streams are bounded by the session limit.
- A per-peer token bucket allows 20 requests per second with a burst of 40.
- The existing `MCP_MAX_SESSIONS` limit caps live legacy sessions only; stateless `2026-07-28` traffic neither reserves nor consumes session capacity.
- Session creation, authentication failures, malformed requests, legacy GET streams, legacy DELETE requests, and stateless POST requests all consume rate budget.
- Rate or concurrency saturation returns `429` with a bounded `Retry-After` value.
- Per-peer limiter state is capped at 4096 entries, evicts least-recently-used inactive peers, and removes entries after 10 minutes without activity.
- Admission state is garbage-collected after inactivity and remains bounded even under source-address churn.
- Request cancellation propagates into MCP tool contexts.
- Client disconnects cancel their request context and release concurrency admission.
- Existing tool-specific file, line, output, batch, match, and execution limits remain authoritative after HTTP admission.

## Timeouts and graceful shutdown

- `ReadHeaderTimeout` is 5 seconds.
- `IdleTimeout` is 2 minutes.
- Normal POST handling is bounded to 11 minutes unless an earlier tool limit or client cancellation ends it. This remains longer than the public 10-minute execution maximum.
- No short global `WriteTimeout` is applied to SSE streams.
- SSE lifetime is bounded by session expiry, client disconnect, server shutdown, and authenticated keepalive activity; an open SSE stream alone does not pause the idle timer.
- Shutdown first marks readiness false and rejects new sessions.
- `http.Server.Shutdown` receives a 15-second deadline.
- If graceful shutdown expires, the listener and remaining connections are force-closed.
- Cancellation must reach active MCP calls and execution processes.
- Process exit is the final cleanup boundary for SDK-internal idle session state that has no public close-all API.

## Execution tools over HTTP

`run_script` and `shell` remain disabled by default.

HTTP execution requires both:

1. the existing tool-specific or combined execution flag; and
2. `MCP_HTTP_ENABLE_EXECUTION=1`.

Therefore enabling an execution tool for stdio does not automatically expose it over HTTP. R13 must make the transport-specific decision explicit in server policy rather than infer it from client input.

`run_script` retains path validation, script snapshot verification, timeout, output bounds, and process-tree cancellation. External-process cleanup is bounded even when a descendant retains inherited output handles, and the Windows tree-termination helper has its own bounded timeout. `shell` remains unrestricted after working-directory validation and is suitable only for a trusted deployment.

## Health and readiness

- `GET /healthz` returns only process liveness.
- `GET /readyz` returns `200` only after configuration is valid and the listener is accepting MCP work; it returns `503` during shutdown.
- `HEAD` may mirror these statuses without a body.
- Other methods return `405`.
- Responses contain no version, roots, tool names, session counts, configuration, hostnames, or dependency details.
- Health routes remain subject to Host, Origin, and rate checks.

## Logging and observability

HTTP access logs may contain only:

- generated request correlation ID;
- method;
- fixed route name rather than raw URL;
- status code;
- duration;
- bounded byte counts;
- coarse authentication outcome;
- trusted socket peer or a privacy-preserving peer fingerprint;
- tool name after MCP dispatch, when already available without decoding arguments.

Logs must not contain:

- Authorization headers or bearer tokens;
- cookies;
- TLS private material;
- complete session IDs, event IDs, or edit/package preview capability identifiers;
- query strings;
- request or response bodies;
- tool arguments, script arguments, shell commands, file contents, or diffs;
- unsanitized filesystem paths;
- forwarded headers from untrusted peers.

Session identifiers may be represented only by a short one-way fingerprint when correlation is necessary. Error logging must use stable categories and redact path-bearing details at the HTTP access layer.

The existing tool middleware can log human-readable failure messages that may include filesystem paths. Therefore R13 must either keep the handler `Logger` nil for HTTP or first introduce category-only redacted tool logging. The HTTP access logger and SDK logger must be separate from any verbose local diagnostic logger.

## SDK integration constraints

R13 introduced the pinned `github.com/modelcontextprotocol/go-sdk` stateful Streamable HTTP handler, and R20 Phase 4 adds a second stateless handler behind the same explicit outer controls:

- return the same R11-built `*mcp.Server` for every legacy session and every stateless request;
- keep SDK localhost protection enabled;
- add an explicit all-method Origin validator because browser protection for unsafe methods alone is insufficient for MCP GET streams;
- bound the body before the SDK can call `io.ReadAll`, while avoiding an additional full-body copy and enforcing an aggregate reservation budget;
- keep `EventStore` unset;
- configure the stateful SDK session idle timeout;
- configure the stateless SDK handler with `Stateless`, the same bounded SDK body limit, and `PropagateRequestCancellation`;
- populate authenticated principal information in context for legacy SDK session binding;
- implement an outer bounded session-admission tracker because legacy SDK session storage is internal, while stateless traffic bypasses that tracker;
- use one idempotent lifecycle record per admitted session to prevent capacity leaks or double release;
- keep the R11 handler logger disabled for HTTP until category-only path-redacted logging exists;
- coordinate HTTP shutdown without relying on the SDK's unexported test-only close-all helper.

No SDK fork or new dependency is planned unless R13 proves that these requirements cannot be met safely through public APIs and small local middleware.

## Required security tests

### Startup and configuration

- default bind is loopback;
- HTTP startup fails without a token;
- conflicting token sources fail;
- malformed token files and insecure listener combinations fail;
- non-loopback requires explicit opt-in and TLS or a trusted proxy boundary;
- malformed Host, Origin, CIDR, duration, per-request body, aggregate body, and concurrency limit values fail closed;
- token environment variables are removed after a successful configuration snapshot.

### Authentication

- missing, malformed, wrong, and truncated bearer tokens return `401`;
- valid bearer token succeeds;
- query, cookie, and alternate-header tokens are ignored;
- every `POST`, `GET`, and `DELETE` request reauthenticates;
- credentials and raw headers never appear in captured logs.

### Host, Origin, and proxy

- disallowed Host returns `403` even with a valid token;
- localhost DNS-rebinding Host values are rejected;
- absent Origin succeeds for a valid non-browser client;
- malformed, `null`, wildcard, and unlisted origins return `403` on `GET`, `POST`, and `DELETE`;
- no CORS allow header is emitted;
- forwarding headers from an untrusted peer are ignored;
- only configured proxy CIDRs may influence client address or public scheme/host.

### Limits and denial of service

- known-length and chunked bodies at the limit succeed and bodies over the limit return `413`;
- aggregate in-flight body saturation returns bounded `429` responses and releases reservations after completion;
- oversized headers are rejected;
- concurrency and rate saturation return bounded `429` responses;
- the session limit rejects only new sessions;
- abandoned, failed, expired, and explicitly deleted sessions release admission;
- limiter state is bounded and expires;
- slow headers, slow bodies, disconnected clients, and stalled SSE streams do not leak goroutines or admission permits.

### Protocol generations and sessions

- exact `2026-07-28` negotiates stateless HTTP, emits no session identifier, and leaves legacy session counts unchanged even when legacy capacity is full;
- absent-version legacy initialization and all four exact supported legacy versions route to the stateful handler;
- empty, duplicate, comma-joined, and whitespace-variant protocol-version values fail before SDK dispatch, while a well-formed unsupported singleton returns the SDK's structured unsupported-version error without entering legacy session admission;
- modern `GET`, `DELETE`, `Last-Event-ID`, and any session header are rejected;
- a contradictory standard method header and JSON-RPC method is rejected by SDK validation;
- session IDs are non-empty, globally unique in the legacy test population, and valid visible ASCII;
- missing session header returns `400` where required;
- unknown or expired session returns `404`;
- a different authenticated principal cannot reuse a session;
- valid concurrent requests in one session and across sessions remain isolated at protocol level while sharing process roots;
- active POST requests pause idle expiry, while SSE-only sessions expire without keepalive traffic;
- expiry, DELETE, failed initialization, and shutdown are race-tested;
- session identifiers never appear in logs.

### Filesystem and execution policy

- all HTTP sessions list the same startup roots;
- HTTP roots notifications cannot mutate process roots;
- representative read, write, encoding, error, and cancellation results match stdio;
- `run_script` and `shell` remain denied unless both authorization layers are enabled;
- enabling HTTP execution does not weaken path validation, timeout, output, or cancellation behavior.

### Lifecycle and observability

- readiness changes to `503` before shutdown admission closes;
- graceful shutdown completes cleanly with idle sessions;
- forced shutdown cancels active calls after the deadline;
- access logs contain only approved fields;
- panic and malformed-request paths expose no stack trace or sensitive state to clients.

## Release-blocking findings

R13 cannot be completed or released with any of the following:

- authentication bypass on any MCP method;
- token acceptance from a URL or cookie;
- Host or Origin bypass, including DNS rebinding;
- wildcard CORS or browser credential exposure;
- session reuse by a different authenticated principal;
- HTTP mutation of process roots;
- execution available without the dual opt-in;
- unbounded per-request body, aggregate in-flight body, header, request, session, rate-limiter, or SSE state;
- forwarding-header trust from an untrusted peer;
- non-loopback plaintext exposure outside an explicit trusted proxy boundary;
- secrets, complete session IDs, file contents, commands, or sensitive paths in HTTP logs;
- shutdown, disconnect, expiry, or session-cleanup races detected by tests;
- divergence between stdio and HTTP tool metadata or representative results.

## Accepted risks

The following risks are explicit and accepted for the initial native HTTP profile:

- every authenticated session has the complete process-wide filesystem authority configured at startup;
- one static bearer token represents one trust domain rather than distinct users;
- trusted agents sharing that token are not isolated from one another by ACLs;
- operating-system permissions of the server process remain the final host boundary;
- session and SSE state is lost on restart;
- interrupted stream replay is unavailable without an event store;
- direct TLS certificate lifecycle, bearer-token rotation, and reverse-proxy correctness remain operator responsibilities;
- changing the static token requires a controlled process restart and invalidates the existing trust domain;
- `shell`, when deliberately enabled through both gates, is not sandboxed.

## References

- [R20 MCP 2026-07-28 adoption design](MCP_2026_07_28_ADOPTION.md)
- [MCP 2026-07-28 specification announcement](https://blog.modelcontextprotocol.io/posts/2026-07-28/)
- [MCP Streamable HTTP transport specification](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports)
- [MCP authorization specification](https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization)
- [MCP security best practices](https://modelcontextprotocol.io/specification/2025-11-25/basic/security_best_practices)
- [Go `net/http` package](https://pkg.go.dev/net/http)
- [OAuth 2.0 Bearer Token Usage, RFC 6750](https://www.rfc-editor.org/rfc/rfc6750)